package plasma

import (
	"fmt"
	"net/url"

	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/urfave/cli/v2"
)

const (
	EnabledFlagName         = "plasma.enabled"
	DaServerAddressFlagName = "plasma.da-server"
	DaBackendFlagName       = "plasma.da-backend"
	VerifyOnReadFlagName    = "plasma.verify-on-read"
)

func plasmaEnv(envprefix, v string) []string {
	return []string{envprefix + "_PLASMA_" + v}
}

func CLIFlags(envPrefix string, category string) []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:     EnabledFlagName,
			Usage:    "Enable plasma mode",
			Value:    false,
			EnvVars:  plasmaEnv(envPrefix, "ENABLED"),
			Category: category,
		},
		&cli.StringFlag{
			Name:     DaServerAddressFlagName,
			Usage:    "HTTP address of a DA Server",
			EnvVars:  plasmaEnv(envPrefix, "DA_SERVER"),
			Category: category,
		},
		&cli.StringFlag{
			Name:    DaBackendFlagName,
			Usage:   "Plamsa mode backend ('celestia')",
			EnvVars: plasmaEnv(envPrefix, "DA_BACKEND"),
		},
		&cli.BoolFlag{
			Name:     VerifyOnReadFlagName,
			Usage:    "Verify input data matches the commitments from the DA storage service",
			Value:    true,
			EnvVars:  plasmaEnv(envPrefix, "VERIFY_ON_READ"),
			Category: category,
		},
	}
}

type CLIConfig struct {
	Enabled      bool
	DAServerURL  string
	DABackend    string
	VerifyOnRead bool
}

func (c CLIConfig) Check() error {
	if !c.Enabled {
		return fmt.Errorf("plasma must be enabled")
	}
	if c.DAServerURL == "" {
		return fmt.Errorf("DA server URL is required when plasma da is enabled")
	}
	if _, err := url.Parse(c.DAServerURL); err != nil {
		return fmt.Errorf("DA server URL is invalid: %w", err)
	}
	if c.DABackend != "celestia" {
		return fmt.Errorf("plasma backend must be set to celestia")
	}
	return nil
}

func (c CLIConfig) NewDAClient() DAStorage {
	var client DAStorage
	switch c.DABackend {
	case "celestia":
		client = celestia.NewDAClient(c.DAServerURL, c.VerifyOnRead)
	default:
		client = &DAClient{url: c.DAServerURL, verify: c.VerifyOnRead}
	}
	return client
}

func ReadCLIConfig(c *cli.Context) CLIConfig {
	return CLIConfig{
		Enabled:      c.Bool(EnabledFlagName),
		DAServerURL:  c.String(DaServerAddressFlagName),
		DABackend:    c.String(DaBackendFlagName),
		VerifyOnRead: c.Bool(VerifyOnReadFlagName),
	}
}
