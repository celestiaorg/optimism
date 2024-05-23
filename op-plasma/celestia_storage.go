package plasma

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	client "github.com/celestiaorg/celestia-openrpc"
	"github.com/ethereum/go-ethereum/log"
)

const VersionByte = 0x0c

type CelestiaConfig struct {
	URL       string
	AuthToken string
	Namespace []byte
}

// CelestiaStore implements DAStorage with celestia backend
type CelestiaStore struct {
	Log        log.Logger
	GetTimeout time.Duration
	Namespace []byte
	Client *client.Client
}

// NewCelestiaStore returns a celestia store.
func NewCelestiaStore(cfg CelestiaConfig) *CelestiaStore {
	Log := log.New()
	ctx := context.Background()
	client, err := client.NewClient(ctx, cfg.URL, cfg.AuthToken)
	if err != nil {
		Log.Crit(err.Error())
	}
	return &CelestiaStore{
		Log:        Log,
		Client:         client,
		GetTimeout: time.Minute,
		Namespace: cfg.Namespace,
	}
}

func (d *CelestiaStore) Get(ctx context.Context, key []byte) ([]byte, error) {
	log.Info("celestia: blob request", "id", hex.EncodeToString(key))
	ctx, cancel := context.WithTimeout(context.Background(), d.GetTimeout)
	blobs, err := d.Client.DA.Get(ctx, [][]byte{key[2:]}, d.Namespace)
	cancel()
	if err != nil || len(blobs) == 0 {
		return nil, fmt.Errorf("celestia: failed to resolve frame: %w", err)
	}
	if len(blobs) != 1 {
		d.Log.Warn("celestia: unexpected length for blobs", "expected", 1, "got", len(blobs))
	}
	return blobs[0], nil
}

func (d *CelestiaStore) Put(ctx context.Context, data []byte) ([]byte, error) {
	ids, err := d.Client.DA.Submit(ctx, [][]byte{data}, -1, d.Namespace)
	if err == nil && len(ids) == 1 {
		d.Log.Info("celestia: blob successfully submitted", "id", hex.EncodeToString(ids[0]))
		commitment := NewGenericCommitment(append([]byte{VersionByte}, ids[0]...))
		return commitment.Encode(), nil
	}
	return nil, err
}
