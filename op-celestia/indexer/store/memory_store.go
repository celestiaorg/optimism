package store

import (
	"encoding/json"
	"fmt"
	"sync"
)

// MemoryStore provides thread-safe storage for L2 block -> DA location mapping
type MemoryStore struct {
	mu sync.RWMutex

	// l2BlockToLocation maps L2 block number to its Celestia location
	l2BlockToLocation map[uint64]*CelestiaLocation

	// l2BlockToEthLocation maps L2 block number to its Ethereum DA location
	l2BlockToEthLocation map[uint64]*EthereumLocation

	// l2BlockDAType maps L2 block number to its DA type ("celestia" or "ethereum")
	l2BlockDAType map[uint64]string

	// lastIndexedBlock tracks the last L2 block that was indexed
	lastIndexedBlock uint64

	// commitmentToLocation maps Celestia commitment to location for quick lookup
	commitmentToLocation map[string]*CelestiaLocation

	// txHashToLocation maps Ethereum tx hash to location for quick lookup
	txHashToLocation map[string]*EthereumLocation
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		l2BlockToLocation:    make(map[uint64]*CelestiaLocation),
		l2BlockToEthLocation: make(map[uint64]*EthereumLocation),
		l2BlockDAType:        make(map[uint64]string),
		commitmentToLocation: make(map[string]*CelestiaLocation),
		txHashToLocation:     make(map[string]*EthereumLocation),
		lastIndexedBlock:     0,
	}
}

// SetLastIndexedBlock sets the last indexed L2 block number
func (s *MemoryStore) SetLastIndexedBlock(blockNum uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastIndexedBlock = blockNum
	return nil
}

// GetLastIndexedBlock returns the last indexed L2 block number
func (s *MemoryStore) GetLastIndexedBlock() (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastIndexedBlock, nil
}

// StoreLocation stores the Celestia location for a range of L2 blocks
func (s *MemoryStore) StoreLocation(location *CelestiaLocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store mapping for each L2 block in the range
	for blockNum := location.L2Range.Start; blockNum <= location.L2Range.End; blockNum++ {
		s.l2BlockToLocation[blockNum] = location
		s.l2BlockDAType[blockNum] = "celestia"
	}

	// Store commitment mapping
	s.commitmentToLocation[location.Commitment] = location
	return nil
}

// StoreEthLocation stores the Ethereum DA location for a range of L2 blocks
func (s *MemoryStore) StoreEthLocation(location *EthereumLocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store mapping for each L2 block in the range
	for blockNum := location.L2Range.Start; blockNum <= location.L2Range.End; blockNum++ {
		s.l2BlockToEthLocation[blockNum] = location
		s.l2BlockDAType[blockNum] = "ethereum"
	}

	// Store tx hash mapping
	s.txHashToLocation[location.TxHash] = location
	return nil
}

// GetDALocation returns the DA location (either Celestia or Ethereum) for a given L2 block number
func (s *MemoryStore) GetDALocation(l2BlockNum uint64) (DALocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	daType, exists := s.l2BlockDAType[l2BlockNum]
	if !exists {
		return nil, fmt.Errorf("location not found for block %d", l2BlockNum)
	}

	switch daType {
	case "celestia":
		location, exists := s.l2BlockToLocation[l2BlockNum]
		if !exists {
			return nil, fmt.Errorf("celestia location not found for block %d", l2BlockNum)
		}
		return location, nil

	case "ethereum":
		location, exists := s.l2BlockToEthLocation[l2BlockNum]
		if !exists {
			return nil, fmt.Errorf("ethereum location not found for block %d", l2BlockNum)
		}
		return location, nil

	default:
		return nil, fmt.Errorf("unknown DA type: %s", daType)
	}
}

// GetIndexedBlockCount returns the number of indexed L2 blocks on all DA providers
func (s *MemoryStore) GetIndexedBlockCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.l2BlockDAType), nil
}

// GetL2BlockRange returns the minimum and maximum L2 block numbers indexed
func (s *MemoryStore) GetL2BlockRange() (min uint64, max uint64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.l2BlockDAType) == 0 {
		return 0, 0, nil
	}

	// Find min and max L2 block numbers
	first := true
	for blockNum := range s.l2BlockDAType {
		if first {
			min = blockNum
			max = blockNum
			first = false
		} else {
			if blockNum < min {
				min = blockNum
			}
			if blockNum > max {
				max = blockNum
			}
		}
	}

	return min, max, nil
}

// Clear removes all stored data (useful for testing)
func (s *MemoryStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.l2BlockToLocation = make(map[uint64]*CelestiaLocation)
	s.l2BlockToEthLocation = make(map[uint64]*EthereumLocation)
	s.l2BlockDAType = make(map[uint64]string)
	s.commitmentToLocation = make(map[string]*CelestiaLocation)
	s.txHashToLocation = make(map[string]*EthereumLocation)
	s.lastIndexedBlock = 0
	return nil
}

// String returns a JSON representation of the storage state (for debugging)
func (s *MemoryStore) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state := map[string]any{
		"last_indexed_block":        s.lastIndexedBlock,
		"total_indexed_blocks":      len(s.l2BlockDAType),
		"celestia_blocks":           len(s.l2BlockToLocation),
		"ethereum_blocks":           len(s.l2BlockToEthLocation),
		"unique_celestia_locations": len(s.commitmentToLocation),
		"unique_eth_locations":      len(s.txHashToLocation),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Sprintf("Storage{error: %v}", err)
	}

	return string(data)
}
