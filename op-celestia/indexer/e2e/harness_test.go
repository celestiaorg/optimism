package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	// Ethereum
	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"

	// Celestia (repo wrapper + node tx client)
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
	l1RPC      = env("E2E_L1_RPC", "http://127.0.0.1:8545")
	indexerRPC = env("E2E_INDEXER_RPC", "http://127.0.0.1:57220")

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
	// addr: celestia1hkrz4dw26z69kmrfzjy0r0kjnkh5jkle3376nl
	// priv: 86877e42c2d145b694e12e1f1bea7c837113737a4dd52e0ea7e900251d51bfe9
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

	// Preflight: give actionable hints instead of raw TCP errors.
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

	// ---- Celestia DA (tx client, deterministic key import; no random mnemonic) ----
	nsBytes := mustNamespaceBytes(t)
	daClient := mustNewDAClientWithImportedKey(t, ctx, nsBytes)

	// ---- Indexer ----
	memStore := store.NewMemoryStore()

	cfg := indexer.IndexerConfig{
		StartL1Block:      head.Number.Uint64(),
		BatchInboxAddress: batchInboxAddr,
		L1EthRpc:          l1RPC,
		L2EthRpc:          l1RPC, // unused by this harness (we avoid span batches)
		PollInterval:      200 * time.Millisecond,
		NetworkTimeout:    5 * time.Second,
		VerifyParentCheck: false,
		L2BlockTime:       2,
		L2GenesisTime:     0,
		ChainID:           big.NewInt(900),
	}

	driver := indexer.NewIndexerDriver(indexer.DriverSetup{
		Log:            logger,
		Metr:           metrics.NoopMetrics,
		Cfg:            cfg,
		L1Client:       l1,
		CelestiaClient: daClient,
		Store:          memStore,
	})

	if err := driver.Start(); err != nil {
		t.Fatalf("start indexer driver: %v", err)
	}
	defer func() { _ = driver.Stop() }()

	// ---- Ensure there will be at least one NEW L1 block after start ----
	// The indexer scans blocks [StartL1Block..Head] at startup. If your devnet is not mining
	// automatically, and we don't mine, it may never see any transactions.
	// We send tx(s) and then wait for a *new block* containing them.
	payload := []byte("e2e-rollup-payload")

	// 1) ETH calldata batch tx (to batch inbox)
	calldataTx := mustSendInboxTx(t, ctx, l1raw, append([]byte{0x00}, payload...))

	// Wait until included; this effectively forces "a new block exists" to be indexable.
	calldataReceipt := mustWaitReceipt(t, ctx, l1raw, calldataTx, "calldata batch tx")
	t.Logf("[hint] calldata tx included in L1 block %d (%s)", calldataReceipt.BlockNumber.Uint64(), calldataTx.Hex())

	// 2) Celestia PFB + L1 Alt-DA commitment tx
	height, commitment := mustSubmitCelestiaBlob(t, ctx, daClient, payload)
	t.Logf("[hint] submitted celestia blob at height=%d commitment=%s", height, base64.StdEncoding.EncodeToString(commitment))

	altTx := mustSendAltDACommitmentTx(t, ctx, l1raw, height, commitment)
	altReceipt := mustWaitReceipt(t, ctx, l1raw, altTx, "alt-da commitment tx")
	t.Logf("[hint] alt-da commitment tx included in L1 block %d (%s)", altReceipt.BlockNumber.Uint64(), altTx.Hex())

	// ---- Wait for indexing (with hints on what to check) ----
	waitIndexedOrHint(t, memStore)

	// ---- Validate admin_getDALocation OpenRPC shape strictly ----
	resp := mustGetDALocation(t, indexerRPC, 1)

	switch resp.Type {
	case "ethereum":
		// Depending on which tx the indexer indexed first, this may or may not be calldataTx.
		// We still want shape drift detection, so we validate required keys exist and types are sane.
		mustHaveKeys(t, resp.Data, "tx_hash", "l2_range", "l1_block")
	case "celestia":
		mustHaveKeys(t, resp.Data, "height", "commitment", "l2_range", "l1_block")
		want := base64.StdEncoding.EncodeToString(commitment)
		if resp.Data["commitment"] != want {
			t.Fatalf("unexpected celestia commitment: got=%v want=%s", resp.Data["commitment"], want)
		}
	default:
		t.Fatalf("unknown DA type %q", resp.Type)
	}
}

type testL1Client struct {
	*ethclient.Client
}

func (c *testL1Client) GetBlobs(ctx context.Context, ref opeth.L1BlockRef, hashes []opeth.IndexedBlobHash) ([]*opeth.Blob, error) {
	return nil, fmt.Errorf("blob DA not enabled in this e2e test")
}

type daResp struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

func mustGetDALocation(t *testing.T, url string, l2 uint64) daResp {
	t.Helper()

	body := fmt.Sprintf(`{"jsonrpc":"2.0","method":"admin_getDALocation","params":[%d],"id":1}`, l2)
	r, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("rpc call (%s): %s", url, hintRPCDial(err, "indexer RPC", url))
	}
	defer r.Body.Close()

	var out struct {
		Result daResp `json:"result"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("decode indexer RPC response (shape drift?): %v", err)
	}
	return out.Result
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

	deadline := time.After(40 * time.Second)
	for {
		select {
		case <-deadline:
			last, _ := st.GetLastIndexedBlock()
			cnt, _ := st.GetIndexedBlockCount()
			t.Fatalf("indexer did not index any blocks in time.\nCurrent store status: indexed_blocks=%d last_indexed_block=%d\nHints:\n- Ensure geth is producing blocks (mining/automine). The indexer only discovers data by scanning L1 blocks.\n- Ensure the batch inbox address matches the tx 'to' address: %s\n- Ensure indexerRPC (%s) is reachable if you're also verifying RPC.\n- If your devnet does not mine automatically, the tx receipts will never appear and indexing will never progress.",
				cnt, last, batchInboxAddr.Hex(), indexerRPC,
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
			t.Fatalf("OpenRPC shape drift? missing key %q in data: %s", k, string(b))
		}
	}
}

func mustNamespaceBytes(t *testing.T) []byte {
	t.Helper()

	// Celestia "version 0" blob namespace rules:
	// - total 29 bytes
	// - first byte = version (0)
	// - next 28 bytes = namespace ID
	// - the 28-byte ID must start with 18 leading 0 bytes (blob namespace)
	//
	// Valid example:
	// 0x00 || 18*0x00 || 10*0x01
	ns := make([]byte, 29)
	ns[0] = 0x00
	// ns[1:19] are already zero
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
			"L1 RPC not reachable at %s\nerror: %s\n\nHints:\n- Is geth running?\n- Is port 8545 mapped?\n- Try: curl %s",
			l1RPC, msg, l1RPC,
		)
	}

	// Try a real eth RPC call
	client := &http.Client{Timeout: 1 * time.Second}
	req := `{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}`
	resp, err := client.Post(l1RPC, "application/json", strings.NewReader(req))
	if err != nil || resp.StatusCode/100 != 2 {
		t.Fatalf(
			"L1 RPC reachable but not responding correctly at %s\nHints:\n- Is geth fully started?\n- Check logs for RPC errors",
			l1RPC,
		)
	}
	_ = resp.Body.Close()

	// --- Celestia bridge: TCP ONLY ---
	if ok, msg := tcpReachable(celestiaRPC); !ok {
		t.Fatalf(
			"Celestia bridge not reachable at %s\nerror: %s\n\nHints:\n- Is celestia-node bridge running?\n- Is port 26658 mapped?\n- Docker shows: 0.0.0.0:26658->26658/tcp",
			celestiaRPC, msg,
		)
	}

	// --- Indexer RPC: optional ---
	if ok, msg := tcpReachable(indexerRPC); !ok {
		t.Logf(
			"[hint] indexer RPC not reachable at %s (%s)\n[hint] This is OK if the test starts its own driver.",
			indexerRPC, msg,
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
	// strip path
	if i := strings.IndexByte(u, '/'); i >= 0 {
		u = u[:i]
	}
	if u == "" {
		return "", errors.New("empty host")
	}
	// default port
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
		return fmt.Sprintf("%v\nHints:\n- %s not reachable at %s\n- Is the devnet up and ports mapped?\n- If using docker: check `docker compose ps` and port mappings\n", err, who, url)
	}
	return err.Error()
}

func hintRPCCall(err error, who, url, method string) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v\nHints:\n- RPC call failed: %s @ %s (%s)\n- Verify endpoint responds to JSON-RPC:\n  curl -s %s -H 'content-type: application/json' --data '{\"jsonrpc\":\"2.0\",\"method\":\"%s\",\"params\":[],\"id\":1}'\n",
		err, who, url, method, url, method,
	)
}
