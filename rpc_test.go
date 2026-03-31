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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewRPCClient(t *testing.T) {
	c := NewRPCClient("http://localhost:7041", "admin", "secret")
	if c.baseURL != "http://localhost:7041" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://localhost:7041")
	}
	if c.user != "admin" {
		t.Errorf("user = %q, want %q", c.user, "admin")
	}
	if c.password != "secret" {
		t.Errorf("password = %q, want %q", c.password, "secret")
	}
	if c.http == nil {
		t.Fatal("http client is nil")
	}
}

func TestRPCCall_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q, want application/json", ct)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			t.Errorf("basic auth = (%q, %q, %v), want (admin, secret, true)", user, pass, ok)
		}

		body, _ := io.ReadAll(r.Body)
		var req rpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		if req.Method != "getblockchaininfo" {
			t.Errorf("rpc method = %q, want getblockchaininfo", req.Method)
		}
		if req.JSONRpc != "1.1" {
			t.Errorf("jsonrpc = %q, want 1.1", req.JSONRpc)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":{"blocks":100},"error":null}`))
	}))
	defer srv.Close()

	c := NewRPCClient(srv.URL, "admin", "secret")
	result, err := c.Call("", "getblockchaininfo")
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}

	var data struct {
		Blocks int `json:"blocks"`
	}
	if err := json.Unmarshal(result, &data); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if data.Blocks != 100 {
		t.Errorf("blocks = %d, want 100", data.Blocks)
	}
}

func TestRPCCall_WalletURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"result":"ok","error":null}`))
	}))
	defer srv.Close()

	c := NewRPCClient(srv.URL, "u", "p")

	// Without wallet
	c.Call("", "getinfo")
	if gotPath != "/" {
		t.Errorf("path without wallet = %q, want /", gotPath)
	}

	// With wallet
	c.Call("mywallet", "getbalances")
	if gotPath != "/wallet/mywallet" {
		t.Errorf("path with wallet = %q, want /wallet/mywallet", gotPath)
	}
}

func TestRPCCall_RPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":null,"error":{"code":-1,"message":"Method not found"}}`))
	}))
	defer srv.Close()

	c := NewRPCClient(srv.URL, "u", "p")
	_, err := c.Call("", "badmethod")
	if err == nil {
		t.Fatal("expected error for RPC error response")
	}
	if !strings.Contains(err.Error(), "code=-1") {
		t.Errorf("error %q should contain 'code=-1'", err.Error())
	}
	if !strings.Contains(err.Error(), "Method not found") {
		t.Errorf("error %q should contain 'Method not found'", err.Error())
	}
}

func TestRPCCall_NetworkError(t *testing.T) {
	c := NewRPCClient("http://127.0.0.1:1", "u", "p") // port 1 = won't connect
	_, err := c.Call("", "getinfo")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestRPCCall_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := NewRPCClient(srv.URL, "u", "p")
	_, err := c.Call("", "getinfo")
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "decode RPC") {
		t.Errorf("error %q should contain 'decode RPC'", err.Error())
	}
}

func TestRPCCall_NilParams(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"result":true,"error":null}`))
	}))
	defer srv.Close()

	c := NewRPCClient(srv.URL, "u", "p")
	// Call with no params — should serialize as empty array, not null
	c.Call("", "getinfo")

	var req rpcRequest
	json.Unmarshal(gotBody, &req)
	raw, _ := json.Marshal(req.Params)
	if string(raw) != "[]" {
		t.Errorf("params = %s, want []", raw)
	}
}

func TestRPCCall_WithParams(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"result":true,"error":null}`))
	}))
	defer srv.Close()

	c := NewRPCClient(srv.URL, "u", "p")
	c.Call("w1", "listtransactions", "*", 500, 0, true)

	var req rpcRequest
	json.Unmarshal(gotBody, &req)
	raw, _ := json.Marshal(req.Params)
	if !strings.Contains(string(raw), `"*"`) {
		t.Errorf("params %s should contain '*'", raw)
	}
}

func TestRPCCall_HTTPNon200(t *testing.T) {
	// Server returns 401 with HTML body — should produce a decode error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`<html>Unauthorized</html>`))
	}))
	defer srv.Close()

	c := NewRPCClient(srv.URL, "u", "p")
	_, err := c.Call("", "getinfo")
	if err == nil {
		t.Fatal("expected error for HTTP 401 response")
	}
	if !strings.Contains(err.Error(), "decode RPC") {
		t.Errorf("error %q should contain 'decode RPC'", err.Error())
	}
}

func TestRPCCall_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty body
	}))
	defer srv.Close()

	c := NewRPCClient(srv.URL, "u", "p")
	_, err := c.Call("", "getinfo")
	if err == nil {
		t.Fatal("expected error for empty response body")
	}
}

func TestCachedCall_ReturnsCachedWithinRefresh(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(fmt.Sprintf(`{"result":%d,"error":null}`, calls)))
	}))
	defer srv.Close()

	c := NewRPCClientWithCache(srv.URL, "u", "p", 1*time.Second, 5*time.Second)

	r1, err := c.CachedCall("w1", "listunspent", 1, 9999999)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	r2, err := c.CachedCall("w1", "listunspent", 1, 9999999)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if string(r1) != string(r2) {
		t.Errorf("expected cached result %s, got %s", r1, r2)
	}
	if calls != 1 {
		t.Errorf("expected 1 RPC call, got %d", calls)
	}
}

func TestCachedCall_RefreshesAfterRefreshInterval(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(fmt.Sprintf(`{"result":%d,"error":null}`, calls)))
	}))
	defer srv.Close()

	c := NewRPCClientWithCache(srv.URL, "u", "p", 10*time.Millisecond, 5*time.Second)

	_, err := c.CachedCall("w1", "listunspent")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	r2, err := c.CachedCall("w1", "listunspent")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if string(r2) != "2" {
		t.Errorf("expected refreshed result '2', got %s", r2)
	}
	if calls != 2 {
		t.Errorf("expected 2 RPC calls, got %d", calls)
	}
}

func TestCachedCall_StaleButWithinTTL_FallsBackOnError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"result":"cached_value","error":null}`))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := NewRPCClientWithCache(srv.URL, "u", "p", 10*time.Millisecond, 5*time.Second)

	r1, err := c.CachedCall("w1", "method")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if string(r1) != `"cached_value"` {
		t.Fatalf("expected cached_value, got %s", r1)
	}

	time.Sleep(20 * time.Millisecond)

	// Refresh fails, but TTL hasn't expired — should return stale cache.
	r2, err := c.CachedCall("w1", "method")
	if err != nil {
		t.Fatalf("expected no error with stale cache fallback, got: %v", err)
	}
	if string(r2) != `"cached_value"` {
		t.Errorf("expected stale cached_value, got %s", r2)
	}
}

func TestCachedCall_ExpiredTTL_ReturnsError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"result":"old","error":null}`))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := NewRPCClientWithCache(srv.URL, "u", "p", 5*time.Millisecond, 10*time.Millisecond)

	_, err := c.CachedCall("w1", "method")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	// Both refresh and TTL expired, RPC fails — should return error.
	_, err = c.CachedCall("w1", "method")
	if err == nil {
		t.Fatal("expected error when cache expired and RPC fails")
	}
}

func TestCachedCall_DifferentKeysNotShared(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(fmt.Sprintf(`{"result":%d,"error":null}`, calls)))
	}))
	defer srv.Close()

	c := NewRPCClientWithCache(srv.URL, "u", "p", 1*time.Second, 5*time.Second)

	c.CachedCall("w1", "listunspent")
	c.CachedCall("w2", "listunspent")
	c.CachedCall("w1", "listtransactions")

	if calls != 3 {
		t.Errorf("expected 3 RPC calls for different keys, got %d", calls)
	}
}
