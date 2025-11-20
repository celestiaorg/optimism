package indexer

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	libshare "github.com/celestiaorg/go-square/v2/share"
	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	"github.com/ethereum-optimism/optimism/op-celestia/indexer/store"
	"github.com/ethereum-optimism/optimism/op-celestia/metrics"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/sources"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

var (
	ErrIndexerNotRunning = errors.New("indexer is not running")
	ErrBlockNotFound     = errors.New("L2 block not found in index")
)

// L1Client wraps both Ethereum's EL (execution, calldata DA) and CL (beacon, EIP4844 DA) clients
type L1Client interface {
	// execution
	BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error)

	// consensus / blobs (EIP-4844)
	GetBlobs(ctx context.Context, ref eth.L1BlockRef, hashes []eth.IndexedBlobHash) ([]*eth.Blob, error)

	// lifecycle
	Close()
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
	Store          store.Store
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

// GetDALocation returns the DA location (either Celestia or Ethereum) for a given L2 block number
func (d *IndexerDriver) GetDALocation(l2BlockNum uint64) (store.DALocation, error) {
	return d.Store.GetDALocation(l2BlockNum)
}

// GetStatus returns the current status of the indexer
func (d *IndexerDriver) GetStatus() (lastIndexedBlock uint64, indexedBlocks int, running bool, l2Start uint64, l2End uint64, err error) {
	lastIndexedBlock, err = d.Store.GetLastIndexedBlock()
	if err != nil {
		return 0, 0, false, 0, 0, fmt.Errorf("failed to get last indexed block: %w", err)
	}

	indexedBlocks, err = d.Store.GetIndexedBlockCount()
	if err != nil {
		return lastIndexedBlock, 0, false, 0, 0, fmt.Errorf("failed to get indexed block count: %w", err)
	}

	l2Start, l2End, err = d.Store.GetL2BlockRange()
	if err != nil {
		return lastIndexedBlock, indexedBlocks, false, 0, 0, fmt.Errorf("failed to get L2 block range: %w", err)
	}

	running = d.running.Load()

	return lastIndexedBlock, indexedBlocks, running, l2Start, l2End, nil
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

	lastIndexed, err := d.Store.GetLastIndexedBlock()
	if err != nil {
		return fmt.Errorf("failed to get last indexed block: %w", err)
	}
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
	lastIndexed, err := d.Store.GetLastIndexedBlock()
	if err != nil {
		return fmt.Errorf("failed to get last indexed block: %w", err)
	}

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

		if err := d.Store.SetLastIndexedBlock(blockNum); err != nil {
			d.Log.Error("Failed to update last indexed block", "block", blockNum, "err", err)
			return fmt.Errorf("failed to update last indexed block: %w", err)
		}
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
			if err := d.processBatchTransaction(tx, blockNum); err != nil {
				d.Log.Warn("Failed to process batch transaction", "tx", tx.Hash(), "err", err)
				// Continue processing other transactions
			}
		}
	}

	return nil
}

// processBatchTransaction processes a transaction to the batch inbox
// processBatchTransaction processes a transaction to the batch inbox
func (d *IndexerDriver) processBatchTransaction(tx *types.Transaction, blockNum uint64) error {
	// First: EIP-4844 blob batches.
	// Blob txs can have empty calldata, so we must not early-return on len(tx.Data()) == 0.
	if d.L1Client != nil {
		blobHashes := tx.BlobHashes()
		if len(blobHashes) > 0 {
			d.Log.Debug("Found ETH blob DA batch", "tx", tx.Hash(), "l1_block", blockNum, "blob_count", len(blobHashes))
			return d.processBlobDABatch(tx, blockNum, blobHashes)
		}
	}

	// Fallback to existing calldata / Celestia paths
	data := tx.Data()
	if len(data) == 0 {
		// No calldata and no blobs => nothing to index
		return nil
	}

	// Check version byte to determine DA type
	switch data[0] {
	case 0x00:
		d.Log.Debug("Found ETH DA batch", "tx", tx.Hash(), "l1_block", blockNum)
		return d.processEthDABatch(tx, blockNum)

	case 0x01:
		// Check if this is an OP Stack Celestia commitment
		// Format: version_byte(0x01) + commitment_type(0x01) + da_layer_byte(0x0c) + payload
		if len(data) < 3 {
			return nil // Not enough data
		}

		if data[1] == 0x01 && data[2] == 0x0c {
			// Commitment type and DA layer byte for Celestia
			if len(data) < 43 { // 3 bytes header + 8 bytes height + 32 bytes commitment
				return fmt.Errorf("invalid OP Stack Celestia commitment length: %d", len(data))
			}

			// Skip the 3-byte header to get the payload
			payload := data[3:]
			height, commitmentBytes := celestia.SplitID(payload)
			commitment := base64.StdEncoding.EncodeToString(commitmentBytes)

			d.Log.Debug("Found OP Stack Celestia commitment", "height", height, "commitment", commitment, "tx", tx.Hash())

			// Fetch and parse frames from Celestia
			return d.processCelestiaFrames(payload, blockNum)
		}
	}

	return nil
}

// processCelestiaFrames fetches frames from Celestia and extracts L2 block ranges
func (d *IndexerDriver) processCelestiaFrames(id []byte, blockNum uint64) error {
	ctx, cancel := context.WithTimeout(d.ctx, d.Cfg.NetworkTimeout)
	defer cancel()

	height, commitment := celestia.SplitID(id)
	namespace, err := libshare.NewNamespaceFromBytes(d.CelestiaClient.Namespace)
	if err != nil {
		return err
	}

	d.Log.Debug("Found Celestia reference", "height", height, "commitment", base64.StdEncoding.EncodeToString(commitment))

	blob, err := d.CelestiaClient.Client.Get(ctx, height, namespace, commitment)
	if err != nil {
		return fmt.Errorf("failed to fetch blobs from Celestia: %w", err)
	}

	// Parse frames from blob data
	frameData := blob.Blob.Data()
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
	location := &store.CelestiaLocation{
		Height:     height,
		Commitment: base64.StdEncoding.EncodeToString(commitment),
		L2Range:    *l2Range,
		L1Block:    blockNum,
	}

	err = d.Store.StoreLocation(location)
	if err != nil {
		return err
	}
	d.Metr.RecordLocationStored(location.L2Range.Start, location.L2Range.End)

	d.Log.Info("Stored Celestia location",
		"height", height,
		"commitment", base64.StdEncoding.EncodeToString(commitment),
		"l2_start", l2Range.Start,
		"l2_end", l2Range.End,
		"l1_block", blockNum)

	// Optional verification against op-node
	if d.OpNodeClient != nil {
		if err := d.verifyWithOpNode(l2Range.Start); err != nil {
			d.Log.Warn("Verification with op-node failed", "err", err, "l2_block", l2Range.Start)
		}
	}

	return nil
}

// extractL2Range extracts the L2 block range from parsed frames by actually parsing batch data
func (d *IndexerDriver) extractL2Range(frames []derive.Frame) (*store.L2Range, error) {
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
				// If current block is not set, calculate it from the span batch timestamp
				if d.currentBlock.Load() == 0 {
					// Calculate the starting block number from the first block's timestamp
					// The first block in the span has timestamp: genesisTime + relTimestamp
					firstBlockTimestamp := spanBatch.GetTimestamp()
					startingBlockNum := (firstBlockTimestamp - d.Cfg.L2GenesisTime) / d.Cfg.L2BlockTime
					d.currentBlock.Store(startingBlockNum)
					d.Log.Info("Calculated starting block from span batch",
						"starting_block", startingBlockNum,
						"timestamp", firstBlockTimestamp,
						"frame_index", frameIndex,
						"channel_id", frame.ID.String())
				}
				if spanBatch != nil {
					// Get the starting block number for this span
					startBlock := d.currentBlock.Load()
					for batchIndex := range spanBatch.Batches {
						// Each batch in the span represents one consecutive L2 block
						currentBlock := startBlock + uint64(batchIndex) + 1
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
					}
					// Update currentBlock to the last processed block
					d.currentBlock.Store(startBlock + uint64(len(spanBatch.Batches)))
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

	// Check if we have any L2 blocks
	if len(l2Blocks) == 0 {
		return nil, fmt.Errorf("no L2 blocks found in frames")
	}

	// pre-holocene batches may be out of order
	slices.Sort(l2Blocks)

	l2Range := &store.L2Range{
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

// processBlobDABatch processes ETH DA batches that use EIP-4844 blobs instead of calldata.
func (d *IndexerDriver) processBlobDABatch(tx *types.Transaction, blockNum uint64, blobHashes []common.Hash) error {
	if d.L1Client == nil {
		return fmt.Errorf("blob batch encountered but no L1 client configured")
	}

	ctx, cancel := context.WithTimeout(d.ctx, d.Cfg.NetworkTimeout)
	defer cancel()

	// Build indexed blob hashes, preserving order
	indexed := make([]eth.IndexedBlobHash, 0, len(blobHashes))
	for i, h := range blobHashes {
		indexed = append(indexed, eth.IndexedBlobHash{
			Index: uint64(i),
			Hash:  h,
		})
	}

	// Get the L1 header so we can construct an L1BlockRef for this block.
	header, err := d.L1Client.HeaderByNumber(ctx, big.NewInt(int64(blockNum)))
	if err != nil {
		return fmt.Errorf("failed to get L1 header for blob batch: %w", err)
	}

	ref := eth.L1BlockRef{
		Hash:       header.Hash(),
		Number:     header.Number.Uint64(),
		ParentHash: header.ParentHash,
		Time:       header.Time,
	}

	// Fetch blobs via the L1Client (this wraps the beacon client under the hood).
	blobs, err := d.L1Client.GetBlobs(ctx, ref, indexed)
	if err != nil {
		return fmt.Errorf("failed to fetch blobs for tx %s: %w", tx.Hash(), err)
	}
	if len(blobs) == 0 {
		return fmt.Errorf("no blobs returned for tx %s", tx.Hash())
	}

	// Decode blobs into contiguous frame data
	var buf bytes.Buffer
	for i, b := range blobs {
		if b == nil {
			continue
		}
		data, err := b.ToData()
		if err != nil {
			d.Log.Warn("Failed to decode blob data", "tx", tx.Hash(), "blob_index", i, "err", err)
			continue
		}
		if len(data) == 0 {
			continue
		}
		if _, err := buf.Write(data); err != nil {
			return fmt.Errorf("failed to concatenate blob data: %w", err)
		}
	}

	frameData := buf.Bytes()
	if len(frameData) == 0 {
		return fmt.Errorf("empty blob frame data for tx %s", tx.Hash())
	}

	// Parse frames from stitched blob data
	frames, err := derive.ParseFrames(frameData)
	if err != nil {
		return fmt.Errorf("failed to parse frames from blob data: %w", err)
	}
	if len(frames) == 0 {
		return fmt.Errorf("no frames found in blob data for tx %s", tx.Hash())
	}

	// Extract L2 block range from frames
	l2Range, err := d.extractL2Range(frames)
	if err != nil {
		return fmt.Errorf("failed to extract L2 range from blob DA: %w", err)
	}

	// Store the ETH DA location same as calldata-based batches.
	location := &store.EthereumLocation{
		TxHash:  tx.Hash().Hex(),
		L2Range: *l2Range,
		L1Block: blockNum,
	}

	if err := d.Store.StoreEthLocation(location); err != nil {
		return fmt.Errorf("failed to store blob ETH DA location: %w", err)
	}
	d.Metr.RecordLocationStored(location.L2Range.Start, location.L2Range.End)

	d.Log.Info("Stored blob ETH DA location",
		"tx_hash", location.TxHash,
		"l2_start", l2Range.Start,
		"l2_end", l2Range.End,
		"l1_block", blockNum,
		"blob_count", len(blobs),
	)

	if d.OpNodeClient != nil {
		if err := d.verifyWithOpNode(l2Range.Start); err != nil {
			d.Log.Warn("Verification with op-node failed", "err", err, "l2_block", l2Range.Start)
		}
	}

	return nil
}

// processEthDABatch processes ETH DA batches
func (d *IndexerDriver) processEthDABatch(tx *types.Transaction, blockNum uint64) error {
	// Parse frames directly from calldata
	frames, err := derive.ParseFrames(tx.Data())
	if err != nil {
		return fmt.Errorf("failed to parse ETH DA frames: %w", err)
	}

	if len(frames) == 0 {
		return fmt.Errorf("no frames found in ETH DA batch")
	}

	// Extract L2 block range from frames
	l2Range, err := d.extractL2Range(frames)
	if err != nil {
		return fmt.Errorf("failed to extract L2 range from ETH DA: %w", err)
	}

	// Store the ETH DA location
	location := &store.EthereumLocation{
		TxHash:  tx.Hash().Hex(),
		L2Range: *l2Range,
		L1Block: blockNum,
	}

	err = d.Store.StoreEthLocation(location)
	if err != nil {
		return fmt.Errorf("failed to store ETH DA location: %w", err)
	}
	d.Metr.RecordLocationStored(location.L2Range.Start, location.L2Range.End)

	d.Log.Info("Stored ETH DA location",
		"tx_hash", tx.Hash().Hex(),
		"l2_start", l2Range.Start,
		"l2_end", l2Range.End,
		"l1_block", blockNum)

	// Optional verification against op-node
	if d.OpNodeClient != nil {
		if err := d.verifyWithOpNode(l2Range.Start); err != nil {
			d.Log.Warn("Verification with op-node failed", "err", err, "l2_block", l2Range.Start)
		}
	}

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
