package main

import (
	"fmt"
	"strings"
	"time"

	irc "github.com/thoj/go-ircevent"
)

const (
	host   = "https://ioserv.hellomouse.net/graph/json"
	ircd   = "irc.awesome-dragon.science:6697"
	prefix = "~"
)

func main() {
	b := NewBot("graphbot", "pissing-on-graphs")
	b.run(ircd)
}

type bot struct {
	ircCon         *irc.Connection
	commands       map[string]string
	commandAliases map[string][]string
}

func NewBot(nick, user string) *bot {
	irccon := irc.IRC(nick, user)
	irccon.Debug = true
	irccon.UseTLS = true
	b := &bot{irccon, make(map[string]string), make(map[string][]string)}
	defaultSources := []string{"A_Dragon", "#opers"}

	b.addChatCommand("biggesthop", "Find largest number of hops between two servers", defaultSources, -1, b.maxHops, "bh", "howfucked")
	b.addChatCommand("biggesthopfrom", "Find the furthest server from the given server", defaultSources, 1, b.maxHopsFrom, "bhf", "howfuckedis")
	b.addChatCommand("singlepointoffailure", "Find the server with the most peers", defaultSources, -1, b.singlePointOfFailure, "spof")
	b.addChatCommand("peercount", "Get the number of peers for the given server", defaultSources, 1, b.peerCount, "pc", "peecount")
	b.addChatCommand("hopsbetween", "get the number of hops between two servers", defaultSources, 2, b.hopsBetween, "hb")
	b.addChatCommand("help", "Take a guess.", nil, -1, b.doHelp)
	b.addChatCommand("count", "Current server count", defaultSources, 0, func(e *irc.Event, args []string) {
		go func() {
			g, err := getGraph(host)
			if err != nil {
				b.replyTof(e, "Error: %s", err)
			}
			b.replyTof(e, "Currently there are %d servers on the network", len(g))
		}()
	})

	return b
}

func (b *bot) run(server string) {
	b.ircCon.Connect(server)
	b.ircCon.Loop()
}

func (b *bot) addChatCommand(command, desc string, allowedSources []string, numArgs int, callback func(e *irc.Event, args []string), aliases ...string) {
	b.commands[command] = desc
	b.commandAliases[command] = append(b.commandAliases[command], aliases...)
	b.ircCon.AddCallback("PRIVMSG", b.commandWrapper(command, allowedSources, numArgs, callback))
}

func (b *bot) matchesCommandOrAlias(s string) (string, bool) {
	s = strings.TrimPrefix(s, prefix)

	for c := range b.commands {
		if c == s {
			return s, true
		}

		for _, alias := range b.commandAliases[c] {
			if alias == s {
				return c, true
			}
		}
	}
	return "", false
}

func (b *bot) commandWrapper(command string, allowedSources []string, numArgs int, callback func(e *irc.Event, args []string)) func(e *irc.Event) {
	cmd := "~" + command
	return func(e *irc.Event) {
		message := strings.TrimSpace(e.MessageWithoutFormat())
		splitMsg := strings.Split(message, " ")
		if !strings.HasPrefix(splitMsg[0], prefix) {
			return
		}

		realCommand, exists := b.matchesCommandOrAlias(splitMsg[0])
		if !exists {
			return
		}

		if len(splitMsg) == 0 || realCommand != command {
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
	go func() {
		gr, err := getGraph(host)
		if err != nil {
			b.replyTof(e, "Error: %s", err)
			return
		}
		t := time.Now()
		biggestHop, pair := gr.largestDistance()
		b.replyTof(
			e,
			"Largest hop size is %d! between %s and %s (search took %s)",
			biggestHop, pair[0].NameID(), pair[1].NameID(), time.Since(t),
		)
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
		t := time.Now()
		biggestHop, srv := gr.largestDistanceFrom(from.ID)
		b.replyTof(
			e, "Largest hop size from %s is %d! other side is %s (search took %s)",
			from.NameID(), biggestHop, srv.NameID(), time.Since(t),
		)
	}()
}

func (b *bot) singlePointOfFailure(e *irc.Event, _ []string) {
	go func() {
		gr, err := getGraph(host)
		if err != nil {
			b.replyTof(e, "Error: %s", err)
			return
		}

		t := time.Now()
		mostPeers := gr.mostPeers()
		b.replyTof(
			e, "Server with the most peers is %s with %d peers! (Search took %s)",
			mostPeers.NameID(), len(mostPeers.Peers), time.Since(t),
		)
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
			aliases := ""
			if a := b.commandAliases[k]; len(a) > 0 {
				aliases = fmt.Sprintf(" (%s)", strings.Join(a, ", "))
			}
			keys = append(keys, fmt.Sprint(k, aliases))

		}
		b.replyTof(e, "available commands: %s", strings.Join(keys, ", "))
		return
	}

	asked := args[0]
	realCmd, exists := b.matchesCommandOrAlias(asked)
	if !exists {
		b.replyTof(e, "unknown command: %q", asked)
		return
	}

	desc := b.commands[realCmd]
	aliases := ""
	if len(b.commandAliases[realCmd]) > 0 {
		aliases = fmt.Sprintf(" -- Aliases: %s", strings.Join(b.commandAliases[realCmd], ", "))
	}

	b.replyTof(e, "help for %q: %s%s", realCmd, desc, aliases)
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
		t := time.Now()
		dst := gr.distanceToPeer(one.ID, two.ID)

		b.replyTof(
			e, "there are %d hops between %s and %s (Search took %s)",
			dst, one.NameID(), two.NameID(), time.Since(t),
		)
	}()
}
