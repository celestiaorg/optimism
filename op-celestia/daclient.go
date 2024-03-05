package celestia

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
	goDA "github.com/rollkit/go-da"
	"github.com/rollkit/go-da/proxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// DAClient implements DAStorage with celestia backend
type DAClient struct {
	Log        log.Logger
	DA         goDA.DA
	GetTimeout time.Duration
	Verify     bool
}

// NewDAClient returns a celestia DA client.
func NewDAClient(rpc string, verify bool) *DAClient {
	client := proxy.NewClient()
	client.Start(rpc, grpc.WithTransportCredentials(insecure.NewCredentials()))
	return &DAClient{
		Log:        log.New(),
		DA:         client,
		GetTimeout: time.Minute,
		Verify:     verify,
	}
}

func (c *DAClient) GetInput(ctx context.Context, key []byte) ([]byte, error) {
	log.Info("celestia: blob request", "id", hex.EncodeToString(key))
	ctx, cancel := context.WithTimeout(context.Background(), c.GetTimeout)
	blobs, err := c.DA.Get(ctx, [][]byte{key[1:]})
	cancel()
	if err != nil || len(blobs) == 0 {
		return nil, fmt.Errorf("celestia: failed to resolve frame: %w", err)
	}
	if len(blobs) != 1 {
		c.Log.Warn("celestia: unexpected length for blobs", "expected", 1, "got", len(blobs))
	}
	// TODO: verify
	return blobs[0], nil
}

func (c *DAClient) SetInput(ctx context.Context, data []byte) ([]byte, error) {
	ids, _, err := c.DA.Submit(ctx, [][]byte{data}, -1)
	if err == nil && len(ids) == 1 {
		c.Log.Info("celestia: blob successfully submitted", "id", hex.EncodeToString(ids[0]))
		key := append([]byte{DerivationVersionCelestia}, ids[0]...)
		return key, nil
	}
	return nil, err
}
