package store

import (
	"testing"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
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

func (s *StoreTestSuite) TestCelestiaLocation() {
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
	retrievedLocation, err := s.store.GetDALocation(355)
	require.NoError(t, err)
	require.NotNil(t, retrievedLocation)

	// It should be a CelestiaLocation with type "celestia"
	celLoc, ok := retrievedLocation.(*CelestiaLocation)
	require.True(t, ok)
	require.Equal(t, "celestia", celLoc.GetType())
	require.Equal(t, location, celLoc)

	// Test non-existent block
	_, err = s.store.GetDALocation(1000)
	require.Error(t, err)
}

func (s *StoreTestSuite) TestEthereumCalldataLocation() {
	t := s.T()

	// Calldata-backed Ethereum DA: BlobHashes == nil
	location := &EthereumLocation{
		TxHash: "0xcalldata",
		L2Range: L2Range{
			Start: 200,
			End:   205,
		},
		L1Block:    12345,
		BlobHashes: nil,
	}

	err := s.store.StoreEthLocation(location)
	require.NoError(t, err)

	// 200–205 inclusive
	blockCount, err := s.store.GetIndexedBlockCount()
	require.NoError(t, err)
	require.Equal(t, 6, blockCount)

	// Retrieve one block in the range
	got, err := s.store.GetDALocation(202)
	require.NoError(t, err)
	require.NotNil(t, got)

	ethLoc, ok := got.(*EthereumLocation)
	require.True(t, ok, "expected EthereumLocation")
	require.Equal(t, "ethereum", ethLoc.GetType())

	require.Equal(t, location.TxHash, ethLoc.TxHash)
	require.Equal(t, location.L1Block, ethLoc.L1Block)
	require.Equal(t, location.L2Range, ethLoc.L2Range)

	// Calldata-backed: BlobHashes should be nil
	require.Nil(t, ethLoc.BlobHashes)

	// Block outside the range should error
	_, err = s.store.GetDALocation(300)
	require.Error(t, err)
}

func (s *StoreTestSuite) TestEthereumBlobLocation() {
	t := s.T()

	// Blob-backed Ethereum DA: BlobHashes != nil
	blobHashes := []eth.IndexedBlobHash{
		{
			Index: 0,
			Hash:  common.HexToHash("0x01"),
		},
		{
			Index: 1,
			Hash:  common.HexToHash("0x02"),
		},
	}

	location := &EthereumLocation{
		TxHash: "0xblob",
		L2Range: L2Range{
			Start: 400,
			End:   403,
		},
		L1Block:    54321,
		BlobHashes: blobHashes,
	}

	err := s.store.StoreEthLocation(location)
	require.NoError(t, err)

	// 400–403 inclusive
	blockCount, err := s.store.GetIndexedBlockCount()
	require.NoError(t, err)
	require.Equal(t, 4, blockCount)

	// Retrieve one block in the range
	got, err := s.store.GetDALocation(401)
	require.NoError(t, err)
	require.NotNil(t, got)

	ethLoc, ok := got.(*EthereumLocation)
	require.True(t, ok, "expected EthereumLocation")
	require.Equal(t, "ethereum", ethLoc.GetType())

	require.Equal(t, location.TxHash, ethLoc.TxHash)
	require.Equal(t, location.L1Block, ethLoc.L1Block)
	require.Equal(t, location.L2Range, ethLoc.L2Range)

	// Blob-backed: BlobHashes should be non-nil and equal to what we stored
	require.NotNil(t, ethLoc.BlobHashes)
	require.Len(t, ethLoc.BlobHashes, len(blobHashes))
	require.Equal(t, blobHashes, ethLoc.BlobHashes)
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
			s.store.GetDALocation(5)
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
