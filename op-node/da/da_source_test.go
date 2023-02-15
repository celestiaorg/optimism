package da_test

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethereum-optimism/optimism/op-node/testlog"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-node/client"
	"github.com/ethereum-optimism/optimism/op-node/da"
	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/sources"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"

	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/testutils"
)

type testTx struct {
	to      *common.Address
	dataLen int
	author  *ecdsa.PrivateKey
	good    bool
	value   int
}

func (tx *testTx) Create(t *testing.T, signer types.Signer, rng *rand.Rand) *types.Transaction {
	t.Helper()
	out, err := types.SignNewTx(tx.author, signer, &types.DynamicFeeTx{
		ChainID:   signer.ChainID(),
		Nonce:     0,
		GasTipCap: big.NewInt(2 * params.GWei),
		GasFeeCap: big.NewInt(30 * params.GWei),
		Gas:       100_000,
		To:        tx.to,
		Value:     big.NewInt(int64(tx.value)),
		Data:      testutils.RandomData(rng, tx.dataLen),
	})
	require.NoError(t, err)
	return out
}

func randHash() (out common.Hash) {
	rand.Read(out[:])
	return out
}

func randBlock(t *testing.T) (*types.Header, *sources.RpcBlock) {
	hdr := &types.Header{
		ParentHash:  randHash(),
		UncleHash:   randHash(),
		Coinbase:    common.Address{},
		Root:        randHash(),
		TxHash:      randHash(),
		ReceiptHash: randHash(),
		Bloom:       types.Bloom{},
		Difficulty:  big.NewInt(42),
		Number:      big.NewInt(1234),
		GasLimit:    0,
		GasUsed:     0,
		Time:        123456,
		Extra:       make([]byte, 0),
		MixDigest:   randHash(),
		Nonce:       types.BlockNonce{},
		BaseFee:     big.NewInt(100),
	}

	block := &sources.RpcBlock{}
	return hdr, block
}

type mockRPC struct {
	mock.Mock
}

func (m *mockRPC) BatchCallContext(ctx context.Context, b []rpc.BatchElem) error {
	return m.MethodCalled("BatchCallContext", ctx, b).Get(0).([]error)[0]
}

func (m *mockRPC) CallContext(ctx context.Context, result any, method string, args ...any) error {
	called := m.MethodCalled("CallContext", ctx, result, method, args)
	err, ok := called.Get(0).(*error)
	if !ok {
		return nil
	}
	return *err
}

func (m *mockRPC) ExpectCallContext(ctx context.Context, t *testing.T, err error, block *sources.RpcBlock, args ...any) {
	_, n := randBlock(t)
	m.On("CallContext", ctx, new(*sources.RpcBlock),
		"eth_getBlockByNumber", []any{n.Number.String(), true}).Run(func(args mock.Arguments) {
		*args[1].(**sources.RpcBlock) = block
	}).Return([]error{nil})
}

func (m *mockRPC) EthSubscribe(ctx context.Context, channel any, args ...any) (ethereum.Subscription, error) {
	called := m.MethodCalled("EthSubscribe", channel, args)
	return called.Get(0).(*rpc.ClientSubscription), called.Get(1).([]error)[0]
}

func (m *mockRPC) Close() {
	m.MethodCalled("Close")
}

var _ client.RPC = (*mockRPC)(nil)

var testEthClientConfig = &sources.EthClientConfig{
	ReceiptsCacheSize:     10,
	TransactionsCacheSize: 10,
	HeadersCacheSize:      10,
	PayloadsCacheSize:     10,
	MaxRequestsPerBatch:   20,
	MaxConcurrentRequests: 10,
	TrustRPC:              false,
	MustBePostMerge:       false,
	RPCProviderKind:       sources.RPCKindBasic,
}

type calldataTest struct {
	name string
	txs  []testTx
}

// TestDataFromDASource creates some transactions from a specified template and asserts
// that DataFromEVMTransactions properly filters and returns the data from the authorized transactions
// inside the transaction set.
func TestDataFromDASource(t *testing.T) {
	inboxPriv := testutils.RandomKey()
	batcherPriv := testutils.RandomKey()
	rng := rand.New(rand.NewSource(1234))
	cfg := &rollup.Config{
		L1ChainID:         big.NewInt(100),
		BatchInboxAddress: crypto.PubkeyToAddress(inboxPriv.PublicKey),
	}

	batcherAddr := crypto.PubkeyToAddress(batcherPriv.PublicKey)

	altInbox := testutils.RandomAddress(rand.New(rand.NewSource(1234)))
	altAuthor := testutils.RandomKey()

	testCases := []calldataTest{
		{
			name: "simple",
			txs:  []testTx{{to: &cfg.BatchInboxAddress, dataLen: 1, author: batcherPriv, good: true}},
		},
		{
			name: "other inbox",
			txs:  []testTx{{to: &altInbox, dataLen: 1, author: batcherPriv, good: false}}},
		{
			name: "other author",
			txs:  []testTx{{to: &cfg.BatchInboxAddress, dataLen: 1234, author: altAuthor, good: false}},
		},
		{
			name: "inbox is author",
			txs:  []testTx{{to: &cfg.BatchInboxAddress, dataLen: 1234, author: inboxPriv, good: false}}},
		{
			name: "author is inbox",
			txs:  []testTx{{to: &batcherAddr, dataLen: 1234, author: batcherPriv, good: false}}},
		{
			name: "unrelated",
			txs:  []testTx{{to: &altInbox, dataLen: 1234, author: altAuthor, good: false}}},
		{
			name: "contract creation",
			txs:  []testTx{{to: nil, dataLen: 1234, author: batcherPriv, good: false}}},
		{
			name: "empty tx",
			txs:  []testTx{{to: &cfg.BatchInboxAddress, dataLen: 0, author: batcherPriv, good: true}}},
		{
			name: "value tx",
			txs:  []testTx{{to: &cfg.BatchInboxAddress, dataLen: 1234, value: 42, author: batcherPriv, good: true}}},
		{
			name: "empty block", txs: []testTx{},
		},
		{
			name: "mixed txs",
			txs: []testTx{
				{to: &cfg.BatchInboxAddress, dataLen: 1234, value: 42, author: batcherPriv, good: true},
				{to: &cfg.BatchInboxAddress, dataLen: 1234, value: 32, author: altAuthor, good: false},
				{to: &cfg.BatchInboxAddress, dataLen: 1234, value: 22, author: batcherPriv, good: true},
				{to: &altInbox, dataLen: 2020, value: 1234, author: batcherPriv, good: false},
			},
		},
	}

	for _, tc := range testCases {
		signer := cfg.L1Signer()
		ctx := context.Background()
		var txs []*types.Transaction
		_, block := randBlock(t)
		var expectedData []da.Frame
		cfg.Rpc = &mockRPC{}
		blockId := eth.BlockID{
			Hash:   testutils.RandomHash(rng),
			Number: uint64(block.Number),
		}
		for i, tx := range tc.txs {
			txs = append(txs, tx.Create(t, signer, rng))
			if tx.good {
				expectedData = append(expectedData, da.Frame{Data: txs[i].Data(), Ref: &da.FrameRef{Number: uint64(block.Number), Index: uint64(len(block.Transactions))}})
				block.Transactions = append(block.Transactions, txs[i])
			}
		}
		cfg.Rpc.(*mockRPC).ExpectCallContext(
			ctx,
			t,
			nil,
			block,
			[]any{
				nil,
				mock.Anything,
				"eth_getBlockByNumber",
				[]any{
					block.Number,
					true,
				},
			}...,
		)
		dab, _ := sources.NewEthereumDA(testlog.Logger(t, log.LvlCrit), cfg, nil)
		out := da.DataFromDASource(ctx, blockId, dab, testlog.Logger(t, log.LvlCrit))
		require.ElementsMatch(t, expectedData, out)
	}

}

func TestEthClient_InfoAndTxsByNumber(t *testing.T) {
	m := new(mockRPC)
	_, block := randBlock(t)
	expectedInfo, expectedTxs, _ := block.Info(true, false)
	n := block.Number
	ctx := context.Background()
	m.On("CallContext", ctx, new(*sources.RpcBlock),
		"eth_getBlockByNumber", []any{n.String(), true}).Run(func(args mock.Arguments) {
		*args[1].(**sources.RpcBlock) = block
	}).Return([]error{nil})
	s, err := sources.NewL1Client(m, nil, nil, sources.L1ClientDefaultConfig(&rollup.Config{SeqWindowSize: 10}, true, sources.RPCKindBasic))
	require.NoError(t, err)
	info, txs, err := s.InfoAndTxsByNumber(ctx, uint64(n))
	require.Equal(t, txs, expectedTxs)
	require.NoError(t, err)
	require.Equal(t, info, expectedInfo)
	m.Mock.AssertExpectations(t)
}
