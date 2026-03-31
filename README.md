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

| Flag                  | Env Variable              | Default                 | Description                                                       |
|-----------------------|---------------------------|-------------------------|-------------------------------------------------------------------|
| `-rpc.url`            | `ELEMENTS_RPC_URL`        | `http://localhost:7041` | Elements node RPC URL                                             |
| `-rpc.user`           | `ELEMENTS_RPC_USER`       | *(required)*            | RPC username                                                      |
| `-rpc.password`       | `ELEMENTS_RPC_PASSWORD`   | *(required)*            | RPC password                                                      |
| `-web.listen-address` | `ELEMENTS_LISTEN_ADDRESS` | `:9101`                 | Address to serve metrics                                          |
| `-web.telemetry-path` | `ELEMENTS_METRICS_PATH`   | `/metrics`              | Metrics endpoint path                                             |
| `-cache.refresh`      | `ELEMENTS_CACHE_REFRESH`  | `15s`                   | Min pause between RPC calls per method (e.g. `30s`, `1m`)         |
| `-cache.ttl`          | `ELEMENTS_CACHE_TTL`      | `60s`                   | Max age of cached data before metrics are omitted                 |
| `-version`            | —                         | —                       | Print version and exit                                            |

Flags take precedence over environment variables. RPC credentials are mandatory.

## Endpoints

| Path       | Description                                                       |
|------------|-------------------------------------------------------------------|
| `/metrics` | Prometheus metrics                                                |
| `/healthz` | Health check — returns `200 OK` if the Elements node is reachable |
| `/`        | Landing page with links                                           |

## Metrics

All metrics use the `elements_` prefix.

### Blockchain

| Metric                              | Description                           |
|-------------------------------------|---------------------------------------|
| `elements_blockchain_blocks_total`  | Block count on the longest chain      |
| `elements_blockchain_headers_total` | Number of headers received            |
| `elements_blockchain_difficulty`    | Current network difficulty            |
| `elements_blockchain_sync_ratio`    | Verification progress (0 to 1)        |
| `elements_blockchain_disk_bytes`    | Estimated blockchain size on disk     |
| `elements_blockchain_pruned`        | Whether the chain is pruned (1 = yes) |

### Network

| Metric                               | Description            |
|--------------------------------------|------------------------|
| `elements_network_connections_total` | Total peer connections |
| `elements_network_connections_in`    | Inbound connections    |
| `elements_network_connections_out`   | Outbound connections   |

### Mempool (global)

| Metric                              | Description                        |
|-------------------------------------|------------------------------------|
| `elements_mempool_transactions`     | Transaction count in mempool       |
| `elements_mempool_size_bytes`       | Total size of mempool transactions |
| `elements_mempool_memory_bytes`     | Memory used by mempool             |
| `elements_mempool_max_memory_bytes` | Mempool memory limit               |
| `elements_mempool_min_fee_rate`     | Minimum accepted fee rate (BTC/kB) |

### Wallet

Labels: `wallet`, `asset`

| Metric                                | Labels            | Description                   |
|---------------------------------------|-------------------|-------------------------------|
| `elements_wallet_balance`             | `wallet`, `asset` | Confirmed balance             |
| `elements_wallet_unconfirmed_balance` | `wallet`, `asset` | Unconfirmed balance (mempool) |
| `elements_wallet_immature_balance`    | `wallet`, `asset` | Immature balance (coinbase)   |
| `elements_wallet_transactions_total`  | `wallet`          | Total historical transactions |
| `elements_wallet_utxo_count`          | `wallet`          | Number of available UTXOs     |

### Wallet Pending Transactions (mempool)

These track transactions created by each wallet that are still waiting for confirmation:

| Metric                                         | Labels                         | Description               |
|------------------------------------------------|--------------------------------|---------------------------|
| `elements_wallet_mempool_pending_transactions` | `wallet`, `direction`          | Pending transaction count |
| `elements_wallet_mempool_pending_value`        | `wallet`, `asset`, `direction` | Total pending value       |

The `direction` label is `send`, `receive`, or `other`.

### Scrape Health

| Metric                                | Labels      | Description                    |
|---------------------------------------|-------------|--------------------------------|
| `elements_scrape_success`             | `collector` | 1 if the last scrape succeeded |
| `elements_scrape_duration_seconds`    | `collector` | Duration of the last scrape    |

The `collector` label is one of: `blockchain`, `mempool`, `wallets`.

## Building

Requires Go 1.23+.

```bash
make build          # Build for host platform
make build-linux    # Cross-compile for linux/amd64
make docker         # Build Docker image
make docker-push    # Push Docker image to registry
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

## Docker

```bash
# Build the image
make docker

# Or with a custom image name and version
make docker IMAGE=ghcr.io/eulen/elements-exporter VERSION=v0.1.0

# Run the container
docker run -d \
  --name elements-exporter \
  -p 9101:9101 \
  -e ELEMENTS_RPC_URL=http://elements-node:7041 \
  -e ELEMENTS_RPC_USER=admin \
  -e ELEMENTS_RPC_PASSWORD=secret \
  elements-exporter:latest
```

The image is based on Alpine Linux (~15 MB), runs as a non-root user, and contains only the static binary plus CA certificates.

## CI/CD

### CI (every push and pull request)

Runs formatting check, `go vet`, tests with race detector, and verifies the build compiles.

### Release (push to main)

Automatically increments the version tag and creates a GitHub Release with a linux/amd64 binary:

| Commit message pattern   | Version bump                  |
|--------------------------|-------------------------------|
| `[major]` or `breaking:` | Major (e.g. v1.0.0 -> v2.0.0) |
| `[minor]` or `feat:`     | Minor (e.g. v0.1.0 -> v0.2.0) |
| *(default)*              | Patch (e.g. v0.1.0 -> v0.1.1) |

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

## RPC Caching

All RPC calls made by the collector are cached with two configurable parameters:

- **`cache.refresh`** (default `15s`) — minimum pause between actual RPC calls to the node for the same method. Within this window, scrapes receive cached data instantly without touching the node.
- **`cache.ttl`** (default `60s`) — maximum age of cached data. After this, cached values are discarded and metrics are omitted until the next successful RPC call.

When cached data is older than `refresh` but younger than `ttl`, the exporter attempts to refresh from the node. If the RPC call fails, it falls back to the stale cached data — metrics remain available as long as the cache hasn't expired. This prevents the exporter from competing with wallet operations (e.g. transaction signing) for the node's RPC lock.

The `/healthz` endpoint always queries the node directly and is not affected by the cache.

```bash
# Increase refresh interval to 30s, keep TTL at 2 minutes
./elements-exporter -cache.refresh=30s -cache.ttl=2m ...

# Disable caching entirely
./elements-exporter -cache.refresh=0 -cache.ttl=0 ...
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
