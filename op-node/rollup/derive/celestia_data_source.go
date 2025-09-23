package derive

import (
	"context"
	"fmt"

	libshare "github.com/celestiaorg/go-square/v2/share"
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

	height, commitment := celestia.SplitID(s.comm)
	namespace, err := libshare.NewNamespaceFromBytes(daClient.Namespace)
	if err != nil {
		return nil, err
	}
	blob, err := daClient.Client.Get(ctx, height, namespace, commitment)
	if err != nil {
		// return temporary error so we can keep retrying.
		return nil, NewTemporaryError(fmt.Errorf("celestia: failed to resolve frame: %w", err))
	}
	if blob == nil {
		s.log.Warn("celestia: skipping empty blobs")
		s.comm = nil
		// skip the input
		return s.Next(ctx)
	}

	// reset the commitment so we can fetch the next one from the source at the next iteration.
	s.comm = nil
	return blob.Data(), nil
}
