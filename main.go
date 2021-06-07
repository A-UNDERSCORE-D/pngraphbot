package main

import (
	"fmt"
	"strings"

	irc "github.com/thoj/go-ircevent"
)

const (
	host = "https://ioserv.hellomouse.net/graph/json"
	ircd = "irc.awesome-dragon.science:6697"
)

func main() {
	b := NewBot("graphbot", "pissing-on-graphs")
	b.run(ircd)
}

type bot struct {
	ircCon   *irc.Connection
	commands map[string]string
}

func NewBot(nick, user string) *bot {
	irccon := irc.IRC(nick, user)
	irccon.Debug = true
	irccon.UseTLS = true
	b := &bot{irccon, make(map[string]string)}
	defaultSources := []string{"A_Dragon", "#opers"}

	irccon.AddCallback("PRIVMSG", b.commandWrapper(
		"biggesthop", "Find largest number of hops between two servers", defaultSources, -1, b.maxHops,
	))

	b.ircCon.AddCallback("PRIVMSG", b.commandWrapper(
		"biggesthopfrom", "Find the furthest server from the given server", defaultSources, 1, b.maxHopsFrom,
	))

	irccon.AddCallback("PRIVMSG", b.commandWrapper(
		"spof", "Find the server with the most peers", defaultSources, -1, b.singlePointOfFailure,
	))

	irccon.AddCallback("PRIVMSG", b.commandWrapper(
		"singlepointoffailure", "Find the server with the most peers", defaultSources, -1, b.singlePointOfFailure,
	))

	irccon.AddCallback("PRIVMSG", b.commandWrapper(
		"peercount", "Get the number of peers for the given server", defaultSources, 1, b.peerCount,
	))

	irccon.AddCallback("PRIVMSG", b.commandWrapper(
		"hopsbetween", "get the number of hops between two servers", defaultSources, 2, b.peerCount,
	))
	irccon.AddCallback("PRIVMSG", b.commandWrapper(
		"hb", "get the number of hops between two servers", defaultSources, 2, b.peerCount,
	))

	irccon.AddCallback("PRIVMSG", b.commandWrapper("help", "Take a guess.", nil, -1, b.doHelp))

	return b
}

func (b *bot) run(server string) {
	b.ircCon.Connect(server)
	b.ircCon.Loop()
}

func (b *bot) commandWrapper(command, desc string, allowedSources []string, numArgs int, callback func(e *irc.Event, args []string)) func(e *irc.Event) {
	cmd := "~" + command
	b.commands[cmd] = desc
	b.commands[command] = desc
	return func(e *irc.Event) {
		message := e.MessageWithoutFormat()
		splitMsg := strings.Split(message, " ")

		if len(splitMsg) == 0 || splitMsg[0] != cmd {
			return
		}

		if allowedSources != nil && !(stringSliceContains(e.Arguments[0], allowedSources) || stringSliceContains(e.Nick, allowedSources)) {
			b.ircCon.Log.Printf("Skipping as %s or %s not found in %v", e.Source, e.Nick, allowedSources)
			return
		}

		if numArgs != -1 && len(splitMsg[1:]) < numArgs {
			b.replyTof(e, "%q requires at least %d arguments", cmd, numArgs)
			return
		}

		callback(e, splitMsg[1:])
	}
}

func (b *bot) maxHops(e *irc.Event, _ []string) {
	target := e.Arguments[0]
	if target == b.ircCon.GetNick() {
		// was a PM
		target = e.Nick
	}

	go func() {
		gr, err := getGraph(host)
		if err != nil {
			b.ircCon.Privmsgf(target, "Error: %s", err)
			return
		}

		biggestHop, pair := gr.largestDistance()
		b.ircCon.Privmsgf(target, "Largest hop size is %d! between %s and %s", biggestHop, pair[0].NameID(), pair[1].NameID())
	}()
}

func (b *bot) replyTo(e *irc.Event, message string) {
	target := e.Arguments[0]
	if target == b.ircCon.GetNick() {
		// was a PM
		target = e.Nick
	}

	b.ircCon.Privmsg(target, message)
}

func (b *bot) replyTof(e *irc.Event, format string, args ...interface{}) {
	b.replyTo(e, fmt.Sprintf(format, args...))
}

func (b *bot) maxHopsFrom(e *irc.Event, args []string) {
	go func() {
		gr, err := getGraph(host)
		if err != nil {
			b.replyTof(e, "Error: %s", err)
			return
		}

		from := gr.getServer(args[0])
		if from == nil {
			b.replyTof(e, "Server ID %q doesnt exist!", args[0])
			return
		}

		biggestHop, srv := gr.largestDistanceFrom(from.ID)
		b.replyTof(e, "Largest hop size from %s is %d! other side is %s", from.NameID(), biggestHop, srv.NameID())
	}()
}

func (b *bot) singlePointOfFailure(e *irc.Event, _ []string) {
	go func() {
		gr, err := getGraph(host)
		if err != nil {
			b.replyTof(e, "Error: %s", err)
			return
		}

		mostPeers := gr.mostPeers()
		b.replyTof(e, "Server with the most peers is %s with %d peers!", mostPeers.NameID(), len(mostPeers.Peers))
	}()
}

func (b *bot) peerCount(e *irc.Event, args []string) {
	go func() {
		gr, err := getGraph(host)
		if err != nil {
			b.replyTof(e, "Error: %s", err)
			return
		}

		srv := gr.getServer(args[0])
		if srv == nil {
			b.replyTof(e, "Server ID / name %q doesn't exist!", args[0])
			return
		}

		b.replyTof(e, "%s has %d peers!", srv.NameID(), len(srv.Peers))
	}()
}

func (b *bot) doHelp(e *irc.Event, args []string) {
	if len(args) == 0 {
		keys := []string{}
		for k := range b.commands {
			keys = append(keys, k)
		}
		b.replyTof(e, "available commands: %s", strings.Join(keys, ", "))
		return
	}

	asked := args[0]
	desc, exists := b.commands[asked]
	if !exists {
		b.replyTof(e, "unknown command: %q", asked)
		return
	}

	b.replyTof(e, "help for %q: %s", asked, desc)
}

func (b *bot) hopsBetween(e *irc.Event, args []string) {
	go func() {
		gr, err := getGraph(host)
		if err != nil {
			b.replyTof(e, "Error: %s", err)
			return
		}

		one := gr.getServer(args[0])
		two := gr.getServer(args[1])

		if one == nil {
			b.replyTof(e, "Server ID / name %q doesn't exist!", args[2])
			return
		}

		if two == nil {
			b.replyTof(e, "Server ID / name %q doesn't exist!", args[1])
			return
		}

		dst := gr.distanceToPeer(one.ID, two.ID)

		b.replyTof(e, "there are %d hops between %s and %s", dst, one.NameID(), two.NameID())
	}()
}
