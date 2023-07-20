package celestia

import (
	"encoding/binary"
)

// FrameRef contains the reference to the specific frame on celestia.
type FrameRef struct {
	Height uint64
	Commitment []byte
}

// DecodeFrameRef returns the celestia inclusion height and commitment from the
// decoded frame reference.
//
// serialization format: height + commitment
//  ----------------------------------------
// | 8 byte uint64  |  32 byte commitment   |
//  ----------------------------------------
// | <-- height --> | <-- commitment -->    |
//  ----------------------------------------
func DecodeFrameRef(ref []byte) FrameRef {
  height := binary.LittleEndian.Uint64(ref[:8])
  return FrameRef{height, ref[8:]}
}

// EncodeFrameRef returns the serialized frame reference from the celestia
// inclusion height and commitment.
//
// serialization format: height + commitment
//  ----------------------------------------
// | 8 byte uint64  |  32 byte commitment   |
//  ----------------------------------------
// | <-- height --> | <-- commitment -->    |
//  ----------------------------------------
func EncodeFrameRef(frame FrameRef) []byte {
	serialized := make([]byte, 8 + len(frame.Commitment))

	binary.LittleEndian.PutUint64(serialized, frame.Height)
	copy(serialized[8:], frame.Commitment)

  	return serialized
}
