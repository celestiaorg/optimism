package rpc

import (
	"context"

	"github.com/ethereum/go-ethereum/log"
	gethrpc "github.com/ethereum/go-ethereum/rpc"

	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/ethereum-optimism/optimism/op-service/apis"
	"github.com/ethereum-optimism/optimism/op-service/rpc"
)

type BatcherDriver interface {
	StartBatchSubmitting() error
	StopBatchSubmitting(ctx context.Context) error
	GetCelestiaLocation(ctx context.Context, l2Block uint64) (*celestia.CelestiaLocation, error)
}

type adminAPI struct {
	*rpc.CommonAdminAPI
	b BatcherDriver
}

var _ apis.BatcherAdminServer = (*adminAPI)(nil)

func NewAdminAPI(dr BatcherDriver, log log.Logger) *adminAPI {
	return &adminAPI{
		CommonAdminAPI: rpc.NewCommonAdminAPI(log),
		b:              dr,
	}
}

func GetAdminAPI(api *adminAPI) gethrpc.API {
	return gethrpc.API{
		Namespace: "admin",
		Service:   api,
	}
}

func (a *adminAPI) StartBatcher(_ context.Context) error {
	return a.b.StartBatchSubmitting()
}

func (a *adminAPI) StopBatcher(ctx context.Context) error {
	return a.b.StopBatchSubmitting(ctx)
}

func (a *adminAPI) GetCelestiaLocation(ctx context.Context, l2Block uint64) (*celestia.CelestiaLocation, error) {
	return a.b.GetCelestiaLocation(ctx, l2Block)
}
