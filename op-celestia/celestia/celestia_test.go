package celestia

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeFrameRef(t *testing.T) {
	tests := []struct {
		name string
		frameRefHex string
		frameRef FrameRef
	}{
		{
		"valid frame reference",
		"d20400000000000068656c6c6f20776f726c64", // 1234 + "hello world"
		FrameRef{Height: 1234, Commitment: []byte{0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x20, 0x77, 0x6f, 0x72, 0x6c, 0x64}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frameRef, err := hex.DecodeString(tt.frameRefHex)
			require.NoError(t, err)
			gotFrameRef := DecodeFrameRef(frameRef)
			require.Equal(t, tt.frameRef, gotFrameRef)
			gotFrameRefHex := hex.EncodeToString(EncodeFrameRef(tt.frameRef))
			require.Equal(t, tt.frameRefHex, gotFrameRefHex)
		})
	}
}
