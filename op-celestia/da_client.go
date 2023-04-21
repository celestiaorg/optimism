package celestia

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/rollkit/go-da"
	"github.com/rollkit/go-da/proxy"
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
	Client       da.DA
	GetTimeout   time.Duration
	Namespace    da.Namespace
	FallbackMode string
	GasPrice     float64
}

func NewDAClient(rpc, token, namespace, fallbackMode string, gasPrice float64) (*DAClient, error) {
	client, err := proxy.NewClient(rpc, token)
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
