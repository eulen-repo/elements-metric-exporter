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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// fakeRPCServer creates an httptest server that responds to Elements RPC methods
// with realistic data. The responses map is keyed by "method" or "wallet/method".
// Values can be any Go type (marshalled to JSON) or a string starting with "RAW:"
// which is sent as-is (useful for triggering parse errors).
func fakeRPCServer(responses map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Write([]byte(`{"result":null,"error":{"code":-1,"message":"bad request"}}`))
			return
		}

		// Build lookup key: if path has /wallet/<name>, prefix it
		key := req.Method
		if strings.HasPrefix(r.URL.Path, "/wallet/") {
			wallet := strings.TrimPrefix(r.URL.Path, "/wallet/")
			key = wallet + "/" + req.Method
		}

		result, ok := responses[key]
		if !ok {
			w.Write([]byte(fmt.Sprintf(`{"result":null,"error":{"code":-32601,"message":"Method %s not found"}}`, req.Method)))
			return
		}

		// If value is a "RAW:..." string, send the raw suffix as the result
		// (allows injecting invalid JSON to trigger parse-error paths).
		if s, isStr := result.(string); isStr && strings.HasPrefix(s, "RAW:") {
			raw := strings.TrimPrefix(s, "RAW:")
			w.Write([]byte(fmt.Sprintf(`{"result":%s,"error":null}`, raw)))
			return
		}

		data, _ := json.Marshal(result)
		w.Write([]byte(fmt.Sprintf(`{"result":%s,"error":null}`, data)))
	}))
}

// drainMetrics collects all metrics from a channel into a map keyed by metric fqName.
// For labeled metrics, the key includes label values: "fqName{l1=v1,l2=v2}".
func drainMetrics(ch chan prometheus.Metric) map[string]float64 {
	result := make(map[string]float64)
	for {
		select {
		case m := <-ch:
			var d dto.Metric
			m.Write(&d)
			desc := m.Desc().String()
			// Extract fqName from desc string: fqName: "xxx"
			fqName := extractFQName(desc)

			labelParts := []string{}
			for _, l := range d.GetLabel() {
				labelParts = append(labelParts, l.GetName()+"="+l.GetValue())
			}
			key := fqName
			if len(labelParts) > 0 {
				key = fqName + "{" + strings.Join(labelParts, ",") + "}"
			}
			result[key] = d.GetGauge().GetValue()
		default:
			return result
		}
	}
}

func extractFQName(desc string) string {
	// desc format: Desc{fqName: "elements_blockchain_blocks_total", ...}
	start := strings.Index(desc, `fqName: "`)
	if start < 0 {
		return desc
	}
	start += len(`fqName: "`)
	end := strings.Index(desc[start:], `"`)
	if end < 0 {
		return desc
	}
	return desc[start : start+end]
}

func TestCollectBlockchain(t *testing.T) {
	responses := map[string]any{
		"getblockchaininfo": blockchainInfo{
			Blocks:               850000,
			Headers:              850001,
			Difficulty:           67.3,
			VerificationProgress: 0.9999,
			SizeOnDisk:           600000000,
			Pruned:               true,
			Chain:                "liquidv1",
		},
		"getnetworkinfo": networkInfo{
			Connections:    10,
			ConnectionsIn:  3,
			ConnectionsOut: 7,
		},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectBlockchain(ch)
	metrics := drainMetrics(ch)

	checks := map[string]float64{
		"elements_blockchain_blocks_total":   850000,
		"elements_blockchain_headers_total":  850001,
		"elements_blockchain_difficulty":     67.3,
		"elements_blockchain_sync_ratio":     0.9999,
		"elements_blockchain_disk_bytes":     600000000,
		"elements_blockchain_pruned":         1,
		"elements_network_connections_total": 10,
		"elements_network_connections_in":    3,
		"elements_network_connections_out":   7,
	}

	for name, want := range checks {
		got, ok := metrics[name]
		if !ok {
			t.Errorf("metric %q not found", name)
			continue
		}
		if got != want {
			t.Errorf("%s = %f, want %f", name, got, want)
		}
	}

	// Scrape health should report success
	if v := metrics["elements_scrape_success{collector=blockchain}"]; v != 1 {
		t.Errorf("scrape success = %f, want 1", v)
	}
}

func TestCollectBlockchain_RPCError(t *testing.T) {
	// Empty responses → all methods return errors
	srv := fakeRPCServer(map[string]any{})
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectBlockchain(ch)
	metrics := drainMetrics(ch)

	// Should report scrape failure
	if v := metrics["elements_scrape_success{collector=blockchain}"]; v != 0 {
		t.Errorf("scrape success = %f, want 0 on RPC error", v)
	}
}

func TestCollectMempool(t *testing.T) {
	responses := map[string]any{
		"getmempoolinfo": mempoolInfo{
			Loaded:        true,
			Size:          42,
			Bytes:         12345,
			Usage:         67890,
			MaxMempool:    300000000,
			MempoolMinFee: 0.00001,
		},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectMempool(ch)
	metrics := drainMetrics(ch)

	checks := map[string]float64{
		"elements_mempool_transactions":     42,
		"elements_mempool_size_bytes":       12345,
		"elements_mempool_memory_bytes":     67890,
		"elements_mempool_max_memory_bytes": 300000000,
		"elements_mempool_min_fee_rate":     0.00001,
	}

	for name, want := range checks {
		got, ok := metrics[name]
		if !ok {
			t.Errorf("metric %q not found", name)
			continue
		}
		if got != want {
			t.Errorf("%s = %f, want %f", name, got, want)
		}
	}

	if v := metrics["elements_scrape_success{collector=mempool}"]; v != 1 {
		t.Errorf("scrape success = %f, want 1", v)
	}
}

func TestCollectMempool_RPCError(t *testing.T) {
	srv := fakeRPCServer(map[string]any{})
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectMempool(ch)
	metrics := drainMetrics(ch)

	if v := metrics["elements_scrape_success{collector=mempool}"]; v != 0 {
		t.Errorf("scrape success = %f, want 0 on RPC error", v)
	}
}

func TestCollectWallets_FullFlow(t *testing.T) {
	responses := map[string]any{
		"listwallets": []string{"wallet1"},
		"wallet1/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{"lbtc": 10.5, "usdt": 100.0},
				"untrusted_pending": map[string]float64{"lbtc": 0.1},
				"immature":          map[string]float64{},
			},
		},
		"wallet1/getwalletinfo": map[string]any{
			"txcount": 42,
		},
		"wallet1/listunspent": []map[string]string{
			{"txid": "aaa"},
			{"txid": "bbb"},
			{"txid": "ccc"},
		},
		"wallet1/listtransactions": []map[string]any{
			{"txid": "tx1", "category": "send", "amount": -1.0, "asset": "lbtc", "confirmations": 0},
			{"txid": "tx2", "category": "receive", "amount": 0.5, "asset": "lbtc", "confirmations": 0},
			{"txid": "tx3", "category": "send", "amount": -2.0, "asset": "lbtc", "confirmations": 3}, // confirmed, should be ignored
		},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	// Wallet balances
	if v := metrics["elements_wallet_balance{asset=lbtc,wallet=wallet1}"]; v != 10.5 {
		t.Errorf("wallet balance lbtc = %f, want 10.5", v)
	}
	if v := metrics["elements_wallet_balance{asset=usdt,wallet=wallet1}"]; v != 100.0 {
		t.Errorf("wallet balance usdt = %f, want 100.0", v)
	}
	if v := metrics["elements_wallet_unconfirmed_balance{asset=lbtc,wallet=wallet1}"]; v != 0.1 {
		t.Errorf("unconfirmed balance = %f, want 0.1", v)
	}

	// TX count
	if v := metrics["elements_wallet_transactions_total{wallet=wallet1}"]; v != 42 {
		t.Errorf("tx count = %f, want 42", v)
	}

	// UTXO count
	if v := metrics["elements_wallet_utxo_count{wallet=wallet1}"]; v != 3 {
		t.Errorf("utxo count = %f, want 3", v)
	}

	// Pending transactions (only confirmations==0)
	if v := metrics["elements_wallet_mempool_pending_transactions{direction=send,wallet=wallet1}"]; v != 1 {
		t.Errorf("pending send count = %f, want 1", v)
	}
	if v := metrics["elements_wallet_mempool_pending_transactions{direction=receive,wallet=wallet1}"]; v != 1 {
		t.Errorf("pending receive count = %f, want 1", v)
	}

	// Pending values (absolute)
	if v := metrics["elements_wallet_mempool_pending_value{asset=lbtc,direction=send,wallet=wallet1}"]; v != 1.0 {
		t.Errorf("pending send value = %f, want 1.0", v)
	}
	if v := metrics["elements_wallet_mempool_pending_value{asset=lbtc,direction=receive,wallet=wallet1}"]; v != 0.5 {
		t.Errorf("pending receive value = %f, want 0.5", v)
	}

	// Scrape health
	if v := metrics["elements_scrape_success{collector=wallets}"]; v != 1 {
		t.Errorf("scrape success = %f, want 1", v)
	}
}

func TestCollectWallets_ListWalletsError(t *testing.T) {
	srv := fakeRPCServer(map[string]any{})
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	if v := metrics["elements_scrape_success{collector=wallets}"]; v != 0 {
		t.Errorf("scrape success = %f, want 0", v)
	}
}

func TestCollectWalletLegacy(t *testing.T) {
	// getbalances fails → falls back to getwalletinfo (legacy path)
	responses := map[string]any{
		"listwallets": []string{"oldwallet"},
		"oldwallet/getwalletinfo": map[string]any{
			"balance":             5.0,
			"unconfirmed_balance": 0.1,
			"immature_balance":    0.0,
			"txcount":             10,
		},
		"oldwallet/listtransactions": []map[string]any{},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	if v := metrics["elements_wallet_balance{asset=bitcoin,wallet=oldwallet}"]; v != 5.0 {
		t.Errorf("legacy balance = %f, want 5.0", v)
	}
	if v := metrics["elements_wallet_unconfirmed_balance{asset=bitcoin,wallet=oldwallet}"]; v != 0.1 {
		t.Errorf("legacy unconfirmed = %f, want 0.1", v)
	}
	if v := metrics["elements_wallet_transactions_total{wallet=oldwallet}"]; v != 10 {
		t.Errorf("legacy txcount = %f, want 10", v)
	}
}

func TestCollectPendingTxs_Deduplication(t *testing.T) {
	// Same txid+direction should be counted only once
	responses := map[string]any{
		"listwallets": []string{"w"},
		"w/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		"w/getwalletinfo": map[string]any{"txcount": 0},
		"w/listunspent":   []map[string]string{},
		"w/listtransactions": []map[string]any{
			{"txid": "dup1", "category": "send", "amount": -1.0, "asset": "lbtc", "confirmations": 0},
			{"txid": "dup1", "category": "send", "amount": -1.0, "asset": "lbtc", "confirmations": 0},   // duplicate
			{"txid": "dup1", "category": "receive", "amount": 0.5, "asset": "lbtc", "confirmations": 0}, // same txid, different direction → not a dup
		},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	if v := metrics["elements_wallet_mempool_pending_transactions{direction=send,wallet=w}"]; v != 1 {
		t.Errorf("dedup send count = %f, want 1", v)
	}
	if v := metrics["elements_wallet_mempool_pending_transactions{direction=receive,wallet=w}"]; v != 1 {
		t.Errorf("dedup receive count = %f, want 1", v)
	}
}

func TestCollectPendingTxs_EmptyAssetDefaultsBitcoin(t *testing.T) {
	responses := map[string]any{
		"listwallets": []string{"w"},
		"w/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		"w/getwalletinfo": map[string]any{"txcount": 0},
		"w/listunspent":   []map[string]string{},
		"w/listtransactions": []map[string]any{
			{"txid": "tx1", "category": "send", "amount": -1.0, "asset": "", "confirmations": 0},
		},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	if v, ok := metrics["elements_wallet_mempool_pending_value{asset=bitcoin,direction=send,wallet=w}"]; !ok || v != 1.0 {
		t.Errorf("empty asset should default to bitcoin: got %f, ok=%v", v, ok)
	}
}

func TestCollectPendingTxs_OtherCategory(t *testing.T) {
	responses := map[string]any{
		"listwallets": []string{"w"},
		"w/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		"w/getwalletinfo": map[string]any{"txcount": 0},
		"w/listunspent":   []map[string]string{},
		"w/listtransactions": []map[string]any{
			{"txid": "tx1", "category": "generate", "amount": 50.0, "asset": "bitcoin", "confirmations": 0},
		},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	// "generate" category should be mapped to "other"
	if v := metrics["elements_wallet_mempool_pending_transactions{direction=other,wallet=w}"]; v != 1 {
		t.Errorf("other direction count = %f, want 1", v)
	}
}

func TestCollectPendingTxs_NegativeAmountAbsoluteValue(t *testing.T) {
	responses := map[string]any{
		"listwallets": []string{"w"},
		"w/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		"w/getwalletinfo": map[string]any{"txcount": 0},
		"w/listunspent":   []map[string]string{},
		"w/listtransactions": []map[string]any{
			{"txid": "tx1", "category": "send", "amount": -3.5, "asset": "lbtc", "confirmations": 0},
		},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	// Negative amounts should become positive (absolute value)
	if v := metrics["elements_wallet_mempool_pending_value{asset=lbtc,direction=send,wallet=w}"]; v != 3.5 {
		t.Errorf("pending value = %f, want 3.5 (absolute)", v)
	}
}

func TestDescribe_AllDescriptors(t *testing.T) {
	rpc := NewRPCClient("http://unused", "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan *prometheus.Desc, 50)
	col.Describe(ch)
	close(ch)

	count := 0
	for range ch {
		count++
	}

	// 6 blockchain + 3 network + 5 mempool + 2 pending + 5 wallet + 2 scrape = 23
	expected := 23
	if count != expected {
		t.Errorf("Describe sent %d descriptors, want %d", count, expected)
	}
}

func TestCollectMultipleWallets(t *testing.T) {
	responses := map[string]any{
		"listwallets": []string{"w1", "w2"},
		"w1/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{"lbtc": 5.0},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		"w1/getwalletinfo":    map[string]any{"txcount": 10},
		"w1/listunspent":      []map[string]string{{"txid": "u1"}},
		"w1/listtransactions": []map[string]any{},
		"w2/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{"lbtc": 20.0},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		"w2/getwalletinfo":    map[string]any{"txcount": 5},
		"w2/listunspent":      []map[string]string{{"txid": "u2"}, {"txid": "u3"}},
		"w2/listtransactions": []map[string]any{},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	if v := metrics["elements_wallet_balance{asset=lbtc,wallet=w1}"]; v != 5.0 {
		t.Errorf("w1 balance = %f, want 5.0", v)
	}
	if v := metrics["elements_wallet_balance{asset=lbtc,wallet=w2}"]; v != 20.0 {
		t.Errorf("w2 balance = %f, want 20.0", v)
	}
	if v := metrics["elements_wallet_utxo_count{wallet=w1}"]; v != 1 {
		t.Errorf("w1 utxo = %f, want 1", v)
	}
	if v := metrics["elements_wallet_utxo_count{wallet=w2}"]; v != 2 {
		t.Errorf("w2 utxo = %f, want 2", v)
	}
}

// ---------------------------------------------------------------------------
// Coverage gap: Collect (top-level entry point)
// ---------------------------------------------------------------------------

func TestCollect_CallsAllSubCollectors(t *testing.T) {
	responses := map[string]any{
		"getblockchaininfo": blockchainInfo{Blocks: 1},
		"getnetworkinfo":    networkInfo{Connections: 1},
		"getmempoolinfo":    mempoolInfo{Size: 1},
		"listwallets":       []string{},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.Collect(ch)
	metrics := drainMetrics(ch)

	// Verify all three sub-collectors ran by checking their scrape_success metrics
	for _, collector := range []string{"blockchain", "mempool", "wallets"} {
		key := "elements_scrape_success{collector=" + collector + "}"
		if _, ok := metrics[key]; !ok {
			t.Errorf("missing scrape_success for %q — sub-collector did not run", collector)
		}
	}
}

// ---------------------------------------------------------------------------
// Coverage gaps: JSON parse error paths
// ---------------------------------------------------------------------------

func TestCollectBlockchain_ParseError(t *testing.T) {
	// "not_an_object" is valid JSON but can't unmarshal into blockchainInfo struct
	responses := map[string]any{
		"getblockchaininfo": "RAW:\"not_an_object\"",
		"getnetworkinfo":    networkInfo{Connections: 1},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectBlockchain(ch)
	metrics := drainMetrics(ch)

	if v := metrics["elements_scrape_success{collector=blockchain}"]; v != 0 {
		t.Errorf("scrape success = %f, want 0 on parse error", v)
	}
}

func TestCollectBlockchain_NetworkInfoParseError(t *testing.T) {
	responses := map[string]any{
		"getblockchaininfo": blockchainInfo{Blocks: 100},
		"getnetworkinfo":    "RAW:\"not_an_object\"",
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectBlockchain(ch)
	metrics := drainMetrics(ch)

	// Blockchain info should still succeed
	if v := metrics["elements_blockchain_blocks_total"]; v != 100 {
		t.Errorf("blocks = %f, want 100", v)
	}
	// Network metrics should be absent (parse failed silently)
	if _, ok := metrics["elements_network_connections_total"]; ok {
		t.Error("network connections should be absent on parse error")
	}
}

func TestCollectMempool_ParseError(t *testing.T) {
	responses := map[string]any{
		"getmempoolinfo": "RAW:\"not_an_object\"",
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectMempool(ch)
	metrics := drainMetrics(ch)

	if v := metrics["elements_scrape_success{collector=mempool}"]; v != 0 {
		t.Errorf("scrape success = %f, want 0 on parse error", v)
	}
}

func TestCollectWallets_ListParseError(t *testing.T) {
	responses := map[string]any{
		"listwallets": "RAW:\"not_an_array\"",
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	if v := metrics["elements_scrape_success{collector=wallets}"]; v != 0 {
		t.Errorf("scrape success = %f, want 0 on listwallets parse error", v)
	}
}

func TestCollectWallet_GetBalancesParseError(t *testing.T) {
	responses := map[string]any{
		"listwallets":        []string{"w"},
		"w/getbalances":      "RAW:\"not_an_object\"",
		"w/listtransactions": []map[string]any{},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	// getbalances returns invalid JSON → collectWallet returns error → ok=0
	if v := metrics["elements_scrape_success{collector=wallets}"]; v != 0 {
		t.Errorf("scrape success = %f, want 0 on getbalances parse error", v)
	}
}

func TestCollectWalletLegacy_ParseError(t *testing.T) {
	// getbalances fails (not found) → falls back to legacy → legacy also returns bad JSON
	responses := map[string]any{
		"listwallets":        []string{"w"},
		"w/getwalletinfo":    "RAW:\"not_an_object\"",
		"w/listtransactions": []map[string]any{},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	if v := metrics["elements_scrape_success{collector=wallets}"]; v != 0 {
		t.Errorf("scrape success = %f, want 0 on legacy parse error", v)
	}
}

func TestCollectPendingTxs_ParseError(t *testing.T) {
	responses := map[string]any{
		"listwallets": []string{"w"},
		"w/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		"w/getwalletinfo":    map[string]any{"txcount": 0},
		"w/listunspent":      []map[string]string{},
		"w/listtransactions": "RAW:\"not_an_array\"",
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	// collectPendingTxs logs and returns silently — wallet collection still succeeds
	if v := metrics["elements_scrape_success{collector=wallets}"]; v != 1 {
		t.Errorf("scrape success = %f, want 1 (pending parse error is non-fatal)", v)
	}
	// No pending metrics should be emitted
	for k := range metrics {
		if strings.Contains(k, "pending") {
			t.Errorf("unexpected pending metric %q on parse error", k)
		}
	}
}

// ---------------------------------------------------------------------------
// collectWallet: silent skip paths for getwalletinfo/listunspent failures
// ---------------------------------------------------------------------------

func TestCollectWallet_GetWalletInfoFailure(t *testing.T) {
	// getwalletinfo fails → txcount should be silently skipped, no error
	responses := map[string]any{
		"listwallets": []string{"w"},
		"w/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{"lbtc": 1.0},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		// w/getwalletinfo intentionally missing → RPC error
		"w/listunspent":      []map[string]string{{"txid": "u1"}},
		"w/listtransactions": []map[string]any{},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	// Balance should still be collected
	if v := metrics["elements_wallet_balance{asset=lbtc,wallet=w}"]; v != 1.0 {
		t.Errorf("balance = %f, want 1.0", v)
	}
	// txcount should be absent
	if _, ok := metrics["elements_wallet_transactions_total{wallet=w}"]; ok {
		t.Error("txcount should be absent when getwalletinfo fails")
	}
	// UTXO count should still work
	if v := metrics["elements_wallet_utxo_count{wallet=w}"]; v != 1 {
		t.Errorf("utxo count = %f, want 1", v)
	}
	// Scrape should still succeed (these are non-fatal)
	if v := metrics["elements_scrape_success{collector=wallets}"]; v != 1 {
		t.Errorf("scrape success = %f, want 1", v)
	}
}

func TestCollectWallet_ListUnspentFailure(t *testing.T) {
	// listunspent fails → UTXO count should be silently skipped
	responses := map[string]any{
		"listwallets": []string{"w"},
		"w/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{"lbtc": 2.0},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		"w/getwalletinfo": map[string]any{"txcount": 5},
		// w/listunspent intentionally missing → RPC error
		"w/listtransactions": []map[string]any{},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	if v := metrics["elements_wallet_balance{asset=lbtc,wallet=w}"]; v != 2.0 {
		t.Errorf("balance = %f, want 2.0", v)
	}
	if v := metrics["elements_wallet_transactions_total{wallet=w}"]; v != 5 {
		t.Errorf("txcount = %f, want 5", v)
	}
	if _, ok := metrics["elements_wallet_utxo_count{wallet=w}"]; ok {
		t.Error("utxo count should be absent when listunspent fails")
	}
	if v := metrics["elements_scrape_success{collector=wallets}"]; v != 1 {
		t.Errorf("scrape success = %f, want 1", v)
	}
}

// ---------------------------------------------------------------------------
// Pending TXs: conflicted transactions and value accumulation
// ---------------------------------------------------------------------------

func TestCollectPendingTxs_ConflictedTransactionsIgnored(t *testing.T) {
	responses := map[string]any{
		"listwallets": []string{"w"},
		"w/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		"w/getwalletinfo": map[string]any{"txcount": 0},
		"w/listunspent":   []map[string]string{},
		"w/listtransactions": []map[string]any{
			{"txid": "tx1", "category": "send", "amount": -1.0, "asset": "lbtc", "confirmations": -1},  // conflicted
			{"txid": "tx2", "category": "send", "amount": -2.0, "asset": "lbtc", "confirmations": -2},  // conflicted
			{"txid": "tx3", "category": "send", "amount": -0.5, "asset": "lbtc", "confirmations": 0},   // pending (should count)
			{"txid": "tx4", "category": "send", "amount": -3.0, "asset": "lbtc", "confirmations": 100}, // confirmed
		},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	// Only tx3 (confirmations==0) should be counted
	if v := metrics["elements_wallet_mempool_pending_transactions{direction=send,wallet=w}"]; v != 1 {
		t.Errorf("pending count = %f, want 1 (only confirmations==0)", v)
	}
	if v := metrics["elements_wallet_mempool_pending_value{asset=lbtc,direction=send,wallet=w}"]; v != 0.5 {
		t.Errorf("pending value = %f, want 0.5", v)
	}
}

func TestCollectPendingTxs_ValueAccumulation(t *testing.T) {
	// Multiple pending TXs with same direction+asset should sum their values
	responses := map[string]any{
		"listwallets": []string{"w"},
		"w/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		"w/getwalletinfo": map[string]any{"txcount": 0},
		"w/listunspent":   []map[string]string{},
		"w/listtransactions": []map[string]any{
			{"txid": "tx1", "category": "send", "amount": -1.0, "asset": "lbtc", "confirmations": 0},
			{"txid": "tx2", "category": "send", "amount": -2.5, "asset": "lbtc", "confirmations": 0},
			{"txid": "tx3", "category": "send", "amount": -0.5, "asset": "usdt", "confirmations": 0},
			{"txid": "tx4", "category": "receive", "amount": 3.0, "asset": "lbtc", "confirmations": 0},
		},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	// send count: 3 unique (tx1+tx2 lbtc, tx3 usdt)
	if v := metrics["elements_wallet_mempool_pending_transactions{direction=send,wallet=w}"]; v != 3 {
		t.Errorf("send count = %f, want 3", v)
	}
	// receive count: 1
	if v := metrics["elements_wallet_mempool_pending_transactions{direction=receive,wallet=w}"]; v != 1 {
		t.Errorf("receive count = %f, want 1", v)
	}
	// send lbtc value: |−1.0| + |−2.5| = 3.5
	if v := metrics["elements_wallet_mempool_pending_value{asset=lbtc,direction=send,wallet=w}"]; v != 3.5 {
		t.Errorf("send lbtc value = %f, want 3.5", v)
	}
	// send usdt value: |−0.5| = 0.5
	if v := metrics["elements_wallet_mempool_pending_value{asset=usdt,direction=send,wallet=w}"]; v != 0.5 {
		t.Errorf("send usdt value = %f, want 0.5", v)
	}
	// receive lbtc value: 3.0
	if v := metrics["elements_wallet_mempool_pending_value{asset=lbtc,direction=receive,wallet=w}"]; v != 3.0 {
		t.Errorf("receive lbtc value = %f, want 3.0", v)
	}
}

func TestCollectPendingTxs_ListTransactionsRPCFailure(t *testing.T) {
	// listtransactions RPC call fails entirely (not parse error)
	responses := map[string]any{
		"listwallets": []string{"w"},
		"w/getbalances": map[string]any{
			"mine": map[string]any{
				"trusted":           map[string]float64{"lbtc": 1.0},
				"untrusted_pending": map[string]float64{},
				"immature":          map[string]float64{},
			},
		},
		"w/getwalletinfo": map[string]any{"txcount": 0},
		"w/listunspent":   []map[string]string{},
		// w/listtransactions intentionally missing → RPC error
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 100)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	// Wallet collection should still succeed (listtransactions failure is non-fatal)
	if v := metrics["elements_scrape_success{collector=wallets}"]; v != 1 {
		t.Errorf("scrape success = %f, want 1", v)
	}
	// No pending metrics
	for k := range metrics {
		if strings.Contains(k, "pending") {
			t.Errorf("unexpected pending metric %q when listtransactions RPC fails", k)
		}
	}
}

// ---------------------------------------------------------------------------
// Blockchain: pruned=false path
// ---------------------------------------------------------------------------

func TestCollectBlockchain_PrunedFalse(t *testing.T) {
	responses := map[string]any{
		"getblockchaininfo": blockchainInfo{
			Blocks: 10,
			Pruned: false,
		},
		"getnetworkinfo": networkInfo{},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectBlockchain(ch)
	metrics := drainMetrics(ch)

	if v := metrics["elements_blockchain_pruned"]; v != 0 {
		t.Errorf("pruned = %f, want 0 for non-pruned chain", v)
	}
}

// ---------------------------------------------------------------------------
// Empty wallets list
// ---------------------------------------------------------------------------

func TestCollectWallets_EmptyList(t *testing.T) {
	responses := map[string]any{
		"listwallets": []string{},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	ch := make(chan prometheus.Metric, 50)
	col.collectWallets(ch)
	metrics := drainMetrics(ch)

	// Should succeed with no wallet metrics
	if v := metrics["elements_scrape_success{collector=wallets}"]; v != 1 {
		t.Errorf("scrape success = %f, want 1 for empty wallets", v)
	}
	// Only scrape health metrics should be present
	for k := range metrics {
		if strings.Contains(k, "wallet_balance") || strings.Contains(k, "wallet_utxo") {
			t.Errorf("unexpected wallet metric %q for empty wallet list", k)
		}
	}
}

// ---------------------------------------------------------------------------
// Concurrency: multiple Collect calls should not race
// ---------------------------------------------------------------------------

func TestCollect_ConcurrentSafety(t *testing.T) {
	responses := map[string]any{
		"getblockchaininfo": blockchainInfo{Blocks: 1},
		"getnetworkinfo":    networkInfo{},
		"getmempoolinfo":    mempoolInfo{Size: 1},
		"listwallets":       []string{},
	}

	srv := fakeRPCServer(responses)
	defer srv.Close()

	rpc := NewRPCClient(srv.URL, "u", "p")
	col := NewElementsCollector(rpc)

	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		go func() {
			ch := make(chan prometheus.Metric, 100)
			col.Collect(ch)
			drainMetrics(ch)
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
	// If we get here without a panic/race, the mutex works correctly
}
