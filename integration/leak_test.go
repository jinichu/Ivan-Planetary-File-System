package integration

import (
	"testing"

	"github.com/fortytw2/leaktest"
)

func TestLeakNode(t *testing.T) {
	defer leaktest.Check(t)()

	ts := NewTestCluster(t, 1)
	defer ts.Close()
}

func TestLeakCluster(t *testing.T) {
	defer leaktest.Check(t)()

	ts := NewTestCluster(t, 3)
	defer ts.Close()
}
