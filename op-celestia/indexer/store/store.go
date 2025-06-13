package store

// Store defines the interface for L2 block -> Celestia location storage
// Can be implemented by in-memory storage or database-backed storage
type Store interface {
	// SetLastIndexedBlock sets the last indexed L2 block number
	SetLastIndexedBlock(blockNum uint64) error

	// GetLastIndexedBlock returns the last indexed L2 block number
	GetLastIndexedBlock() (uint64, error)

	// StoreLocation stores the Celestia location for a range of L2 blocks
	StoreLocation(location *CelestiaLocation) error

	// GetLocation returns the Celestia location for a given L2 block number
	GetLocation(l2BlockNum uint64) (*CelestiaLocation, error)

	// GetLocationByCommitment returns the Celestia location for a given commitment
	GetLocationByCommitment(commitment string) (*CelestiaLocation, error)

	// GetIndexedBlockCount returns the number of indexed L2 blocks
	GetIndexedBlockCount() (int, error)

	// GetAllLocations returns all stored locations
	GetAllLocations() ([]*CelestiaLocation, error)

	// Clear removes all stored data
	Clear() error

	// String returns a string representation of the storage state
	String() string
}

// CelestiaLocation represents the location of L2 blocks on Celestia
type CelestiaLocation struct {
	Height     uint64  `json:"height"`
	Commitment string  `json:"commitment"`
	L2Range    L2Range `json:"l2_range"`
	L1Block    uint64  `json:"l1_block"`
}

// L2Range represents a range of L2 block numbers
type L2Range struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}
