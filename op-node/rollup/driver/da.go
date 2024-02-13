package driver

import (
	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/metrics"
)

func SetDAClient(cfg celestia.Config, m *metrics.RPCMetrics) error {
	client, err := celestia.NewDAClient(cfg.DaRpc, m)
	if err != nil {
		return err
	}
	return derive.SetDAClient(client)
}
