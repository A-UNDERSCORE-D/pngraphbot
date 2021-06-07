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
		panic(err)
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

func (g graph) distanceToPeer(startID, endID string) int {
	if startID == endID {
		return 0
	}

	current := g[startID]
	toCheck := []*Server{}
	visited := []string{} // Unlikely but just in case
	toCheck = append(toCheck, current.Peers...)
	depth := 1
	for {
		if len(toCheck) == 0 {
			break
		}

		next := []*Server{}
		for _, c := range toCheck {
			if c.ID == endID {
				return depth
			}

			visited = append(visited, c.ID)
			next = append(next, c.Peers...)
		}

		// we didn't find it that round
		toCheck = toCheck[:0]
		for _, v := range next {
			if stringSliceContains(v.ID, visited) {
				continue
			}

			toCheck = append(toCheck, v)
		}
		depth++
	}

	return -1
}

func (g graph) keys() []string {
	out := []string{}
	for k := range g {
		out = append(out, k)
	}

	sort.Strings(out)
	return out
}

func (g graph) largestDistance() (int, [2]*Server) {
	bestHopCount := -1
	var bestHopPair [2]*Server
	servers := g.keys()
	for i, id := range servers {
		for _, other := range servers[i:] {
			dst := g.distanceToPeer(id, other)
			if dst > bestHopCount {
				bestHopCount = dst
				bestHopPair = [2]*Server{g[id], g[other]}
			}
		}
	}

	return bestHopCount, bestHopPair
}

func (g graph) largestDistanceFrom(sourceID string) (int, *Server) {
	bestHopCount := -1
	var bestServer *Server
	servers := g.keys()

	for _, other := range servers {
		if other == sourceID {
			continue
		}

		if dst := g.distanceToPeer(sourceID, other); dst > bestHopCount {
			bestHopCount = dst
			bestServer = g[other]
		}
	}

	return bestHopCount, bestServer
}
