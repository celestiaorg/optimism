package rpc

import (
	"context"
	"fmt"

	"github.com/ethereum-optimism/optimism/op-celestia/indexer/store"
	"github.com/ethereum/go-ethereum/log"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
)

// IndexerAPI provides RPC methods for the Celestia indexer
type IndexerAPI struct {
	log    log.Logger
	driver IndexerDriver
}

// IndexerDriver interface for the indexer operations
type IndexerDriver interface {
	GetDALocation(l2BlockNum uint64) (store.DALocation, error)
	GetStatus() (lastIndexedBlock uint64, indexedBlocks int, running bool, l2Start uint64, l2End uint64, err error)
}

// NewIndexerAPI creates a new IndexerAPI instance
func NewIndexerAPI(driver IndexerDriver, log log.Logger) *IndexerAPI {
	return &IndexerAPI{
		log:    log,
		driver: driver,
	}
}

// GetAPI returns the RPC API descriptor
func GetAPI(api *IndexerAPI) gethrpc.API {
	return gethrpc.API{
		Namespace: "admin",
		Service:   api,
	}
}

// GetDALocationResponse represents the response for getDALocation
type GetDALocationResponse struct {
	Type string `json:"type"` // "celestia" or "ethereum plain calldata" or "ethereum EIP4844 blobs"
	Data any    `json:"data"` // CelestiaLocation or EthereumLocation
}

// GetIndexerStatus returns the current status of the indexer (useful for debugging)
type IndexerStatusResponse struct {
	LastIndexedBlock uint64 `json:"last_indexed_block"`
	IndexedBlocks    int    `json:"indexed_blocks"`
	Running          bool   `json:"running"`
	L2StartBlock     uint64 `json:"l2_start_block"`
	L2EndBlock       uint64 `json:"l2_end_block"`
}

// GetIndexerStatus returns the current indexer status
func (api *IndexerAPI) GetIndexerStatus(ctx context.Context) (*IndexerStatusResponse, error) {
	api.log.Debug("GetIndexerStatus called")

	lastIndexedBlock, indexedBlocks, running, l2Start, l2End, err := api.driver.GetStatus()
	if err != nil {
		api.log.Warn("Failed to get indexer status", "err", err)
		return nil, fmt.Errorf("failed to get indexer status: %w", err)
	}

	response := &IndexerStatusResponse{
		LastIndexedBlock: lastIndexedBlock,
		IndexedBlocks:    indexedBlocks,
		Running:          running,
		L2StartBlock:     l2Start,
		L2EndBlock:       l2End,
	}

	api.log.Debug("GetIndexerStatus successful",
		"last_indexed_block", lastIndexedBlock,
		"indexed_blocks", indexedBlocks,
		"running", running,
		"l2_start_block", l2Start,
		"l2_end_block", l2End)

	return response, nil
}

// GetDALocation returns the DA location (either Celestia or Ethereum plain calldata, or Ethereum EIP4844 blobs) for a given L2 block number
//
// This method provides a generic endpoint that works with both DA types:
//
//	curl -X POST -H "Content-Type: application/json" -s \
//	  --data '{"jsonrpc":"2.0","method":"admin_getDALocation","params":[355],"id":1}' \
//	  http://localhost:57220
//
// Response format for Celestia:
//
//	{
//	  "jsonrpc": "2.0",
//	  "id": 1,
//	  "result": {
//	    "type": "celestia",
//	    "data": {
//	      "height": 353,
//	      "commitment": "YQEAAAAAAADg6goIrTykl5jyHlGz6Bl2tYTDYzffUY39g3inPvMGDQ==",
//	      "l2_range": {
//	        "start": 354,
//	        "end": 359
//	      },
//	      "l1_block": 12345
//	    }
//	  }
//	}
//
// Response format for Ethereum:
//
//	{
//	  "jsonrpc": "2.0",
//	  "id": 1,
//	  "result": {
//	    "type": "ethereum plain calldata",
//	    "data": {
//	      "tx_hash": "0x123...",
//	      "l2_range": {
//	        "start": 354,
//	        "end": 359
//	      },
//	      "l1_block": 12345
//	    }
//	  }
//	}
func (api *IndexerAPI) GetDALocation(ctx context.Context, l2BlockNumber uint64) (*GetDALocationResponse, error) {
	api.log.Debug("GetDALocation called", "l2_block", l2BlockNumber)

	// Validate input
	if l2BlockNumber == 0 {
		return nil, fmt.Errorf("L2 block number must be greater than 0")
	}

	// Get location from driver
	location, err := api.driver.GetDALocation(l2BlockNumber)
	if err != nil {
		api.log.Warn("Failed to get DA location", "l2_block", l2BlockNumber, "err", err)
		return nil, fmt.Errorf("failed to get DA location for L2 block %d: %w", l2BlockNumber, err)
	}

	// Build response based on type
	response := &GetDALocationResponse{
		Type: location.GetType(),
		Data: location,
	}

	api.log.Debug("GetDALocation successful",
		"l2_block", l2BlockNumber,
		"type", response.Type)

	return response, nil
}
