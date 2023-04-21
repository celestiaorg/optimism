package rpc

import (
	"context"
	"fmt"

	"github.com/ethereum-optimism/optimism/op-celestia/indexer/store"
	"github.com/ethereum/go-ethereum/log"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
)

// IndexerAPI provides RPC methods for the Celestia/Ethereum DA indexer.
type IndexerAPI struct {
	log    log.Logger
	driver IndexerDriver
}

type IndexerDriver interface {
	GetDALocation(l2BlockNum uint64) (store.DALocation, error)
	GetStatus() (lastIndexedBlock uint64, indexedBlocks int, running bool, l2Start uint64, l2End uint64, err error)
}

func NewIndexerAPI(driver IndexerDriver, log log.Logger) *IndexerAPI {
	return &IndexerAPI{
		log:    log,
		driver: driver,
	}
}

// GetAPI returns the RPC API descriptor.
func GetAPI(api *IndexerAPI) gethrpc.API {
	return gethrpc.API{
		Namespace: "admin",
		Service:   api,
	}
}

// GetDALocationResponse represents the response for getDALocation.
//
// Type is a coarse DA backend identifier:
//   - "celestia"  → data is a CelestiaLocation
//   - "ethereum"  → data is an EthereumLocation
//
// For Ethereum:
//
//   - If data.blob_hashes is nil, the batch is calldata-backed.
//   - If data.blob_hashes is non-nil (including an empty slice), the batch is blob-backed,
//     and callers MUST ignore calldata per the [derivation spec](https://specs.optimism.io/protocol/ecotone/derivation.html#ecotone-blob-retrieval).

type GetDALocationResponse struct {
	Type string `json:"type"` // "celestia" or "ethereum"
	Data any    `json:"data"` // CelestiaLocation or EthereumLocation
}

type IndexerStatusResponse struct {
	LastIndexedBlock uint64 `json:"last_indexed_block"`
	IndexedBlocks    int    `json:"indexed_blocks"`
	Running          bool   `json:"running"`
	L2StartBlock     uint64 `json:"l2_start_block"`
	L2EndBlock       uint64 `json:"l2_end_block"`
}

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

// GetDALocation returns the DA location for a given L2 block number.
//
// This method provides a generic endpoint that works with both Celestia and Ethereum DA.
//
//	curl -X POST -H "Content-Type: application/json" -s \
//	  --data '{"jsonrpc":"2.0","method":"admin_getDALocation","params":[355],"id":1}' \
//	  http://localhost:57220
//
// Celestia response:
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
// Ethereum response:
//
//	{
//	  "jsonrpc": "2.0",
//	  "id": 1,
//	  "result": {
//	    "type": "ethereum",
//	    "data": {
//	      "tx_hash": "0x123...",
//	      "l2_range": {
//	        "start": 354,
//	        "end": 359
//	      },
//	      "l1_block": 12345,
//	      "blob_hashes": [
//	        {
//	          "index": 0,
//	          "hash": "0xabcde..."
//	        },
//	        {
//	          "index": 1,
//	          "hash": "0xf00ba4..."
//	        }
//	      ]
//	    }
//	  }
//	}
//
// Notes:
//   - If "blob_hashes" is present (even empty array) -> blob-backed batch.
//   - If "blob_hashes" is null or omitted -> calldata-backed batch.
func (api *IndexerAPI) GetDALocation(ctx context.Context, l2BlockNumber uint64) (*GetDALocationResponse, error) {
	api.log.Debug("GetDALocation called", "l2_block", l2BlockNumber)

	// Validate input.
	if l2BlockNumber == 0 {
		return nil, fmt.Errorf("L2 block number must be greater than 0")
	}

	// Get location from driver.
	location, err := api.driver.GetDALocation(l2BlockNumber)
	if err != nil {
		api.log.Warn("Failed to get DA location", "l2_block", l2BlockNumber, "err", err)
		return nil, fmt.Errorf("failed to get DA location for L2 block %d: %w", l2BlockNumber, err)
	}

	// Build response based on type. We rely on the DALocation implementation to return
	// a coarse type string, e.g. "celestia" or "ethereum". Blob vs calldata for
	// Ethereum is determined by inspecting the concrete EthereumLocation (blob_hashes).
	response := &GetDALocationResponse{
		Type: location.GetType(),
		Data: location,
	}

	api.log.Debug("GetDALocation successful",
		"l2_block", l2BlockNumber,
		"type", response.Type)

	return response, nil
}
