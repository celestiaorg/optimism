package celestia

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/rollkit/go-da"
	"github.com/rollkit/go-da/proxy"
)

type DAClient struct {
	Client       da.DA
	GetTimeout   time.Duration
	Namespace    da.Namespace
	FallbackMode string
	GasPrice     float64
	Indexer      BlockIndexer
}

func (dac *DAClient) IndexMapping(l2Range [2]*big.Int, celestiaHeight uint64, commitment []byte) {
	if dac.Indexer != nil {
		dac.Indexer.StoreMapping(l2Range[0], l2Range[1], celestiaHeight, commitment)
	}
}

func NewDAClient(rpc, token, namespace, fallbackMode string, gasPrice float64, idx BlockIndexer) (*DAClient, error) {
	client, err := proxy.NewClient(rpc, token)
	if err != nil {
		return nil, err
	}
	ns, err := hex.DecodeString(namespace)
	if err != nil {
		return nil, err
	}
	if fallbackMode != "disabled" && fallbackMode != "blobdata" && fallbackMode != "calldata" {
		return nil, fmt.Errorf("celestia: unknown fallback mode: %s", fallbackMode)
	}
	return &DAClient{
		Client:       client,
		GetTimeout:   time.Minute,
		Namespace:    ns,
		FallbackMode: fallbackMode,
		GasPrice:     gasPrice,
		Indexer:      idx,
	}, nil
}
