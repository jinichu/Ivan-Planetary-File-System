package integration

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
)

func randType(t reflect.Type) reflect.Value {
	return reflect.New(t.Elem())
}

func TestFuzzRPC(t *testing.T) {
	ts := NewTestCluster(t, 3)
	defer ts.Close()

	n := ts.Nodes[0]
	nv := reflect.ValueOf(n)
	grpcServer := n.TestGRPCServer()
	info := grpcServer.GetServiceInfo()

	ifaces := []interface{}{(*serverpb.NodeServer)(nil), (*serverpb.ClientServer)(nil)}

	ctx := reflect.ValueOf(context.Background())

	if len(info) != len(ifaces) {
		t.Fatalf("grpcServer interfaces don't match tests")
	}

	for _, iface := range ifaces {
		typ := reflect.TypeOf(iface).Elem()
		typeName := filepath.Base(typ.PkgPath()) + "." + strings.TrimSuffix(typ.Name(), "Server")
		if _, ok := info[typeName]; !ok {
			t.Fatalf("server missing type %q, has %+v", typeName, info)
		}

		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			t.Run(typeName+"."+m.Name, func(t *testing.T) {
				reqType := m.Type.In(1)

				{
					req := reflect.Zero(reqType)
					_ = nv.MethodByName(m.Name).Call([]reflect.Value{ctx, req})
				}

				{
					req := randType(reqType)
					_ = nv.MethodByName(m.Name).Call([]reflect.Value{ctx, req})
				}
			})
		}
	}
}
