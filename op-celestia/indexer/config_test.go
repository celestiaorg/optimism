package indexer

import (
	"testing"
	"time"

	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	opmetrics "github.com/ethereum-optimism/optimism/op-service/metrics"
	"github.com/ethereum-optimism/optimism/op-service/oppprof"
	oprpc "github.com/ethereum-optimism/optimism/op-service/rpc"
	"github.com/stretchr/testify/require"
)

func validCLIConfig() *CLIConfig {
	return &CLIConfig{
		StartL1Block:      1000,
		BatchInboxAddress: "0x6F54Ca6F6EdE96662024Ffd61BFd18f3f4e34DFf",
		L1EthRpc:          "http://localhost:8545",
		L2EthRpc:          "http://localhost:9545",
		OpNodeRpc:         "http://localhost:8547",
		PollInterval:      12 * time.Second,
		NetworkTimeout:    10 * time.Second,
		CelestiaConfig:    celestia.NewCLIConfig(),
		RPCConfig:         oprpc.DefaultCLIConfig(),
		LogConfig:         oplog.DefaultCLIConfig(),
		MetricsConfig:     opmetrics.DefaultCLIConfig(),
		PprofConfig:       oppprof.DefaultCLIConfig(),
	}
}

func TestCLIConfig_Valid(t *testing.T) {
	cfg := validCLIConfig()
	require.NoError(t, cfg.Check())
}

func TestCLIConfig_InvalidStartBlock(t *testing.T) {
	cfg := validCLIConfig()
	cfg.StartL1Block = 0
	require.Error(t, cfg.Check())
	require.Contains(t, cfg.Check().Error(), "start L1 block must be greater than 0")
}

func TestCLIConfig_InvalidBatchInboxAddress(t *testing.T) {
	cfg := validCLIConfig()
	cfg.BatchInboxAddress = ""
	require.Error(t, cfg.Check())
	require.Contains(t, cfg.Check().Error(), "batch inbox address is required")

	cfg.BatchInboxAddress = "invalid-address"
	require.Error(t, cfg.Check())
	require.Contains(t, cfg.Check().Error(), "invalid batch inbox address format")
}

func TestCLIConfig_InvalidL1EthRpc(t *testing.T) {
	cfg := validCLIConfig()
	cfg.L1EthRpc = ""
	require.Error(t, cfg.Check())
	require.Contains(t, cfg.Check().Error(), "L1 ETH RPC is required")
}

func TestCLIConfig_InvalidL2EthRpc(t *testing.T) {
	cfg := validCLIConfig()
	cfg.L2EthRpc = ""
	require.Error(t, cfg.Check())
	require.Contains(t, cfg.Check().Error(), "L2 ETH RPC is required")
}

func TestCLIConfig_InvalidPollInterval(t *testing.T) {
	cfg := validCLIConfig()
	cfg.PollInterval = 0
	require.Error(t, cfg.Check())
	require.Contains(t, cfg.Check().Error(), "poll interval must be greater than 0")
}

func TestCLIConfig_InvalidNetworkTimeout(t *testing.T) {
	cfg := validCLIConfig()
	cfg.NetworkTimeout = 0
	require.Error(t, cfg.Check())
	require.Contains(t, cfg.Check().Error(), "network timeout must be greater than 0")
}
