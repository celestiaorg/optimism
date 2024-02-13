package celestia

import (
	"time"

	"github.com/ethereum-optimism/optimism/op-service/metrics"
	"github.com/rollkit/go-da/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type DAClient struct {
	Client     *proxy.Client
	Metrics    metrics.RPCMetricer
	GetTimeout time.Duration
}

func NewDAClient(rpc string, m metrics.RPCMetricer) (*DAClient, error) {
	client := proxy.NewClient()
	err := client.Start(rpc, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &DAClient{
		Client:     client,
		Metrics:    m,
		GetTimeout: time.Minute,
	}, nil
}
