package celestia

import "math/big"

// BlockIndexer defines interface for L2→Celestia block mapping
type BlockIndexer interface {
	StoreMapping(l2Start, l2End *big.Int, height uint64, commitment []byte)
	GetCelestiaLocation(l2Block uint64) (*CelestiaLocation, error)
	GetCelestiaLocationBatch(l2Blocks []uint64) (map[uint64]*CelestiaLocation, error)
	Close() error
}

// CelestiaLocation represents where an L2 block can be found in Celestia
type CelestiaLocation struct {
	Height     uint64 `json:"height"`
	Commitment []byte `json:"commitment"`
	L2Range    struct {
		Start *big.Int `json:"start"`
		End   *big.Int `json:"end"`
	} `json:"l2_range"`
}

// RPC method names
const (
	MethodGetCelestiaLocation      = "celestia_getCelestiaLocation"
	MethodGetCelestiaLocationBatch = "celestia_getCelestiaLocationBatch"
)
