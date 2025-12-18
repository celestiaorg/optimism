package e2e

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	// Ethereum
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"

	// Celestia
	txClient "github.com/celestiaorg/celestia-node/api/client"
	celblob "github.com/celestiaorg/celestia-node/blob"
	"github.com/celestiaorg/celestia-node/nodebuilder/p2p"
	libshare "github.com/celestiaorg/go-square/v2/share"
	celestia "github.com/ethereum-optimism/optimism/op-celestia"

	// Cosmos keyring (needs codec)
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"

	// Indexer
	"github.com/ethereum-optimism/optimism/op-celestia/indexer"
	indexerrpc "github.com/ethereum-optimism/optimism/op-celestia/indexer/rpc"
	"github.com/ethereum-optimism/optimism/op-celestia/indexer/store"
	"github.com/ethereum-optimism/optimism/op-celestia/metrics"
)

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

var (
	// Ethereum devnet defaults
	l1RPC = env("E2E_L1_RPC", "http://127.0.0.1:8545")

	// Celestia devnet defaults
	celestiaRPC  = env("E2E_CELESTIA_RPC", "http://127.0.0.1:26658")
	celestiaGRPC = env("E2E_CELESTIA_GRPC", "127.0.0.1:9090")

	// Default prefunded L1 devnet account:
	// address: 0x8943545177806ED17B9F23F0a21ee5948eCaa776
	// path:    m/44'/60'/0'/0/0
	l1PrivKeyHex = env(
		"E2E_L1_PRIVKEY",
		"bcdf20249abf0ed6d944c0288fad489e33f66b3960d9e6229c1cd214ed3bbe31",
	)

	// Batch inbox address (defaults to the same devnet funded account)
	batchInboxAddr = common.HexToAddress(env(
		"E2E_BATCH_INBOX",
		"0x8943545177806ED17B9F23F0a21ee5948eCaa776",
	))

	// Prefunded Celestia dev account (from your docker-compose):
	celKeyName    = env("E2E_CELESTIA_KEYNAME", "dev")
	celPrivKeyHex = env("E2E_CELESTIA_PRIVKEY", "86877e42c2d145b694e12e1f1bea7c837113737a4dd52e0ea7e900251d51bfe9")

	// You said --rpc.skip-auth so defaults are empty/false
	celBridgeAuth = env("E2E_CELESTIA_AUTH", "")
	celP2PNetwork = env("E2E_CELESTIA_P2P_NETWORK", "private")

	celCoreAuth  = env("E2E_CELESTIA_CORE_AUTH", "")
	celCoreTLSEn = env("E2E_CELESTIA_CORE_TLS", "false") // "true" to enable
)

func TestIndexer_E2E_CalldataAndCelestia(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	preflightOrFail(t)

	logger := log.New()

	// ---- L1 ----
	l1raw, err := ethclient.DialContext(ctx, l1RPC)
	if err != nil {
		t.Fatalf("dial L1 (%s): %s", l1RPC, hintRPCDial(err, "L1 execution (geth)", l1RPC))
	}
	l1 := &testL1Client{Client: l1raw}

	head, err := l1.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatalf("get L1 head: %s", hintRPCCall(err, "L1 execution (geth)", l1RPC, "eth_getBlockByNumber(latest)"))
	}

	// ---- Celestia DA client ----
	nsBytes := mustNamespaceBytes(t)
	daClient := mustNewDAClientWithImportedKey(t, ctx, nsBytes)

	// ---- Indexer driver (NO op-node, NO L2 client) ----
	memStore := store.NewMemoryStore()

	cfg := indexer.IndexerConfig{
		StartL1Block:      head.Number.Uint64(),
		BatchInboxAddress: batchInboxAddr,
		L1EthRpc:          l1RPC,
		L2EthRpc:          "", // not used
		PollInterval:      200 * time.Millisecond,
		NetworkTimeout:    5 * time.Second,
		VerifyParentCheck: false, // critical: no L2 checks
		L2BlockTime:       2,
		L2GenesisTime:     0,
		ChainID:           big.NewInt(900),
	}

	driver := indexer.NewIndexerDriver(indexer.DriverSetup{
		Log:            logger,
		Metr:           metrics.NoopMetrics,
		Cfg:            cfg,
		L1Client:       l1,
		L2Client:       nil, // IMPORTANT: we do not require L2 services
		OpNodeClient:   nil,
		CelestiaClient: daClient,
		Store:          memStore,
	})

	// ---- Start real RPC server with real API types ----
	indexerRPC, stopRPC := mustStartIndexerRPC(t, logger, driver)
	defer stopRPC()

	if err := driver.Start(); err != nil {
		t.Fatalf("start indexer driver: %v", err)
	}
	defer func() { _ = driver.Stop() }()

	// ---- Build a real derivation payload: DerivationVersion0 ++ Frame(channel=zlib(span-batch)) ----
	// We use a *span batch* (batch_version=1) so the driver can derive L2 block numbers from timestamp without L2 RPC.
	fixtureTxData := mustBuildOPFixtureTxData(t)

	// 1) ETH calldata DA tx
	calldataTx := mustSendInboxTx(t, ctx, l1raw, fixtureTxData)
	calldataReceipt := mustWaitReceipt(t, ctx, l1raw, calldataTx, "calldata batch tx")
	t.Logf("[hint] calldata tx included in L1 block %d (%s)", calldataReceipt.BlockNumber.Uint64(), calldataTx.Hex())

	// 2) Celestia PFB + L1 Alt-DA commitment tx
	height, commitment := mustSubmitCelestiaBlob(t, ctx, daClient, fixtureTxData)
	t.Logf("[hint] submitted celestia blob at height=%d commitment=%s", height, base64.StdEncoding.EncodeToString(commitment))

	altTx := mustSendAltDACommitmentTx(t, ctx, l1raw, height, commitment)
	altReceipt := mustWaitReceipt(t, ctx, l1raw, altTx, "alt-da commitment tx")
	t.Logf("[hint] alt-da commitment tx included in L1 block %d (%s)", altReceipt.BlockNumber.Uint64(), altTx.Hex())

	// ---- Wait until indexer has stored at least one location ----
	waitIndexedOrHint(t, memStore)

	// ---- Query via RPC using the real shape ----
	// Our fixture encodes a span batch starting at L2 block 1.
	loc := mustWaitDALocation(t, indexerRPC, 1)

	// Validate keys depending on backend
	switch loc.Type {
	case "ethereum":
		mustHaveKeys(t, loc.Data, "tx_hash", "l2_range", "l1_block")
	case "celestia":
		mustHaveKeys(t, loc.Data, "height", "commitment", "l2_range", "l1_block")
	default:
		t.Fatalf("unknown DA type %q", loc.Type)
	}

	// Also validate status endpoint exists (method name per rpc/api.go)
	st := mustGetIndexerStatus(t, indexerRPC)
	if st.IndexedBlocks <= 0 {
		t.Fatalf("expected indexed_blocks > 0, got %+v", st)
	}
}

//
// -------------------- Build a minimal OP fixture --------------------
//

// mustBuildOPFixtureTxData builds:
// data = 0x00 (DerivationVersion0) ++ Frame(channel=zlib(span-batch))
func mustBuildOPFixtureTxData(t *testing.T) []byte {
	t.Helper()

	// Build a single span-batch that implies:
	// - first L2 timestamp = rel_timestamp + genesis_time = 0
	// - L2 block time = 2
	// => starting block number computed by driver: (0 - 0)/2 = 0, then +1 => L2 block 1
	spanBatch := buildSpanBatchOneEmptyBlock()

	// Channel payload is the batch stream; compress it (BatchReader(..., compressed=true) expects zlib)
	var ch bytes.Buffer
	zw := zlib.NewWriter(&ch)
	_, _ = zw.Write(spanBatch)
	_ = zw.Close()
	channelCompressed := ch.Bytes()

	// Wrap into a frame
	frame := derive.Frame{
		ID: derive.ChannelID{0x01}, // deterministic
		// Data is the channel data portion. (frame format stays the same for span batches)
		Data: channelCompressed,
	}
	var fb bytes.Buffer
	if err := frame.MarshalBinary(&fb); err != nil {
		t.Fatalf("marshal frame: %v", err)
	}

	// DerivationVersion0 prefix
	out := append([]byte{0x00}, fb.Bytes()...)
	// Sanity: ParseFrames should succeed on what we built.
	if _, err := derive.ParseFrames(out); err != nil {
		t.Fatalf("fixture ParseFrames sanity failed: %v", err)
	}
	return out
}

// buildSpanBatchOneEmptyBlock builds a batch_version=1 span batch per spec.
// Encoding: 0x01 ++ prefix ++ payload
//
// prefix = rel_timestamp(uvarint=0) ++ l1_origin_num(uvarint=0) ++ parent_check(20B=0) ++ l1_origin_check(20B=0)
// payload = block_count(uvarint=1) ++ origin_bits(1 bit => 1 byte 0x00) ++ block_tx_counts(uvarint(0)) ++ txs(empty)
func buildSpanBatchOneEmptyBlock() []byte {
	var b bytes.Buffer

	// batch_version = 1 (SpanBatchType)
	b.WriteByte(0x01)

	// prefix
	b.Write(uvarint(0)) // rel_timestamp
	b.Write(uvarint(0)) // l1_origin_num
	b.Write(make([]byte, 20))
	b.Write(make([]byte, 20))

	// payload
	b.Write(uvarint(1))   // block_count
	b.Write([]byte{0x00}) // origin_bits: 1 bit padded to 1 byte (false)
	b.Write(uvarint(0))   // block_tx_counts[0] = 0
	// txs section is empty because sum(block_tx_counts)=0

	return b.Bytes()
}

// uvarint encodes unsigned base-128 varint (protobuf-style).
func uvarint(x uint64) []byte {
	var out [10]byte
	n := binary.PutUvarint(out[:], x)
	return out[:n]
}

//
// -------------------- RPC helpers (real API shapes) --------------------
//

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonRPCResponse[T any] struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Result  T             `json:"result"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type daResp struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

type indexerStatus struct {
	LastIndexedBlock uint64 `json:"last_indexed_block"`
	IndexedBlocks    int    `json:"indexed_blocks"`
	Running          bool   `json:"running"`
	L2StartBlock     uint64 `json:"l2_start_block"`
	L2EndBlock       uint64 `json:"l2_end_block"`
}

func mustStartIndexerRPC(t *testing.T, lg log.Logger, driver *indexer.IndexerDriver) (string, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for rpc: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	_ = ln.Close()

	srv := rpc.NewServer(
		"127.0.0.1",
		addr.Port,
		"e2e",
		rpc.WithLogger(lg),
	)
	adapter := &driverAdapter{driver: driver}
	api := indexerrpc.NewIndexerAPI(adapter, lg)
	srv.AddAPI(indexerrpc.GetAPI(api))

	if err := srv.Start(); err != nil {
		t.Fatalf("start rpc server: %v", err)
	}

	endpoint := "http://" + srv.Endpoint()
	stop := func() {
		_ = srv.Stop()
	}
	return endpoint, stop
}

type driverAdapter struct {
	driver *indexer.IndexerDriver
}

func (da *driverAdapter) GetDALocation(l2BlockNum uint64) (store.DALocation, error) {
	return da.driver.GetDALocation(l2BlockNum)
}

func (da *driverAdapter) GetStatus() (lastIndexedBlock uint64, indexedBlocks int, running bool, l2Start uint64, l2End uint64, err error) {
	return da.driver.GetStatus()
}

func mustGetIndexerStatus(t *testing.T, url string) indexerStatus {
	t.Helper()

	body := `{"jsonrpc":"2.0","method":"admin_getIndexerStatus","params":[],"id":1}`
	r, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("rpc call (%s): %s", url, hintRPCDial(err, "indexer RPC", url))
	}
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var out jsonRPCResponse[indexerStatus]
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("decode indexer status response (shape drift?): %v", err)
	}
	if out.Error != nil {
		t.Fatalf("indexer status RPC returned error: code=%d message=%q", out.Error.Code, out.Error.Message)
	}
	return out.Result
}

func mustGetDALocation(t *testing.T, url string, l2 uint64) (daResp, *jsonRPCError) {
	t.Helper()

	body := fmt.Sprintf(`{"jsonrpc":"2.0","method":"admin_getDALocation","params":[%d],"id":1}`, l2)
	r, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("rpc call (%s): %s", url, hintRPCDial(err, "indexer RPC", url))
	}
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var out jsonRPCResponse[daResp]
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("decode indexer RPC response (shape drift?): %v", err)
	}
	return out.Result, out.Error
}

func mustWaitDALocation(t *testing.T, url string, l2 uint64) daResp {
	t.Helper()

	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline.C:
			st := mustGetIndexerStatus(t, url)
			t.Fatalf("timed out waiting for admin_getDALocation(%d) to exist.\nstatus=%+v\nHints:\n- Did the indexer parse the fixture frames/batch successfully?\n- Check geth logs and indexer logs for decode errors.\n- Confirm inbox address matches tx to=%s",
				l2, st, batchInboxAddr.Hex(),
			)
		case <-tick.C:
			res, rpcErr := mustGetDALocation(t, url, l2)
			if rpcErr == nil {
				return res
			}
			// keep polling until location exists
		}
	}
}

//
// -------------------- ETH + Celestia helpers --------------------
//

type testL1Client struct {
	*ethclient.Client
}

func (c *testL1Client) GetBlobs(ctx context.Context, ref opeth.L1BlockRef, hashes []opeth.IndexedBlobHash) ([]*opeth.Blob, error) {
	return nil, fmt.Errorf("blob DA not enabled in this e2e test")
}

func mustSendInboxTx(t *testing.T, ctx context.Context, l1 *ethclient.Client, data []byte) common.Hash {
	t.Helper()

	priv, err := crypto.HexToECDSA(strings.TrimPrefix(l1PrivKeyHex, "0x"))
	if err != nil {
		t.Fatalf("parse L1 privkey: %v", err)
	}
	from := crypto.PubkeyToAddress(priv.PublicKey)

	nonce, err := l1.PendingNonceAt(ctx, from)
	if err != nil {
		t.Fatalf("get nonce: %s", hintRPCCall(err, "L1 execution (geth)", l1RPC, "eth_getTransactionCount(pending)"))
	}
	gasPrice, err := l1.SuggestGasPrice(ctx)
	if err != nil {
		t.Fatalf("suggest gas price: %s", hintRPCCall(err, "L1 execution (geth)", l1RPC, "eth_gasPrice"))
	}
	chainID, err := l1.ChainID(ctx)
	if err != nil {
		t.Fatalf("chain id: %s", hintRPCCall(err, "L1 execution (geth)", l1RPC, "eth_chainId"))
	}

	tx := types.NewTransaction(
		nonce,
		batchInboxAddr,
		big.NewInt(0),
		3_000_000,
		gasPrice,
		data,
	)

	signed, err := types.SignTx(tx, types.NewEIP155Signer(chainID), priv)
	if err != nil {
		t.Fatalf("sign tx: %v", err)
	}
	if err := l1.SendTransaction(ctx, signed); err != nil {
		t.Fatalf("send tx: %s", hintRPCCall(err, "L1 execution (geth)", l1RPC, "eth_sendRawTransaction"))
	}
	return signed.Hash()
}

func mustSendAltDACommitmentTx(t *testing.T, ctx context.Context, l1 *ethclient.Client, height uint64, commitment []byte) common.Hash {
	t.Helper()

	// OP alt-da commitment calldata format expected by indexer:
	// version=0x01, commitment_type=0x01, da_layer=0x0c, payload=[height(8 LE) || commitment(32)]
	id := celestia.MakeID(height, commitment)
	data := append([]byte{0x01, 0x01, 0x0c}, id...)
	return mustSendInboxTx(t, ctx, l1, data)
}

func mustSubmitCelestiaBlob(t *testing.T, ctx context.Context, da *celestia.DAClient, data []byte) (uint64, []byte) {
	t.Helper()

	ns, err := libshare.NewNamespaceFromBytes(da.Namespace)
	if err != nil {
		t.Fatalf("parse namespace: %v", err)
	}

	blob, err := celblob.NewBlobV0(ns, data)
	if err != nil {
		t.Fatalf("new blob: %v", err)
	}

	height, err := da.Client.Submit(ctx, []*celblob.Blob{blob}, nil)
	if err != nil {
		t.Fatalf("submit celestia blob (is celestia-app + bridge up?): %v", err)
	}
	return height, blob.Commitment
}

func mustWaitReceipt(t *testing.T, ctx context.Context, l1 *ethclient.Client, tx common.Hash, what string) *types.Receipt {
	t.Helper()

	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()

	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline.C:
			t.Fatalf("%s not mined within 30s. Hints:\n- Is geth mining / producing blocks?\n- Is txpool accepting unprotected txs? (your geth has --rpc.allow-unprotected-txs)\n- Check geth logs for 'mined block' or tx errors.\n- You can inspect with: curl -s %s -H 'content-type: application/json' --data '{\"jsonrpc\":\"2.0\",\"method\":\"eth_getTransactionReceipt\",\"params\":[\"%s\"],\"id\":1}'",
				what, l1RPC, tx.Hex(),
			)
		case <-tick.C:
			rcpt, err := l1.TransactionReceipt(ctx, tx)
			if err == nil && rcpt != nil {
				return rcpt
			}
			// ignore not-found
		}
	}
}

func waitIndexedOrHint(t *testing.T, st store.Store) {
	t.Helper()

	deadline := time.After(60 * time.Second)
	for {
		select {
		case <-deadline:
			last, _ := st.GetLastIndexedBlock()
			cnt, _ := st.GetIndexedBlockCount()
			t.Fatalf("indexer did not store any locations in time.\nCurrent store status: indexed_blocks=%d last_indexed_block=%d\nHints:\n- Ensure geth is producing blocks (mining/automine).\n- Ensure the batch inbox address matches the tx 'to' address: %s\n- Check indexer logs for batch/frame decode errors.",
				cnt, last, batchInboxAddr.Hex(),
			)
		default:
			n, _ := st.GetIndexedBlockCount()
			if n > 0 {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func mustHaveKeys(t *testing.T, m map[string]interface{}, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			b, _ := json.Marshal(m)
			t.Fatalf("RPC shape drift? missing key %q in data: %s", k, string(b))
		}
	}
}

func mustNamespaceBytes(t *testing.T) []byte {
	t.Helper()

	// Celestia "version 0" blob namespace rules:
	// 0x00 || 18*0x00 || 10*0x01
	ns := make([]byte, 29)
	ns[0] = 0x00
	copy(ns[19:], bytes.Repeat([]byte{0x01}, 10))
	return ns
}

func mustNewDAClientWithImportedKey(t *testing.T, ctx context.Context, nsBytes []byte) *celestia.DAClient {
	t.Helper()

	if _, err := libshare.NewNamespaceFromBytes(nsBytes); err != nil {
		t.Fatalf("invalid namespace: %v", err)
	}

	krDir := filepath.Join(t.TempDir(), "cel-keyring")

	ir := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(ir)
	cdc := codec.NewProtoCodec(ir)

	kr, err := keyring.New(
		"celestia-e2e",
		keyring.BackendTest,
		krDir,
		strings.NewReader(""),
		cdc,
	)
	if err != nil {
		t.Fatalf("create keyring: %v", err)
	}

	if err := kr.ImportPrivKeyHex(celKeyName, celPrivKeyHex, "secp256k1"); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "exists") {
			t.Fatalf("import celestia privkey: %v", err)
		}
	}

	cfg := txClient.Config{
		ReadConfig: txClient.ReadConfig{
			BridgeDAAddr: celestiaRPC,
			DAAuthToken:  celBridgeAuth,
			EnableDATLS:  false,
		},
		SubmitConfig: txClient.SubmitConfig{
			DefaultKeyName: celKeyName,
			Network:        p2p.Network(celP2PNetwork),
			CoreGRPCConfig: txClient.CoreGRPCConfig{
				Addr:       celestiaGRPC,
				TLSEnabled: strings.EqualFold(celCoreTLSEn, "true"),
				AuthToken:  celCoreAuth,
			},
		},
	}

	cli, err := txClient.New(ctx, cfg, kr)
	if err != nil {
		t.Fatalf("create celestia tx client: %v", err)
	}

	return &celestia.DAClient{
		Client:        cli.Blob,
		GetTimeout:    time.Minute,
		SubmitTimeout: time.Minute,
		Namespace:     nsBytes,
		FallbackMode:  "",
		GasPrice:      0,
	}
}

//
// -------------------- Preflight + friendly hints --------------------
//

func preflightOrFail(t *testing.T) {
	t.Helper()

	// --- L1: must be Ethereum JSON-RPC ---
	if ok, msg := tcpReachable(l1RPC); !ok {
		t.Fatalf(
			"L1 execution (geth) not reachable at %s\nerror: %s",
			l1RPC, msg,
		)
	}

	// Try a real eth RPC call
	client := &http.Client{Timeout: 1 * time.Second}
	req := `{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}`
	resp, err := client.Post(l1RPC, "application/json", strings.NewReader(req))
	if err != nil || resp.StatusCode/100 != 2 {
		t.Fatalf("L1 RPC reachable but not responding correctly at %s", l1RPC)
	}
	_ = resp.Body.Close()

	// --- Celestia bridge: TCP ONLY ---
	if ok, msg := tcpReachable(celestiaRPC); !ok {
		t.Fatalf(
			"Celestia bridge not reachable at %s\nerror: %s",
			celestiaRPC, msg,
		)
	}
}

func tcpReachable(rawURL string) (bool, string) {
	hostport, err := hostPortFromURL(rawURL)
	if err != nil {
		return false, err.Error()
	}
	d := net.Dialer{Timeout: 800 * time.Millisecond}
	c, err := d.Dial("tcp", hostport)
	if err != nil {
		return false, err.Error()
	}
	_ = c.Close()
	return true, "ok"
}

func hostPortFromURL(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "https://")
	if i := strings.IndexByte(u, '/'); i >= 0 {
		u = u[:i]
	}
	if u == "" {
		return "", errors.New("empty host")
	}
	if !strings.Contains(u, ":") {
		return u + ":80", nil
	}
	return u, nil
}

func hintRPCDial(err error, who, url string) string {
	if err == nil {
		return ""
	}
	var ne *net.OpError
	if errors.As(err, &ne) {
		return fmt.Sprintf("%v\nHints:\n- %s not reachable at %s\n- Is the devnet up and ports mapped?\n", err, who, url)
	}
	return err.Error()
}

func hintRPCCall(err error, who, url, method string) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v\nHints:\n- RPC call failed: %s @ %s (%s)\n  curl -s %s -H 'content-type: application/json' --data '{\"jsonrpc\":\"2.0\",\"method\":\"%s\",\"params\":[],\"id\":1}'\n",
		err, who, url, method, url, method,
	)
}

var _ = io.EOF
