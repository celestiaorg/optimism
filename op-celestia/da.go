package celestia

import (
	"encoding/binary"
	"github.com/celestiaorg/celestia-openrpc/types/blob"
)

// heightLen is a length (in bytes) of serialized height.
//
// This is 8 as uint64 consist of 8 bytes.
const heightLen = 8

type ID []byte

func MakeID(height uint64, commitment blob.Commitment) ID {
	id := make([]byte, heightLen+len(commitment))
	binary.LittleEndian.PutUint64(id, height)
	copy(id[heightLen:], commitment)
	return id
}

func SplitID(id ID) (uint64, blob.Commitment) {
	if len(id) <= heightLen {
		return 0, nil
	}
	commitment := blob.Commitment(id[heightLen:])
	return binary.LittleEndian.Uint64(id[:heightLen]), commitment
}

// DerivationVersionCelestia is a byte marker for celestia references submitted
// to the batch inbox address as calldata.
// Mnemonic 0xce = celestia
// version 0xce references are encoded as:
// [8]byte block height ++ [32]byte commitment
// in little-endian encoding.
// see: https://github.com/rollkit/celestia-da/blob/1f2df375fd2fcc59e425a50f7eb950daa5382ef0/celestia.go#L141-L160
const DerivationVersionCelestia = 0xce
