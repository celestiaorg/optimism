package celestia

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"

	opservice "github.com/ethereum-optimism/optimism/op-service"
	cliiface "github.com/ethereum-optimism/optimism/op-service/cliiface"
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
	// TLSEnabledFlagName defines the flag for whether rpc TLS is enabled
	TLSEnabledFlagName = "da.tls-enabled"
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

	// tx client config flags
	DefaultKeyNameFlagName     = "da.tx-client.key-name"
	KeyringPathFlagName        = "da.tx-client.keyring-path"
	CoreGRPCAddrFlagName       = "da.tx-client.core-grpc.addr"
	CoreGRPCTLSEnabledFlagName = "da.tx-client.core-grpc.tls-enabled"
	CoreGRPCAuthTokenFlagName  = "da.tx-client.core-grpc.auth-token"
	P2PNetworkFlagName         = "da.tx-client.p2p-network"

	// NamespaceSize is the size of the hex encoded namespace string
	NamespaceSize = 58

	// defaultRPC is the default rpc dial address
	defaultRPC = "http://localhost:26658"

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
		&cli.BoolFlag{
			Name:    TLSEnabledFlagName,
			Usage:   "enable TLS for the data availability rpc client",
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_TLS_ENABLED"),
			Value:   true,
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
		&cli.StringFlag{
			Name:    DefaultKeyNameFlagName,
			Usage:   "celestia tx client key name",
			Value:   "my_celes_key",
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_TX_CLIENT_KEY_NAME"),
		},
		&cli.StringFlag{
			Name:    KeyringPathFlagName,
			Usage:   "celestia tx client keyring path e.g. ~/.celestia-light-mocha-4/keys",
			Value:   "",
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_TX_CLIENT_KEYRING_PATH"),
		},
		&cli.StringFlag{
			Name:    CoreGRPCAddrFlagName,
			Usage:   "celestia tx client core grpc addr",
			Value:   "http://localhost:9090",
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_TX_CLIENT_CORE_GRPC_ADDR"),
		},
		&cli.BoolFlag{
			Name:    CoreGRPCTLSEnabledFlagName,
			Usage:   "celestia tx client core grpc TLS",
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_TX_CLIENT_CORE_GRPC_TLS_ENABLED"),
			Value:   true,
		},
		&cli.StringFlag{
			Name:    CoreGRPCAuthTokenFlagName,
			Usage:   "celestia tx client core grpc auth token",
			Value:   "",
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_TX_CLIENT_CORE_GRPC_AUTH_TOKEN"),
		},
		&cli.StringFlag{
			Name:    P2PNetworkFlagName,
			Usage:   "celestia tx client p2p network",
			Value:   "mocha-4",
			EnvVars: opservice.PrefixEnvVar(envPrefix, "DA_TX_CLIENT_P2P_NETWORK"),
		},
	}
}

type CLIConfig struct {
	Rpc            string
	TLSEnabled     bool
	AuthToken      string
	Namespace      string
	FallbackMode   string
	GasPrice       float64
	TxClientConfig TxClientConfig
}

func (c CLIConfig) TxClientEnabled() bool {
	return (c.TxClientConfig.KeyringPath != "" || c.TxClientConfig.CoreGRPCAuthToken != "")
}

func (c CLIConfig) IsEnabled() bool {
	return c.Rpc != "" && c.Namespace != ""
}

func (c CLIConfig) CelestiaConfig() RPCClientConfig {
	ns, _ := hex.DecodeString(c.Namespace)
	var cfg *TxClientConfig
	if c.TxClientEnabled() {
		cfg = &c.TxClientConfig
	}
	return RPCClientConfig{
		URL:            c.Rpc,
		TLSEnabled:     c.TLSEnabled,
		AuthToken:      c.AuthToken,
		Namespace:      ns,
		FallbackMode:   c.FallbackMode,
		GasPrice:       c.GasPrice,
		TxClientConfig: cfg,
	}
}

func (c CLIConfig) Check() error {
	if !c.IsEnabled() {
		return nil
	}
	if c.TxClientEnabled() {
		// If tx client is enabled, ensure tx client flags are set
		if c.TxClientConfig.DefaultKeyName == "" {
			return errors.New("--da.tx-client.key-name must be set")
		}
		if c.TxClientConfig.KeyringPath == "" {
			return errors.New("--da.tx-client.keyring-path must be set")
		}
		if c.TxClientConfig.CoreGRPCAddr == "" {
			return errors.New("--da.tx-client.core-grpc.addr must be set")
		}
		if c.TxClientConfig.P2PNetwork == "" {
			return errors.New("--da.tx-client.p2p-network must be set")
		}
	}
	if _, err := url.Parse(c.Rpc); err != nil {
		return fmt.Errorf("rpc url is invalid: %w", err)
	}
	if _, err := hex.DecodeString(c.Namespace); err != nil {
		return fmt.Errorf("namespace is invalid hex: %w", err)
	}
	return nil
}

func NewCLIConfig() CLIConfig {
	return CLIConfig{
		Rpc: defaultRPC,
	}
}

func ReadCLIConfig(ctx cliiface.Context) CLIConfig {
	return CLIConfig{
		Rpc:          ctx.String(RPCFlagName),
		TLSEnabled:   ctx.Bool(TLSEnabledFlagName),
		AuthToken:    ctx.String(AuthTokenFlagName),
		Namespace:    ctx.String(NamespaceFlagName),
		FallbackMode: ctx.String(FallbackModeFlagName),
		GasPrice:     ctx.Float64(GasPriceFlagName),
		TxClientConfig: TxClientConfig{
			DefaultKeyName:     ctx.String(DefaultKeyNameFlagName),
			KeyringPath:        ctx.String(KeyringPathFlagName),
			CoreGRPCAddr:       ctx.String(CoreGRPCAddrFlagName),
			CoreGRPCTLSEnabled: ctx.Bool(CoreGRPCTLSEnabledFlagName),
			CoreGRPCAuthToken:  ctx.String(CoreGRPCAuthTokenFlagName),
			P2PNetwork:         ctx.String(P2PNetworkFlagName),
		},
	}
}

func ReadCLIConfigFromEnv(envPrefix string) CLIConfig {
	result := CLIConfig{
		Rpc:          defaultRPC,
		FallbackMode: FallbackModeCallData,
		GasPrice:     defaultGasPrice,
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

	if value := os.Getenv(envPrefix + "_" + "DA_TX_CLIENT_KEY_NAME"); value != "" {
		result.TxClientConfig.DefaultKeyName = value
	}
	if value := os.Getenv(envPrefix + "_" + "DA_TX_CLIENT_KEYRING_PATH"); value != "" {
		result.TxClientConfig.KeyringPath = value
	}
	if value := os.Getenv(envPrefix + "_" + "DA_TX_CLIENT_CORE_GRPC_ADDR"); value != "" {
		result.TxClientConfig.CoreGRPCAddr = value
	}
	if value := os.Getenv(envPrefix + "_" + "DA_TX_CLIENT_CORE_GRPC_TLS_ENABLED"); value != "" {
		result.TxClientConfig.CoreGRPCTLSEnabled = value == "true"
	}
	if value := os.Getenv(envPrefix + "_" + "DA_TX_CLIENT_CORE_GRPC_AUTH_TOKEN"); value != "" {
		result.TxClientConfig.CoreGRPCAuthToken = value
	}
	if value := os.Getenv(envPrefix + "_" + "DA_TX_CLIENT_P2P_NETWORK"); value != "" {
		result.TxClientConfig.P2PNetwork = value
	}

	return result
}
