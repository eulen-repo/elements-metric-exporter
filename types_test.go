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
	"testing"
)

func TestParseAssetBalance_MapMultiAsset(t *testing.T) {
	raw := json.RawMessage(`{"6f0279e9ed041c3d710a9f57d0c02928416460c4b722ae3457a11eec381c526d":1.5,"abc123":0.25}`)
	m := parseAssetBalance(raw)
	if len(m) != 2 {
		t.Fatalf("got %d assets, want 2", len(m))
	}
	if v := m["6f0279e9ed041c3d710a9f57d0c02928416460c4b722ae3457a11eec381c526d"]; v != 1.5 {
		t.Errorf("asset value = %f, want 1.5", v)
	}
	if v := m["abc123"]; v != 0.25 {
		t.Errorf("asset abc123 = %f, want 0.25", v)
	}
}

func TestParseAssetBalance_SingleFloat(t *testing.T) {
	raw := json.RawMessage(`3.14`)
	m := parseAssetBalance(raw)
	if len(m) != 1 {
		t.Fatalf("got %d assets, want 1", len(m))
	}
	if v := m["bitcoin"]; v != 3.14 {
		t.Errorf("bitcoin = %f, want 3.14", v)
	}
}

func TestParseAssetBalance_Zero(t *testing.T) {
	raw := json.RawMessage(`0`)
	m := parseAssetBalance(raw)
	if v, ok := m["bitcoin"]; !ok || v != 0 {
		t.Errorf("bitcoin = %f, ok=%v; want 0, true", v, ok)
	}
}

func TestParseAssetBalance_EmptyMap(t *testing.T) {
	raw := json.RawMessage(`{}`)
	m := parseAssetBalance(raw)
	if len(m) != 0 {
		t.Errorf("got %d assets from empty map, want 0", len(m))
	}
}

func TestParseAssetBalance_Nil(t *testing.T) {
	m := parseAssetBalance(nil)
	if len(m) != 0 {
		t.Errorf("got %d assets from nil, want 0", len(m))
	}
}

func TestParseAssetBalance_EmptyBytes(t *testing.T) {
	m := parseAssetBalance(json.RawMessage{})
	if len(m) != 0 {
		t.Errorf("got %d assets from empty bytes, want 0", len(m))
	}
}

func TestParseAssetBalance_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`"not a number or map"`)
	m := parseAssetBalance(raw)
	if len(m) != 0 {
		t.Errorf("got %d assets from invalid input, want 0", len(m))
	}
}

func TestBlockchainInfoDeserialization(t *testing.T) {
	data := `{
		"blocks": 850000,
		"headers": 850001,
		"difficulty": 67.305,
		"verificationprogress": 0.9999,
		"size_on_disk": 600000000,
		"pruned": true,
		"chain": "liquidv1"
	}`
	var info blockchainInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.Blocks != 850000 {
		t.Errorf("Blocks = %d, want 850000", info.Blocks)
	}
	if info.Headers != 850001 {
		t.Errorf("Headers = %d, want 850001", info.Headers)
	}
	if info.Difficulty != 67.305 {
		t.Errorf("Difficulty = %f, want 67.305", info.Difficulty)
	}
	if info.VerificationProgress != 0.9999 {
		t.Errorf("VerificationProgress = %f, want 0.9999", info.VerificationProgress)
	}
	if info.SizeOnDisk != 600000000 {
		t.Errorf("SizeOnDisk = %d, want 600000000", info.SizeOnDisk)
	}
	if !info.Pruned {
		t.Error("Pruned = false, want true")
	}
	if info.Chain != "liquidv1" {
		t.Errorf("Chain = %q, want liquidv1", info.Chain)
	}
}

func TestMempoolInfoDeserialization(t *testing.T) {
	data := `{
		"loaded": true,
		"size": 42,
		"bytes": 12345,
		"usage": 67890,
		"maxmempool": 300000000,
		"mempoolminfee": 0.00001,
		"minrelaytxfee": 0.00001
	}`
	var info mempoolInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !info.Loaded {
		t.Error("Loaded = false, want true")
	}
	if info.Size != 42 {
		t.Errorf("Size = %d, want 42", info.Size)
	}
	if info.Bytes != 12345 {
		t.Errorf("Bytes = %d, want 12345", info.Bytes)
	}
	if info.MaxMempool != 300000000 {
		t.Errorf("MaxMempool = %d, want 300000000", info.MaxMempool)
	}
}

func TestNetworkInfoDeserialization(t *testing.T) {
	data := `{"connections": 10, "connections_in": 3, "connections_out": 7}`
	var info networkInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.Connections != 10 {
		t.Errorf("Connections = %d, want 10", info.Connections)
	}
	if info.ConnectionsIn != 3 {
		t.Errorf("ConnectionsIn = %d, want 3", info.ConnectionsIn)
	}
	if info.ConnectionsOut != 7 {
		t.Errorf("ConnectionsOut = %d, want 7", info.ConnectionsOut)
	}
}

func TestWalletBalancesDeserialization(t *testing.T) {
	data := `{
		"mine": {
			"trusted": {"6f0279e9ed041c3d710a9f57d0c02928416460c4b722ae3457a11eec381c526d": 10.5},
			"untrusted_pending": {"6f0279e9ed041c3d710a9f57d0c02928416460c4b722ae3457a11eec381c526d": 0.1},
			"immature": {}
		}
	}`
	var b walletBalances
	if err := json.Unmarshal([]byte(data), &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	trusted := parseAssetBalance(b.Mine.Trusted)
	if len(trusted) != 1 {
		t.Fatalf("trusted assets = %d, want 1", len(trusted))
	}
	pending := parseAssetBalance(b.Mine.UntrustedPending)
	if len(pending) != 1 {
		t.Fatalf("pending assets = %d, want 1", len(pending))
	}
	immature := parseAssetBalance(b.Mine.Immature)
	if len(immature) != 0 {
		t.Fatalf("immature assets = %d, want 0", len(immature))
	}
}

func TestWalletBalancesDeserialization_RegtestFloat(t *testing.T) {
	// Regtest returns a plain float instead of a map
	data := `{
		"mine": {
			"trusted": 50.0,
			"untrusted_pending": 0,
			"immature": 0
		}
	}`
	var b walletBalances
	if err := json.Unmarshal([]byte(data), &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	trusted := parseAssetBalance(b.Mine.Trusted)
	if v := trusted["bitcoin"]; v != 50.0 {
		t.Errorf("bitcoin trusted = %f, want 50.0", v)
	}
}

func TestParseAssetBalance_NegativeValues(t *testing.T) {
	raw := json.RawMessage(`{"asset_a":-0.5,"asset_b":1.0}`)
	m := parseAssetBalance(raw)
	if v := m["asset_a"]; v != -0.5 {
		t.Errorf("asset_a = %f, want -0.5", v)
	}
	if v := m["asset_b"]; v != 1.0 {
		t.Errorf("asset_b = %f, want 1.0", v)
	}
}

func TestParseAssetBalance_NegativeFloat(t *testing.T) {
	raw := json.RawMessage(`-1.5`)
	m := parseAssetBalance(raw)
	if v := m["bitcoin"]; v != -1.5 {
		t.Errorf("bitcoin = %f, want -1.5", v)
	}
}

func TestWalletInfoDeserialization(t *testing.T) {
	data := `{"txcount": 42}`
	var info walletInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.TxCount != 42 {
		t.Errorf("TxCount = %d, want 42", info.TxCount)
	}
}

func TestUtxoDeserialization(t *testing.T) {
	data := `{"txid":"abc123","vout":0,"amount":1.5}`
	var u utxo
	if err := json.Unmarshal([]byte(data), &u); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u.TXID != "abc123" {
		t.Errorf("TXID = %q, want abc123", u.TXID)
	}
}

func TestBlockchainInfoDeserialization_ExtraFields(t *testing.T) {
	// Verify that unknown fields from newer node versions are silently ignored
	data := `{
		"blocks": 100,
		"headers": 100,
		"difficulty": 1.0,
		"verificationprogress": 1.0,
		"size_on_disk": 1000,
		"pruned": false,
		"chain": "regtest",
		"some_future_field": "should be ignored",
		"another_new_field": 42
	}`
	var info blockchainInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		t.Fatalf("unmarshal with extra fields should succeed: %v", err)
	}
	if info.Blocks != 100 {
		t.Errorf("Blocks = %d, want 100", info.Blocks)
	}
}

func TestListTxDeserialization_ConflictedTransaction(t *testing.T) {
	data := `{
		"txid": "conflict1",
		"category": "send",
		"amount": -1.0,
		"asset": "lbtc",
		"confirmations": -1
	}`
	var tx listTx
	if err := json.Unmarshal([]byte(data), &tx); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if tx.Confirmations != -1 {
		t.Errorf("Confirmations = %d, want -1", tx.Confirmations)
	}
}

func TestListTxDeserialization(t *testing.T) {
	data := `{
		"txid": "abc123",
		"category": "send",
		"amount": -0.5,
		"asset": "6f0279e9",
		"confirmations": 0
	}`
	var tx listTx
	if err := json.Unmarshal([]byte(data), &tx); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if tx.TXID != "abc123" {
		t.Errorf("TXID = %q, want abc123", tx.TXID)
	}
	if tx.Category != "send" {
		t.Errorf("Category = %q, want send", tx.Category)
	}
	if tx.Amount != -0.5 {
		t.Errorf("Amount = %f, want -0.5", tx.Amount)
	}
	if tx.Asset != "6f0279e9" {
		t.Errorf("Asset = %q, want 6f0279e9", tx.Asset)
	}
	if tx.Confirmations != 0 {
		t.Errorf("Confirmations = %d, want 0", tx.Confirmations)
	}
}
