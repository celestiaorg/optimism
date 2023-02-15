package testutils

import (
	"context"

	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type MockL1Source struct {
	MockEthClient
}

func (m *MockL1Source) L1BlockRefByLabel(ctx context.Context, label eth.BlockLabel) (eth.L1BlockRef, error) {
	out := m.Mock.MethodCalled("L1BlockRefByLabel", label)
	return out[0].(eth.L1BlockRef), *out[1].(*error)
}

func (m *MockL1Source) ExpectL1BlockRefByLabel(label eth.BlockLabel, ref eth.L1BlockRef, err error) {
	m.Mock.On("L1BlockRefByLabel", label).Once().Return(ref, &err)
}

func (m *MockL1Source) L1BlockRefByNumber(ctx context.Context, num uint64) (eth.L1BlockRef, error) {
	out := m.Mock.MethodCalled("L1BlockRefByNumber", num)
	return out[0].(eth.L1BlockRef), *out[1].(*error)
}

func (m *MockL1Source) ExpectL1BlockRefByNumber(num uint64, ref eth.L1BlockRef, err error) {
	m.Mock.On("L1BlockRefByNumber", num).Once().Return(ref, &err)
}

func (m *MockL1Source) L1BlockRefByHash(ctx context.Context, hash common.Hash) (eth.L1BlockRef, error) {
	out := m.Mock.MethodCalled("L1BlockRefByHash", hash)
	return out[0].(eth.L1BlockRef), *out[1].(*error)
}

func (m *MockL1Source) ExpectL1BlockRefByHash(hash common.Hash, ref eth.L1BlockRef, err error) {
	m.Mock.On("L1BlockRefByHash", hash).Once().Return(ref, &err)
}

func (m *MockL1Source) TxsByNumber(ctx context.Context, number uint64) (types.Transactions, error) {
	out := m.Mock.MethodCalled("TxsByNumber", number)
	return out[0].(types.Transactions), *out[1].(*error)
}

func (m *MockL1Source) ExpectTxsByNumber(number uint64, txs types.Transactions, err error) {
	m.Mock.On("TxsByNumber", number).Once().Return(txs, &err)
}
