package driver

import (
	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
)

func SetDAClient(cfg celestia.CLIConfig) error {
	// NOTE: we always read using blob_data_source.go
	// If the transaction has calldata, based on the prefix byte.
	//     - If the prefix byte is 0xce
	//         - We interpret the calldata as a celestia reference and fetch
	//           the data from celestia.
	//     - Otherwise, we use the calldata fallback mode.
	// If the transaction has blobs, we use blobdata fallback mode.
	// See dataAndHashesFromTxs and DataFromEVMTransactions
	// The read path always operates in the most permissive mode and is
	// independent of the fallback mode.
	// Therefore the configuration value for FallbackMode passed here does not matter.
	client, err := celestia.NewDAClient(cfg.Rpc, cfg.AuthToken, cfg.Namespace, cfg.FallbackMode, cfg.GasPrice)
	if err != nil {
		return err
	}
	return derive.SetDAClient(client)
}
