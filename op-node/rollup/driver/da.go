package driver

import (
	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
)

func SetDAClient(cfg celestia.CLIConfig) error {
	// NOTE: for reading from the DA, we always read using blob_data_source.go
	// Based on the calldata prefix, it may either read from the DA or from the L1 chain.
	// In addition, it always reads from the DA for blobs.
	// See dataAndHashesFromTxs and DataFromEVMTransactions
	// The read path is independent of the fallback mode.
	// Therefore the configuration value for FallbackModeBlobData passed here does not matter.
	client, err := celestia.NewDAClient(cfg.Rpc, cfg.AuthToken, cfg.Namespace, cfg.FallbackMode)
	if err != nil {
		return err
	}
	return derive.SetDAClient(client)
}
