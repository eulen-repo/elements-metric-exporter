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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
