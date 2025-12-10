package rpc_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ethereum-optimism/optimism/op-celestia/indexer/rpc"
	"github.com/ethereum-optimism/optimism/op-celestia/indexer/store"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

/* -----------------------------------------------------------------------
   Fake driver implementing IndexerDriver interface
------------------------------------------------------------------------ */

type fakeDriver struct {
	loc store.DALocation
	err error
}

func (f *fakeDriver) GetDALocation(n uint64) (store.DALocation, error) {
	return f.loc, f.err
}

func (f *fakeDriver) GetStatus() (uint64, int, bool, uint64, uint64, error) {
	return 0, 0, false, 0, 0, nil
}

/* -----------------------------------------------------------------------
   Utility: create a test RPC server exposing IndexerAPI
------------------------------------------------------------------------ */

func newTestServer(t *testing.T, driver rpc.IndexerDriver) *gethrpc.Server {
	srv := gethrpc.NewServer()
	api := rpc.NewIndexerAPI(driver, log.New())
	err := srv.RegisterName("admin", api)
	require.NoError(t, err)
	return srv
}

func callRPC(t *testing.T, srv *gethrpc.Server, method string, params any) json.RawMessage {
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	b, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest("POST", "/", bytes.NewReader(b))
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httpReq)
	require.Equal(t, 200, w.Code)

	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error, "RPC returned an error: %+v", resp.Error)

	return resp.Result
}

/* -----------------------------------------------------------------------
   Test: Celestia DA location
------------------------------------------------------------------------ */

func TestAPI_GetDALocation_Celestia(t *testing.T) {
	loc := &store.CelestiaLocation{
		Commitment: "cm",
		Height:     100,
		L1Block:    12345,
		L2Range:    store.L2Range{Start: 10, End: 20},
	}

	driver := &fakeDriver{loc: loc}
	srv := newTestServer(t, driver)

	res := callRPC(t, srv, "admin_getDALocation", []any{10})

	// Parse into the response struct
	var out rpc.GetDALocationResponse
	err := json.Unmarshal(res, &out)
	require.NoError(t, err)

	require.Equal(t, "celestia", out.Type)

	data, _ := out.Data.(map[string]any)
	require.Equal(t, "cm", data["commitment"])
	require.EqualValues(t, 100, data["height"])
}

/* -----------------------------------------------------------------------
   Test: Ethereum calldata DA location
------------------------------------------------------------------------ */

func TestAPI_GetDALocation_EthCalldata(t *testing.T) {
	loc := &store.EthereumLocation{
		TxHash:     "0xabc",
		L1Block:    333,
		L2Range:    store.L2Range{Start: 50, End: 60},
		BlobHashes: nil, // calldata-backed
	}

	driver := &fakeDriver{loc: loc}
	srv := newTestServer(t, driver)

	res := callRPC(t, srv, "admin_getDALocation", []any{50})

	var out rpc.GetDALocationResponse
	err := json.Unmarshal(res, &out)
	require.NoError(t, err)

	require.Equal(t, "ethereum", out.Type)

	// Extract payload
	var data struct {
		TxHash     string                `json:"tx_hash"`
		L1Block    uint64                `json:"l1_block"`
		BlobHashes []eth.IndexedBlobHash `json:"blob_hashes"`
	}
	b, _ := json.Marshal(out.Data)
	err = json.Unmarshal(b, &data)
	require.NoError(t, err)

	require.Equal(t, "0xabc", data.TxHash)
	require.EqualValues(t, 333, data.L1Block)
	require.Nil(t, data.BlobHashes) // calldata-backed => omitted => nil after unmarshalling
}

/* -----------------------------------------------------------------------
   Test: Ethereum blob-backed DA location
------------------------------------------------------------------------ */

func TestAPI_GetDALocation_EthBlob(t *testing.T) {
	loc := &store.EthereumLocation{
		TxHash:  "0xblob",
		L1Block: 444,
		L2Range: store.L2Range{Start: 77, End: 80},
		BlobHashes: []eth.IndexedBlobHash{
			{Index: 0, Hash: common.HexToHash("0x01")},
			{Index: 1, Hash: common.HexToHash("0x02")},
		},
	}

	driver := &fakeDriver{loc: loc}
	srv := newTestServer(t, driver)

	res := callRPC(t, srv, "admin_getDALocation", []any{77})

	var out rpc.GetDALocationResponse
	err := json.Unmarshal(res, &out)
	require.NoError(t, err)

	require.Equal(t, "ethereum", out.Type)

	// Parse Typed
	var data struct {
		TxHash     string                `json:"tx_hash"`
		L1Block    uint64                `json:"l1_block"`
		BlobHashes []eth.IndexedBlobHash `json:"blob_hashes"`
	}
	b, _ := json.Marshal(out.Data)
	err = json.Unmarshal(b, &data)
	require.NoError(t, err)

	require.Equal(t, "0xblob", data.TxHash)
	require.EqualValues(t, 444, data.L1Block)

	require.Len(t, data.BlobHashes, 2)
	require.Equal(t, uint64(0), data.BlobHashes[0].Index)
	require.Equal(t, common.HexToHash("0x01"), data.BlobHashes[0].Hash)
}
