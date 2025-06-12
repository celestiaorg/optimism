package store

import (
	"encoding/json"
	"fmt"
	"sync"
)

// MemoryStore provides thread-safe storage for L2 block -> Celestia location mapping
type MemoryStore struct {
	mu sync.RWMutex

	// l2BlockToLocation maps L2 block number to its Celestia location
	l2BlockToLocation map[uint64]*CelestiaLocation

	// lastIndexedBlock tracks the last L2 block that was indexed
	lastIndexedBlock uint64

	// commitmentToLocation maps Celestia commitment to location for quick lookup
	commitmentToLocation map[string]*CelestiaLocation
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		l2BlockToLocation:    make(map[uint64]*CelestiaLocation),
		commitmentToLocation: make(map[string]*CelestiaLocation),
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
	}

	// Store commitment mapping
	s.commitmentToLocation[location.Commitment] = location
	return nil
}

// GetLocation returns the Celestia location for a given L2 block number
func (s *MemoryStore) GetLocation(l2BlockNum uint64) (*CelestiaLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	location, exists := s.l2BlockToLocation[l2BlockNum]
	if !exists {
		return nil, fmt.Errorf("location not found for block %d", l2BlockNum)
	}
	return location, nil
}

// GetLocationByCommitment returns the Celestia location for a given commitment
func (s *MemoryStore) GetLocationByCommitment(commitment string) (*CelestiaLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	location, exists := s.commitmentToLocation[commitment]
	if !exists {
		return nil, fmt.Errorf("location not found for commitment %s", commitment)
	}
	return location, nil
}

// GetIndexedBlockCount returns the number of indexed L2 blocks
func (s *MemoryStore) GetIndexedBlockCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.l2BlockToLocation), nil
}

// GetAllLocations returns all stored locations (useful for debugging/admin)
func (s *MemoryStore) GetAllLocations() ([]*CelestiaLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	var locations []*CelestiaLocation

	for _, location := range s.l2BlockToLocation {
		if !seen[location.Commitment] {
			seen[location.Commitment] = true
			locations = append(locations, location)
		}
	}

	return locations, nil
}

// Clear removes all stored data (useful for testing)
func (s *MemoryStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.l2BlockToLocation = make(map[uint64]*CelestiaLocation)
	s.commitmentToLocation = make(map[string]*CelestiaLocation)
	s.lastIndexedBlock = 0
	return nil
}

// String returns a JSON representation of the storage state (for debugging)
func (s *MemoryStore) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state := map[string]any{
		"last_indexed_block": s.lastIndexedBlock,
		"indexed_blocks":     len(s.l2BlockToLocation),
		"unique_locations":   len(s.commitmentToLocation),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Sprintf("Storage{error: %v}", err)
	}

	return string(data)
}
