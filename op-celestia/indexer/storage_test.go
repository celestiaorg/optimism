package indexer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStorage_BasicOperations(t *testing.T) {
	storage := NewStorage()

	// Test initial state
	require.Equal(t, uint64(0), storage.GetLastIndexedBlock())
	require.Equal(t, 0, storage.GetIndexedBlockCount())

	// Test setting last indexed block
	storage.SetLastIndexedBlock(100)
	require.Equal(t, uint64(100), storage.GetLastIndexedBlock())

	// Test storing location
	location := &CelestiaLocation{
		Height:     353,
		Commitment: "test-commitment",
		L2Range: L2Range{
			Start: 354,
			End:   359,
		},
	}

	storage.StoreLocation(location)
	require.Equal(t, 6, storage.GetIndexedBlockCount()) // 354-359 inclusive

	// Test retrieving location
	retrievedLocation, exists := storage.GetLocation(355)
	require.True(t, exists)
	require.Equal(t, location, retrievedLocation)

	// Test non-existent block
	_, exists = storage.GetLocation(1000)
	require.False(t, exists)

	// Test retrieving by commitment
	retrievedLocation, exists = storage.GetLocationByCommitment("test-commitment")
	require.True(t, exists)
	require.Equal(t, location, retrievedLocation)

	// Test non-existent commitment
	_, exists = storage.GetLocationByCommitment("non-existent")
	require.False(t, exists)
}

func TestStorage_ThreadSafety(t *testing.T) {
	storage := NewStorage()
	location := &CelestiaLocation{
		Height:     100,
		Commitment: "test",
		L2Range: L2Range{
			Start: 1,
			End:   10,
		},
	}

	// Test concurrent access
	done := make(chan bool, 2)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			storage.StoreLocation(location)
			storage.SetLastIndexedBlock(uint64(i))
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			storage.GetLocation(5)
			storage.GetLastIndexedBlock()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done
}

func TestStorage_Clear(t *testing.T) {
	storage := NewStorage()

	// Add some data
	storage.SetLastIndexedBlock(100)
	location := &CelestiaLocation{
		Height:     353,
		Commitment: "test-commitment",
		L2Range: L2Range{
			Start: 354,
			End:   359,
		},
	}
	storage.StoreLocation(location)

	// Verify data exists
	require.Equal(t, uint64(100), storage.GetLastIndexedBlock())
	require.Equal(t, 6, storage.GetIndexedBlockCount())

	// Clear storage
	storage.Clear()

	// Verify data is cleared
	require.Equal(t, uint64(0), storage.GetLastIndexedBlock())
	require.Equal(t, 0, storage.GetIndexedBlockCount())
}
