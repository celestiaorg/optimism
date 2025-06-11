package indexer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/ethereum-optimism/optimism/op-celestia/metrics"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/sources"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

var (
	ErrIndexerNotRunning = errors.New("indexer is not running")
	ErrBlockNotFound     = errors.New("L2 block not found in index")
)

// heightLen is a length (in bytes) of serialized height.
//
// This is 8 as uint64 consist of 8 bytes.
const heightLen = 8

func SplitID(id []byte) (uint64, []byte) {
	if len(id) <= heightLen {
		return 0, nil
	}
	commitment := id[heightLen:]
	return binary.LittleEndian.Uint64(id[:heightLen]), commitment
}

// L1Client interface for L1 operations
type L1Client interface {
	BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)
}

// OpNodeClient interface for op-node operations (optional)
type OpNodeClient interface {
	OutputAtBlock(ctx context.Context, blockNum uint64) (*eth.OutputResponse, error)
}

// DriverSetup contains the configuration and dependencies for the indexer driver
type DriverSetup struct {
	Log      log.Logger
	Metr     metrics.Metricer
	Cfg      IndexerConfig
	L1Client L1Client
	// TODO: use interface?
	L2Client       *sources.L2Client
	OpNodeClient   OpNodeClient // optional, for verification
	CelestiaClient *celestia.DAClient
	Storage        *Storage
}

// IndexerDriver is responsible for indexing L2 block locations on Celestia
type IndexerDriver struct {
	DriverSetup

	currentBlock atomic.Uint64

	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	running atomic.Bool

	// Synchronization channels
	done chan struct{}
}

// NewIndexerDriver creates a new IndexerDriver instance
func NewIndexerDriver(setup DriverSetup) *IndexerDriver {
	ctx, cancel := context.WithCancel(context.Background())

	return &IndexerDriver{
		DriverSetup: setup,
		ctx:         ctx,
		cancel:      cancel,
		done:        make(chan struct{}),
	}
}

// Start begins the indexer operation
func (d *IndexerDriver) Start() error {
	d.Log.Info("Starting Celestia Indexer")

	if !d.running.CompareAndSwap(false, true) {
		return errors.New("indexer is already running")
	}

	// Start the main indexing loop
	d.wg.Add(1)
	go d.indexingLoop()

	d.Log.Info("Celestia Indexer started")
	return nil
}

// Stop gracefully shuts down the indexer
func (d *IndexerDriver) Stop() error {
	d.Log.Info("Stopping Celestia Indexer")

	if !d.running.CompareAndSwap(true, false) {
		return ErrIndexerNotRunning
	}

	d.cancel()
	close(d.done)
	d.wg.Wait()

	d.Log.Info("Celestia Indexer stopped")
	return nil
}

// GetLocation returns the Celestia location for a given L2 block number
func (d *IndexerDriver) GetLocation(l2BlockNum uint64) (*CelestiaLocation, error) {
	location, exists := d.Storage.GetLocation(l2BlockNum)
	if !exists {
		return nil, fmt.Errorf("%w: L2 block %d", ErrBlockNotFound, l2BlockNum)
	}
	return location, nil
}

// indexingLoop is the main loop that performs indexing operations
func (d *IndexerDriver) indexingLoop() {
	defer d.wg.Done()
	defer d.Log.Info("Indexing loop stopped")

	ticker := time.NewTicker(d.Cfg.PollInterval)
	defer ticker.Stop()

	// Perform initial catch-up
	if err := d.catchUp(); err != nil {
		d.Log.Error("Failed to catch up during startup", "err", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := d.indexNewBlocks(); err != nil {
				d.Log.Error("Failed to index new blocks", "err", err)
			}
		case <-d.done:
			return
		}
	}
}

// catchUp performs initial indexing from the start block to current head
func (d *IndexerDriver) catchUp() error {
	d.Log.Info("Starting catch-up indexing")

	lastIndexed := d.Storage.GetLastIndexedBlock()
	startBlock := d.Cfg.StartL1Block

	if lastIndexed > 0 {
		startBlock = lastIndexed + 1
		d.Log.Info("Resuming from last indexed block", "last_indexed", lastIndexed, "start_block", startBlock)
	}

	// Get current L1 head to determine how far to catch up
	currentL1Head, err := d.getCurrentL1Head()
	if err != nil {
		return fmt.Errorf("failed to get current L1 head: %w", err)
	}

	d.Log.Info("Catching up to current L1 head", "start_block", startBlock, "l1_head", currentL1Head)

	// Process blocks in batches to avoid overwhelming the system
	return d.indexBlockRange(startBlock, currentL1Head)
}

// indexNewBlocks indexes newly available blocks
func (d *IndexerDriver) indexNewBlocks() error {
	lastIndexed := d.Storage.GetLastIndexedBlock()
	currentL1Head, err := d.getCurrentL1Head()
	if err != nil {
		return fmt.Errorf("failed to get current L1 head: %w", err)
	}

	if currentL1Head <= lastIndexed {
		// No new blocks to index
		return nil
	}

	d.Log.Debug("Indexing new blocks", "last_indexed", lastIndexed, "l1_head", currentL1Head)
	return d.indexBlockRange(lastIndexed+1, currentL1Head)
}

// indexBlockRange indexes a range of L1 blocks to find Celestia references
func (d *IndexerDriver) indexBlockRange(startBlock, endBlock uint64) error {
	for blockNum := startBlock; blockNum <= endBlock; blockNum++ {
		select {
		case <-d.done:
			return nil
		default:
		}

		if err := d.indexL1Block(blockNum); err != nil {
			d.Log.Error("Failed to index L1 block", "block", blockNum, "err", err)
			// Continue with next block rather than failing entirely
			continue
		}

		d.Storage.SetLastIndexedBlock(blockNum)
		d.Metr.RecordIndexedBlock(blockNum)
	}

	return nil
}

// indexL1Block processes a single L1 block to find Celestia references
func (d *IndexerDriver) indexL1Block(blockNum uint64) error {
	ctx, cancel := context.WithTimeout(d.ctx, d.Cfg.NetworkTimeout)
	defer cancel()

	// Get the L1 block
	block, err := d.L1Client.BlockByNumber(ctx, big.NewInt(int64(blockNum)))
	if err != nil {
		return fmt.Errorf("failed to get L1 block %d: %w", blockNum, err)
	}

	// Look for transactions to the batch inbox
	for _, tx := range block.Transactions() {
		if tx.To() != nil && *tx.To() == d.Cfg.BatchInboxAddress {
			if err := d.processBatchTransaction(tx); err != nil {
				d.Log.Warn("Failed to process batch transaction", "tx", tx.Hash(), "err", err)
				// Continue processing other transactions
			}
		}
	}

	return nil
}

// processBatchTransaction processes a transaction to the batch inbox
func (d *IndexerDriver) processBatchTransaction(tx *types.Transaction) error {
	data := tx.Data()
	if len(data) == 0 {
		return nil
	}

	// Check if this is a Celestia reference (version byte 0xce)
	if data[0] != celestia.DerivationVersionCelestia {
		return nil // Not a Celestia reference, skip
	}

	if len(data) < 41 { // 1 byte version + 8 bytes height + 32 bytes commitment
		return fmt.Errorf("invalid Celestia reference data length: %d", len(data))
	}

	height, commitmentBytes := SplitID(data[1:])
	commitment := base64.StdEncoding.EncodeToString(commitmentBytes)

	d.Log.Debug("Found Celestia reference", "height", height, "commitment", commitment, "tx", tx.Hash())

	// Fetch and parse frames from Celestia
	return d.processCelestiaFrames(data[1:], height, commitment)
}

// processCelestiaFrames fetches frames from Celestia and extracts L2 block ranges
func (d *IndexerDriver) processCelestiaFrames(ids []byte, height uint64, commitment string) error {
	ctx, cancel := context.WithTimeout(d.ctx, d.Cfg.NetworkTimeout)
	defer cancel()

	blobs, err := d.CelestiaClient.Client.Get(ctx, [][]byte{ids}, d.CelestiaClient.Namespace)
	if err != nil {
		return fmt.Errorf("failed to fetch blobs from Celestia: %w", err)
	}

	if len(blobs) == 0 {
		return fmt.Errorf("no blobs returned from Celestia for commitment %s", commitment)
	}

	// Parse frames from blob data
	frameData := blobs[0] // Assuming single blob per commitment
	frames, err := derive.ParseFrames(frameData)
	if err != nil {
		return fmt.Errorf("failed to parse frames: %w", err)
	}

	if len(frames) == 0 {
		return fmt.Errorf("no frames found in blob data")
	}

	// Extract L2 block range from frames
	l2Range, err := d.extractL2Range(frames)
	if err != nil {
		return fmt.Errorf("failed to extract L2 range: %w", err)
	}

	// Store the location
	location := &CelestiaLocation{
		Height:     height,
		Commitment: commitment,
		L2Range:    *l2Range,
	}

	d.Storage.StoreLocation(location)
	d.Metr.RecordLocationStored(location.L2Range.Start, location.L2Range.End)

	d.Log.Info("Stored Celestia location",
		"height", height,
		"commitment", commitment,
		"l2_start", l2Range.Start,
		"l2_end", l2Range.End)

	// Optional verification against op-node
	if d.OpNodeClient != nil {
		if err := d.verifyWithOpNode(l2Range.Start); err != nil {
			d.Log.Warn("Verification with op-node failed", "err", err, "l2_block", l2Range.Start)
		}
	}

	return nil
}

// extractL2Range extracts the L2 block range from parsed frames by actually parsing batch data
func (d *IndexerDriver) extractL2Range(frames []derive.Frame) (*L2Range, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames provided")
	}

	var l2Blocks []uint64

	// Parse each frame to extract batch data and determine L2 block numbers
	for frameIndex, frame := range frames {
		// Create reader from frame data
		frameReader := bytes.NewReader(frame.Data)

		// Create batch reader for this frame with a reasonable size limit
		// TODO: use rollup config for maxRLPBytesPerChannel
		br, err := derive.BatchReader(frameReader, 100_000_000, true)
		if err != nil {
			d.Log.Warn("Error creating batch reader for frame",
				"frame_index", frameIndex,
				"channel_id", frame.ID.String(),
				"err", err)
			continue
		}

		// Read batches from this specific frame
		for batchData, err := br(); err != io.EOF; batchData, err = br() {
			if err != nil {
				d.Log.Warn("Error reading batch data from frame",
					"frame_index", frameIndex,
					"channel_id", frame.ID.String(),
					"err", err)
				continue
			}

			batchType := batchData.GetBatchType()
			switch batchType {
			case derive.SingularBatchType:
				singularBatch, err := derive.GetSingularBatch(batchData)
				if err != nil {
					d.Log.Warn("Error deriving singular batch",
						"frame_index", frameIndex,
						"channel_id", frame.ID.String(),
						"err", err)
					continue
				}
				if singularBatch != nil {
					l2Block, err := d.L2Client.BlockRefByHash(context.Background(), singularBatch.ParentHash)
					if err != nil {
						d.Log.Warn("Error getting L2 block by hash",
							"frame_index", frameIndex,
							"channel_id", frame.ID.String(),
							"parent_hash", singularBatch.ParentHash.String(),
							"err", err)
						continue
					}
					// currentBlock is the next block number after the parent block
					currentBlock := l2Block.Number + 1
					l2Blocks = append(l2Blocks, currentBlock)
					d.currentBlock.Store(currentBlock)
				} else {
					d.Log.Warn("Got nil singular batch",
						"frame_index", frameIndex,
						"channel_id", frame.ID.String())
					continue
				}

			case derive.SpanBatchType:
				// For span batches, we need the rollup config to derive properly
				// We can't use parent hash to get block number, so we need to keep track of current block
				spanBatch, err := derive.DeriveSpanBatch(batchData, d.Cfg.L2BlockTime, d.Cfg.L2GenesisTime, d.Cfg.ChainID)
				if err != nil {
					d.Log.Warn("Error deriving span batch",
						"frame_index", frameIndex,
						"channel_id", frame.ID.String(),
						"err", err)
					continue
				}
				// Span batches require current block to be set
				if d.currentBlock.Load() == 0 {
					d.Log.Warn("Need to have processed at least one singular batch to derive span batch",
						"frame_index", frameIndex,
						"channel_id", frame.ID.String())
					continue
				}
				if spanBatch != nil {
					for batchIndex := range spanBatch.Batches {
						currentBlock := d.currentBlock.Add(uint64(batchIndex)+1)
						if batchIndex == 0 && d.Cfg.VerifyParentCheck {
							l2Block, err := d.L2Client.BlockRefByNumber(context.Background(), currentBlock)
							if err != nil {
								d.Log.Warn("Error getting L2 block by hash",
									"frame_index", frameIndex,
									"channel_id", frame.ID.String(),
									"err", err)
								continue
							}
							if !bytes.Equal(l2Block.Hash[:20], spanBatch.ParentCheck[:]) {
								d.Log.Warn("Parent check mismatch",
									"frame_index", frameIndex,
									"channel_id", frame.ID.String(),
									"l2_block", l2Block.Hash.String())
							}
						}
						l2Blocks = append(l2Blocks, currentBlock)
						d.currentBlock.Store(currentBlock)
					}
				} else {
					d.Log.Warn("Got nil span batch",
						"frame_index", frameIndex,
						"channel_id", frame.ID.String())
					continue
				}

			default:
				d.Log.Warn("Unrecognized batch type",
					"batch_type", batchType,
					"frame_index", frameIndex,
					"channel_id", frame.ID.String())
			}
		}
	}

	// pre-holocene batches may be out of order
	slices.Sort(l2Blocks)

	l2Range := &L2Range{
		Start: l2Blocks[0],
		End:   l2Blocks[len(l2Blocks)-1],
	}

	d.Log.Debug("Extracted L2 range from frames",
		"start", l2Range.Start,
		"end", l2Range.End,
		"frame_count", len(frames))

	return l2Range, nil
}

// verifyWithOpNode verifies a block hash with op-node if available
func (d *IndexerDriver) verifyWithOpNode(l2BlockNum uint64) error {
	if d.OpNodeClient == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(d.ctx, d.Cfg.NetworkTimeout)
	defer cancel()

	output, err := d.OpNodeClient.OutputAtBlock(ctx, l2BlockNum)
	if err != nil {
		return fmt.Errorf("failed to get output from op-node: %w", err)
	}

	// Simple verification - just log the block info for now
	d.Log.Debug("Verified with op-node",
		"l2_block", l2BlockNum,
		"block_hash", output.BlockRef.Hash,
		"output_root", output.OutputRoot)

	return nil
}

// getCurrentL1Head returns the current L1 head block number
func (d *IndexerDriver) getCurrentL1Head() (uint64, error) {
	ctx, cancel := context.WithTimeout(d.ctx, d.Cfg.NetworkTimeout)
	defer cancel()

	header, err := d.L1Client.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get L1 head: %w", err)
	}

	return header.Number.Uint64(), nil
}
