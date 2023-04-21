package indexer

import (
	"database/sql"
	"math/big"
	"time"

	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	_ "modernc.org/sqlite"
)

type BlockIndexer struct {
	db *sql.DB
}

func NewBlockIndexer(dbPath string) (*BlockIndexer, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	idx := &BlockIndexer{db: db}
	if err := idx.createTables(); err != nil {
		return nil, err
	}

	return idx, nil
}

func (idx *BlockIndexer) createTables() error {
	query := `
	CREATE TABLE IF NOT EXISTS l2_celestia_mapping (
		l2_block_start INTEGER,
		l2_block_end INTEGER,
		celestia_height INTEGER,
		celestia_commitment BLOB,
		created_at INTEGER,
		PRIMARY KEY (l2_block_start, l2_block_end)
	);
	CREATE INDEX IF NOT EXISTS idx_l2_lookup ON l2_celestia_mapping(l2_block_start, l2_block_end);
	CREATE INDEX IF NOT EXISTS idx_celestia_height ON l2_celestia_mapping(celestia_height);
	`
	_, err := idx.db.Exec(query)
	return err
}

func (idx *BlockIndexer) StoreMapping(l2Start, l2End *big.Int, celestiaHeight uint64, commitment []byte) {
	query := `INSERT OR REPLACE INTO l2_celestia_mapping
		(l2_block_start, l2_block_end, celestia_height, celestia_commitment, created_at)
		VALUES (?, ?, ?, ?, ?)`
	idx.db.Exec(query, l2Start, l2End, celestiaHeight, commitment, time.Now().Unix())
}

func (idx *BlockIndexer) GetCelestiaLocation(l2Block uint64) (*celestia.CelestiaLocation, error) {
	query := `SELECT l2_block_start, l2_block_end, celestia_height, celestia_commitment
		FROM l2_celestia_mapping
		WHERE l2_block_start <= ? AND l2_block_end >= ?
		LIMIT 1`

		var height uint64
		var start, end *big.Int
	var commitment []byte

	err := idx.db.QueryRow(query, l2Block, l2Block).Scan(&start, &end, &height, &commitment)
	if err != nil {
		return nil, err
	}

	return &celestia.CelestiaLocation{
		Height:     height,
		Commitment: commitment,
		L2Range:    struct {
				Start *big.Int `json:"start"`
				End   *big.Int `json:"end"`
			}{Start: start, End: end},
	}, nil
}

func (idx *BlockIndexer) GetCelestiaLocationBatch(l2Blocks []uint64) (map[uint64]*celestia.CelestiaLocation, error) {
	result := make(map[uint64]*celestia.CelestiaLocation)
	for _, block := range l2Blocks {
		if loc, err := idx.GetCelestiaLocation(block); err == nil {
			result[block] = loc
		}
	}
	return result, nil
}

func (idx *BlockIndexer) Close() error {
	return idx.db.Close()
}
