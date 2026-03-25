# Elements Metric Exporter

Prometheus exporter for [Elements](https://github.com/ElementsProject/elements) (Blockstream) nodes. Exposes blockchain, network, mempool, and per-wallet metrics including pending transactions in the mempool.

## Quick Start

```bash
# Build
make build

# Show version
./elements-exporter -version

# Run with flags
./elements-exporter \
  -rpc.url=http://localhost:7041 \
  -rpc.user=admin \
  -rpc.password=secret \
  -web.listen-address=:9101

# Or using environment variables (container-friendly)
export ELEMENTS_RPC_URL=http://localhost:7041
export ELEMENTS_RPC_USER=admin
export ELEMENTS_RPC_PASSWORD=secret
./elements-exporter
```

Metrics are available at `http://localhost:9101/metrics`.

## Configuration

| Flag | Env Variable | Default | Description |
|------|-------------|---------|-------------|
| `-rpc.url` | `ELEMENTS_RPC_URL` | `http://localhost:7041` | Elements node RPC URL |
| `-rpc.user` | `ELEMENTS_RPC_USER` | *(required)* | RPC username |
| `-rpc.password` | `ELEMENTS_RPC_PASSWORD` | *(required)* | RPC password |
| `-web.listen-address` | `ELEMENTS_LISTEN_ADDRESS` | `:9101` | Address to serve metrics |
| `-web.telemetry-path` | `ELEMENTS_METRICS_PATH` | `/metrics` | Metrics endpoint path |
| `-version` | — | — | Print version and exit |

Flags take precedence over environment variables. RPC credentials are mandatory.

## Endpoints

| Path | Description |
|------|-------------|
| `/metrics` | Prometheus metrics |
| `/healthz` | Health check — returns `200 OK` if the Elements node is reachable |
| `/` | Landing page with links |

## Metrics

All metrics use the `elements_` prefix.

### Blockchain

| Metric | Description |
|--------|-------------|
| `elements_blockchain_blocks_total` | Block count on the longest chain |
| `elements_blockchain_headers_total` | Number of headers received |
| `elements_blockchain_difficulty` | Current network difficulty |
| `elements_blockchain_sync_ratio` | Verification progress (0 to 1) |
| `elements_blockchain_disk_bytes` | Estimated blockchain size on disk |
| `elements_blockchain_pruned` | Whether the chain is pruned (1 = yes) |

### Network

| Metric | Description |
|--------|-------------|
| `elements_network_connections_total` | Total peer connections |
| `elements_network_connections_in` | Inbound connections |
| `elements_network_connections_out` | Outbound connections |

### Mempool (global)

| Metric | Description |
|--------|-------------|
| `elements_mempool_transactions` | Transaction count in mempool |
| `elements_mempool_size_bytes` | Total size of mempool transactions |
| `elements_mempool_memory_bytes` | Memory used by mempool |
| `elements_mempool_max_memory_bytes` | Mempool memory limit |
| `elements_mempool_min_fee_rate` | Minimum accepted fee rate (BTC/kB) |

### Wallet

Labels: `wallet`, `asset`

| Metric | Labels | Description |
|--------|--------|-------------|
| `elements_wallet_balance` | `wallet`, `asset` | Confirmed balance |
| `elements_wallet_unconfirmed_balance` | `wallet`, `asset` | Unconfirmed balance (mempool) |
| `elements_wallet_immature_balance` | `wallet`, `asset` | Immature balance (coinbase) |
| `elements_wallet_transactions_total` | `wallet` | Total historical transactions |
| `elements_wallet_utxo_count` | `wallet` | Number of available UTXOs |

### Wallet Pending Transactions (mempool)

These track transactions created by each wallet that are still waiting for confirmation:

| Metric | Labels | Description |
|--------|--------|-------------|
| `elements_wallet_mempool_pending_transactions` | `wallet`, `direction` | Pending transaction count |
| `elements_wallet_mempool_pending_value` | `wallet`, `asset`, `direction` | Total pending value |

The `direction` label is `send`, `receive`, or `other`.

### Scrape Health

| Metric | Labels | Description |
|--------|--------|-------------|
| `elements_scrape_success` | `collector` | 1 if the last scrape succeeded |
| `elements_scrape_duration_seconds` | `collector` | Duration of the last scrape |

The `collector` label is one of: `blockchain`, `mempool`, `wallets`.

## Building

Requires Go 1.23+.

```bash
make build          # Build for host platform
make build-linux    # Cross-compile for linux/amd64
make test           # Run unit tests
make test-race      # Run tests with race detector
make test-cover     # Run tests with coverage report
make vet            # Run go vet
make fmt            # Check gofmt compliance
make lint           # Run staticcheck
make check          # Run fmt + vet + tests
make clean          # Remove build artefacts
make help           # Show all targets
```

The build embeds version info via `-ldflags`. If the repository has a git tag (e.g. `v0.1.0`), the version is picked up automatically via `git describe`. You can override it:

```bash
make build VERSION=v1.0.0
```

## CI/CD

### CI (every push and pull request)

Runs formatting check, `go vet`, tests with race detector, and verifies the build compiles.

### Release (push to main)

Automatically increments the version tag and creates a GitHub Release with a linux/amd64 binary:

| Commit message pattern | Version bump |
|------------------------|-------------|
| `[major]` or `breaking:` | Major (e.g. v1.0.0 -> v2.0.0) |
| `[minor]` or `feat:` | Minor (e.g. v0.1.0 -> v0.2.0) |
| *(default)* | Patch (e.g. v0.1.0 -> v0.1.1) |

The first release starts at `v0.1.0`.

## Project Structure

```
├── main.go            # Entry point, config, HTTP server
├── rpc.go             # Elements RPC client
├── types.go           # RPC response types and parsing
├── collector.go       # Prometheus collector and metric collection
├── helpers.go         # Utility functions
├── *_test.go          # Unit tests
├── Makefile           # Build targets
└── .github/workflows/
    ├── ci.yml         # CI: lint, test, build
    └── release.yml    # Auto-tag and release
```

## RPC Methods Used

The exporter calls the following Elements RPC methods:

- `getblockchaininfo` — blockchain state
- `getnetworkinfo` — peer connections
- `getmempoolinfo` — global mempool state
- `listwallets` — discover loaded wallets
- `getbalances` — per-asset wallet balances (with `getwalletinfo` fallback for older nodes)
- `getwalletinfo` — transaction count
- `listunspent` — UTXO count
- `listtransactions` — pending mempool transactions per wallet

## License

Copyright 2026 Eulen

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
