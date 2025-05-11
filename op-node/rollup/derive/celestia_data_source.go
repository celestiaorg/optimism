package derive

import (
	"context"
	"fmt"

	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/log"
)

var daClient *celestia.DAClient

func CelestiaDAEnabled() bool {
	return daClient != nil
}

func SetCelestiaDA(c *celestia.DAClient) error {
	daClient = c
	return nil
}

type CelestiaDataSource struct {
	log log.Logger
	src DataIter
	// keep track of a pending commitment so we can keep trying to fetch the input.
	comm eth.Data
}

func NewCelestiaDataSource(log log.Logger, src DataIter) *CelestiaDataSource {
	return &CelestiaDataSource{
		log: log,
		src: src,
	}
}

func (s *CelestiaDataSource) Next(ctx context.Context) (eth.Data, error) {
	if s.comm == nil {
		// The L1 source provides the input commitment corresponding to the batch.
		data, err := s.src.Next(ctx)
		if err != nil {
			return nil, err
		}

		if len(data) == 0 {
			return nil, NotEnoughData
		}
		// If the transaction data type isn't Celestia,
		// pass it downstream for further validation
		// and potential parsing as L1 DA inputs.
		if data[0] != celestia.DerivationVersionCelestia {
			return data, nil
		}

		s.comm = data[1:]
	}

	cCtx, cancel := context.WithTimeout(ctx, daClient.GetTimeout)
	blobs, err := daClient.Client.Get(cCtx, [][]byte{s.comm}, daClient.Namespace)
	cancel()

	if err != nil {
		return nil, NewResetError(fmt.Errorf("celestia: failed to resolve frame: %w", err))
	}

	if len(blobs) != 1 {
		log.Warn("celestia: unexpected length for blobs", "expected", 1, "got", len(blobs))
		if len(blobs) == 0 {
			log.Warn("celestia: skipping empty blobs")
			s.comm = nil
			// skip the input
			return s.Next(ctx)
		}
	}

	// reset the commitment so we can fetch the next one from the source at the next iteration.
	s.comm = nil
	return blobs[0], nil
}
