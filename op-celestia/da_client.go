package celestia

import (
	"encoding/hex"
	"time"

	"github.com/rollkit/go-da"
	"github.com/rollkit/go-da/proxy"
)

type DAClient struct {
	Client     da.DA
	GetTimeout time.Duration
	Namespace da.Namespace
}

func NewDAClient(rpc, token, namespace string) (*DAClient, error) {
	client, err := proxy.NewClient(rpc, token)
	if err != nil {
		return nil, err
	}
	ns, err := hex.DecodeString(namespace)
	if err != nil {
		return nil, err
	}
	return &DAClient{
		Client:     client,
		GetTimeout: time.Minute,
		Namespace: ns,
	}, nil
}
