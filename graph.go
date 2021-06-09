package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

type graph map[string]*Server

type Server struct {
	Name        string    `json:"name"`
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Version     string    `json:"version"`
	Peers       []*Server `json:"-"`
}

func (s Server) NameID() string {
	return fmt.Sprintf("%s (%s)", s.Name, s.ID)
}

func (s *Server) String() string {
	peers := []string{}
	for _, v := range s.Peers {
		peers = append(peers, v.NameID())
	}

	return fmt.Sprintf("%s connected to %s", s.NameID(), strings.Join(peers, ", "))
}

func (s *Server) HasPeer(other *Server) bool {
	for _, p := range s.Peers {
		if p.Name == other.Name {
			return true
		}
	}

	return false
}

func getJSON(target string) ([]byte, error) {
	r, err := http.Get(target)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func getGraph(target string) (out graph, err error) {
	defer func() {
		res := recover()
		if res != nil {
			err = res.(error)
		}
	}()

	type jsonStruct struct {
		Servers map[string]*Server `json:"nodes"`
		Links   [][2]string        `json:"links"`
	}

	parsedJSON := new(jsonStruct)
	jsonData, err := getJSON(target)
	if err != nil {
		return nil, fmt.Errorf("could not get JSON data: %w", err)
	}

	if err := json.Unmarshal(jsonData, parsedJSON); err != nil {
		return nil, err
	}

	for id, server := range parsedJSON.Servers {
		server.ID = id
	}

	for _, linkPair := range parsedJSON.Links {
		one := parsedJSON.Servers[linkPair[0]]
		two := parsedJSON.Servers[linkPair[1]]

		one.Peers = append(one.Peers, two)
		two.Peers = append(two.Peers, one)

	}

	return parsedJSON.Servers, nil
}

var mapRe = regexp.MustCompile(`^(?P<name>\S+)\s*\(\d+\)\s(?P<id>\S+)$`)

func graphFromLinksAndMap(links [][]string, sMap []string) (graph, error) {
	servers := graph(make(map[string]*Server, len(links)))

	for _, line := range sMap {
		line = strings.TrimLeft(line, "`|- ")
		match := mapRe.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("%s does not match regexp", line)
		}

		name := match[mapRe.SubexpIndex("name")]
		id := match[mapRe.SubexpIndex("id")]
		fmt.Println(name, id)
		servers[id] = &Server{Name: name, ID: id, Version: "Unknown"}
	}

	/*
		>> @time=2021-06-09T12:08:37.995Z :irc.awesome-dragon.science 364 A_Dragon urine.trouble.pissnet.xyz irc.awesome-dragon.science :1 Urine Trouble
		>> @time=2021-06-09T12:08:37.996Z :irc.awesome-dragon.science 364 A_Dragon irc.awesome-dragon.science irc.awesome-dragon.science :0 Draconic Pissnet.
		>> @time=2021-06-09T12:08:37.996Z :irc.awesome-dragon.science 365 A_Dragon * :End of /LINKS list.
	*/

	fmt.Println("And now, onto the LINKS")
	for _, line := range links {
		serv1Name := line[0]
		serv2Name := line[1]
		serv1Desc := line[2]

		serv1 := servers.getServer(serv1Name)
		serv2 := servers.getServer(serv2Name)
		if serv1 == nil {
			// MAP didnt contain this server. Do our best to add data for it
			fmt.Printf("UNKNOWN SERVER %s! Creating fake ID\n", serv1Name)
			serv1 = &Server{Name: serv1Name, Description: serv1Desc}
			servers["UNKNOWN"+serv1Name] = serv1
		}

		if serv2 == nil {
			fmt.Printf("UNKNOWN SERVER %s! Creating fake ID\n", serv2Name)
			serv2 = &Server{Name: serv2Name}
			servers["UNKNOWN"+serv2Name] = serv2
		}

		if serv1.Description == "" {
			serv1.Description = serv1Desc
		}

		if !serv1.HasPeer(serv2) {
			serv1.Peers = append(serv1.Peers, serv2)
		}

		if !serv2.HasPeer(serv1) {
			serv2.Peers = append(serv2.Peers, serv1)
		}
	}

	return servers, nil
}

// func graphFromList(list []string) graph {
// 	//<serverfrom> <serverto> :<hops-from-server-you-are-on> <serverfrom-description>
// 	servers = []*Server{}
// }

func stringSliceContains(s string, slice []string) bool {
	for _, v := range slice {
		if s == v {
			return true
		}
	}
	return false
}

func (g graph) distanceToPeer(start, end *Server) int {
	if start == end {
		return 0
	}
	distances := g.allDistancesFrom(start)

	if res, exists := distances[end]; exists {
		return res
	} else {
		return -1
	}
}

func (g graph) keys() []string {
	out := []string{}
	for k := range g {
		out = append(out, k)
	}

	sort.Strings(out)
	return out
}

func (g graph) values() (out []*Server) {
	keys := g.keys()
	for _, k := range keys {
		out = append(out, g[k])
	}

	return out
}

func (g graph) allDistancesFrom(source *Server) map[*Server]int {
	toCheck := []*Server{}
	toCheck = append(toCheck, source.Peers...)
	count := 0

	out := make(map[*Server]int)
	out[source] = 0
	for {
		next := []*Server{}
		for _, server := range toCheck {
			out[server] = count + 1
			for _, s := range server.Peers {
				if _, exists := out[s]; exists {
					continue // skip servers we've already seen
				}
				next = append(next, s)
			}
		}
		toCheck = next
		count++
		if len(toCheck) == 0 {
			break
		}
	}
	return out
}

// func (g graph) largestDistance2(source *Server) (int, *Server) {
// }

func (g graph) largestDistanceFrom(source *Server) (int, *Server) {
	bestHopCount := -1
	var bestServer *Server
	servers := g.values()
	distances := g.allDistancesFrom(source)

	for _, other := range servers {
		if hc := distances[other]; hc > bestHopCount {
			bestHopCount = hc
			bestServer = other
		}
	}

	return bestHopCount, bestServer
}

func (g graph) getServer(nameOrID string) *Server {
	res, exists := g[nameOrID]
	if exists {
		return res
	}

	for _, s := range g {
		if s.Name == nameOrID {
			return s
		}
	}

	return nil
}

func (g graph) mostPeers() *Server {
	best := -1
	var bestServer *Server
	for _, srv := range g {
		if len(srv.Peers) > best {
			best = len(srv.Peers)
			bestServer = srv
		}
	}

	return bestServer
}
