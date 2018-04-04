package integration

import (
	"reflect"
	"runtime"
	"testing"
)

type Topology func(c *cluster)

func TopologyStar(c *cluster) {
	meta, err := c.Nodes[0].NodeMeta()
	if err != nil {
		c.t.Fatal(err)
	}
	for _, node := range c.Nodes[1:] {
		if err := node.AddNode(meta); err != nil {
			c.t.Fatalf("%+v", err)
		}
	}
}

func TopologyFullyConnected(c *cluster) {
	for i, node := range c.Nodes {
		meta, err := node.NodeMeta()
		if err != nil {
			c.t.Fatal(err)
		}
		for j, node := range c.Nodes {
			if i == j {
				continue
			}

			if err := node.AddNode(meta); err != nil {
				c.t.Fatalf("%+v", err)
			}
		}
	}
}

func TopologyLine(c *cluster) {
	for i, node := range c.Nodes[1:] {
		meta, err := c.Nodes[i-1].NodeMeta()
		if err != nil {
			c.t.Fatal(err)
		}
		if err := node.AddNode(meta); err != nil {
			c.t.Fatalf("%+v", err)
		}
	}
}

func MultiTopologyTest(
	t *testing.T, topos []Topology, n int, f func(t *testing.T, ts *cluster), opts ...func(*cluster),
) {
	if len(topos) == 0 {
		t.Fatal("no topologies specified")
	}
	for _, topo := range topos {
		name := runtime.FuncForPC(reflect.ValueOf(topo).Pointer()).Name()
		t.Run(name, func(t *testing.T) {
			topts := append([]func(*cluster){
				func(c *cluster) {
					c.Topology = topo
				},
				func(c *cluster) {
					c.NodeConfig.MaxPeers = 0
				},
			}, opts...)
			ts := NewTestCluster(t, n, topts...)
			defer ts.Close()

			f(t, ts)
		})
	}
}
