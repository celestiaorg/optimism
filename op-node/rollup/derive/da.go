package derive

import (
	"os"

	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/log"
)

var daClient *rollup.DAClient

func init() {
	daRpc := os.Getenv("OP_NODE_DA_RPC")
	if daRpc == "" {
		daRpc = "localhost:26650"
	}
	var err error
	daClient, err = rollup.NewDAClient(daRpc)
	if err != nil {
		log.Error("celestia: unable to create DA client", "rpc", daRpc, "err", err)
		panic(err)
	}
}
