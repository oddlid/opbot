# OPBot

A very simple IRC bot for maintaining OPs in a channel.

The bot is implemented as a library, so you can include it in your own bot based on `github.com/oddlid/bot` (a fork of `github.com/go-chat-bot/bot`).
For a stand-alone version, see [github.com/oddlid/opbot/cmd/](cmd/).

Installation
------------

```console
$ go get -d -u github.com/oddlid/opbot
```

Usage
-----


To create and use an instance of this bot, you need to:

* Import the package `github.com/oddlid/bot/irc`
* Import the package `github.com/oddlid/opbot`
* Fill the `irc.Config` struct
* Call `irc.SetUpConn` with the `irc.Config` struct
* Call `opbot.InitBot`, passing in the values returned from `SetUpConn`, the `Config` struct and a filename
* Call `irc.Run(nil)`

Example:
```Go
package main

import (
	"os"
	"strings"

	"github.com/oddlid/bot/irc"
	"github.com/oddlid/opbot"
)

func main() {
	cfg := &irc.Config{
		Server:   os.Getenv("IRC_SERVER"),
		User:     os.Getenv("IRC_USER"),
		Nick:     os.Getenv("IRC_NICK"),
		Password: os.Getenv("IRC_PASSWORD"),
		Debug:    os.Getenv("DEBUG") != "",
		Channels: strings.Split(os.Getenv("IRC_CHANNELS"), ","),
		UseTLS:   true,
	}

	botinst, conn := irc.SetUpConn(cfg)
	err := opbot.InitBot(botinst, cfg, conn, "/path/to/oplist.json")
	if err != nil {
		return
	}

	irc.Run(nil) // pass nil as we've ran SetUpConn with cfg
}
```
Once the bot is in your irc channel, you can interact with it like this:

```
16:57  @Oddlid | !help
16:57    opbot | Type: '!help <command>' to see details about a specific command.
16:57    opbot | Available commands: op
16:57  @Oddlid | !help op
16:57    opbot | Description: Add or remove nicks for auto-OP
16:57    opbot | Usage: !op arguments...
16:57    opbot | Where arguments can be one of:
16:57    opbot |   ADD  <nick>
16:57    opbot |   DEL  <nick>
16:58    opbot |   LS   [nick]
16:58    opbot |   WMSG <GET|SET> <message>
16:58    opbot |   RELOAD
16:58    opbot |   CLEAR
17:20  @Oddlid | !op add Oddlid
17:21    opbot | OPBot: Nick "Oddlid" added to OPs list
17:23  @Oddlid | !op ls Oddlid
17:23    opbot | OPBot: Oddlid is registered as OP
17:23  @Oddlid | !op ls NoNick
17:24    opbot | OPBot: NoNick is NOT registered as OP
17:23  @Oddlid | !op ls
17:24    opbot | OPBot: OPs for #channel: Oddlid
17:26  @Oddlid | !op del Oddlid
17:26    opbot | OPBot: Nick "Oddlid" removed from OPs list
17:27  @Oddlid | !op ls
17:27    opbot | OPBot: No configured OPs for channel "#channel"
17:27  @Oddlid | !op wmsg set Welcome back, dear %s
17:27    opbot | OPBot: Welcome message for channel #channel: "Welcome back, dear %s"

```
