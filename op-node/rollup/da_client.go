package rollup

import (
	"context"
	"encoding/hex"

	openrpc "github.com/rollkit/celestia-openrpc"
	"github.com/rollkit/celestia-openrpc/types/share"
)

type DAClient struct {
	Rpc       string
	Namespace share.Namespace
	Client    *openrpc.Client
	AuthToken string
}

func NewDAClient(cfg *DAConfig) (*DAClient, error) {
	nsBytes, err := hex.DecodeString(cfg.NamespaceID)
	if err != nil {
		return &DAClient{}, err
	}

	namespace, err := share.NewBlobNamespaceV0(nsBytes)
	if err != nil {
		return nil, err
	}

	client, err := openrpc.NewClient(context.Background(), cfg.RPC, cfg.AuthToken)
	if err != nil {
		return &DAClient{}, err
	}

	return &DAClient{
		Namespace: namespace,
		Rpc:       cfg.RPC,
		Client:    client,
	}, nil
}
