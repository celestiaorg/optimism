package flags

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestFlags_AllFlagsHaveEnvVars(t *testing.T) {
	for _, flag := range Flags {
		envFlag, ok := flag.(interface {
			GetEnvVars() []string
		})
		require.True(t, ok, "flag %s should support environment variables", flag.Names()[0])
		envVars := envFlag.GetEnvVars()
		require.NotEmpty(t, envVars, "flag %s should have at least one environment variable", flag.Names()[0])
	}
}

func TestFlags_RequiredFlagsCheck(t *testing.T) {
	app := &cli.App{
		Flags: Flags,
		Action: func(ctx *cli.Context) error {
			return CheckRequired(ctx)
		},
	}

	// Test missing required flags
	err := app.Run([]string{"test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "is required")

	// Test with all required flags
	err = app.Run([]string{
		"test",
		"--start-l1-block", "1000",
		"--batch-inbox-address", "0x6F54Ca6F6EdE96662024Ffd61BFd18f3f4e34DFf",
		"--l1-eth-rpc", "http://localhost:11111",
		"--l2-eth-rpc", "http://localhost:22221",
		"--op-node-rpc", "http://localhost:33331",
	})
	require.NoError(t, err)
}

func TestFlags_NoConflicts(t *testing.T) {
	seen := make(map[string]bool)
	for _, flag := range Flags {
		for _, name := range flag.Names() {
			require.False(t, seen[name], "duplicate flag name: %s", name)
			seen[name] = true
		}
	}
}
