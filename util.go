package opbot

import (
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	glob "github.com/ryanuber/go-glob"
)

// Having this as a separate func makes it easier to debug output in dev
func HelpMsg() string {
	// Arguments:
	//	add  <nick>
	//	del  <nick>
	//	ls   [nick]
	//	wmsg <get|set> <message>
	//  mask <add|del|clear|ls> <nick> [hostmask]
	//  get
	//	reload
	//	clear
	n := "nick"
	return fmt.Sprintf(
		`arguments...
Where arguments can be one of:
  %s   <%s>
  %s   <%s>
  %s    [%s]
  %s  <%s|%s> <message>
  %s  <%s|%s|%s|%s> <%s> [hostmask]
  %s
  %s
  %s
`,
		ADD, n,
		DEL, n,
		LS, n,
		WMSG, GET, SET,
		MASK, ADD, DEL, CLEAR, LS, n,
		GET,
		RELOAD,
		CLEAR,
	)
}

func matchMask(pattern, mask string) bool {
	return glob.Glob(pattern, mask)
}

//func hostmask(mask string) *HostMask {
//	parts := strings.Split(mask, "@")
//	uparts := strings.Split(parts[0], "!")
//
//	return &HostMask{
//		Nick:   uparts[0],
//		UserID: uparts[1],
//		Host:   parts[1],
//	}
//}

func reload() {
	_ops = NewOPData().LoadFile(_opfile)
}

func clear() {
	_ops = NewOPData()
	err := _ops.SaveFile(_opfile)
	if err != nil {
		log.Error(err)
	}
}

func match(in, compare string) bool {
	return strings.ToUpper(in) == compare
}

func safeArgs(num int, args []string) []string {
	alen := len(args)
	res := make([]string, num)
	for i := 0; i < num; i++ {
		if i < alen {
			res[i] = args[i]
		} else {
			res[i] = ""
		}
	}
	return res
}

func okCmd(channel, nick, cmd, arg string) bool {
	c := _ops.Get(channel)
	if c.Empty() {
		// Need this "hack/hole", otherwise one can't start to fill the list
		return true
	}
	if c.Has(nick) {
		// compensate for onPRIVMSG not having been run if a bot command is the
		// first thing to be said in a channel after bot join
		if _caller.Nick == "" && _caller.Hostmask == "" {
			return true
		}
		// If nick and _caller.Nick are not the same, we are not in sync, and can
		// not check reliably against hostmask
		if nick != _caller.Nick {
			return true
		}
		// At this point it should be ok to check against hostmask
		if c.MatchHostMask(nick, _caller.Hostmask) {
			return true
		}
	}
	// Read only commands are always OK
	if match(cmd, LS) {
		return true
	}
	if match(cmd, WMSG) {
		if match(arg, "GET") {
			return true
		}
	}
	if match(cmd, MASK) {
		if match(arg, LS) {
			return true
		}
	}
	return false
}

func readWhois(timeout time.Duration) *HostMask {
	select {
	case hm := <-_wchan:
		return hm
	case <-time.After(timeout):
		return nil
	}
	return nil
}
