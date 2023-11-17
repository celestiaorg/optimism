package rollup

import (
	"github.com/rollkit/go-da/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type DAClient struct {
	Client *proxy.Client
}

func NewDAClient(rpc string) (*DAClient, error) {
	client := proxy.NewClient()
	err := client.Start(rpc, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &DAClient{
		Client: client,
	}, nil
}
