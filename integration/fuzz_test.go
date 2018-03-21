package integration

import (
	"context"
	"crypto/rand"
	"log"
	mrand "math/rand"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
)

const (
	fuzzRunsPerFunc = 1000
	nilChance       = 0.9
)

func randType(t reflect.Type) reflect.Value {
	k := t.Kind()
	switch k {
	case reflect.Ptr:
		v := reflect.New(t.Elem())
		if mrand.Float64() < nilChance {
			v.Elem().Set(randType(t.Elem()))
		}
		return v
	case reflect.Struct:
		v := reflect.New(t).Elem()
		for i := 0; i < v.NumField(); i++ {
			v.Field(i).Set(randType(v.Field(i).Type()))
		}
		return v
	case reflect.String:
		buf := make([]byte, mrand.Intn(1000))
		if _, err := rand.Reader.Read(buf); err != nil {
			log.Panic(err)
		}
		return reflect.ValueOf(string(buf))
	case reflect.Array, reflect.Slice:
		len := mrand.Intn(100)
		v := reflect.MakeSlice(t, len, len)
		for i := 0; i < v.Len(); i++ {
			v.Index(i).Set(randType(t.Elem()))
		}
		return v
	case reflect.Int32:
		return reflect.ValueOf(mrand.Int31())
	case reflect.Int64:
		return reflect.ValueOf(mrand.Int63())
	case reflect.Uint8:
		return reflect.ValueOf(uint8(mrand.Uint32()))
	case reflect.Map:
		v := reflect.MakeMap(t)
		len := mrand.Intn(100)
		for i := 0; i < len; i++ {
			v.SetMapIndex(randType(t.Key()), randType(t.Elem()))
		}
		return v
	default:
		log.Panicf("no support for kind %+v", t)
		return reflect.Value{}
	}
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

				for j := 0; j < fuzzRunsPerFunc; j++ {
					req := randType(reqType)
					_ = nv.MethodByName(m.Name).Call([]reflect.Value{ctx, req})
				}
			})
		}
	}
}
