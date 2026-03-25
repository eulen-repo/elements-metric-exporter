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
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// /healthz endpoint
// ---------------------------------------------------------------------------

func TestHealthz_OK(t *testing.T) {
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":{"blocks":1},"error":null}`))
	}))
	defer node.Close()

	rpc := NewRPCClient(node.URL, "u", "p")

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := rpc.Call("", "getblockchaininfo")
		if err != nil {
			http.Error(w, "node unreachable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	if body != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}

func TestHealthz_NodeDown(t *testing.T) {
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":null,"error":{"code":-28,"message":"Loading block index..."}}`))
	}))
	defer node.Close()

	rpc := NewRPCClient(node.URL, "u", "p")

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := rpc.Call("", "getblockchaininfo")
		if err != nil {
			http.Error(w, "node unreachable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "node unreachable") {
		t.Errorf("body = %q, should contain 'node unreachable'", rec.Body.String())
	}
}

func TestHealthz_NetworkError(t *testing.T) {
	rpc := NewRPCClient("http://127.0.0.1:1", "u", "p")

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := rpc.Call("", "getblockchaininfo")
		if err != nil {
			http.Error(w, "node unreachable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// / landing page
// ---------------------------------------------------------------------------

func TestLandingPage(t *testing.T) {
	metricsPath := "/metrics"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><head><title>Elements Exporter</title></head><body>
<h1>Elements Prometheus Exporter</h1>
<ul>
  <li><a href="%s">Metrics</a></li>
  <li><a href="/healthz">Health check</a></li>
</ul>
</body></html>`, metricsPath)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Elements Prometheus Exporter") {
		t.Error("landing page should contain title")
	}
	if !strings.Contains(body, `href="/metrics"`) {
		t.Error("landing page should link to /metrics")
	}
	if !strings.Contains(body, `href="/healthz"`) {
		t.Error("landing page should link to /healthz")
	}
}

// ---------------------------------------------------------------------------
// envOrDefault helper
// ---------------------------------------------------------------------------

func TestEnvOrDefault_Set(t *testing.T) {
	t.Setenv("TEST_ELEMENTS_VAR", "fromenv")
	if v := envOrDefault("TEST_ELEMENTS_VAR", "fallback"); v != "fromenv" {
		t.Errorf("envOrDefault = %q, want fromenv", v)
	}
}

func TestEnvOrDefault_Unset(t *testing.T) {
	os.Unsetenv("TEST_ELEMENTS_MISSING")
	if v := envOrDefault("TEST_ELEMENTS_MISSING", "fallback"); v != "fallback" {
		t.Errorf("envOrDefault = %q, want fallback", v)
	}
}

func TestEnvOrDefault_Empty(t *testing.T) {
	t.Setenv("TEST_ELEMENTS_EMPTY", "")
	if v := envOrDefault("TEST_ELEMENTS_EMPTY", "fallback"); v != "fallback" {
		t.Errorf("envOrDefault = %q, want fallback (empty string should use default)", v)
	}
}

// ---------------------------------------------------------------------------
// Env var config: all options
// ---------------------------------------------------------------------------

func TestEnvVarConfig_RPCURL(t *testing.T) {
	t.Setenv("ELEMENTS_RPC_URL", "http://custom:1234")
	v := envOrDefault("ELEMENTS_RPC_URL", "http://localhost:7041")
	if v != "http://custom:1234" {
		t.Errorf("RPC URL = %q, want http://custom:1234", v)
	}
}

func TestEnvVarConfig_RPCPassword(t *testing.T) {
	// ELEMENTS_RPC_PASSWORD is the primary env var
	t.Setenv("ELEMENTS_RPC_PASSWORD", "newpass")
	os.Unsetenv("ELEMENTS_RPC_PASS")
	v := envOrDefault("ELEMENTS_RPC_PASSWORD", os.Getenv("ELEMENTS_RPC_PASS"))
	if v != "newpass" {
		t.Errorf("RPC password = %q, want newpass", v)
	}
}

func TestEnvVarConfig_RPCPasswordLegacyFallback(t *testing.T) {
	// ELEMENTS_RPC_PASS works as fallback when ELEMENTS_RPC_PASSWORD is not set
	os.Unsetenv("ELEMENTS_RPC_PASSWORD")
	t.Setenv("ELEMENTS_RPC_PASS", "legacypass")
	v := envOrDefault("ELEMENTS_RPC_PASSWORD", os.Getenv("ELEMENTS_RPC_PASS"))
	if v != "legacypass" {
		t.Errorf("RPC password = %q, want legacypass (legacy fallback)", v)
	}
}

func TestEnvVarConfig_ListenAddress(t *testing.T) {
	t.Setenv("ELEMENTS_LISTEN_ADDRESS", ":8080")
	v := envOrDefault("ELEMENTS_LISTEN_ADDRESS", ":9101")
	if v != ":8080" {
		t.Errorf("listen address = %q, want :8080", v)
	}
}

func TestEnvVarConfig_MetricsPath(t *testing.T) {
	t.Setenv("ELEMENTS_METRICS_PATH", "/custom-metrics")
	v := envOrDefault("ELEMENTS_METRICS_PATH", "/metrics")
	if v != "/custom-metrics" {
		t.Errorf("metrics path = %q, want /custom-metrics", v)
	}
}

// ---------------------------------------------------------------------------
// Build version variables exist and have defaults
// ---------------------------------------------------------------------------

func TestVersionDefaults(t *testing.T) {
	if version == "" {
		t.Error("version should have a default value")
	}
	if commit == "" {
		t.Error("commit should have a default value")
	}
	if date == "" {
		t.Error("date should have a default value")
	}
}

// ---------------------------------------------------------------------------
// Regression: RPC request ID is always "elements-exporter"
// ---------------------------------------------------------------------------

func TestRPCRequestID(t *testing.T) {
	var gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req rpcRequest
		json.Unmarshal(body, &req)
		gotID = req.ID
		w.Write([]byte(`{"result":true,"error":null}`))
	}))
	defer srv.Close()

	c := NewRPCClient(srv.URL, "u", "p")
	c.Call("", "getinfo")

	if gotID != "elements-exporter" {
		t.Errorf("request ID = %q, want elements-exporter", gotID)
	}
}
