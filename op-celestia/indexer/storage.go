package indexer

import (
	"encoding/json"
	"fmt"
	"sync"
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

// Storage provides thread-safe storage for L2 block -> Celestia location mapping
type Storage struct {
	mu sync.RWMutex

	// l2BlockToLocation maps L2 block number to its Celestia location
	l2BlockToLocation map[uint64]*CelestiaLocation

	// lastIndexedBlock tracks the last L2 block that was indexed
	lastIndexedBlock uint64

	// commitmentToLocation maps Celestia commitment to location for quick lookup
	commitmentToLocation map[string]*CelestiaLocation
}

// NewStorage creates a new Storage instance
func NewStorage() *Storage {
	return &Storage{
		l2BlockToLocation:    make(map[uint64]*CelestiaLocation),
		commitmentToLocation: make(map[string]*CelestiaLocation),
		lastIndexedBlock:     0,
	}
}

// SetLastIndexedBlock sets the last indexed L2 block number
func (s *Storage) SetLastIndexedBlock(blockNum uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastIndexedBlock = blockNum
}

// GetLastIndexedBlock returns the last indexed L2 block number
func (s *Storage) GetLastIndexedBlock() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastIndexedBlock
}

// StoreLocation stores the Celestia location for a range of L2 blocks
func (s *Storage) StoreLocation(location *CelestiaLocation) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store mapping for each L2 block in the range
	for blockNum := location.L2Range.Start; blockNum <= location.L2Range.End; blockNum++ {
		s.l2BlockToLocation[blockNum] = location
	}

	// Store commitment mapping
	s.commitmentToLocation[location.Commitment] = location
}

// GetLocation returns the Celestia location for a given L2 block number
func (s *Storage) GetLocation(l2BlockNum uint64) (*CelestiaLocation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	location, exists := s.l2BlockToLocation[l2BlockNum]
	return location, exists
}

// GetLocationByCommitment returns the Celestia location for a given commitment
func (s *Storage) GetLocationByCommitment(commitment string) (*CelestiaLocation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	location, exists := s.commitmentToLocation[commitment]
	return location, exists
}

// GetIndexedBlockCount returns the number of indexed L2 blocks
func (s *Storage) GetIndexedBlockCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.l2BlockToLocation)
}

// GetAllLocations returns all stored locations (useful for debugging/admin)
func (s *Storage) GetAllLocations() []*CelestiaLocation {
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

	return locations
}

// Clear removes all stored data (useful for testing)
func (s *Storage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.l2BlockToLocation = make(map[uint64]*CelestiaLocation)
	s.commitmentToLocation = make(map[string]*CelestiaLocation)
	s.lastIndexedBlock = 0
}

// String returns a JSON representation of the storage state (for debugging)
func (s *Storage) String() string {
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
