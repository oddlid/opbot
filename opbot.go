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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/oddlid/bot"
	"github.com/oddlid/bot/irc"
	ircevent "github.com/thoj/go-ircevent"
)

const (
	JOIN       string = "JOIN"
	ADD        string = "ADD"
	DEL        string = "DEL"
	LS         string = "LS"
	RELOAD     string = "RELOAD"
	CLEAR      string = "CLEAR"
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
)

type Channel struct {
	sync.RWMutex
	WelcomeMsg string          `json:"wmsg"`
	OPs        map[string]bool `json:"ops"`
}

type OPData struct {
	sync.RWMutex
	Modified time.Time           `json:"modified"`
	Channels map[string]*Channel `json:"channels"`
}

func InitBot(b *bot.Bot, cfg *irc.Config, conn *ircevent.Connection, opfile string) error {
	_bot = b
	_cfg = cfg
	_conn = conn
	_opfile = opfile
	reload()                 // initializes _ops
	bot.UseUnidecode = false // needed if we want to save messages with unicode characters

	_conn.AddCallback(JOIN, onJOIN)

	register()

	return nil
}

// Having this as a separate func makes it easier to debug output in dev
func HelpMsg() string {
	// Arguments:
	//	add  <nick>
	//	del  <nick>
	//	ls   [nick]
	//	wmsg <message>
	//	reload
	//	clear
	n := "nick"
	return fmt.Sprintf(
		`arguments...
Where arguments can be one of:
  %s   <%s>
  %s   <%s>
  %s    [%s]
  %s  <GET|SET> <message>
  %s
  %s
`,
		ADD, n, DEL, n, LS, n, WMSG, RELOAD, CLEAR,
	)
}

func register() {
	bot.RegisterCommand(
		"op",
		"Add or remove nicks for auto-OP",
		HelpMsg(),
		op,
	)
}

func NewOPData() *OPData {
	return &OPData{
		Modified: time.Now(),
		Channels: make(map[string]*Channel),
	}
}

func (o *OPData) Load(r io.Reader) error {
	jb, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(jb, o)
}

func (o *OPData) LoadFile(filename string) *OPData {
	file, err := os.Open(filename)
	if err != nil {
		log.Errorf("%s: OPData.LoadFile() Error: %q", PLUGIN, err.Error())
		return o
	}
	defer file.Close()
	err = o.Load(file)
	if err != nil {
		log.Error(err)
		return NewOPData()
	}
	log.Infof("%s: OPs list (re)loaded from file %q", PLUGIN, filename)
	return o
}

func (o *OPData) Save(w io.Writer) (int, error) {
	o.Modified = time.Now() // update timestamp
	jb, err := json.MarshalIndent(o, "", "\t")
	if err != nil {
		return 0, err
	}
	jb = append(jb, '\n')
	return w.Write(jb)
}

func (o *OPData) SaveFile(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	n, err := o.Save(file)
	if err != nil {
		return err
	}
	log.Infof("%s: Saved %d bytes to %q", PLUGIN, n, filename)
	return nil
}

func (o *OPData) Get(channel string) *Channel {
	c, found := o.Channels[channel]
	if !found {
		log.Debugf("%s: Creating channel %q with empty oplist", PLUGIN, channel)
		c = &Channel{
			OPs: make(map[string]bool),
		}
		o.Channels[channel] = c
	}
	return c
}

func (c *Channel) Has(nick string) bool {
	c.RLock()
	_, found := c.OPs[nick]
	c.RUnlock()
	return found
}

func (c *Channel) Add(nick string) {
	c.Lock()
	c.OPs[nick] = true
	c.Unlock()
}

func (c *Channel) Remove(nick string) {
	c.Lock()
	delete(c.OPs, nick)
	c.Unlock()
}

func (c *Channel) Nicks() []string {
	nicks := make([]string, 0, len(c.OPs))
	c.RLock()
	for k := range c.OPs {
		nicks = append(nicks, k)
	}
	c.RUnlock()
	sort.Strings(nicks)
	return nicks
}

func (c *Channel) Empty() bool {
	return len(c.OPs) == 0
}

func (c *Channel) GetWMsg(nick string) string {
	if nick != "" && strings.Index(c.WelcomeMsg, "%s") > -1 {
		return fmt.Sprintf(c.WelcomeMsg, nick)
	}
	return c.WelcomeMsg
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
	_ops.Get(channel).Add(nick)
	err := _ops.SaveFile(_opfile)
	if err != nil {
		log.Error(err)
	}
	_conn.Mode(channel, "+o", nick) // try to OP right away
	return fmt.Sprintf("%s: Nick %q added to OPs list", PLUGIN, nick), err
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
		return true
	}
	if match(cmd, LS) {
		return true
	}
	if match(cmd, WMSG) {
		if match(arg, "GET") {
			return true
		}
	}
	return false
}

func op(cmd *bot.Cmd) (string, error) {
	// Arguments:
	//  add  <nick>
	//  del  <nick>
	//  ls   [nick]
	//  wmsg <get|set> <message>
	//  reload
	//  clear
	//

	alen := len(cmd.Args)
	if alen == 0 {
		return PLUGIN + ": Arguments missing", nil
	}

	args := safeArgs(2, cmd.Args) // 2 is the longest possible set of valid args

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
