package main

import (
	"log"

	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/server"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/routing_table"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/routing_tablepb"
)

func main() {


	go runRT()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	s, err := server.New(serverpb.NodeConfig{
		Path:     "tmp/node1",
		MaxPeers: 10,
	})
	if err != nil {
		return err
	}
	return s.Listen(":0")
}

func runRT() error {
	rt, err := routing_table.NewRT(routing_tablepb.NodeConfig{
		Path: "tmp/node1",
	});

	if err != nil {
		return err
	}

	return rt.ListenRT(":0")
}