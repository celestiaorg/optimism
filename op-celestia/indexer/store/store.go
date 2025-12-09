package store

import "github.com/ethereum-optimism/optimism/op-service/eth"

// Store defines the interface for L2 block -> DA location storage
// Can be implemented by in-memory storage or database-backed storage
type Store interface {
	// SetLastIndexedBlock sets the last indexed L2 block number
	SetLastIndexedBlock(blockNum uint64) error

	// GetLastIndexedBlock returns the last indexed L2 block number
	GetLastIndexedBlock() (uint64, error)

	// StoreLocation stores the Celestia location for a range of L2 blocks
	StoreLocation(location *CelestiaLocation) error

	// StoreEthLocation stores the Ethereum DA location for a range of L2 blocks
	StoreEthLocation(location *EthereumLocation) error

	// GetDALocation returns the DA location (either Celestia or Ethereum) for a given L2 block number
	GetDALocation(l2BlockNum uint64) (DALocation, error)

	// GetIndexedBlockCount returns the number of indexed L2 blocks
	GetIndexedBlockCount() (int, error)

	// GetL2BlockRange returns the minimum and maximum L2 block numbers indexed
	GetL2BlockRange() (min uint64, max uint64, err error)

	// Clear removes all stored data
	Clear() error

	// String returns a string representation of the storage state
	String() string
}

// DALocation is a generic interface for all DA locations
type DALocation interface {
	GetType() string
	GetL2Range() L2Range
	GetL1Block() uint64
}

// CelestiaLocation represents the location of L2 blocks on Celestia
type CelestiaLocation struct {
	Height     uint64  `json:"height"`
	Commitment string  `json:"commitment"`
	L2Range    L2Range `json:"l2_range"`
	L1Block    uint64  `json:"l1_block"`
}

// EthereumLocation represents the location of L2 blocks on Ethereum DA (blobs if BobHashes != nil or = [], calldata otherwise)
type EthereumLocation struct {
	TxHash     string                `json:"tx_hash"`
	L2Range    L2Range               `json:"l2_range"`
	L1Block    uint64                `json:"l1_block"`
	BlobHashes []eth.IndexedBlobHash `json:"blob_hashes,omitempty"`
}

// L2Range represents a range of L2 block numbers
type L2Range struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

// Implement DALocation interface for CelestiaLocation
func (c *CelestiaLocation) GetType() string {
	return "celestia"
}

func (c *CelestiaLocation) GetL2Range() L2Range {
	return c.L2Range
}

func (c *CelestiaLocation) GetL1Block() uint64 {
	return c.L1Block
}

func (e *EthereumLocation) GetType() string {
	return "ethereum"
}

func (e *EthereumLocation) GetL2Range() L2Range {
	return e.L2Range
}

func (e *EthereumLocation) GetL1Block() uint64 {
	return e.L1Block
}
