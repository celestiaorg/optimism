package indexer

import (
	"errors"
	"math/big"
	"time"

	"github.com/urfave/cli/v2"

	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/ethereum-optimism/optimism/op-celestia/flags"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	opmetrics "github.com/ethereum-optimism/optimism/op-service/metrics"
	"github.com/ethereum-optimism/optimism/op-service/oppprof"
	oprpc "github.com/ethereum-optimism/optimism/op-service/rpc"
	"github.com/ethereum/go-ethereum/common"
)

// CLIConfig is a well typed config that is parsed from the CLI params.
// This also contains config options for auxiliary services.
// It is transformed into a `Config` before the indexer is started.
type CLIConfig struct {
	/* Required Params */

	// StartL1Block is the starting L1 block number for indexing
	StartL1Block uint64

	// BatchInboxAddress is the address of the batch inbox contract
	BatchInboxAddress string

	// L1EthRpc is the HTTP provider URL for L1 *execution* client
	// (required for calldata DA)
	L1EthRpc string

	// L2EthRpc is the HTTP provider URL for L2
	L2EthRpc string

	/* Optional Params */

	// L1BeaconRpc is the HTTP endpoint of the L1 *consensus* (AKA beacon) client
	// (required for EIP4844 blob DA)
	L1BeaconRpc string

	// OpNodeRpc is the HTTP provider URL for op-node (optional, for verification)
	OpNodeRpc string

	// PollInterval is the polling interval for checking new blocks
	PollInterval time.Duration

	// NetworkTimeout is the timeout for network requests
	NetworkTimeout time.Duration

	// VerifyParentCheck is a flag to enable parent check verification in span batches
	VerifyParentCheck bool

	// DbPath is the path to the SQLite database
	DbPath string

	// Celestia DA configuration
	CelestiaConfig celestia.CLIConfig

	// L2BlockTime is the L2 block time in seconds
	L2BlockTime uint64

	// L2GenesisTime is the L2 genesis timestamp
	L2GenesisTime uint64

	// L2ChainID is the L2 chain ID
	L2ChainID *big.Int

	RPCConfig oprpc.CLIConfig

	LogConfig oplog.CLIConfig

	MetricsConfig opmetrics.CLIConfig

	PprofConfig oppprof.CLIConfig
}

func (c *CLIConfig) Check() error {
	if err := c.RPCConfig.Check(); err != nil {
		return err
	}
	if err := c.MetricsConfig.Check(); err != nil {
		return err
	}
	if err := c.PprofConfig.Check(); err != nil {
		return err
	}
	if err := c.CelestiaConfig.Check(); err != nil {
		return err
	}

	if c.StartL1Block == 0 {
		return errors.New("start L1 block must be greater than 0")
	}
	if c.BatchInboxAddress == "" {
		return errors.New("batch inbox address is required")
	}
	if c.L1EthRpc == "" {
		return errors.New("L1 ETH RPC is required")
	}
	if c.L2EthRpc == "" {
		return errors.New("L2 ETH RPC is required")
	}
	if c.PollInterval == 0 {
		return errors.New("poll interval must be greater than 0")
	}
	if c.NetworkTimeout == 0 {
		return errors.New("network timeout must be greater than 0")
	}

	// Validate batch inbox address format
	if !common.IsHexAddress(c.BatchInboxAddress) {
		return errors.New("invalid batch inbox address format")
	}

	return nil
}

// IndexerConfig contains the configuration for the indexer service
type IndexerConfig struct {
	StartL1Block      uint64
	BatchInboxAddress common.Address
	L1EthRpc          string
	L2EthRpc          string
	OpNodeRpc         string
	PollInterval      time.Duration
	NetworkTimeout    time.Duration
	VerifyParentCheck bool
	SqlitePath        string
	CelestiaConfig    celestia.CLIConfig

	// Additional fields needed for precise batch parsing
	L2BlockTime   uint64   // L2 block time in seconds
	L2GenesisTime uint64   // L2 genesis timestamp
	ChainID       *big.Int // L2 chain ID
}

// NewConfig parses the Config from the provided flags or environment variables.
func NewConfig(ctx *cli.Context) *CLIConfig {
	return &CLIConfig{
		StartL1Block:      ctx.Uint64(flags.StartL1BlockFlag.Name),
		BatchInboxAddress: ctx.String(flags.BatchInboxAddressFlag.Name),
		L1EthRpc:          ctx.String(flags.L1EthRpcFlag.Name),
		L1BeaconRpc:       ctx.String(flags.L1BeaconRpcFlag.Name),
		L2EthRpc:          ctx.String(flags.L2EthRpcFlag.Name),
		OpNodeRpc:         ctx.String(flags.OpNodeRpcFlag.Name),
		PollInterval:      ctx.Duration(flags.PollIntervalFlag.Name),
		NetworkTimeout:    ctx.Duration(flags.NetworkTimeoutFlag.Name),
		VerifyParentCheck: ctx.Bool(flags.VerifyParentCheckFlag.Name),
		DbPath:            ctx.String(flags.DbPathFlag.Name),
		L2BlockTime:       ctx.Uint64(flags.L2BlockTimeFlag.Name),
		L2GenesisTime:     ctx.Uint64(flags.L2GenesisTimeFlag.Name),
		L2ChainID:         big.NewInt(int64(ctx.Uint64(flags.L2ChainIDFlag.Name))),
		CelestiaConfig:    celestia.ReadCLIConfig(ctx),
		RPCConfig:         oprpc.ReadCLIConfig(ctx),
		LogConfig:         oplog.ReadCLIConfig(ctx),
		MetricsConfig:     opmetrics.ReadCLIConfig(ctx),
		PprofConfig:       oppprof.ReadCLIConfig(ctx),
	}
}
