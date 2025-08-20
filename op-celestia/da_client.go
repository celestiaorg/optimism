package celestia

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/celestiaorg/celestia-node/api/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
)

// heightLen is a length (in bytes) of serialized height.
//
// This is 8 as uint64 consist of 8 bytes.
const heightLen = 8

func MakeID(height uint64, commitment []byte) []byte {
	id := make([]byte, heightLen+len(commitment))
	binary.LittleEndian.PutUint64(id, height)
	copy(id[heightLen:], commitment)
	return id
}

func SplitID(id []byte) (uint64, []byte) {
	if len(id) <= heightLen {
		return 0, nil
	}
	commitment := id[heightLen:]
	return binary.LittleEndian.Uint64(id[:heightLen]), commitment
}

type DAClient struct {
	Client       *client.Client
	GetTimeout   time.Duration
	Namespace    []byte
	FallbackMode string
	GasPrice     float64
}

func NewDAClient(rpc, token, namespace, fallbackMode string, gasPrice float64) (*DAClient, error) {
	keyname := "my_celes_key"
	kr, err := client.KeyringWithNewKey(client.KeyringConfig{
		KeyName:     keyname,
		BackendName: keyring.BackendTest,
	}, "~/.celestia-light-mocha-4/keys")
	if err != nil {
		return nil, err
	}
	cfg := client.Config{
		ReadConfig: client.ReadConfig{
			BridgeDAAddr: "http://localhost:26658",
			DAAuthToken:  "token",
		},
		SubmitConfig: client.SubmitConfig{
			DefaultKeyName: keyname,
			Network:        "mocha-4",
			CoreGRPCConfig: client.CoreGRPCConfig{
				Addr:       "celestia-testnet-consensus.itrocket.net:9090",
				TLSEnabled: false,
				AuthToken:  "token",
			},
		},
	}
	client, err := client.New(context.Background(), cfg, kr)
	if err != nil {
		return nil, err
	}
	ns, err := hex.DecodeString(namespace)
	if err != nil {
		return nil, err
	}
	if fallbackMode != "disabled" && fallbackMode != "blobdata" && fallbackMode != "calldata" {
		return nil, fmt.Errorf("celestia: unknown fallback mode: %s", fallbackMode)
	}
	return &DAClient{
		Client:       client,
		GetTimeout:   time.Minute,
		Namespace:    ns,
		FallbackMode: fallbackMode,
		GasPrice:     gasPrice,
	}, nil
}
