# op-celestia-indexer

The `op-celestia-indexer` is a service that indexes L2 block locations on both
Celestia DA and Ethereum DA. It tracks where L2 blocks are
stored by parsing batch transactions and maintaining a mapping between L2 block
numbers and their corresponding DA locations.

## Overview

When using Celestia as the DA layer for Optimism, L2 batch data (frames) are posted to Celestia instead of being included as calldata in L1 transactions. L1 transactions contain:
- OP Stack Alt-DA format: version byte (`0x01`) + commitment type + DA layer byte (`0x0c`) + 8 bytes height + 32 bytes commitment

When using Ethereum DA, L2 batch data is included as calldata in L1 transactions with frame version byte `0x00`.

The indexer service monitors L1 batch inbox transactions, determines the DA type based on the version byte,
fetches the corresponding frame data from Celestia or L1 calldata, parses frames to determine
which L2 blocks they contain, maintains an index mapping L2 block numbers to
DA locations, and provides an RPC API to query L2 block locations.

## CLI Flags

### Required Flags
- `--start-l1-block`: Starting L1 block number for indexing
- `--batch-inbox-address`: Address of the batch inbox contract
- `--l1-eth-rpc`: HTTP provider URL for L1 Ethereum (Execution node)
- `--l2-eth-rpc`: HTTP provider URL for L2 Ethereum
- `--op-node-rpc`: HTTP provider URL for op-node (for verification)

### Optional Flags
- `--l1-beacon-rpc`: HTTP provider URL for L1 Ethereum (Consensus Node) - required for `4844 blobs` on L1.
- `--poll-interval`: Polling interval for new blocks (default: 12s)
- `--network-timeout`: Timeout for network requests (default: 10s)
- `--verify-parent-check`: Enable parent check verification in span batches (default: true)
- `--db-path`: Path to the SQLite database (default: in memory)

### Celestia DA Flags
- `--da.rpc`: Celestia DA client RPC endpoint
- `--da.auth_token`: Authentication token for Celestia client
- `--da.namespace`: Namespace for Celestia DA operations
- `--da.fallback_mode`: Fallback mode (disabled/blobdata/calldata)
- `--da.gas_price`: Gas price for Celestia operations

### Standard op-service Flags
- RPC server configuration (`--rpc.addr`, `--rpc.port`, `--rpc.enable-admin`)
- Logging configuration (`--log.level`, `--log.format`)
- Metrics configuration (`--metrics.enabled`, `--metrics.addr`, `--metrics.port`)
- Profiling configuration (`--pprof.enabled`, `--pprof.addr`, `--pprof.port`)

## API Usage

### Get DA Location

Query the DA location for a specific L2 block (works with both Celestia and Ethereum DA):

```bash
curl -X POST -H "Content-Type: application/json" -s \
  --data '{"jsonrpc":"2.0","method":"admin_getDALocation","params":[1],"id":1}' \
  http://localhost:9999 | jq .
```

Response for Celestia DA:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "celestia",
    "data": {
      "height": 12,
      "commitment": "2Pnz/0Maaz8AaDRxpXFRvHarM+fZnC3gCVhaorI8L1I=",
      "l2_range": {
        "start": 1,
        "end": 38
      },
      "l1_block": 122
    }
  }
}
```

Response for Ethereum DA (calldata):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "ethereum",
    "data": {
      "tx_hash": "0xa32638f59abac6465c0860d961917156c748819f214f7847efbcb287b53c192a",
      "l2_range": {
        "start": 1,
        "end": 60
      },
      "l1_block": 123
    }
  }
}
```

Response for Ethereum DA (EIP4844 blobs):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "ethereum",
    "data": {
      "tx_hash": "0xa8b448c9c99703d16bb885f1fb17f71a8792e0dc5f6031c3d89bdddba761a11c",
      "l2_range": {
        "start": 1,
        "end": 69
      },
      "l1_block": 123,
      "blob_hashes": [
        {
          "index": 0,
          "hash": "0x01c4a72eccab5e2ce66c98412e892d3d8902cb9f94df959fa7e90f3e3001607a"
        }
      ]
    }
  }
}
```

### Get Indexer Status

Query the current indexer status:

```bash
curl -X POST -H "Content-Type: application/json" -s \
  --data '{"jsonrpc":"2.0","method":"admin_getIndexerStatus","params":[],"id":1}' \
  http://localhost:9999 | jq .
```

## Example Usage

Start the indexer service:

```bash
./op-celestia-indexer \
  --rpc.port 9999 \
  --start-l1-block 1 \
  --batch-inbox-address 0x00e9bfcadbfb1f294e9a66bc0573878525f5015c \
  --l1-eth-rpc  http://127.0.0.1:9545 \
  --l1-beacon-rpc  http://127.0.0.1:9545 \
  --l2-eth-rpc http://127.0.0.1:8545 \
  --op-node-rpc http://127.0.0.1:7545 \
  --rpc.enable-admin \
  --db-path indexer.db \
  --log.level debug \
  --da.rpc http://127.0.0.1:26658 \
  --da.namespace 00000000000000000000000000000000000000000000006465766e6574

## Testing

Run the test suite:
```bash
just test
```

Run with verbose output:
```bash
go test -v ./...
```

## Building

Build the binary:
```bash
just op-celestia-indexer
```
Clean build artifacts:
```bash
just clean
```
