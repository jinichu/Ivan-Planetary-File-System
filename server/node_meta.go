package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"net"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"
	"strconv"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
)

func validateNodeMeta(meta serverpb.NodeMeta) error {
	if meta.Id == "" {
		return errors.Errorf("missing Id")
	}
	if meta.Signature == "" {
		return errors.Errorf("missing Signature")
	}
	if meta.Cert == "" {
		return errors.Errorf("missing Cert")
	}
	if meta.PublicKey == "" {
		return errors.Errorf("missing PublicKey")
	}
	if meta.Updated == 0 {
		return errors.Errorf("missing Updated")
	}
	if len(meta.Addrs) == 0 {
		return errors.Errorf("missing Addrs")
	}

	wantId := nodeMetaId(meta)
	if wantId != meta.Id {
		return errors.Errorf("invalid Id: got %q, expected %q", meta.Id, wantId)
	}

	publicKey, err := nodeMetaPublicKey(meta)
	if err != nil {
		return err
	}

	rawSig, err := base64.URLEncoding.DecodeString(meta.Signature)
	if err != nil {
		return err
	}
	var sig EcdsaSignature
	if _, err := asn1.Unmarshal(rawSig, &sig); err != nil {
		return err
	}

	hash, err := nodeMetaHash(meta)
	if err != nil {
		return err
	}

	if !ecdsa.Verify(publicKey, hash, sig.R, sig.S) {
		return errors.Errorf("%+v: invalid Signature", meta)
	}

	return nil
}

func nodeMetaHash(meta serverpb.NodeMeta) ([]byte, error) {
	meta.Signature = ""
	body, err := meta.Marshal()
	if err != nil {
		return nil, err
	}
	hash := sha1.Sum(body)
	return hash[:], nil
}

func nodeMetaPublicKey(meta serverpb.NodeMeta) (*ecdsa.PublicKey, error) {
	pub, err := x509.ParsePKIXPublicKey([]byte(meta.PublicKey))
	if err != nil {
		return nil, err
	}

	key, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.Errorf("invalid public key: %+v", pub)
	}
	return key, nil
}

func nodeMetaSign(meta serverpb.NodeMeta, key *ecdsa.PrivateKey) (string, error) {
	hash, err := nodeMetaHash(meta)
	if err != nil {
		return "", err
	}
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return "", err
	}
	sig, err := asn1.Marshal(EcdsaSignature{R: r, S: s})
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(sig), nil
}

func nodeMetaId(meta serverpb.NodeMeta) string {
	id := sha1.Sum([]byte(meta.PublicKey))
	return base64.URLEncoding.EncodeToString(id[:])
}

func (s *Server) NodeMeta() (serverpb.NodeMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	publicKey, err := x509.MarshalPKIXPublicKey(&s.key.PublicKey)
	if err != nil {
		return serverpb.NodeMeta{}, err
	}

	meta := serverpb.NodeMeta{
		Cert:      s.certPublic,
		PublicKey: string(publicKey),
		Updated:   time.Now().Unix(),
	}
	meta.Id = nodeMetaId(meta)

	if s.mu.l != nil {
		addr := s.mu.l.Addr()
		tcpAddr := addr.(*net.TCPAddr)
		if tcpAddr.IP.IsUnspecified() {
			port := strconv.Itoa(tcpAddr.Port)
			meta.Addrs = append(meta.Addrs, net.JoinHostPort(getOutboundIP().String(), port))
			meta.Addrs = append(meta.Addrs, net.JoinHostPort("localhost", port))
			/*
				ifaces, err := net.Interfaces()
				if err != nil {
					return serverpb.NodeMeta{}, err
				}
				for _, i := range ifaces {
					addrs, err := i.Addrs()
					if err != nil {
						return serverpb.NodeMeta{}, err
					}
					for _, addr := range addrs {
						var ip net.IP
						switch v := addr.(type) {
						case *net.IPNet:
							ip = v.IP
						case *net.IPAddr:
							ip = v.IP
						}
						possibleAddr := net.JoinHostPort(ip.String(), strconv.Itoa(tcpAddr.Port))
						meta.Addrs = append(meta.Addrs, possibleAddr)
					}
				}
			*/
		} else {
			meta.Addrs = append(meta.Addrs, addr.String())
		}
	}

	sig, err := nodeMetaSign(meta, s.key)
	if err != nil {
		return serverpb.NodeMeta{}, err
	}
	meta.Signature = sig

	return meta, nil
}

// addNodeMeta adds a node meta object to the server and returns whether or not
// that node has been seen before.
func (s *Server) addNodeMeta(meta serverpb.NodeMeta) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.mu.peerMeta[meta.Id]
	if !ok || old.Updated < meta.Updated {
		s.mu.peerMeta[meta.Id] = meta
	}
	return !ok
}

func (s *Server) persistNodeMeta(meta serverpb.NodeMeta) error {
	body, err := meta.Marshal()
	if err != nil {
		return err
	}
	key := fmt.Sprintf("/NodeMeta/%s", meta.Id)
	if err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), body)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Server) Meta(ctx context.Context, req *serverpb.MetaRequest) (*serverpb.NodeMeta, error) {
	meta, err := s.NodeMeta()
	if err != nil {
		return nil, err
	}
	return &meta, nil
}
