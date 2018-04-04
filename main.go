package main

import (
	"flag"
	"log"
	"strings"

	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/server"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
)

var (
	path      = flag.String("path", "tmp/node1", "the path to store data in")
	bootstrap = flag.String("bootstrap", "", "addresses to bootstrap with, comma separated")
	bind      = flag.String("bind", ":0", "the address to bind to")
	maxPeers  = flag.Int("maxPeers", 10, "maximum number of peers")
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	flag.Parse()

	s, err := server.New(serverpb.NodeConfig{
		Path:     *path,
		MaxPeers: int32(*maxPeers),
	})
	if err != nil {
		return err
	}

	if len(*bootstrap) > 0 {
		addrs := strings.Split(*bootstrap, ",")
		go func() {
			for _, addr := range addrs {
				if err := s.BootstrapAddNode(nil, addr); err != nil {
					log.Printf("bootstrap error: %+v", err)
				}
			}
		}()
	}

	return s.Listen(*bind)
}
