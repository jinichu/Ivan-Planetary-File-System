package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha1"
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
)

var (
	ErrNumHops = errors.New("max number of hops reached")
)

func (s *Server) GetRemoteFile(ctx context.Context, req *serverpb.GetRemoteFileRequest) (*serverpb.GetRemoteFileResponse, error) {
	documentID := req.GetDocumentId()

	var body []byte
	if err := s.db.View(func(txn *badger.Txn) error {
		key := fmt.Sprintf("/document/%s", documentID)
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		body, err = item.Value()
		if err != nil {
			return err
		}
		return nil
	}); err == badger.ErrKeyNotFound {
		if req.GetNumHops() == 0 {
			return nil, errors.Wrapf(ErrNumHops, "documentID: %s", documentID)
		}

		// Look document up via the network.
		routes := s.peersWithFile(documentID)
		if len(routes) == 0 {
			return nil, errors.Errorf("no routes to document: %s", documentID)
		}
		var err error
		for _, route := range routes {
			if err != nil {
				s.log.Printf("GetRemoteFile intermediate error: %+v", err)
				err = nil
			}

			numHops := req.GetNumHops()
			if numHops == -1 {
				numHops = route.NumHops
			}
			var resp *serverpb.GetRemoteFileResponse
			resp, err = route.Client.GetRemoteFile(ctx, &serverpb.GetRemoteFileRequest{
				DocumentId: documentID,
				NumHops:    numHops,
			})
			if err != nil {
				continue
			}

			hash := HashBytes(resp.Body)
			if hash != documentID {
				err = errors.Errorf("document hash didn't match requested ID")
				continue
			}

			//TODO: call caching code here.

			return resp, nil
		}
		return nil, errors.Wrapf(err, "failed to find document: %s", documentID)
	} else if err != nil {
		return nil, err
	}

	resp := &serverpb.GetRemoteFileResponse{
		Body: body,
	}
	return resp, nil
}

func (s *Server) GetRemoteReference(ctx context.Context, req *serverpb.GetRemoteReferenceRequest) (*serverpb.GetRemoteReferenceResponse, error) {

	referenceID := req.GetReferenceId()

	var body []byte

	// Try to get reference locally first
	if err := s.db.View(func(txn *badger.Txn) error {
		key := fmt.Sprintf("/reference/%s", referenceID)
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		body, err = item.Value()
		if err != nil {
			return err
		}
		return nil
	}); err == badger.ErrKeyNotFound {
		if req.GetNumHops() == 0 {
			return nil, errors.Wrapf(ErrNumHops, "referenceID: %s", referenceID)
		}

		// Look reference up via the network.
		routes := s.peersWithFile(referenceID)
		if len(routes) == 0 {
			return nil, errors.Errorf("no routes to reference: %s", referenceID)
		}
		var err error
		for _, route := range routes {
			if err != nil {
				s.log.Printf("GetRemoteReference intermediate error: %+v", err)
				err = nil
			}

			numHops := req.GetNumHops()
			if numHops == -1 {
				numHops = route.NumHops
			}
			var resp *serverpb.GetRemoteReferenceResponse
			err = func() error {
				resp, err = route.Client.GetRemoteReference(ctx, &serverpb.GetRemoteReferenceRequest{
					ReferenceId: referenceID,
					NumHops:     numHops,
				})
				if err != nil {
					return err
				}

				reference := resp.GetReference()

				signature, err := base64.URLEncoding.DecodeString(reference.Signature)
				if err != nil {
					return err
				}
				var sig EcdsaSignature
				if _, err := asn1.Unmarshal(signature, &sig); err != nil {
					return err
				}

				hash, err := Hash(reference.PublicKey)
				if err != nil {
					return err
				}
				if hash != req.GetReferenceId() {
					return errors.Errorf("public key doesn't match reference ID")
				}

				publicKey, err := UnmarshalPublic(reference.PublicKey)
				if err != nil {
					return err
				}
				ref2 := *reference
				ref2.Signature = ""
				bytes, err := ref2.Marshal()
				if err != nil {
					return err
				}
				refHash := sha1.Sum(bytes)
				if !ecdsa.Verify(publicKey, refHash[:], sig.R, sig.S) {
					return errors.Errorf("invalid signature received")
				}

				return nil
			}()
			if err != nil {
				continue
			}

			return resp, nil
		}
		return nil, errors.Wrapf(err, "failed to find reference: %s", referenceID)
	} else if err != nil {
		// Error wasn't an error relating to the reference not being found locally. Return.
		return nil, err
	}

	var reference serverpb.Reference
	if err := reference.Unmarshal(body); err != nil {
		return nil, err
	}

	return &serverpb.GetRemoteReferenceResponse{
		Reference: &reference,
	}, nil
}
