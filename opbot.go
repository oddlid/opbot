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

	_conn.AddCallback(JOIN, onJOIN)         // Triggers giving OP is nick is in list
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
	const fn string = "onPRIVMSG()"
	_caller.Nick = e.Nick
	_caller.Hostmask = e.Source
	devdbg("%s: %s: Caller: %#v", PLUGIN, fn, _caller)
}

// 311 is the reply to WHOIS when nick found
func on311(e *ircevent.Event) {
	//devdbg("%+v", e)
	const fn string = "on311()"
	hm := &HostMask{
		Nick:     e.Arguments[1],
		UserID:   e.Arguments[2],
		Host:     e.Arguments[3],
		//RealName: e.Arguments[5], // never used
	}
	select {
	case _wchan <- hm:
		devdbg("%s: %s: Sent hostmask object on _wchan", PLUGIN, fn)
	default:
		devdbg("%s: %s: Unable to send on _wchan", PLUGIN, fn)
	}
}

// 401 is the reply from WHOIS when nick NOT found
func on401(e *ircevent.Event) {
	const fn string = "on401()"
	select {
	case _wchan <- nil:
		devdbg("%s: %s: Sent NIL hostmask object on _wchan", PLUGIN, fn)
	default:
		devdbg("%s: %s: Unable to send on _wchan", PLUGIN, fn)
	}
}

func onJOIN(e *ircevent.Event) {
	const fn string = "onJOIN()"
	if e.Nick == _conn.GetNick() {
		devdbg("%s: %s: Seems it's myself joining. e.Nick: %s", PLUGIN, fn, e.Nick)
		return
	}

	c := _ops.Get(e.Arguments[0])
	if c.Empty() {
		devdbg("%s: %s: OPs list is empty, nothing to do", PLUGIN, fn)
		return
	}

	if !c.Has(e.Nick) {
		devdbg("%s: %s: %s not in OPs list, ignoring", PLUGIN, fn, e.Nick)
		return
	}

	if !c.MatchHostMask(e.Nick, e.Source) {
		devdbg("%s: %s: No match on hostmask %q for nick %q", PLUGIN, fn, e.Source, e.Nick)
		return
	}

	// Set OP for nick
	devdbg("%s: %s: Setting mode %q for %q in %q", PLUGIN, fn, "+o", e.Nick, e.Arguments[0])
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
	const fn string = "add()"
	if nick == "" {
		emsg := PLUGIN + ": Cannot add empty nick"
		return emsg, fmt.Errorf(emsg)
	}

	go func() {
		devdbg("%s: %s: Goroutine waiting to read from _wchan...", PLUGIN, fn)
		hm := <-_wchan

		if hm == nil {
			devdbg("%s: %s: Got NIL hostmask back on _wchan. %q does not exist on server", PLUGIN, fn, nick)
			_bot.SendMessage(
				channel,
				fmt.Sprintf("%s: Error adding %q - no such nick", PLUGIN, nick),
				nil,
			)
			return
		}

		devdbg("%s: %s: Got back info about nick %q: %#v", PLUGIN, fn, nick, hm)

		added := _ops.Get(channel).Add(nick, hm.String())
		devdbg("%s: %s: Nick %q with mask %q added: %t", PLUGIN, fn, nick, hm.String(), added)

		err := _ops.SaveFile(_opfile)
		if err != nil {
			log.Error(err)
		}

		devdbg("%s: %s: Giving %q OP right away!", PLUGIN, fn, nick)
		_conn.Mode(channel, "+o", nick) // try to OP right away
	}()

	devdbg("%s: %s: Calling WHOIS on nick %q", PLUGIN, fn, nick)
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
	c := _ops.Get(channel)
	if match(action, "SET") {
		c.Lock()
		c.WelcomeMsg = msg
		c.Unlock()
		err = _ops.SaveFile(_opfile)
		if err != nil {
			log.Error(err)
		}
	}
	return fmt.Sprintf(
		"%s: Welcome message for channel %s: %q",
		PLUGIN,
		channel,
		c.WelcomeMsg,
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
	const fn string = "getOP()"
	go func() {
		devdbg("%s: %s: Goroutine waiting to read from _wchan...", PLUGIN, fn)
		hm := <-_wchan

		if hm == nil {
			devdbg("%s: %s: Got NIL hostmask back on _wchan. %q does not exist on server", PLUGIN, fn, nick)
			return
		}

		devdbg("%s: %s: Got back info about nick %q: %#v", PLUGIN, fn, nick, hm)

		if _ops.Get(channel).MatchHostMask(nick, hm.String()) {
			devdbg("%s: %s: Nick %q has matching hostmask (%q), op'ing", PLUGIN, fn, nick, hm.String())
			_conn.Mode(channel, "+o", nick) // try to OP right away
		} else {
			_bot.SendMessage(
				channel,
				fmt.Sprintf("%s: Nick %q has no hostmask matching %q. No OP for you.", PLUGIN, nick, hm.String()),
				nil,
			)
		}
	}()

	devdbg("%s: %s: Calling WHOIS on nick %q", PLUGIN, fn, nick)
	_conn.Whois(nick)

	return "", nil
}

func op(cmd *bot.Cmd) (string, error) {
	const fn string = "op()"
	devdbg("%s: Entered %s with cmd: %#v", PLUGIN, fn, cmd)

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
