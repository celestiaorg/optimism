package sources

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/sources/caching"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"

	"github.com/ethereum-optimism/optimism/op-node/da"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

const networkTimeout = 2 * time.Second // How long a single network request can take. TODO: put in a config somewhere
type SignerFn func(ctx context.Context, rawTx types.TxData) (*types.Transaction, error)

// EthereumDA satisfies DAChain interface
type EthereumDA struct {
	client   *L1Client
	l1Client *ethclient.Client
	cfg      *rollup.Config
	txMgr    txmgr.TxManager
	signerFn SignerFn
	log      log.Logger
}

func (e EthereumDA) TxByBlockNumberAndIndex(ctx context.Context, number uint64, index uint64) (*types.Transaction, error) {
	txs, err := e.TxsByNumber(ctx, number)
	if err != nil {
		return &types.Transaction{}, err
	}
	return txs[index], nil
}

func (e EthereumDA) FetchFrame(ctx context.Context, ref da.FrameRef) (da.Frame, error) {
	// fetching the tx not from API or db - it will just be fetching from the
	// existing types.Transactions by index
	// return types.Transactions[index]
	tx, err := e.TxByBlockNumberAndIndex(ctx, ref.Number, ref.Index)
	to := tx.To()
	if to == nil {
		return da.Frame{}, nil
	}
	if *to != e.cfg.BatchInboxAddress {
		return da.Frame{}, nil
	}
	if err != nil {
		return da.Frame{}, err
	}
	return da.Frame{
		Data: tx.Data(),
		Ref: &da.FrameRef{
			Number:  ref.Number,
			Index:   ref.Index,
			BlockID: eth.BlockID{},
		},
	}, nil
}

func (e EthereumDA) WriteFrame(ctx context.Context, data []byte) (da.FrameRef, error) {
	tx, err := e.CraftTx(ctx, data)
	if err != nil {
		return da.FrameRef{}, fmt.Errorf("failed to create tx: %w", err)
	}
	// Construct a closure that will update the txn with the current gas prices.
	updateGasPrice := func(ctx context.Context) (*types.Transaction, error) {
		return e.UpdateGasPrice(ctx, tx)
	}

	ctx, cancel := context.WithTimeout(ctx, 100*time.Second) // TODO: Select a timeout that makes sense here.
	defer cancel()
	if receipt, err := e.txMgr.Send(ctx, updateGasPrice, e.l1Client.SendTransaction); err != nil {
		e.log.Warn("unable to publish tx", "err", err)
		return da.FrameRef{}, err
	} else {
		e.log.Info("tx successfully published", "tx_hash", receipt.TxHash)
		return da.FrameRef{
			Number: receipt.BlockNumber.Uint64(),
			Index:  uint64(receipt.TransactionIndex),
			BlockID: eth.BlockID{
				Hash:   receipt.BlockHash,
				Number: receipt.BlockNumber.Uint64(),
			},
		}, nil
	}
}

func (e EthereumDA) TxsByNumber(ctx context.Context, number uint64) (types.Transactions, error) {
	// TODO: caching
	_, txs, err := e.client.InfoAndTxsByNumber(ctx, number)
	return txs, err
}

var _ da.DAChain = (&EthereumDA{})

// NewEthereumDA creates a new Ethereum DA
func NewEthereumDA(log log.Logger, cfg *rollup.Config, metrics caching.Metrics) (*EthereumDA, error) {
	//EthClientConfig := L1ClientDefaultConfig(cfg, true, RPCKindBasic)
	client, err := NewL1Client(
		cfg.Rpc, log, metrics, L1ClientDefaultConfig(&rollup.Config{SeqWindowSize: 10}, true, RPCKindBasic),
	)
	if err != nil {
		return nil, err
	}
	return &EthereumDA{
		client: client,
		cfg:    cfg,
	}, nil
}

// calcGasTipAndFeeCap queries L1 to determine what a suitable miner tip & basefee limit would be for timely inclusion
func (e EthereumDA) calcGasTipAndFeeCap(ctx context.Context) (gasTipCap *big.Int, gasFeeCap *big.Int, err error) {
	childCtx, cancel := context.WithTimeout(ctx, networkTimeout)
	gasTipCap, err = e.l1Client.SuggestGasTipCap(childCtx)
	cancel()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get suggested gas tip cap: %w", err)
	}

	if gasTipCap == nil {
		e.log.Warn("unexpected unset gasTipCap, using default 2 gwei")
		gasTipCap = new(big.Int).SetUint64(params.GWei * 2)
	}

	childCtx, cancel = context.WithTimeout(ctx, networkTimeout)
	head, err := e.l1Client.HeaderByNumber(childCtx, nil)
	cancel()
	if err != nil || head == nil {
		return nil, nil, fmt.Errorf("failed to get L1 head block for fee cap: %w", err)
	}
	if head.BaseFee == nil {
		return nil, nil, fmt.Errorf("failed to get L1 basefee in block %d for fee cap", head.Number)
	}
	gasFeeCap = txmgr.CalcGasFeeCap(head.BaseFee, gasTipCap)

	return gasTipCap, gasFeeCap, nil
}

// CraftTx creates the signed transaction to the batchInboxAddress.
// It queries L1 for the current fee market conditions as well as for the nonce.
// NOTE: This method SHOULD NOT publish the resulting transaction.
func (e EthereumDA) CraftTx(ctx context.Context, data []byte) (*types.Transaction, error) {
	gasTipCap, gasFeeCap, err := e.calcGasTipAndFeeCap(ctx)
	if err != nil {
		return nil, err
	}

	childCtx, cancel := context.WithTimeout(ctx, networkTimeout)
	nonce, err := e.l1Client.NonceAt(childCtx, e.cfg.SenderAddress, nil)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	rawTx := &types.DynamicFeeTx{
		ChainID:   e.cfg.ChainID,
		Nonce:     nonce,
		To:        &e.cfg.BatchInboxAddress,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Data:      data,
	}
	e.log.Info("creating tx", "to", rawTx.To, "from", e.cfg.SenderAddress)

	gas, err := core.IntrinsicGas(rawTx.Data, nil, false, true, true)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate intrinsic gas: %w", err)
	}
	rawTx.Gas = gas

	ctx, cancel = context.WithTimeout(ctx, networkTimeout)
	defer cancel()
	return e.signerFn(ctx, rawTx)
}

// UpdateGasPrice signs an otherwise identical txn to the one provided but with
// updated gas prices sampled from the existing network conditions.
//
// NOTE: This method SHOULD NOT publish the resulting transaction.
func (e EthereumDA) UpdateGasPrice(ctx context.Context, tx *types.Transaction) (*types.Transaction, error) {
	gasTipCap, gasFeeCap, err := e.calcGasTipAndFeeCap(ctx)
	if err != nil {
		return nil, err
	}

	rawTx := &types.DynamicFeeTx{
		ChainID:   e.cfg.ChainID,
		Nonce:     tx.Nonce(),
		To:        tx.To(),
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Gas:       tx.Gas(),
		Data:      tx.Data(),
	}
	// Only log the new tip/fee cap because the updateGasPrice closure reuses the same initial transaction
	e.log.Trace("updating gas price", "tip_cap", gasTipCap, "fee_cap", gasFeeCap)

	return e.signerFn(ctx, rawTx)
}
