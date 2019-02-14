# OPBot - the command

The command line stand-alone version of `github.com/oddlid/opbot`  
A very simple IRC bot for maintaining OPs in a channel.

See [github.com/oddlid/opbot](../) for how to interact with the bot on IRC.


Installation
------------

```console
$ go get -d -u github.com/oddlid/opbot/cmd
$ cd $GOPATH/src/github.com/oddlid/opbot/cmd
$ make
```
This will give you a binary `opbot.bin` in the current directory.  
You could also run `make install` to install the binary into `$GOPATH/bin/`.

If you want to cross-compile from Linux or OSX to Windows, run `make opbot.exe` instead.  
If you want to build on Windows, you'll probably need to adjust `Makefile` or run the proper build commands directly yourself. I don't know.


Usage:
------
```
NAME:
   opbot - Run irc OP bot

USAGE:
   opbot.bin [global options] command [command options] [arguments...]

VERSION:
   2019-02-14_13ecfbc (Compiled: 2019-02-14T16:53:11+01:00)

AUTHOR:
   Odd E. Ebbesen <oddebb@gmail.com>

COMMANDS:
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --server address, -s address      IRC server address (default: "irc.oftc.net:6697") [$IRC_SERVER]
   --user username, -u username      IRC username (default: "opbot") [$IRC_USER]
   --nick nick, -n nick              IRC nick (default: "opbot") [$IRC_NICK]
   --password password, -p password  IRC server password [$IRC_PASS]
   --channel value, -c value         Channel to join. May be repeated. Specify "#chan passwd" if a channel needs a password.
   --tls, -t                         Use secure TLS connection [$IRC_TLS]
   --opfile file                     JSON file for loading/saving OPs userlist (default: "/tmp/opbot.json") [$OPBOT_FILE]
   --log-level level, -l level       Log level (options: debug, info, warn, error, fatal, panic) (default: "info")
   --debug, -d                       Run in debug mode [$DEBUG]
   --help, -h                        show help
   --version, -v                     print the version

COPYRIGHT:
   (c) 2019 Odd Eivind Ebbesen
```

Remember to OP your bot after it has joined your channel, so it will be able to give others OP as well.
