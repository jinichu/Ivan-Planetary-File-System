package server

import (
	"context"
	"errors"
	"fmt"
	"proj2_f5w9a_h6v9a_q7w9a_r8u8_w1c0b/serverpb"

	"github.com/dgraph-io/badger"
)

func (s *Server) GetRemoteFile(ctx context.Context, in *serverpb.GetRemoteFileRequest) (*serverpb.GetRemoteFileResponse, error) {
	num_hops := int32(in.GetNumHops())
	documentID := in.GetDocumentId()

	if num_hops > 0 {
		for _, conn := range s.mu.peerConns {
			client := serverpb.NewNodeClient(conn)
			args := &serverpb.GetRemoteFileRequest{
				DocumentId: documentID,
				NumHops:    num_hops - 1,
			}
			resp, err := client.GetRemoteFile(ctx, args)

			if err != nil {
				fmt.Println(err)
			} else {
				return &serverpb.GetRemoteFileResponse{FileHash: resp.FileHash}, nil
			}
		}
		fmt.Println("couldn't find the document at all.")

		return nil, errors.New("missing Document")
	} else {
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
		}); err != nil {
			if err != badger.ErrKeyNotFound {
				return nil, err
			} else if err == badger.ErrKeyNotFound {
				return nil, errors.New("missing Document")
			}
		}
		resp := &serverpb.GetRemoteFileResponse{
			FileHash: body,
		}
		return resp, nil
	}
}
