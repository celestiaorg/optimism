package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// SqliteStore provides thread-safe storage for L2 block -> Celestia location mapping
type SqliteStore struct {
	mu   sync.RWMutex
	db   *sql.DB
	path string
}

var _ Store = (*SqliteStore)(nil)

// NewSqliteStore creates a new SQLite store
func NewSqliteStore(path string) (*SqliteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &SqliteStore{
		db:   db,
		path: path,
	}

	// Initialize tables
	if err := store.initTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return store, nil
}

// initTables creates the necessary tables if they don't exist
func (s *SqliteStore) initTables() error {
	// Create metadata table for lastIndexedBlock
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value INTEGER
		)
	`)
	if err != nil {
		return err
	}

	// Create celestia_locations table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS celestia_locations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			commitment TEXT UNIQUE,
			height INTEGER,
			l2_start INTEGER,
			l2_end INTEGER,
			l1_block INTEGER
		)
	`)
	if err != nil {
		return err
	}

	// Create l2_block_mappings table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS l2_block_mappings (
			l2_block_num INTEGER PRIMARY KEY,
			location_id INTEGER,
			FOREIGN KEY (location_id) REFERENCES celestia_locations(id)
		)
	`)
	if err != nil {
		return err
	}

	// Initialize lastIndexedBlock if it doesn't exist
	_, err = s.db.Exec(`
		INSERT OR IGNORE INTO metadata (key, value) VALUES ('lastIndexedBlock', 0)
	`)
	if err != nil {
		return err
	}

	// Add l1_block column if it doesn't exist (for existing databases)
	_, err = s.db.Exec(`
		ALTER TABLE celestia_locations ADD COLUMN l1_block INTEGER DEFAULT 0
	`)
	// Ignore error if column already exists
	if err != nil && err.Error() != "duplicate column name: l1_block" {
		// Log but don't fail - column might already exist
	}

	return nil
}

// SetLastIndexedBlock sets the last indexed L2 block number
func (s *SqliteStore) SetLastIndexedBlock(blockNum uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE metadata SET value = ? WHERE key = 'lastIndexedBlock'
	`, blockNum)
	return err
}

// GetLastIndexedBlock returns the last indexed L2 block number
func (s *SqliteStore) GetLastIndexedBlock() (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastBlock uint64
	err := s.db.QueryRow(`
		SELECT value FROM metadata WHERE key = 'lastIndexedBlock'
	`).Scan(&lastBlock)
	return lastBlock, err
}

// StoreLocation stores the Celestia location for a range of L2 blocks
func (s *SqliteStore) StoreLocation(location *CelestiaLocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert the location
	result, err := tx.Exec(`
		INSERT OR IGNORE INTO celestia_locations
		(commitment, height, l2_start, l2_end, l1_block)
		VALUES (?, ?, ?, ?, ?)
	`, location.Commitment, location.Height,
		location.L2Range.Start, location.L2Range.End, location.L1Block)
	if err != nil {
		return err
	}

	// Get the location ID
	var locationID int64
	if rowsAffected, err := result.RowsAffected(); err != nil {
		return err
	} else if rowsAffected == 0 {
		// Location already exists, get its ID
		err = tx.QueryRow(`
			SELECT id FROM celestia_locations WHERE commitment = ?
		`, location.Commitment).Scan(&locationID)
		if err != nil {
			return err
		}
	} else {
		locationID, err = result.LastInsertId()
		if err != nil {
			return err
		}
	}

	// Store mapping for each L2 block in the range
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO l2_block_mappings (l2_block_num, location_id)
		VALUES (?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for blockNum := location.L2Range.Start; blockNum <= location.L2Range.End; blockNum++ {
		_, err = stmt.Exec(blockNum, locationID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetLocation returns the Celestia location for a given L2 block number
func (s *SqliteStore) GetLocation(l2BlockNum uint64) (*CelestiaLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var location CelestiaLocation
	var start, end uint64

	err := s.db.QueryRow(`
		SELECT
			c.commitment, c.height, c.l2_start, c.l2_end, c.l1_block
		FROM
			celestia_locations c
		JOIN
			l2_block_mappings m ON c.id = m.location_id
		WHERE
			m.l2_block_num = ?
	`, l2BlockNum).Scan(
		&location.Commitment, &location.Height,
		&start, &end, &location.L1Block,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("location not found for block %d", l2BlockNum)
	}
	if err != nil {
		return nil, err
	}

	location.L2Range = L2Range{Start: start, End: end}
	return &location, nil
}

// GetLocationByCommitment returns the Celestia location for a given commitment
func (s *SqliteStore) GetLocationByCommitment(commitment string) (*CelestiaLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var location CelestiaLocation
	var start, end uint64

	err := s.db.QueryRow(`
		SELECT
			commitment, height, l2_start, l2_end, l1_block
		FROM
			celestia_locations
		WHERE
			commitment = ?
	`, commitment).Scan(
		&location.Commitment, &location.Height,
		&start, &end, &location.L1Block,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("location not found for commitment %s", commitment)
	}
	if err != nil {
		return nil, err
	}

	location.L2Range = L2Range{Start: start, End: end}
	return &location, nil
}

// GetIndexedBlockCount returns the number of indexed L2 blocks
func (s *SqliteStore) GetIndexedBlockCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM l2_block_mappings
	`).Scan(&count)
	return count, err
}

// GetAllLocations returns all stored locations (useful for debugging/admin)
func (s *SqliteStore) GetAllLocations() ([]*CelestiaLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT
			commitment, height, l2_start, l2_end, l1_block
		FROM
			celestia_locations
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locations []*CelestiaLocation
	for rows.Next() {
		var location CelestiaLocation
		var start, end uint64
		if err := rows.Scan(
			&location.Commitment, &location.Height,
			&start, &end, &location.L1Block,
		); err != nil {
			return nil, err
		}
		location.L2Range = L2Range{Start: start, End: end}
		locations = append(locations, &location)
	}

	return locations, rows.Err()
}

// Clear removes all stored data (useful for testing)
func (s *SqliteStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete all data
	_, err = tx.Exec(`DELETE FROM l2_block_mappings`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`DELETE FROM celestia_locations`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`UPDATE metadata SET value = 0 WHERE key = 'lastIndexedBlock'`)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// String returns a JSON representation of the storage state (for debugging)
func (s *SqliteStore) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastIndexedBlock uint64
	err := s.db.QueryRow(`
		SELECT value FROM metadata WHERE key = 'lastIndexedBlock'
	`).Scan(&lastIndexedBlock)
	if err != nil {
		return fmt.Sprintf("Storage{error: %v}", err)
	}

	var indexedBlocks, uniqueLocations int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM l2_block_mappings`).Scan(&indexedBlocks)
	if err != nil {
		return fmt.Sprintf("Storage{error: %v}", err)
	}

	err = s.db.QueryRow(`SELECT COUNT(*) FROM celestia_locations`).Scan(&uniqueLocations)
	if err != nil {
		return fmt.Sprintf("Storage{error: %v}", err)
	}

	state := map[string]any{
		"last_indexed_block": lastIndexedBlock,
		"indexed_blocks":     indexedBlocks,
		"unique_locations":   uniqueLocations,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Sprintf("Storage{error: %v}", err)
	}

	return string(data)
}

// Close closes the database connection
func (s *SqliteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}
