package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	irc "github.com/thoj/go-ircevent"
)

const (
	host   = "https://ioserv.hellomouse.net/graph/json"
	ircd   = "irc.awesome-dragon.science:6697"
	prefix = "~"
)

const (
	RPL_LINKS      = "364"
	RPL_ENDOFLINKS = "365"
	RPL_MAP        = "006"
	RPL_ENDOFMAP   = "007"
	PRIVMSG        = "PRIVMSG"
)

func main() {
	b := NewBot("graphbot", "pissing-on-graphs")
	b.run(ircd)
}

type bot struct {
	ircCon         *irc.Connection
	commands       map[string]string
	commandAliases map[string][]string

	lastLINKS          [][]string
	lastMAP            []string
	mapLinksMutex      sync.Mutex
	updatingLinkAndMap bool
}

func NewBot(nick, user string) *bot {
	irccon := irc.IRC(nick, user)
	irccon.Debug = true
	irccon.UseTLS = true
	b := &bot{
		ircCon:         irccon,
		commands:       make(map[string]string),
		commandAliases: make(map[string][]string),
	}

	b.ircCon.AddCallback("001", func(e *irc.Event) {
		if res := os.Getenv("OPERIDENT"); res != "" {
			b.ircCon.SendRaw("OPER " + res)
		}

		b.ircCon.Join("#opers")
	})

	defaultSources := []string{"A_Dragon", "#opers"}

	b.addChatCommand("biggesthop", "Find largest number of hops between two servers, now fasterer", defaultSources, -1, b.maxHops, "bh", "howfucked")
	b.addChatCommand("biggesthopfrom", "Find the furthest server from the given server", defaultSources, 1, b.maxHopsFrom, "bhf", "howfuckedis")
	b.addChatCommand("singlepointoffailure", "Find the server with the most peers", defaultSources, -1, b.singlePointOfFailure, "spof")
	b.addChatCommand("peercount", "Get the number of peers for the given server", defaultSources, 1, b.peerCount, "pc", "peecount")
	b.addChatCommand("hopsbetween", "get the number of hops between two servers", defaultSources, 2, b.hopsBetween, "hb")
	b.addChatCommand("help", "Take a guess.", nil, -1, b.doHelp)
	b.addChatCommand("count", "Current server count", defaultSources, 0, func(e *irc.Event, _ []string) {
		go func() {
			b.updateLinksAndMap()
			// g, err := getGraph(host)
			g, err := graphFromLinksAndMap(b.lastLINKS, b.lastMAP)
			if err != nil {
				b.replyTof(e, "Error: %s", err)
			}
			b.replyTof(e, "Currently there are %d servers on the network", len(g))
		}()
	})

	b.addChatCommand("test", "", defaultSources, 0, func(e *irc.Event, args []string) {
		go func() {
			b.updateLinksAndMap()
			g, err := graphFromLinksAndMap(b.lastLINKS, b.lastMAP)
			if err != nil {
				fmt.Println(err)
			}

			fmt.Println(g.mostPeers())
		}()
	})

	b.addChatCommand("howfucked2", "", defaultSources, 0, b.maxHops2)
	b.addChatCommand("graphsizes", "", defaultSources, 0, func(e *irc.Event, args []string) {
		go func() {
			g1, err := getGraph(host)
			b.replyTof(e, "net: %d %s", len(g1), err)
		}()

		go func() {
			err1 := b.updateLinksAndMap()
			g2, err2 := graphFromLinksAndMap(b.lastLINKS, b.lastMAP)
			b.replyTof(e, "l+m: %d %s | %s", len(g2), err1, err2)
		}()
	})

	b.addChatCommand("doboth", "", defaultSources, 0, func(e *irc.Event, args []string) {
		b.maxHops(e, args)
		b.maxHops2(e, args)
	})

	b.addChatCommand("compare", "", defaultSources, 0, func(e *irc.Event, args []string) {
		go func() {
			g1, err := getGraph(host)
			if err != nil {
				b.replyTof(e, "Error: %s", err)
			}
			b.updateLinksAndMap()
			g2, err := graphFromLinksAndMap(b.lastLINKS, b.lastMAP)
			if err != nil {
				b.replyTof(e, "Error: %s", err)
			}

			for _, server := range g1 {
				otherServer := g2.getServer(server.Name)
				if otherServer == nil {
					fmt.Println("G2 is missing", server)
					continue
				}
			outer:
				for _, peer := range server.Peers {
					for _, otherPeer := range otherServer.Peers {
						if peer.Name == otherPeer.Name {
							continue outer
						}
					}
					fmt.Printf("Server ||%s|| on g1 has peer %s but g2 does not: ||%s||\n", server.NameID(), peer.NameID(), otherServer)
				}
			}
		}()
	})

	b.addChatCommand("update", "updates cached links and maps", defaultSources, 0, func(e *irc.Event, args []string) {
		go func() {
			if err := b.updateLinksAndMap(); err != nil {
				b.replyTof(e, "Error: %s", err)
			}
			b.replyTo(e, "Done")
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
	b.ircCon.AddCallback(PRIVMSG, b.commandWrapper(command, allowedSources, numArgs, callback))
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
		b.updateLinksAndMap()
		gr, err := graphFromLinksAndMap(b.lastLINKS, b.lastMAP)
		// gr, err := getGraph(host)
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
		biggestHop, srv := gr.largestDistanceFrom(from)
		b.replyTof(
			e, "Largest hop size from %s is %d! other side is %s (search took %s)",
			from.NameID(), biggestHop, srv.NameID(), time.Since(t),
		)
	}()
}

func (b *bot) singlePointOfFailure(e *irc.Event, _ []string) {
	go func() {
		b.updateLinksAndMap()
		gr, err := graphFromLinksAndMap(b.lastLINKS, b.lastMAP)
		// gr, err := getGraph(host)
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
		b.updateLinksAndMap()
		gr, err := graphFromLinksAndMap(b.lastLINKS, b.lastMAP)
		// gr, err := getGraph(host)
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
		b.updateLinksAndMap()
		gr, err := graphFromLinksAndMap(b.lastLINKS, b.lastMAP)
		// gr, err := getGraph(host)
		if err != nil {
			b.replyTof(e, "Error: %s", err)
			return
		}

		one := gr.getServer(args[0])
		two := gr.getServer(args[1])

		if one == nil {
			b.replyTof(e, "Server ID / name %q doesn't exist!", args[0])
			return
		}

		if two == nil {
			b.replyTof(e, "Server ID / name %q doesn't exist!", args[1])
			return
		}
		t := time.Now()
		dst := gr.distanceToPeer(one, two)

		b.replyTof(
			e, "there are %d hops between %s and %s (Search took %s)",
			dst, one.NameID(), two.NameID(), time.Since(t),
		)
	}()
}

func (b *bot) maxHops(e *irc.Event, _ []string) {
	go func() {
		gr, err := getGraph(host)
		if err != nil {
			b.replyTof(e, "Error: %s", err)
			return
		}

		t := time.Now()
		var bestPair [2]*Server
		best := -1
		for _, start := range gr {
			distances := gr.allDistancesFrom(start)
			for other, d := range distances {
				if d > best {
					best = d
					bestPair = [2]*Server{start, other}
				}
			}
		}

		b.replyTof(
			e,
			"Largest hop size is %d! between %s and %s (search took %s)",
			best, bestPair[0].NameID(), bestPair[1].NameID(), time.Since(t),
		)
	}()
}

func (b *bot) maxHops2(e *irc.Event, _ []string) {
	go func() {
		if err := b.updateLinksAndMap(); err != nil {
			b.replyTof(e, "Error: %s", err)
			return
		}
		gr, err := graphFromLinksAndMap(b.lastLINKS, b.lastMAP)
		if err != nil {
			b.replyTof(e, "Error: %s", err)
			return
		}

		t := time.Now()
		var bestPair [2]*Server
		best := -1
		for _, start := range gr {
			distances := gr.allDistancesFrom(start)
			for other, d := range distances {
				if d > best {
					best = d
					bestPair = [2]*Server{start, other}
				}
			}
		}

		b.replyTof(
			e,
			"Largest hop size is %d! between %s and %s (search took %s)",
			best, bestPair[0].NameID(), bestPair[1].NameID(), time.Since(t),
		)
	}()
}

// Parsing LINKS and MAP will work to get all the required data.

func (b *bot) updateLinksAndMap() error {
	if b.updatingLinkAndMap {
		return errors.New("already updating")
	}

	b.mapLinksMutex.Lock()
	defer b.mapLinksMutex.Unlock()

	linksChan := make(chan []string)
	mapChan := make(chan string)
	currentLinks := [][]string{}
	currentMap := []string{}

	go func() {
		for res := range linksChan {
			currentLinks = append(currentLinks, res)
		}
	}()

	go func() {
		for res := range mapChan {
			currentMap = append(currentMap, res)
		}
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	defer func() { b.updatingLinkAndMap = false }()
	linksCB := b.ircCon.AddCallback(RPL_LINKS, func(e *irc.Event) { linksChan <- e.Arguments[1:] })
	linksEndCB := b.ircCon.AddCallback(RPL_ENDOFLINKS, func(_ *irc.Event) {
		defer wg.Done()
		b.ircCon.RemoveCallback(RPL_LINKS, linksCB)
		close(linksChan)
	})

	mapCB := b.ircCon.AddCallback(RPL_MAP, func(e *irc.Event) { mapChan <- e.MessageWithoutFormat() })
	mapEndCB := b.ircCon.AddCallback(RPL_ENDOFMAP, func(_ *irc.Event) {
		defer wg.Done()
		b.ircCon.RemoveCallback(RPL_MAP, mapCB)
		close(mapChan)
	})

	b.ircCon.SendRaw("MAP")
	b.ircCon.SendRaw("LINKS")

	wg.Wait()

	b.ircCon.RemoveCallback(RPL_ENDOFMAP, mapEndCB)
	b.ircCon.RemoveCallback(RPL_ENDOFLINKS, linksEndCB)

	b.lastLINKS = currentLinks
	b.lastMAP = currentMap

	return nil
}
