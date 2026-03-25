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

import "encoding/json"

type blockchainInfo struct {
	Blocks               int64   `json:"blocks"`
	Headers              int64   `json:"headers"`
	Difficulty           float64 `json:"difficulty"`
	VerificationProgress float64 `json:"verificationprogress"`
	SizeOnDisk           int64   `json:"size_on_disk"`
	Pruned               bool    `json:"pruned"`
	Chain                string  `json:"chain"`
}

type mempoolInfo struct {
	Loaded        bool    `json:"loaded"`
	Size          int64   `json:"size"`
	Bytes         int64   `json:"bytes"`
	Usage         int64   `json:"usage"`
	MaxMempool    int64   `json:"maxmempool"`
	MempoolMinFee float64 `json:"mempoolminfee"`
	MinRelayTxFee float64 `json:"minrelaytxfee"`
}

type networkInfo struct {
	Connections    int64 `json:"connections"`
	ConnectionsIn  int64 `json:"connections_in"`
	ConnectionsOut int64 `json:"connections_out"`
}

type walletInfo struct {
	TxCount int64 `json:"txcount"`
}

// walletBalances: getbalances returns values per asset in Elements.
// On Elements Liquid: {"mine":{"trusted":{"6f0279...":1.5},"untrusted_pending":{...},"immature":{...}}}
// On Elements regtest it may be just a float64 for bitcoin.
type walletBalances struct {
	Mine struct {
		Trusted          json.RawMessage `json:"trusted"`
		UntrustedPending json.RawMessage `json:"untrusted_pending"`
		Immature         json.RawMessage `json:"immature"`
	} `json:"mine"`
}

// parseAssetBalance interprets the field as a map[asset]value or float64 (bitcoin).
func parseAssetBalance(raw json.RawMessage) map[string]float64 {
	out := make(map[string]float64)
	if len(raw) == 0 {
		return out
	}
	var m map[string]float64
	if err := json.Unmarshal(raw, &m); err == nil {
		return m
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		out["bitcoin"] = f
	}
	return out
}

// listTx represents a listtransactions entry.
type listTx struct {
	TXID          string  `json:"txid"`
	Category      string  `json:"category"` // send, receive, generate, immature
	Amount        float64 `json:"amount"`
	Asset         string  `json:"asset"`
	Confirmations int64   `json:"confirmations"`
	// Confirmations < 0 = conflicted; == 0 = in mempool; > 0 = confirmed
}

// utxo represents a listunspent entry (only the count is needed).
type utxo struct {
	TXID string `json:"txid"`
}
