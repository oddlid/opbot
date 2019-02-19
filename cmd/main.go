// +build !make

package main

import (
	"fmt"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	//"github.com/go-chat-bot/bot"
	"github.com/go-chat-bot/bot/irc"
	"github.com/oddlid/opbot"
	"github.com/urfave/cli"
)

const (
	DEF_ADDR   string = "irc.oftc.net:6697"
	//DEF_ADDR   string = "ix1.undernet.org:6667" // set UseTLS to default false with Undernet
	DEF_USER   string = "opbot"
	DEF_NICK   string = "opbot"
	DEF_OPFILE string = opbot.DEF_OPFILE
)

const (
	E_OK = iota
	E_INIT_OPBOT
)

var (
	COMMIT_ID  string
	BUILD_DATE string
	VERSION    string
)


func entryPoint(ctx *cli.Context) error {
	//fmt.Println(opbot.HelpMsg())
	//return nil


	opfile := ctx.String("opfile")
	cfg := &irc.Config{
		Server:   ctx.String("server"),
		User:     ctx.String("user"),
		Nick:     ctx.String("nick"),
		Password: ctx.String("password"),
		Channels: ctx.StringSlice("channel"),
		UseTLS:   ctx.Bool("tls"),
		Debug:    ctx.Bool("debug"),
	}

	b, ic := irc.SetUpConn(cfg)
	err := opbot.InitBot(b, cfg, ic, opfile)
	if err != nil {
		return cli.NewExitError(err.Error(), E_INIT_OPBOT)
	}

	irc.Run(nil) // pass nil as we've ran SetUpConn with cfg

	return nil
}


func main() {
	app := cli.NewApp()
	app.Name = "opbot"
	app.Version = fmt.Sprintf("%s_%s (Compiled: %s)", VERSION, COMMIT_ID, BUILD_DATE)
	app.Compiled, _ = time.Parse(time.RFC3339, BUILD_DATE)
	app.Copyright = "(c) 2019 Odd Eivind Ebbesen"
	app.Authors = []cli.Author{
		cli.Author{
			Name:  "Odd E. Ebbesen",
			Email: "oddebb@gmail.com",
		},
	}
	app.Usage = "Run irc OP bot"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "server, s",
			Usage:  "IRC server `address`",
			Value:  DEF_ADDR,
			EnvVar: "IRC_SERVER",
		},
		cli.StringFlag{
			Name:   "user, u",
			Usage:  "IRC `username`",
			Value:  DEF_USER,
			EnvVar: "IRC_USER",
		},
		cli.StringFlag{
			Name:   "nick, n",
			Usage:  "IRC `nick`",
			Value:  DEF_NICK,
			EnvVar: "IRC_NICK",
		},
		cli.StringFlag{
			Name:   "password, p",
			Usage:  "IRC server `password`",
			EnvVar: "IRC_PASS",
		},
		cli.StringSliceFlag{
			Name:  "channel, c",
			Usage: "Channel to join. May be repeated. Specify \"#chan passwd\" if a channel needs a password.",
		},
		cli.BoolFlag{
			Name:   "tls, t",
			Usage:  "Use secure TLS connection",
			EnvVar: "IRC_TLS",
		},
		cli.StringFlag{
			Name:   "opfile",
			Usage:  "JSON `file` for loading/saving OPs userlist",
			EnvVar: "OPBOT_FILE",
			Value:  DEF_OPFILE,
		},
		cli.StringFlag{
			Name:  "log-level, l",
			Value: "info",
			Usage: "Log `level` (options: debug, info, warn, error, fatal, panic)",
		},
		cli.BoolFlag{
			Name:   "debug, d",
			Usage:  "Run in debug mode",
			EnvVar: "DEBUG",
		},
	}
	app.Before = func(c *cli.Context) error {
		log.SetOutput(os.Stderr)
		level, err := log.ParseLevel(c.String("log-level"))
		if err != nil {
			log.Fatal(err.Error())
		}
		log.SetLevel(level)
		if !c.IsSet("log-level") && !c.IsSet("l") && c.Bool("debug") {
			log.SetLevel(log.DebugLevel)
		}
		log.SetFormatter(&log.TextFormatter{
			DisableTimestamp: false,
			FullTimestamp:    true,
		})
		return nil
	}

	app.Action = entryPoint
	app.Run(os.Args)
}
