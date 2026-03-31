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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rpcRequest struct {
	JSONRpc string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cacheEntry struct {
	data json.RawMessage
	ts   time.Time
}

type RPCClient struct {
	baseURL  string
	user     string
	password string
	http     *http.Client

	cacheMu      sync.Mutex
	cache        map[string]cacheEntry
	cacheRefresh time.Duration // min pause between RPC calls per key
	cacheTTL     time.Duration // max age before cached data is discarded
}

func NewRPCClient(url, user, pass string) *RPCClient {
	return NewRPCClientWithCache(url, user, pass, 15*time.Second, 60*time.Second)
}

func NewRPCClientWithCache(url, user, pass string, refresh, ttl time.Duration) *RPCClient {
	return &RPCClient{
		baseURL:      url,
		user:         user,
		password:     pass,
		http:         &http.Client{Timeout: 30 * time.Second},
		cache:        make(map[string]cacheEntry),
		cacheRefresh: refresh,
		cacheTTL:     ttl,
	}
}

// Call executes an RPC method. If wallet != "", calls /wallet/<name>.
func (c *RPCClient) Call(wallet, method string, params ...any) (json.RawMessage, error) {
	url := c.baseURL
	if wallet != "" {
		url = fmt.Sprintf("%s/wallet/%s", c.baseURL, wallet)
	}
	if params == nil {
		params = []any{}
	}

	body, _ := json.Marshal(rpcRequest{
		JSONRpc: "1.1",
		ID:      "elements-exporter",
		Method:  method,
		Params:  params,
	})

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.user, c.password)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RPC %s: %w", method, err)
	}
	defer resp.Body.Close()

	var r rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode RPC %s: %w", method, err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("RPC %s code=%d: %s", method, r.Error.Code, r.Error.Message)
	}
	return r.Result, nil
}

// CachedCall returns cached data when the cache is fresh (age < cacheRefresh).
// When the cache is stale but within the TTL (refresh <= age < cacheTTL), it
// attempts to refresh via RPC; on failure it returns the stale cached data.
// When the cache has expired (age >= cacheTTL) or is empty, it calls the node
// and returns an error if the call fails (metrics will be omitted).
func (c *RPCClient) CachedCall(wallet, method string, params ...any) (json.RawMessage, error) {
	key := cacheKey(wallet, method, params)

	c.cacheMu.Lock()
	entry, cached := c.cache[key]
	c.cacheMu.Unlock()

	if cached {
		age := time.Since(entry.ts)

		// Fresh: return without calling the node.
		if age < c.cacheRefresh {
			return entry.data, nil
		}

		// Stale but within TTL: try to refresh, fall back to cache on error.
		if age < c.cacheTTL {
			result, err := c.Call(wallet, method, params...)
			if err != nil {
				return entry.data, nil
			}
			c.cacheMu.Lock()
			c.cache[key] = cacheEntry{data: result, ts: time.Now()}
			c.cacheMu.Unlock()
			return result, nil
		}
	}

	// Expired or no cache: must call the node.
	result, err := c.Call(wallet, method, params...)
	if err != nil {
		return nil, err
	}

	c.cacheMu.Lock()
	c.cache[key] = cacheEntry{data: result, ts: time.Now()}
	c.cacheMu.Unlock()

	return result, nil
}

func cacheKey(wallet, method string, params []any) string {
	var b strings.Builder
	b.WriteString(wallet)
	b.WriteByte('/')
	b.WriteString(method)
	for _, p := range params {
		b.WriteByte('/')
		fmt.Fprint(&b, p)
	}
	return b.String()
}
