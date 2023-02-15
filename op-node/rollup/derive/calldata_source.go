package derive

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-node/da"
	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/sources"
)

type DataIter interface {
	Next(ctx context.Context) (eth.Data, error)
}

// DataSourceFactory readers raw transactions from a given block & then filters for
// batch submitter transactions.
// This is not a stage in the pipeline, but a wrapper for another stage in the pipeline
type DataSourceFactory struct {
	log     log.Logger
	cfg     *rollup.Config
	fetcher da.L1TransactionFetcher
}

func NewDataSourceFactory(log log.Logger, cfg *rollup.Config, fetcher da.L1TransactionFetcher) *DataSourceFactory {
	return &DataSourceFactory{log: log, cfg: cfg, fetcher: fetcher}
}

// OpenData returns a CalldataSourceImpl. This struct implements the `Next` function.
func (ds *DataSourceFactory) OpenData(ctx context.Context, id eth.BlockID, batcherAddr common.Address) DataIter {
	return NewDataSource(ctx, ds.log, ds.cfg, ds.fetcher, id, batcherAddr)
}

// DataSource is a fault tolerant approach to fetching data.
// The constructor will never fail & it will instead re-attempt the fetcher
// at a later point.
type DataSource struct {
	// Internal state + data
	open bool
	data []da.Frame
	// Required to re-attempt fetching
	id          eth.BlockID
	cfg         *rollup.Config // TODO: `DataFromEVMTransactions` should probably not take the full config
	fetcher     da.L1TransactionFetcher
	log         log.Logger
	da          da.DAChain
	batcherAddr common.Address
}

// NewDataSource creates a new calldata source. It suppresses errors in fetching the L1 block if they occur.
// If there is an error, it will attempt to fetch the result on the next call to `Next`.
func NewDataSource(ctx context.Context, log log.Logger, cfg *rollup.Config, fetcher da.L1TransactionFetcher, block eth.BlockID, batcherAddr common.Address) DataIter {
	// new DAChain - either CelestiaDA or EthereumDA
	// daChain = NewEthereumDA()...
	daChain, err := sources.NewEthereumDA(log, cfg, nil)
	if err != nil {
		return &DataSource{
			open:        false,
			id:          block,
			cfg:         cfg,
			fetcher:     fetcher,
			log:         log,
			da:          daChain,
			batcherAddr: batcherAddr,
		}

	}
	return &DataSource{
		open: true,
		data: da.DataFromDASource(ctx, block, daChain, log.New("origin", block)),
	}
}

// Next returns the next piece of data if it has it. If the constructor failed, this
// will attempt to reinitialize itself. If it cannot find the block it returns a ResetError
// otherwise it returns a temporary error if fetching the block returns an error.
func (ds *DataSource) Next(ctx context.Context) (eth.Data, error) {
	if !ds.open {
		if _, err := ds.da.TxsByNumber(ctx, ds.id.Number); err == nil {
			ds.open = true
			ds.data = da.DataFromDASource(ctx, ds.id, ds.da, log.New("origin", ds.id))
		} else if errors.Is(err, ethereum.NotFound) {
			return nil, NewResetError(fmt.Errorf("failed to open calldata source: %w", err))
		} else {
			return nil, NewTemporaryError(fmt.Errorf("failed to open calldata source: %w", err))
		}
	}
	if len(ds.data) == 0 {
		return nil, io.EOF
	} else {
		data := ds.data[0]
		ds.data = ds.data[1:]
		return data.Data, nil
	}
}
