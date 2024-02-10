package celestia

import (
	"fmt"
	"net"

	"github.com/urfave/cli/v2"

	opservice "github.com/ethereum-optimism/optimism/op-service"
)

const (
	DaRpcFlagName = "da.rpc"
)

var (
	defaultDaRpc = "localhost:26650"
)

func Check(address string) bool {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}

	if port == "" {
		return false
	}

	_, err = net.LookupPort("tcp", port)
	if err != nil {
		return false
	}

	return true
}

func CLIFlags(envPrefix string) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    DaRpcFlagName,
			Usage:   "dial address of data availability grpc client",
			Value:   defaultDaRpc,
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_RPC"),
		},
	}
}

type Config struct {
	DaRpc string
}

func (c Config) Check() error {
	if c.DaRpc == "" {
		c.DaRpc = defaultDaRpc
	}

	if !Check(c.DaRpc) {
		return fmt.Errorf("invalid da rpc")
	}

	return nil
}

type CLIConfig struct {
	DaRpc string
}

func (c CLIConfig) Check() error {
	if c.DaRpc == "" {
		c.DaRpc = defaultDaRpc
	}

	if !Check(c.DaRpc) {
		return fmt.Errorf("invalid da rpc")
	}

	return nil
}

func NewCLIConfig() CLIConfig {
	return CLIConfig{
		DaRpc: defaultDaRpc,
	}
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	return CLIConfig{
		DaRpc: ctx.String(DaRpcFlagName),
	}
}
