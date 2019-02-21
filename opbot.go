package opbot

/*
This plugin watches for irc users joining a channel.
If the nick is in the OP list, it tries to give OP.

NOTE: This plugin does not work as the normal plugins for "go-chat-bot",
			as this one needs a handle to both the bot instance and the ircevent.Connection
			instance, so just importing this prefixed with underscore and rely on init()
			will not work. We need to have a custom setup function, where we add callbacks
			to the ircevent.Connection instance. This should then be called by the importing
			package before irc.Run().

			- Odd E. Ebbesen, 2019-02-09 22:52

*/

/*
TODO:
- [*] Make bot check for calling user being in oplist before accepting modifying commands. "ls" ok for all.
- [*] op/deop user right away when being added to or removed from oplist, if user online
- [*] Make it possible to customize welcome message
- [-] give feedback on wrong arguments?
- [ ] Check hostmask, not just if nick is in list, when calling modifying commands
*/

import (
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/go-chat-bot/bot"
	"github.com/go-chat-bot/bot/irc"
	ircevent "github.com/thoj/go-ircevent"
)

const (
	ADD        string = "ADD"
	CLEAR      string = "CLEAR"
	DEL        string = "DEL"
	GET        string = "GET"
	JOIN       string = "JOIN"
	LS         string = "LS"
	MASK       string = "MASK"
	RELOAD     string = "RELOAD"
	SET        string = "SET"
	WMSG       string = "WMSG"
	PLUGIN     string = "OPBot"
	DEF_OPFILE string = "/tmp/opbot.json"
	DEF_WMSG   string = "Welcome, %s"
)

var (
	_bot    *bot.Bot
	_cfg    *irc.Config
	_conn   *ircevent.Connection
	_ops    *OPData
	_opfile string
	_caller Caller
	_wchan  = make(chan *HostMask, 2)
)

func InitBot(b *bot.Bot, cfg *irc.Config, conn *ircevent.Connection, opfile string) error {
	_bot = b
	_cfg = cfg
	_conn = conn
	_opfile = opfile
	_caller = Caller{}
	reload() // initializes _ops

	_conn.AddCallback(JOIN, onJOIN)
	_conn.AddCallback("PRIVMSG", onPRIVMSG) // for keeping track of calling user
	_conn.AddCallback("311", on311)         // reply from whois when nick found
	_conn.AddCallback("401", on401)         // reply from whois when nick not found

	register()

	return nil
}

func register() {
	bot.RegisterCommand(
		"op",
		"Manage nicks/hostmasks for auto-OP",
		HelpMsg(),
		op,
	)
}

// onPRIVMSG just keeps track of the last nick/mask to say something/give a command.
// It's a bit buggy, as if someone gives a command to the bot first thing after it
// has joined, this func will run afterwards, and so _caller is not updated at first command.
func onPRIVMSG(e *ircevent.Event) {
	_caller.Nick = e.Nick
	_caller.Hostmask = e.Source
	log.Debugf("%s: Caller: %#v", PLUGIN, _caller)
}

// 311 is the reply to WHOIS when nick found
func on311(e *ircevent.Event) {
	//log.Debugf("%+v", e)
	hm := &HostMask{
		Nick:     e.Arguments[1],
		UserID:   e.Arguments[2],
		Host:     e.Arguments[3],
		//RealName: e.Arguments[5],
	}
	select {
	case _wchan <- hm:
		log.Debugf("%s: Sent hostmask object on _wchan", PLUGIN)
	default:
		log.Debugf("%s: Unable to send on _wchan", PLUGIN)
	}
	//	hmstr := hm.String()
	//	hmparsed := hostmask(hmstr)
	//	log.Debugf("Hostmask orig: %#v", hm)
	//	log.Debugf("Hostmask string: %s", hmstr)
	//	log.Debugf("HostMask struct, parsed back: %#v", hmparsed)
}

// 401 is the reply from WHOIS when nick NOT found
func on401(e *ircevent.Event) {
	select {
	case _wchan <- nil:
		log.Debugf("%s: Sent NIL hostmask object on _wchan", PLUGIN)
	default:
		log.Debugf("%s: Unable to send on _wchan", PLUGIN)
	}
}

func onJOIN(e *ircevent.Event) {
	if e.Nick == _conn.GetNick() {
		log.Debugf("%s: Seems it's myself joining. e.Nick: %s", PLUGIN, e.Nick)
		return
	}

	c := _ops.Get(e.Arguments[0])
	if c.Empty() {
		log.Debugf("%s: OPs list is empty, nothing to do", PLUGIN)
		return
	}

	if !c.Has(e.Nick) {
		log.Debugf("%s: %s not in OPs list, ignoring", PLUGIN, e.Nick)
		return
	}

	if !c.MatchHostMask(e.Nick, e.Source) {
		log.Debugf("%s: No match on hostmask %q for nick %q", PLUGIN, e.Source, e.Nick)
		return
	}

	// Set OP for nick
	log.Debugf("%s: Setting mode %q for %q in %q", PLUGIN, "+o", e.Nick, e.Arguments[0])
	_conn.Mode(e.Arguments[0], "+o", e.Nick)

	// Welcome the OP user, if welcome message is configured
	if c.WelcomeMsg != "" {
		_bot.SendMessage(
			e.Arguments[0], // will be the channel name
			c.GetWMsg(e.Nick),
			&bot.User{
				ID:       e.Host,
				Nick:     e.Nick,
				RealName: e.User,
			},
		)
	}
}

func ls(channel, nick string) string {
	c := _ops.Get(channel)
	if c.Empty() {
		return fmt.Sprintf("%s: No configured OPs for channel %q", PLUGIN, channel)
	}
	if nick == "" {
		return fmt.Sprintf("%s: OPs for %s: %s", PLUGIN, channel, strings.Join(c.Nicks(), ", "))
	}
	if c.Has(nick) {
		return fmt.Sprintf("%s: %s is registered as OP", PLUGIN, nick)
	}
	return fmt.Sprintf("%s: %s is NOT registered as OP", PLUGIN, nick)
}

func add(channel, nick string) (string, error) {
	if nick == "" {
		emsg := PLUGIN + ": Cannot add empty nick"
		return emsg, fmt.Errorf(emsg)
	}

	go func() {
		log.Debugf("%s: Goroutine waiting to read from _wchan...", PLUGIN)
		hm := <-_wchan

		if hm == nil {
			log.Debugf("%s: Got NIL hostmask back on _wchan. %q does not exist on server", PLUGIN, nick)
			_bot.SendMessage(
				channel,
				fmt.Sprintf("%s: Error adding %q - no such nick", PLUGIN, nick),
				nil,
			)
			return
		}

		log.Debugf("%s: Got back info about nick %q: %#v", PLUGIN, nick, hm)

		added := _ops.Get(channel).Add(nick, hm.String())
		log.Debugf("%s: Nick %q with mask %q added: %t", PLUGIN, nick, hm.String(), added)

		err := _ops.SaveFile(_opfile)
		if err != nil {
			log.Error(err)
		}

		log.Debugf("%s: Giving %q OP right away!", PLUGIN, nick)
		_conn.Mode(channel, "+o", nick) // try to OP right away
	}()

	log.Debugf("%s: Calling WHOIS on nick %q", PLUGIN, nick)
	_conn.Whois(nick)

	return fmt.Sprintf("%s: Adding %q to OPs list", PLUGIN, nick), nil
}

func del(channel, nick string) (string, error) {
	if nick == "" {
		emsg := PLUGIN + ": Cannot delete empty nick"
		return emsg, fmt.Errorf(emsg)
	}
	_ops.Get(channel).Remove(nick)
	err := _ops.SaveFile(_opfile)
	if err != nil {
		log.Error(err)
	}
	_conn.Mode(channel, "-o", nick) // try to DEOP right away
	return fmt.Sprintf("%s: Nick %q removed from OPs list", PLUGIN, nick), err
}

func wmsg(channel, action, msg string) (string, error) {
	var err error
	if match(action, "SET") {
		_ops.Get(channel).WelcomeMsg = msg
		err = _ops.SaveFile(_opfile)
		if err != nil {
			log.Error(err)
		}
	}
	return fmt.Sprintf(
		"%s: Welcome message for channel %s: %q",
		PLUGIN,
		channel,
		_ops.Get(channel).WelcomeMsg,
	), err
}

func mask(channel, action, nick, hostmask string) (retmsg string, err error) {
	c := _ops.Get(channel)
	utmpl := []string{
		fmt.Sprintf("%s: Usage: !op %s %%s <nick>", PLUGIN, MASK),
		fmt.Sprintf("%s: Usage: !op %s %%s <nick> <hostmask>", PLUGIN, MASK),
	}
	dirty := false
	retmsg = PLUGIN + ": Usage: !op mask <add|del|clear|ls> <nick> [hostmask]"

	defer func() {
		if !dirty {
			return
		}
		err = _ops.SaveFile(_opfile)
		if err != nil {
			log.Error(err)
		}
	}()

	if match(action, LS) {
		if nick == "" {
			retmsg = fmt.Sprintf(utmpl[0], LS)
		} else if !c.Has(nick) {
			retmsg = fmt.Sprintf("%s: %q - no such nick", PLUGIN, nick)
		} else {
			retmsg = fmt.Sprintf("%s: Hostmask patterns for %q: %s", PLUGIN, nick, strings.Join(c.Hostmasks(nick), " "))
		}
		return
	}

	if match(action, CLEAR) {
		if nick == "" {
			retmsg = fmt.Sprintf(utmpl[0], CLEAR)
			return
		}
		dirty = c.ClearHostmasks(nick)
		if dirty {
			retmsg = fmt.Sprintf("%s: Hostmasks cleared for %q", PLUGIN, nick)
		} else {
			retmsg = fmt.Sprintf("%s: Nothing to clear for %q", PLUGIN, nick)
		}
		return
	}

	if match(action, ADD) {
		if nick == "" || hostmask == "" {
			retmsg = fmt.Sprintf(utmpl[1], ADD)
			return
		}
		dirty = c.Add(nick, hostmask)
		if dirty {
			retmsg = fmt.Sprintf("%s: Added hostmask %q to nick %s", PLUGIN, hostmask, nick)
		} else {
			retmsg = fmt.Sprintf("%s: Hostmask %q already in list for %q", PLUGIN, hostmask, nick)
		}
		return
	}

	if match(action, DEL) {
		if nick == "" || hostmask == "" {
			retmsg = fmt.Sprintf(utmpl[1], DEL)
			return
		}
		dirty = c.RemoveHostmask(nick, hostmask)
		if dirty {
			retmsg = fmt.Sprintf("%s: Matching hostmask removed from %q", PLUGIN, nick)
		} else {
			retmsg = fmt.Sprintf("%s: No matching hostmask to remove for %q", PLUGIN, nick)
		}
		return
	}

	return
}

func getOP(channel, nick string) (string, error) {
	go func() {
		log.Debugf("%s: Goroutine waiting to read from _wchan...", PLUGIN)
		hm := <-_wchan

		if hm == nil {
			log.Debugf("%s: Got NIL hostmask back on _wchan. %q does not exist on server", PLUGIN, nick)
			return
		}

		log.Debugf("%s: Got back info about nick %q: %#v", PLUGIN, nick, hm)

		if _ops.Get(channel).MatchHostMask(nick, hm.String()) {
			log.Debugf("%s: Nick %q has matching hostmask (%q), op'ing", PLUGIN, nick, hm.String())
			_conn.Mode(channel, "+o", nick) // try to OP right away
		} else {
			_bot.SendMessage(
				channel,
				fmt.Sprintf("%s: Nick %q has no hostmask matching %q. No OP for you.", PLUGIN, nick, hm.String()),
				nil,
			)
		}
	}()

	log.Debugf("%s: Calling WHOIS on nick %q", PLUGIN, nick)
	_conn.Whois(nick)

	return "", nil
}

func op(cmd *bot.Cmd) (string, error) {
	log.Debugf("Entered op() with cmd: %#v", cmd)

	alen := len(cmd.Args)
	if alen == 0 {
		return PLUGIN + ": Arguments missing", nil
	}

	args := safeArgs(4, cmd.Args) // 4 is the longest possible set of valid args

	// check if user is allowed to run this command (is in op list, or read-only command)
	// Anyone is allowed anything if the list is empty
	//
	// There is one edge-case where one could gain OP without a matching hostmask:
	// - The bot has OP
	// - Noone has said anything in the channel since the bot joined
	// - The calling nick is in the list, but with no (matching) hostmask
	// The calling nick can then:
	// !op mask add <nick> <hostmask>
	// !op get
	if !okCmd(cmd.Channel, cmd.User.Nick, args[0], args[1]) {
		return fmt.Sprintf("%s: %s, you must be in the OPs list to run this command", PLUGIN, cmd.User.Nick), nil
		//fmt.Errorf("%s tried to run %q without permission", cmd.User.Nick, strings.Join(args, " "))
	}

	var retmsg string

	// just a little helper to shorten code later
	arg := func(cmd string) bool {
		return match(args[0], cmd)
	}

	if arg(LS) {
		return ls(cmd.Channel, args[1]), nil
	} else if arg(ADD) {
		return add(cmd.Channel, args[1])
	} else if arg(DEL) {
		return del(cmd.Channel, args[1])
	} else if arg(WMSG) {
		return wmsg(cmd.Channel, args[1], strings.Join(cmd.Args[2:len(cmd.Args)], " "))
	} else if arg(MASK) {
		return mask(cmd.Channel, args[1], args[2], args[3])
	} else if arg(GET) {
		return getOP(cmd.Channel, cmd.User.Nick)
	} else if arg(RELOAD) {
		reload()
		retmsg = PLUGIN + ": OPs DB reloaded"
	} else if arg(CLEAR) {
		clear()
		retmsg = PLUGIN + ": OPs DB cleared"
	}

	return retmsg, nil
}
