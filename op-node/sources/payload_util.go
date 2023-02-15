package sources

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum-optimism/optimism/op-bindings/predeploys"
	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	L1InfoFuncSignature = "setL1BlockValues(uint64,uint64,uint256,bytes32,uint64,bytes32,uint256,uint256)"
	L1InfoArguments     = 8
	L1InfoLen           = 4 + 32*L1InfoArguments
)

var (
	L1InfoFuncBytes4       = crypto.Keccak256([]byte(L1InfoFuncSignature))[:4]
	L1InfoDepositerAddress = common.HexToAddress("0xdeaddeaddeaddeaddeaddeaddeaddeaddead0001")
	L1BlockAddress         = predeploys.L1BlockAddr
)

// L1BlockInfo presents the information stored in a L1Block.setL1BlockValues call
type L1BlockInfo struct {
	Number    uint64
	Time      uint64
	BaseFee   *big.Int
	BlockHash common.Hash
	// Not strictly a piece of L1 information. Represents the number of L2 blocks since the start of the epoch,
	// i.e. when the actual L1 info was first introduced.
	SequenceNumber uint64
	// BatcherHash version 0 is just the address with 0 padding to the left.
	BatcherAddr   common.Address
	L1FeeOverhead eth.Bytes32
	L1FeeScalar   eth.Bytes32
}

func (info *L1BlockInfo) MarshalBinary() ([]byte, error) {
	data := make([]byte, L1InfoLen)
	offset := 0
	copy(data[offset:4], L1InfoFuncBytes4)
	offset += 4
	binary.BigEndian.PutUint64(data[offset+24:offset+32], info.Number)
	offset += 32
	binary.BigEndian.PutUint64(data[offset+24:offset+32], info.Time)
	offset += 32
	// Ensure that the baseFee is not too large.
	if info.BaseFee.BitLen() > 256 {
		return nil, fmt.Errorf("base fee exceeds 256 bits: %d", info.BaseFee)
	}
	info.BaseFee.FillBytes(data[offset : offset+32])
	offset += 32
	copy(data[offset:offset+32], info.BlockHash.Bytes())
	offset += 32
	binary.BigEndian.PutUint64(data[offset+24:offset+32], info.SequenceNumber)
	offset += 32
	copy(data[offset+12:offset+32], info.BatcherAddr[:])
	offset += 32
	copy(data[offset:offset+32], info.L1FeeOverhead[:])
	offset += 32
	copy(data[offset:offset+32], info.L1FeeScalar[:])
	return data, nil
}

func (info *L1BlockInfo) UnmarshalBinary(data []byte) error {
	if len(data) != L1InfoLen {
		return fmt.Errorf("data is unexpected length: %d", len(data))
	}
	var padding [24]byte
	offset := 4
	info.Number = binary.BigEndian.Uint64(data[offset+24 : offset+32])
	if !bytes.Equal(data[offset:offset+24], padding[:]) {
		return fmt.Errorf("l1 info number exceeds uint64 bounds: %x", data[offset:offset+32])
	}
	offset += 32
	info.Time = binary.BigEndian.Uint64(data[offset+24 : offset+32])
	if !bytes.Equal(data[offset:offset+24], padding[:]) {
		return fmt.Errorf("l1 info time exceeds uint64 bounds: %x", data[offset:offset+32])
	}
	offset += 32
	info.BaseFee = new(big.Int).SetBytes(data[offset : offset+32])
	offset += 32
	info.BlockHash.SetBytes(data[offset : offset+32])
	offset += 32
	info.SequenceNumber = binary.BigEndian.Uint64(data[offset+24 : offset+32])
	if !bytes.Equal(data[offset:offset+24], padding[:]) {
		return fmt.Errorf("l1 info sequence number exceeds uint64 bounds: %x", data[offset:offset+32])
	}
	offset += 32
	info.BatcherAddr.SetBytes(data[offset+12 : offset+32])
	offset += 32
	copy(info.L1FeeOverhead[:], data[offset:offset+32])
	offset += 32
	copy(info.L1FeeScalar[:], data[offset:offset+32])
	return nil
}

// L1InfoDepositTxData is the inverse of L1InfoDeposit, to see where the L2 chain is derived from
func L1InfoDepositTxData(data []byte) (L1BlockInfo, error) {
	var info L1BlockInfo
	err := info.UnmarshalBinary(data)
	return info, err
}

// PayloadToBlockRef extracts the essential L2BlockRef information from an execution payload,
// falling back to genesis information if necessary.
func PayloadToBlockRef(payload *eth.ExecutionPayload, genesis *rollup.Genesis) (eth.L2BlockRef, error) {
	var l1Origin eth.BlockID
	var sequenceNumber uint64
	if uint64(payload.BlockNumber) == genesis.L2.Number {
		if payload.BlockHash != genesis.L2.Hash {
			return eth.L2BlockRef{}, fmt.Errorf("expected L2 genesis hash to match L2 block at genesis block number %d: %s <> %s", genesis.L2.Number, payload.BlockHash, genesis.L2.Hash)
		}
		l1Origin = genesis.L1
		sequenceNumber = 0
	} else {
		if len(payload.Transactions) == 0 {
			return eth.L2BlockRef{}, fmt.Errorf("l2 block is missing L1 info deposit tx, block hash: %s", payload.BlockHash)
		}
		var tx types.Transaction
		if err := tx.UnmarshalBinary(payload.Transactions[0]); err != nil {
			return eth.L2BlockRef{}, fmt.Errorf("failed to decode first tx to read l1 info from: %w", err)
		}
		if tx.Type() != types.DepositTxType {
			return eth.L2BlockRef{}, fmt.Errorf("first payload tx has unexpected tx type: %d", tx.Type())
		}
		info, err := L1InfoDepositTxData(tx.Data())
		if err != nil {
			return eth.L2BlockRef{}, fmt.Errorf("failed to parse L1 info deposit tx from L2 block: %w", err)
		}
		l1Origin = eth.BlockID{Hash: info.BlockHash, Number: info.Number}
		sequenceNumber = info.SequenceNumber
	}

	return eth.L2BlockRef{
		Hash:           payload.BlockHash,
		Number:         uint64(payload.BlockNumber),
		ParentHash:     payload.ParentHash,
		Time:           uint64(payload.Timestamp),
		L1Origin:       l1Origin,
		SequenceNumber: sequenceNumber,
	}, nil
}

func PayloadToSystemConfig(payload *eth.ExecutionPayload, cfg *rollup.Config) (eth.SystemConfig, error) {
	if uint64(payload.BlockNumber) == cfg.Genesis.L2.Number {
		if payload.BlockHash != cfg.Genesis.L2.Hash {
			return eth.SystemConfig{}, fmt.Errorf("expected L2 genesis hash to match L2 block at genesis block number %d: %s <> %s", cfg.Genesis.L2.Number, payload.BlockHash, cfg.Genesis.L2.Hash)
		}
		return cfg.Genesis.SystemConfig, nil
	} else {
		if len(payload.Transactions) == 0 {
			return eth.SystemConfig{}, fmt.Errorf("l2 block is missing L1 info deposit tx, block hash: %s", payload.BlockHash)
		}
		var tx types.Transaction
		if err := tx.UnmarshalBinary(payload.Transactions[0]); err != nil {
			return eth.SystemConfig{}, fmt.Errorf("failed to decode first tx to read l1 info from: %w", err)
		}
		if tx.Type() != types.DepositTxType {
			return eth.SystemConfig{}, fmt.Errorf("first payload tx has unexpected tx type: %d", tx.Type())
		}
		info, err := L1InfoDepositTxData(tx.Data())
		if err != nil {
			return eth.SystemConfig{}, fmt.Errorf("failed to parse L1 info deposit tx from L2 block: %w", err)
		}
		return eth.SystemConfig{
			BatcherAddr: info.BatcherAddr,
			Overhead:    info.L1FeeOverhead,
			Scalar:      info.L1FeeScalar,
			GasLimit:    uint64(payload.GasLimit),
		}, err
	}
}
