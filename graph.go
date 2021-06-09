package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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

func getGraph(target string) (graph, error) {
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

// Parsing LINKS and MAP will work to get all the required data.
