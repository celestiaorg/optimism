package rpc

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/log"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
)

// CelestiaLocation represents the location of L2 blocks on Celestia
type CelestiaLocation struct {
	Height     uint64  `json:"height"`
	Commitment string  `json:"commitment"`
	L2Range    L2Range `json:"l2_range"`
}

// L2Range represents a range of L2 block numbers
type L2Range struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

// IndexerAPI provides RPC methods for the Celestia indexer
type IndexerAPI struct {
	log    log.Logger
	driver IndexerDriver
}

// IndexerDriver interface for the indexer operations
type IndexerDriver interface {
	GetLocation(l2BlockNum uint64) (*CelestiaLocation, error)
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

// GetCelestiaLocationRequest represents the request parameters for getCelestiaLocation
type GetCelestiaLocationRequest struct {
	L2BlockNumber uint64 `json:"l2_block_number"`
}

// GetCelestiaLocationResponse represents the response for getCelestiaLocation
type GetCelestiaLocationResponse struct {
	Height     uint64  `json:"height"`
	Commitment string  `json:"commitment"`
	L2Range    L2Range `json:"l2_range"`
}

// GetCelestiaLocation returns the Celestia location for a given L2 block number
//
// This method implements the required API:
//
//	curl -X POST -H "Content-Type: application/json" -s \
//	  --data '{"jsonrpc":"2.0","method":"admin_getCelestiaLocation","params":[355],"id":1}' \
//	  http://localhost:57220
//
// Response format:
//
//	{
//	  "jsonrpc": "2.0",
//	  "id": 1,
//	  "result": {
//	    "height": 353,
//	    "commitment": "YQEAAAAAAADg6goIrTykl5jyHlGz6Bl2tYTDYzffUY39g3inPvMGDQ==",
//	    "l2_range": {
//	      "start": 354,
//	      "end": 359
//	    }
//	  }
//	}
func (api *IndexerAPI) GetCelestiaLocation(ctx context.Context, l2BlockNumber uint64) (*GetCelestiaLocationResponse, error) {
	api.log.Debug("GetCelestiaLocation called", "l2_block", l2BlockNumber)

	// Validate input
	if l2BlockNumber == 0 {
		return nil, fmt.Errorf("L2 block number must be greater than 0")
	}

	// Get location from driver
	location, err := api.driver.GetLocation(l2BlockNumber)
	if err != nil {
		api.log.Warn("Failed to get Celestia location", "l2_block", l2BlockNumber, "err", err)
		return nil, fmt.Errorf("failed to get Celestia location for L2 block %d: %w", l2BlockNumber, err)
	}

	// Build response
	response := &GetCelestiaLocationResponse{
		Height:     location.Height,
		Commitment: location.Commitment,
		L2Range:    location.L2Range,
	}

	api.log.Debug("GetCelestiaLocation successful",
		"l2_block", l2BlockNumber,
		"height", response.Height,
		"commitment", response.Commitment,
		"l2_start", response.L2Range.Start,
		"l2_end", response.L2Range.End)

	return response, nil
}

// GetIndexerStatus returns the current status of the indexer (useful for debugging)
type IndexerStatusResponse struct {
	LastIndexedBlock uint64 `json:"last_indexed_block"`
	IndexedBlocks    int    `json:"indexed_blocks"`
	Running          bool   `json:"running"`
}

// GetIndexerStatus returns the current indexer status
func (api *IndexerAPI) GetIndexerStatus(ctx context.Context) (*IndexerStatusResponse, error) {
	api.log.Debug("GetIndexerStatus called")

	// For now, return basic info - this would need to be extended
	// to get actual status from the driver
	response := &IndexerStatusResponse{
		LastIndexedBlock: 0,    // Would get from driver/storage
		IndexedBlocks:    0,    // Would get from driver/storage
		Running:          true, // Would get from driver
	}

	return response, nil
}
