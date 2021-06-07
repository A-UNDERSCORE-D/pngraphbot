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
	ircCon *irc.Connection
}

func NewBot(nick, user string) *bot {
	irccon := irc.IRC(nick, user)
	irccon.Debug = true
	irccon.UseTLS = true
	b := &bot{irccon}
	defaultSources := []string{"A_Dragon", "#opers"}

	irccon.AddCallback("PRIVMSG", b.commandWrapper(
		"biggesthop", defaultSources, -1, b.maxHops,
	))

	b.ircCon.AddCallback("PRIVMSG", b.commandWrapper(
		"largesthopfrom", defaultSources, 1, b.maxHopsFrom,
	))

	return b
}

func (b *bot) run(server string) {
	b.ircCon.Connect(server)
	b.ircCon.Loop()
}

func (b *bot) commandWrapper(command string, allowedSources []string, numArgs int, callback func(e *irc.Event, args []string)) func(e *irc.Event) {
	cmd := "~" + command
	return func(e *irc.Event) {
		message := e.MessageWithoutFormat()
		splitMsg := strings.Split(message, " ")

		if len(splitMsg) == 0 || splitMsg[0] != cmd {
			return
		}

		if !(stringSliceContains(e.Arguments[0], allowedSources) || stringSliceContains(e.Nick, allowedSources)) {
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
		if _, exists := gr[args[0]]; !exists {
			b.replyTof(e, "Server ID %q doesnt exist!", args[0])
			return
		}

		biggestHop, srv := gr.largestDistanceFrom(args[0])
		b.replyTof(e, "Largest hop size is %d! other side is %s", biggestHop, srv.NameID())
	}()
}

// func (b *bot) getList() <-chan graph {
// }
