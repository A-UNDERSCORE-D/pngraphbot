package main

import (
	"strings"

	irc "github.com/thoj/go-ircevent"
)

const host = "https://ioserv.hellomouse.net/graph/json"

func main() {
	irccon := irc.IRC("graphbot", "adgb")
	// irccon.VerboseCallbackHandler = true
	irccon.Debug = true
	irccon.UseTLS = true
	irccon.AddCallback("PRIVMSG", func(e *irc.Event) {
		msg := e.MessageWithoutFormat()
		target := e.Arguments[0]
		if !strings.HasPrefix("~biggesthop", msg) || target != "#opers" {
			return
		}
		if target == irccon.GetNick() {
			// was a PM
			target = e.Nick
		}
		irccon.Privmsg(target, "Fetching....")
		go func() {
			gr, err := getGraph(host)
			if err != nil {
				irccon.Privmsgf(target, "Error: %s", err)
				return
			}

			biggestHop, pair := gr.largestDistance()
			irccon.Privmsgf(target, "Largest hop size is %d! between %s and %s", biggestHop, pair[0].NameID(), pair[1].NameID())
		}()
	})

	irccon.Connect("irc.awesome-dragon.science:6697")

	irccon.Loop()

	// fmt.Println(g.distanceToPeer("1AD", "248"))

	// fmt.Println(bestHopCount, bestHopPair)
}
