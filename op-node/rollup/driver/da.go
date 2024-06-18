package driver

import (
	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
)

func SetDAClient(cfg celestia.CLIConfig) error {
	// NOTE: for reading from the DA, the fallback mode doesn't matter
	// because we always read using blob_data_source.go which is
	// the most permissive mode.
	client, err := celestia.NewDAClient(cfg.Rpc, cfg.AuthToken, cfg.Namespace, celestia.FallbackModeBlobData)
	if err != nil {
		return err
	}
	return derive.SetDAClient(client)
}
