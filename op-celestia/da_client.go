package celestia

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	client "github.com/celestiaorg/celestia-openrpc"
	"github.com/celestiaorg/celestia-openrpc/types/share"
)

type DAClient struct {
	Client       *client.Client
	GetTimeout   time.Duration
	Namespace    share.Namespace
	FallbackMode string
	GasPrice     float64
}

func NewDAClient(rpc, token, namespace, fallbackMode string, gasPrice float64) (*DAClient, error) {
	client, err := client.NewClient(context.Background(), rpc, token)
	if err != nil {
		return nil, err
	}
	nsBytes, err := hex.DecodeString(namespace)
	if err != nil {
		return nil, err
	}
	ns, err := share.NewBlobNamespaceV0(nsBytes)
	if fallbackMode != "disabled" && fallbackMode != "blobdata" && fallbackMode != "calldata" {
		return nil, fmt.Errorf("celestia: unknown fallback mode: %s", fallbackMode)
	}
	return &DAClient{
		Client:       client,
		GetTimeout:   time.Minute,
		Namespace:    ns,
		FallbackMode: fallbackMode,
		GasPrice:     gasPrice,
	}, nil
}
