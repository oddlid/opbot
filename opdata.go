package opbot

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

	log "github.com/sirupsen/logrus"
)

type OPData struct {
	sync.RWMutex
	Modified time.Time           `json:"modified"`
	Channels map[string]*Channel `json:"channels"`
}

type Channel struct {
	sync.RWMutex
	WelcomeMsg string              `json:"wmsg"`
	OPs        map[string][]string `json:"ops"`
}

type HostMask struct {
	Nick     string `json:"nick"`
	UserID   string `json:"userid"`
	Host     string `json:"host"`
	//RealName string `json:"realname"`
}

type Caller struct {
	Nick     string
	Hostmask string
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
	o.Lock()
	defer o.Unlock()
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
	const fn string = "OPData.Get()"
	c, found := o.Channels[channel]
	if !found {
		devdbg("%s: %s: Creating channel %q with empty oplist", PLUGIN, fn, channel)
		c = &Channel{
			OPs: make(map[string][]string),
		}
		o.Channels[channel] = c
	}
	return c
}

func (c *Channel) MatchHostMask(nick, mask string) bool {
	c.RLock()
	defer c.RUnlock()

	maskList, found := c.OPs[nick]
	if !found {
		return false
	}
	// If nick exists, but have no masks, we deny it.
	if len(maskList) == 0 {
		return false
	}
	for _, pattern := range maskList {
		if matchMask(pattern, mask) {
			return true
		}
	}
	return false
}

func (c *Channel) Has(nick string) bool {
	c.RLock()
	_, found := c.OPs[nick]
	c.RUnlock()
	return found
}

func (c *Channel) addNoDup(nick, mask string) bool {
	c.Lock()
	defer c.Unlock()

	_, found := c.OPs[nick]
	if found && c.OPs[nick] != nil {
		for i := range c.OPs[nick] {
			if c.OPs[nick][i] == mask {
				return false
			}
		}
	}
	c.OPs[nick] = append(c.OPs[nick], mask)
	return true
}

func (c *Channel) Add(nick, mask string) bool {
	const fn string = "Channel.Add()"
	added := c.addNoDup(nick, mask)
	if !added {
		devdbg("%s: %s: Mask %q already in list for %q", PLUGIN, fn, mask, nick)
	} else {
		devdbg("%s: %s: Added nick %q with mask %q", PLUGIN, fn, nick, mask)
	}
	return added
}

func (c *Channel) RemoveHostmask(nick, mask string) bool {
	c.Lock()
	defer c.Unlock()

	_, found := c.OPs[nick]
	if !found {
		return false
	}
	if c.OPs[nick] == nil || len(c.OPs[nick]) == 0 {
		return false
	}

	dirty := false
	newmasks := make([]string, 0, len(c.OPs[nick]))
	for _, m := range c.OPs[nick] {
		if m == mask {
			dirty = true
			continue
		}
		newmasks = append(newmasks, m)
	}
	if dirty {
		c.OPs[nick] = newmasks
	}
	return dirty
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

func (c *Channel) Hostmasks(nick string) []string {
	c.RLock()
	defer c.RUnlock()

	u, found := c.OPs[nick]
	if !found {
		return nil
	}
	return u
}

func (c *Channel) ClearHostmasks(nick string) bool {
	c.Lock()
	defer c.Unlock()

	_, found := c.OPs[nick]
	if !found {
		return false
	}
	c.OPs[nick] = nil
	return true
}

func (c *Channel) Empty() bool {
	c.RLock()
	defer c.RUnlock()
	return len(c.OPs) == 0
}

func (c *Channel) GetWMsg(nick string) string {
	c.RLock()
	defer c.RUnlock()

	if nick != "" && strings.Index(c.WelcomeMsg, "%s") > -1 {
		return fmt.Sprintf(c.WelcomeMsg, nick)
	}
	return c.WelcomeMsg
}

func (h *HostMask) String() string {
	return fmt.Sprintf("%s!%s@%s", h.Nick, h.UserID, h.Host)
}
