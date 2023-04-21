package store

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type StoreTestSuite struct {
	suite.Suite
	store Store
}

func (s *StoreTestSuite) SetupTest() {
	// Will be initialized in the specific test suites
}

func (s *StoreTestSuite) TestBasicOperations() {
	t := s.T()

	// Test initial state
	lastBlock, err := s.store.GetLastIndexedBlock()
	require.NoError(t, err)
	require.Equal(t, uint64(0), lastBlock)

	blockCount, err := s.store.GetIndexedBlockCount()
	require.NoError(t, err)
	require.Equal(t, 0, blockCount)

	// Test setting last indexed block
	s.store.SetLastIndexedBlock(100)
	lastBlock, err = s.store.GetLastIndexedBlock()
	require.NoError(t, err)
	require.Equal(t, uint64(100), lastBlock)

	// Test storing location
	location := &CelestiaLocation{
		Height:     353,
		Commitment: "test-commitment",
		L2Range: L2Range{
			Start: 354,
			End:   359,
		},
	}

	s.store.StoreLocation(location)
	blockCount, err = s.store.GetIndexedBlockCount()
	require.NoError(t, err)
	require.Equal(t, 6, blockCount) // 354-359 inclusive

	// Test retrieving location
	retrievedLocation, err := s.store.GetLocation(355)
	require.NoError(t, err)
	require.NotNil(t, retrievedLocation)
	require.Equal(t, location, retrievedLocation)

	// Test non-existent block
	_, err = s.store.GetLocation(1000)
	require.Error(t, err)

	// Test retrieving by commitment
	retrievedLocation, err = s.store.GetLocationByCommitment("test-commitment")
	require.NoError(t, err)
	require.NotNil(t, retrievedLocation)
	require.Equal(t, location, retrievedLocation)

	// Test non-existent commitment
	_, err = s.store.GetLocationByCommitment("non-existent")
	require.Error(t, err)
}

func (s *StoreTestSuite) TestThreadSafety() {
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
		for i := range 100 {
			s.store.StoreLocation(location)
			s.store.SetLastIndexedBlock(uint64(i))
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for range 100 {
			s.store.GetLocation(5)
			s.store.GetLastIndexedBlock()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done
}

func (s *StoreTestSuite) TestClear() {
	t := s.T()

	// Add some data
	s.store.SetLastIndexedBlock(100)
	location := &CelestiaLocation{
		Height:     353,
		Commitment: "test-commitment",
		L2Range: L2Range{
			Start: 354,
			End:   359,
		},
	}
	s.store.StoreLocation(location)

	// Verify data exists
	lastBlock, err := s.store.GetLastIndexedBlock()
	require.NoError(t, err)
	require.Equal(t, uint64(100), lastBlock)

	blockCount, err := s.store.GetIndexedBlockCount()
	require.NoError(t, err)
	require.Equal(t, 6, blockCount)

	// Clear storage
	s.store.Clear()

	// Verify data is cleared
	lastBlock, err = s.store.GetLastIndexedBlock()
	require.NoError(t, err)
	require.Equal(t, uint64(0), lastBlock)

	blockCount, err = s.store.GetIndexedBlockCount()
	require.NoError(t, err)
	require.Equal(t, 0, blockCount)
}

// Memory store specific test suite
type MemoryStoreTestSuite struct {
	StoreTestSuite
}

func (s *MemoryStoreTestSuite) SetupTest() {
	s.store = NewMemoryStore()
}

func TestMemoryStore(t *testing.T) {
	suite.Run(t, new(MemoryStoreTestSuite))
}

// SQLite store specific test suite
type SQLiteStoreTestSuite struct {
	StoreTestSuite
}

func (s *SQLiteStoreTestSuite) SetupTest() {
	store, err := NewSqliteStore(":memory:")
	require.NoError(s.T(), err)
	s.store = store
}

func TestSQLiteStore(t *testing.T) {
	suite.Run(t, new(SQLiteStoreTestSuite))
}
