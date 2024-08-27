package celestia

import (
	"encoding/hex"
	"fmt"
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
}

func NewDAClient(rpc, token, namespace, fallbackMode string, gasPrice float64) (*DAClient, error) {
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
	}, nil
}
