package celestia

import (
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/ethereum/go-ethereum/log"
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
	// EnabledFlagName defines the flag for enabling Celestia
	EnabledFlagName = "da.enabled"
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
		&cli.BoolFlag{
			Name:    EnabledFlagName,
			Usage:   "enable Celestia",
			Value:   false,
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_ENABLED"),
		},
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
	Enabled      bool
	Rpc          string
	AuthToken    string
	Namespace    string
	FallbackMode string
	GasPrice     float64
}

func (c CLIConfig) Check() error {
	if c.Enabled {
		if c.Rpc == "" {
			return fmt.Errorf("rpc url is required when Celestia is enabled")
		}
		if _, err := url.Parse(c.Rpc); err != nil {
			return fmt.Errorf("rpc url is invalid: %w", err)
		}
		if c.AuthToken == "" {
			return fmt.Errorf("auth token is required when Celestia is enabled")
		}
		if c.Namespace == "" {
			return fmt.Errorf("namespace is required when Celestia is enabled")
		}
	}
	return nil
}

func NewCLIConfig() CLIConfig {
	return CLIConfig{
		Rpc: defaultRPC,
	}
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	return CLIConfig{
		Enabled:      ctx.Bool(EnabledFlagName),
		Rpc:          ctx.String(RPCFlagName),
		AuthToken:    ctx.String(AuthTokenFlagName),
		Namespace:    ctx.String(NamespaceFlagName),
		FallbackMode: ctx.String(FallbackModeFlagName),
		GasPrice:     ctx.Float64(GasPriceFlagName),
	}
}

func ReadCLIConfigFromEnv(envPrefix string) CLIConfig {
	result := CLIConfig{
		Rpc:          defaultRPC,
		FallbackMode: FallbackModeCallData,
		GasPrice:     defaultGasPrice,
	}

	if value := os.Getenv(envPrefix + "_" + "DA_ENABLED"); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			result.Enabled = parsed
		} else {
			log.Crit("invalid enabled flag", "value", value)
		}
	}

	if value := os.Getenv(envPrefix + "_" + "DA_RPC"); value != "" {
		result.Rpc = value
	}

	if value := os.Getenv(envPrefix + "_" + "DA_AUTH_TOKEN"); value != "" {
		result.AuthToken = value
	}

	if value := os.Getenv(envPrefix + "_" + "DA_NAMESPACE"); value != "" {
		result.Namespace = value
	}

	if value := os.Getenv(envPrefix + "_" + "DA_FALLBACK_MODE"); value != "" {
		switch value {
		case FallbackModeDisabled, FallbackModeBlobData, FallbackModeCallData:
			result.FallbackMode = value
		default:
			log.Crit("invalid fallback mode", "value", value)
		}
	}

	if value := os.Getenv(envPrefix + "_" + "DA_GAS_PRICE"); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			result.GasPrice = parsed
		} else {
			log.Crit("invalid gas price", "value", value)
		}
	}

	return result
}
