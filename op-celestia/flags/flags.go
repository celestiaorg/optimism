package flags

import (
	"fmt"
	"time"

	"github.com/urfave/cli/v2"

	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	opservice "github.com/ethereum-optimism/optimism/op-service"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	opmetrics "github.com/ethereum-optimism/optimism/op-service/metrics"
	"github.com/ethereum-optimism/optimism/op-service/oppprof"
	oprpc "github.com/ethereum-optimism/optimism/op-service/rpc"
)

const EnvVarPrefix = "OP_CELESTIA_INDEXER"

func prefixEnvVars(name string) []string {
	return opservice.PrefixEnvVar(EnvVarPrefix, name)
}

var (
	// Required flags
	StartL1BlockFlag = &cli.Uint64Flag{
		Name:    "start-l1-block",
		Usage:   "Starting L1 block number for indexing",
		EnvVars: prefixEnvVars("START_L1_BLOCK"),
	}
	BatchInboxAddressFlag = &cli.StringFlag{
		Name:    "batch-inbox-address",
		Usage:   "Address of the batch inbox contract",
		EnvVars: prefixEnvVars("BATCH_INBOX_ADDRESS"),
	}
	L1EthRpcFlag = &cli.StringFlag{
		Name:    "l1-eth-rpc",
		Usage:   "HTTP provider URL for L1 (for reading batch inbox)",
		EnvVars: prefixEnvVars("L1_ETH_RPC"),
	}
	L2EthRpcFlag = &cli.StringFlag{
		Name:    "l2-eth-rpc",
		Usage:   "HTTP provider URL for L2 (for mapping batches to L2 blocks)",
		EnvVars: prefixEnvVars("L2_ETH_RPC"),
	}
	OpNodeRpcFlag = &cli.StringFlag{
		Name:    "op-node-rpc",
		Usage:   "HTTP provider URL for op-node",
		EnvVars: prefixEnvVars("OP_NODE_RPC"),
	}

	// Optional flags
	PollIntervalFlag = &cli.DurationFlag{
		Name:    "poll-interval",
		Usage:   "Polling interval for checking new blocks",
		Value:   12 * time.Second,
		EnvVars: prefixEnvVars("POLL_INTERVAL"),
	}
	NetworkTimeoutFlag = &cli.DurationFlag{
		Name:    "network-timeout",
		Usage:   "Timeout for network requests",
		Value:   10 * time.Second,
		EnvVars: prefixEnvVars("NETWORK_TIMEOUT"),
	}
	VerifyParentCheckFlag = &cli.BoolFlag{
		Name:    "verify-parent-check",
		Usage:   "Verify parent check for span batches",
		Value:   true,
		EnvVars: prefixEnvVars("VERIFY_PARENT_CHECK"),
	}
	DbPathFlag = &cli.StringFlag{
		Name:    "db-path",
		Usage:   "Path to SQLite database",
		Value:   "",
		EnvVars: prefixEnvVars("DB_PATH"),
	}
	// Additional flags for precise batch parsing
	L2BlockTimeFlag = &cli.Uint64Flag{
		Name:    "l2-block-time",
		Usage:   "L2 block time in seconds",
		Value:   2,
		EnvVars: prefixEnvVars("L2_BLOCK_TIME"),
	}
	L2GenesisTimeFlag = &cli.Uint64Flag{
		Name:    "l2-genesis-time",
		Usage:   "L2 genesis timestamp",
		EnvVars: prefixEnvVars("L2_GENESIS_TIME"),
	}
	L2ChainIDFlag = &cli.Uint64Flag{
		Name:    "l2-chain-id",
		Usage:   "L2 chain ID",
		Value:   10,
		EnvVars: prefixEnvVars("L2_CHAIN_ID"),
	}
)

var requiredFlags = []cli.Flag{
	StartL1BlockFlag,
	BatchInboxAddressFlag,
	L1EthRpcFlag,
	L2EthRpcFlag,
	OpNodeRpcFlag,
}

var optionalFlags = []cli.Flag{
	PollIntervalFlag,
	NetworkTimeoutFlag,
	VerifyParentCheckFlag,
	DbPathFlag,
	L2BlockTimeFlag,
	L2GenesisTimeFlag,
	L2ChainIDFlag,
}

func init() {
	optionalFlags = append(optionalFlags, oprpc.CLIFlags(EnvVarPrefix)...)
	optionalFlags = append(optionalFlags, oplog.CLIFlags(EnvVarPrefix)...)
	optionalFlags = append(optionalFlags, opmetrics.CLIFlags(EnvVarPrefix)...)
	optionalFlags = append(optionalFlags, oppprof.CLIFlags(EnvVarPrefix)...)
	optionalFlags = append(optionalFlags, celestia.CLIFlags(EnvVarPrefix)...)

	Flags = append(requiredFlags, optionalFlags...)
}

// Flags contains the list of configuration options available to the binary.
var Flags []cli.Flag

func CheckRequired(ctx *cli.Context) error {
	for _, f := range requiredFlags {
		if !ctx.IsSet(f.Names()[0]) {
			return fmt.Errorf("flag %s is required", f.Names()[0])
		}
	}
	return nil
}
