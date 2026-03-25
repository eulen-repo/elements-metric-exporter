// Copyright 2026 Eulen
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type ElementsCollector struct {
	mu  sync.Mutex
	rpc *RPCClient

	// ---- Blockchain ----
	descBlocks     *prometheus.Desc
	descHeaders    *prometheus.Desc
	descDifficulty *prometheus.Desc
	descSyncRatio  *prometheus.Desc
	descDiskBytes  *prometheus.Desc
	descPruned     *prometheus.Desc

	// ---- Network ----
	descConnections    *prometheus.Desc
	descConnectionsIn  *prometheus.Desc
	descConnectionsOut *prometheus.Desc

	// ---- Global mempool ----
	descMempoolTxs    *prometheus.Desc
	descMempoolBytes  *prometheus.Desc
	descMempoolMemory *prometheus.Desc
	descMempoolMaxMem *prometheus.Desc
	descMempoolMinFee *prometheus.Desc

	// ---- Local mempool (unconfirmed wallet TXs) ----
	descLocalPendingCount *prometheus.Desc // {wallet, direction}
	descLocalPendingValue *prometheus.Desc // {wallet, asset, direction}

	// ---- Wallets ----
	descWalletBalance            *prometheus.Desc // {wallet, asset}
	descWalletUnconfirmedBalance *prometheus.Desc // {wallet, asset}
	descWalletImmatureBalance    *prometheus.Desc // {wallet, asset}
	descWalletTxCount            *prometheus.Desc // {wallet}
	descWalletUTXOCount          *prometheus.Desc // {wallet}

	// ---- Scrape health ----
	descScrapeOK  *prometheus.Desc
	descScrapeSec *prometheus.Desc
}

func NewElementsCollector(rpc *RPCClient) *ElementsCollector {
	ns := "elements"
	label := func(labels ...string) []string { return labels }

	return &ElementsCollector{
		rpc: rpc,

		descBlocks:     prometheus.NewDesc(ns+"_blockchain_blocks_total", "Block count on the longest chain", nil, nil),
		descHeaders:    prometheus.NewDesc(ns+"_blockchain_headers_total", "Number of headers received", nil, nil),
		descDifficulty: prometheus.NewDesc(ns+"_blockchain_difficulty", "Current network difficulty", nil, nil),
		descSyncRatio:  prometheus.NewDesc(ns+"_blockchain_sync_ratio", "Verification progress [0..1]", nil, nil),
		descDiskBytes:  prometheus.NewDesc(ns+"_blockchain_disk_bytes", "Estimated blockchain size on disk", nil, nil),
		descPruned:     prometheus.NewDesc(ns+"_blockchain_pruned", "Whether the chain is pruned (1=yes)", nil, nil),

		descConnections:    prometheus.NewDesc(ns+"_network_connections_total", "Total peer connections", nil, nil),
		descConnectionsIn:  prometheus.NewDesc(ns+"_network_connections_in", "Inbound connections", nil, nil),
		descConnectionsOut: prometheus.NewDesc(ns+"_network_connections_out", "Outbound connections", nil, nil),

		descMempoolTxs:    prometheus.NewDesc(ns+"_mempool_transactions", "Number of transactions in mempool", nil, nil),
		descMempoolBytes:  prometheus.NewDesc(ns+"_mempool_size_bytes", "Total size of mempool transactions", nil, nil),
		descMempoolMemory: prometheus.NewDesc(ns+"_mempool_memory_bytes", "Memory used by mempool", nil, nil),
		descMempoolMaxMem: prometheus.NewDesc(ns+"_mempool_max_memory_bytes", "Mempool memory limit", nil, nil),
		descMempoolMinFee: prometheus.NewDesc(ns+"_mempool_min_fee_rate", "Minimum accepted fee rate (BTC/kB)", nil, nil),

		descLocalPendingCount: prometheus.NewDesc(
			ns+"_wallet_mempool_pending_transactions",
			"Wallet transactions in mempool awaiting confirmation",
			label("wallet", "direction"), nil,
		),
		descLocalPendingValue: prometheus.NewDesc(
			ns+"_wallet_mempool_pending_value",
			"Total value of pending mempool transactions per wallet and asset",
			label("wallet", "asset", "direction"), nil,
		),

		descWalletBalance:            prometheus.NewDesc(ns+"_wallet_balance", "Confirmed balance", label("wallet", "asset"), nil),
		descWalletUnconfirmedBalance: prometheus.NewDesc(ns+"_wallet_unconfirmed_balance", "Unconfirmed balance (mempool)", label("wallet", "asset"), nil),
		descWalletImmatureBalance:    prometheus.NewDesc(ns+"_wallet_immature_balance", "Immature balance (coinbase)", label("wallet", "asset"), nil),
		descWalletTxCount:            prometheus.NewDesc(ns+"_wallet_transactions_total", "Total wallet transactions", label("wallet"), nil),
		descWalletUTXOCount:          prometheus.NewDesc(ns+"_wallet_utxo_count", "Number of available UTXOs", label("wallet"), nil),

		descScrapeOK:  prometheus.NewDesc(ns+"_scrape_success", "1 if the last scrape succeeded", label("collector"), nil),
		descScrapeSec: prometheus.NewDesc(ns+"_scrape_duration_seconds", "Duration of the last scrape", label("collector"), nil),
	}
}

func (e *ElementsCollector) Describe(ch chan<- *prometheus.Desc) {
	descs := []*prometheus.Desc{
		e.descBlocks, e.descHeaders, e.descDifficulty, e.descSyncRatio, e.descDiskBytes, e.descPruned,
		e.descConnections, e.descConnectionsIn, e.descConnectionsOut,
		e.descMempoolTxs, e.descMempoolBytes, e.descMempoolMemory, e.descMempoolMaxMem, e.descMempoolMinFee,
		e.descLocalPendingCount, e.descLocalPendingValue,
		e.descWalletBalance, e.descWalletUnconfirmedBalance, e.descWalletImmatureBalance,
		e.descWalletTxCount, e.descWalletUTXOCount,
		e.descScrapeOK, e.descScrapeSec,
	}
	for _, d := range descs {
		ch <- d
	}
}

func (e *ElementsCollector) Collect(ch chan<- prometheus.Metric) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.collectBlockchain(ch)
	e.collectMempool(ch)
	e.collectWallets(ch)
}

// collectBlockchain collects blockchain and network metrics.
func (e *ElementsCollector) collectBlockchain(ch chan<- prometheus.Metric) {
	t := time.Now()
	ok := 1.0

	raw, err := e.rpc.Call("", "getblockchaininfo")
	if err != nil {
		slog.Error("RPC call failed", "collector", "blockchain", "method", "getblockchaininfo", "error", err)
		ok = 0
	} else {
		var info blockchainInfo
		if err := json.Unmarshal(raw, &info); err != nil {
			slog.Error("failed to parse response", "collector", "blockchain", "method", "getblockchaininfo", "error", err)
			ok = 0
		} else {
			pruned := boolToFloat(info.Pruned)
			gauge(ch, e.descBlocks, float64(info.Blocks))
			gauge(ch, e.descHeaders, float64(info.Headers))
			gauge(ch, e.descDifficulty, info.Difficulty)
			gauge(ch, e.descSyncRatio, info.VerificationProgress)
			gauge(ch, e.descDiskBytes, float64(info.SizeOnDisk))
			gauge(ch, e.descPruned, pruned)
		}
	}

	rawNet, err := e.rpc.Call("", "getnetworkinfo")
	if err != nil {
		slog.Error("RPC call failed", "collector", "blockchain", "method", "getnetworkinfo", "error", err)
	} else {
		var ni networkInfo
		if err := json.Unmarshal(rawNet, &ni); err == nil {
			gauge(ch, e.descConnections, float64(ni.Connections))
			gauge(ch, e.descConnectionsIn, float64(ni.ConnectionsIn))
			gauge(ch, e.descConnectionsOut, float64(ni.ConnectionsOut))
		}
	}

	scrapeHealth(ch, e.descScrapeOK, e.descScrapeSec, "blockchain", ok, t)
}

// collectMempool collects global mempool metrics.
func (e *ElementsCollector) collectMempool(ch chan<- prometheus.Metric) {
	t := time.Now()
	ok := 1.0

	raw, err := e.rpc.Call("", "getmempoolinfo")
	if err != nil {
		slog.Error("RPC call failed", "collector", "mempool", "method", "getmempoolinfo", "error", err)
		ok = 0
	} else {
		var info mempoolInfo
		if err := json.Unmarshal(raw, &info); err != nil {
			slog.Error("failed to parse response", "collector", "mempool", "method", "getmempoolinfo", "error", err)
			ok = 0
		} else {
			gauge(ch, e.descMempoolTxs, float64(info.Size))
			gauge(ch, e.descMempoolBytes, float64(info.Bytes))
			gauge(ch, e.descMempoolMemory, float64(info.Usage))
			gauge(ch, e.descMempoolMaxMem, float64(info.MaxMempool))
			gauge(ch, e.descMempoolMinFee, info.MempoolMinFee)
		}
	}

	scrapeHealth(ch, e.descScrapeOK, e.descScrapeSec, "mempool", ok, t)
}

// collectWallets collects metrics from all loaded wallets.
func (e *ElementsCollector) collectWallets(ch chan<- prometheus.Metric) {
	t := time.Now()
	ok := 1.0

	raw, err := e.rpc.Call("", "listwallets")
	if err != nil {
		slog.Error("RPC call failed", "collector", "wallets", "method", "listwallets", "error", err)
		scrapeHealth(ch, e.descScrapeOK, e.descScrapeSec, "wallets", 0, t)
		return
	}

	var wallets []string
	if err := json.Unmarshal(raw, &wallets); err != nil {
		slog.Error("failed to parse response", "collector", "wallets", "method", "listwallets", "error", err)
		scrapeHealth(ch, e.descScrapeOK, e.descScrapeSec, "wallets", 0, t)
		return
	}

	for _, w := range wallets {
		if err := e.collectWallet(ch, w); err != nil {
			slog.Error("wallet collection failed", "collector", "wallets", "wallet", w, "error", err)
			ok = 0
		}
	}

	scrapeHealth(ch, e.descScrapeOK, e.descScrapeSec, "wallets", ok, t)
}

func (e *ElementsCollector) collectWallet(ch chan<- prometheus.Metric, wallet string) error {
	raw, err := e.rpc.Call(wallet, "getbalances")
	if err != nil {
		return e.collectWalletLegacy(ch, wallet)
	}

	var b walletBalances
	if err := json.Unmarshal(raw, &b); err != nil {
		return fmt.Errorf("parse getbalances: %w", err)
	}

	for asset, v := range parseAssetBalance(b.Mine.Trusted) {
		gaugeL(ch, e.descWalletBalance, v, wallet, asset)
	}
	for asset, v := range parseAssetBalance(b.Mine.UntrustedPending) {
		gaugeL(ch, e.descWalletUnconfirmedBalance, v, wallet, asset)
	}
	for asset, v := range parseAssetBalance(b.Mine.Immature) {
		gaugeL(ch, e.descWalletImmatureBalance, v, wallet, asset)
	}

	if raw2, err := e.rpc.Call(wallet, "getwalletinfo"); err == nil {
		var wi walletInfo
		if json.Unmarshal(raw2, &wi) == nil {
			gaugeL(ch, e.descWalletTxCount, float64(wi.TxCount), wallet)
		}
	}

	if raw3, err := e.rpc.Call(wallet, "listunspent", 1, 9999999, []any{}); err == nil {
		var utxos []utxo
		if json.Unmarshal(raw3, &utxos) == nil {
			gaugeL(ch, e.descWalletUTXOCount, float64(len(utxos)), wallet)
		}
	}

	e.collectPendingTxs(ch, wallet)

	return nil
}

// collectWalletLegacy uses getwalletinfo for nodes without getbalances (older Elements).
func (e *ElementsCollector) collectWalletLegacy(ch chan<- prometheus.Metric, wallet string) error {
	type legacyInfo struct {
		Balance            float64 `json:"balance"`
		UnconfirmedBalance float64 `json:"unconfirmed_balance"`
		ImmatureBalance    float64 `json:"immature_balance"`
		TxCount            int64   `json:"txcount"`
	}
	raw, err := e.rpc.Call(wallet, "getwalletinfo")
	if err != nil {
		return err
	}
	var info legacyInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return err
	}
	gaugeL(ch, e.descWalletBalance, info.Balance, wallet, "bitcoin")
	gaugeL(ch, e.descWalletUnconfirmedBalance, info.UnconfirmedBalance, wallet, "bitcoin")
	gaugeL(ch, e.descWalletImmatureBalance, info.ImmatureBalance, wallet, "bitcoin")
	gaugeL(ch, e.descWalletTxCount, float64(info.TxCount), wallet)
	e.collectPendingTxs(ch, wallet)
	return nil
}

// collectPendingTxs counts and sums the value of wallet transactions in the mempool
// (confirmations == 0), grouped by direction (send / receive).
func (e *ElementsCollector) collectPendingTxs(ch chan<- prometheus.Metric, wallet string) {
	raw, err := e.rpc.Call(wallet, "listtransactions", "*", 500, 0, true)
	if err != nil {
		slog.Error("RPC call failed", "collector", "pending", "method", "listtransactions", "wallet", wallet, "error", err)
		return
	}

	var txs []listTx
	if err := json.Unmarshal(raw, &txs); err != nil {
		slog.Error("failed to parse response", "collector", "pending", "method", "listtransactions", "wallet", wallet, "error", err)
		return
	}

	type key struct{ dir, asset string }
	counts := map[string]int{}
	values := map[key]float64{}
	seen := map[string]bool{}

	for _, tx := range txs {
		if tx.Confirmations != 0 {
			continue
		}

		dir := tx.Category
		if dir != "send" && dir != "receive" {
			dir = "other"
		}

		dedup := tx.TXID + ":" + dir
		if seen[dedup] {
			continue
		}
		seen[dedup] = true

		asset := tx.Asset
		if asset == "" {
			asset = "bitcoin"
		}

		counts[dir]++
		amt := tx.Amount
		if amt < 0 {
			amt = -amt
		}
		values[key{dir, asset}] += amt
	}

	for dir, cnt := range counts {
		gaugeL(ch, e.descLocalPendingCount, float64(cnt), wallet, dir)
	}
	for k, val := range values {
		gaugeL(ch, e.descLocalPendingValue, val, wallet, k.asset, k.dir)
	}
}
