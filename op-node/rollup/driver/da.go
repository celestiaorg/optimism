package driver

import (
	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
)

func SetDAClient(cfg celestia.Config) error {
	client := celestia.NewDAClient(cfg.DaRpc, false)
	return derive.SetDAClient(client)
}
