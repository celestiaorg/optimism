package celestia

import (
	"fmt"

	"github.com/urfave/cli/v2"

	opservice "github.com/ethereum-optimism/optimism/op-service"
)

const (
	// FallbackModeDisabled is the fallback mode disabled
	FallbackModeDisabled = "disabled"
	// FallbackModeBlobData is the fallback mode blob data
	FallbackModeBlobData = "blobdata"
	// FallbackModeCallData is the fallback mode call data
	FallbackModeCallData = "calldata"
)

const (
	// RPCFlagName defines the flag for the rpc url
	RPCFlagName = "da.rpc"
	// AuthTokenFlagName defines the flag for the auth token
	AuthTokenFlagName = "da.auth_token"
	// NamespaceFlagName defines the flag for the namespace
	NamespaceFlagName = "da.namespace"
	// EthFallbackDisabledFlagName defines the flag for disabling eth fallback
	EthFallbackDisabledFlagName = "da.eth_fallback_disabled"
	// FallbackModeFlagName defines the flag for fallback mode
	FallbackModeFlagName = "da.fallback_mode"
	// GasPriceFlagName defines the flag for gas price
	GasPriceFlagName = "da.gas_price"

	// NamespaceSize is the size of the hex encoded namespace string
	NamespaceSize = 58

	// defaultRPC is the default rpc dial address
	defaultRPC = "grpc://localhost:26650"

	// defaultGasPrice is the default gas price
	defaultGasPrice = -1
)

func CLIFlags(envPrefix string) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    RPCFlagName,
			Usage:   "dial address of the data availability rpc client; supports grpc, http, https",
			Value:   defaultRPC,
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_RPC"),
		},
		&cli.StringFlag{
			Name:    AuthTokenFlagName,
			Usage:   "authentication token of the data availability client",
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_AUTH_TOKEN"),
		},
		&cli.StringFlag{
			Name:    NamespaceFlagName,
			Usage:   "namespace of the data availability client",
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_NAMESPACE"),
		},
		&cli.BoolFlag{
			Name:    EthFallbackDisabledFlagName,
			Usage:   "disable eth fallback (deprecated, use FallbackModeFlag instead)",
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_ETH_FALLBACK_DISABLED"),
			Action: func(c *cli.Context, e bool) error {
				if e {
					return c.Set(FallbackModeFlagName, FallbackModeDisabled)
				}
				return nil
			},
		},
		&cli.StringFlag{
			Name:    FallbackModeFlagName,
			Usage:   fmt.Sprintf("fallback mode; must be one of: %s, %s or %s", FallbackModeDisabled, FallbackModeBlobData, FallbackModeCallData),
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_FALLBACK_MODE"),
			Value:   FallbackModeCallData,
			Action: func(c *cli.Context, s string) error {
				if s != FallbackModeDisabled && s != FallbackModeBlobData && s != FallbackModeCallData {
					return fmt.Errorf("invalid fallback mode: %s; must be one of: %s, %s or %s", s, FallbackModeDisabled, FallbackModeBlobData, FallbackModeCallData)
				}
				return nil
			},
		},
		&cli.Float64Flag{
			Name:    GasPriceFlagName,
			Usage:   "gas price of the data availability client",
			Value:   defaultGasPrice,
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_GAS_PRICE"),
		},
	}
}

type CLIConfig struct {
	Rpc          string
	AuthToken    string
	Namespace    string
	FallbackMode string
	GasPrice     float64
}

func (c CLIConfig) Check() error {
	return nil
}

func NewCLIConfig() CLIConfig {
	return CLIConfig{
		Rpc: defaultRPC,
	}
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	return CLIConfig{
		Rpc:          ctx.String(RPCFlagName),
		AuthToken:    ctx.String(AuthTokenFlagName),
		Namespace:    ctx.String(NamespaceFlagName),
		FallbackMode: ctx.String(FallbackModeFlagName),
		GasPrice:     ctx.Float64(GasPriceFlagName),
	}
}
