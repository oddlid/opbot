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
- [ ] give feedback on wrong arguments?
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
	DEF_WMSG   string = "Welcome back, %s"
)

var (
	_bot    *bot.Bot
	_cfg    *irc.Config
	_conn   *ircevent.Connection
	_ops    *OPData
	_opfile string
	_wchan  = make(chan *HostMask)
)

func InitBot(b *bot.Bot, cfg *irc.Config, conn *ircevent.Connection, opfile string) error {
	_bot = b
	_cfg = cfg
	_conn = conn
	_opfile = opfile
	reload() // initializes _ops

	_conn.AddCallback(JOIN, onJOIN)
	_conn.AddCallback("311", on311) // reply from whois when nick found
	_conn.AddCallback("401", on401) // reply from whois when nick not found

	register()

	return nil
}

func register() {
	bot.RegisterCommand(
		"op",
		"Add or remove nicks for auto-OP",
		HelpMsg(),
		op,
	)
}

// 311 is the reply to WHOIS when nick found
func on311(e *ircevent.Event) {
	//log.Debugf("%+v", e)
	hm := &HostMask{
		Nick:     e.Arguments[1],
		UserID:   e.Arguments[2],
		Host:     e.Arguments[3],
		RealName: e.Arguments[5],
	}
	select {
	case _wchan <- hm:
		log.Debugf("Sent hostmask object on _wchan")
	default:
		log.Debugf("Unable to send on _wchan")
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
		log.Debugf("Sent NIL hostmask object on _wchan")
	default:
		log.Debugf("Unable to send on _wchan")
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

	// get info about user/nick
	go func() {
		log.Debugf("Goroutine waiting to read from _wchan...")
		hm := <-_wchan

		if hm == nil {
			log.Debugf("Got NIL hostmask back on _wchan. %q does not exist on server", nick)
			_bot.SendMessage(
				channel,
				fmt.Sprintf("Error adding %q - no such nick", nick),
				nil,
			)
			return
		}

		log.Debugf("Got back info about nick %q: %#v", nick, hm)

		_ops.Get(channel).Add(nick, hm.String())

		err := _ops.SaveFile(_opfile)
		if err != nil {
			log.Error(err)
		}

		_conn.Mode(channel, "+o", nick) // try to OP right away
	}()


	log.Debugf("Calling WHOIS on nick %q", nick)
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

func mask(channel, action, nick, hostmask string) (string, error) {
	return "", nil
}

func op(cmd *bot.Cmd) (string, error) {
	alen := len(cmd.Args)
	if alen == 0 {
		return PLUGIN + ": Arguments missing", nil
	}

	args := safeArgs(4, cmd.Args) // 4 is the longest possible set of valid args

	// check if user is allowed to run this command (is in op list, or read-only command)
	// Anyone is allowed anything if the list is empty
	if !okCmd(cmd.Channel, cmd.User.Nick, args[0], args[1]) {
		return fmt.Sprintf("%s: You must be in the OPs list to run this command", cmd.User.Nick), nil
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
	} else if arg(RELOAD) {
		reload()
		retmsg = PLUGIN + ": OPs DB reloaded"
	} else if arg(CLEAR) {
		clear()
		retmsg = PLUGIN + ": OPs DB cleared"
	}

	return retmsg, nil
}
