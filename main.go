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

// elements-exporter: Prometheus exporter for Elements (Blockstream) nodes.
// Exposes wallet, mempool (local unconfirmed TXs), and blockchain metrics.
//
// Usage:
//
//	./elements-exporter -rpc.url=http://localhost:7041 \
//	                    -rpc.user=admin -rpc.password=secret \
//	                    -web.listen-address=:9101
//
// Or via environment variables:
//
//	ELEMENTS_RPC_URL=http://localhost:7041 \
//	ELEMENTS_RPC_USER=admin \
//	ELEMENTS_RPC_PASSWORD=secret \
//	./elements-exporter

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Set by -ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

type config struct {
	rpcURL       string
	rpcUser      string
	rpcPass      string
	listenAddr   string
	metricsPath  string
	cacheRefresh time.Duration
	cacheTTL     time.Duration
	showVersion  bool
}

// envOrDefault returns the value of the environment variable named by key,
// or fallback if the variable is not set or empty.
func parseDurationOrDefault(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseConfig() config {
	cfg := config{}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "elements-exporter %s (%s, %s)\n", version, commit, date)
		fmt.Fprintf(os.Stderr, "Prometheus exporter for Elements (Blockstream) nodes.\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  elements-exporter [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nAll flags can be set via environment variables (flags take precedence):\n")
		fmt.Fprintf(os.Stderr, "  ELEMENTS_RPC_URL, ELEMENTS_RPC_USER, ELEMENTS_RPC_PASSWORD,\n")
		fmt.Fprintf(os.Stderr, "  ELEMENTS_LISTEN_ADDRESS, ELEMENTS_METRICS_PATH\n")
	}

	flag.BoolVar(&cfg.showVersion, "version", false, "Print version and exit")
	flag.StringVar(&cfg.rpcURL, "rpc.url", envOrDefault("ELEMENTS_RPC_URL", "http://localhost:7041"), "Elements node RPC URL (env: ELEMENTS_RPC_URL)")
	flag.StringVar(&cfg.rpcUser, "rpc.user", os.Getenv("ELEMENTS_RPC_USER"), "RPC username (env: ELEMENTS_RPC_USER)")
	flag.StringVar(&cfg.rpcPass, "rpc.password", envOrDefault("ELEMENTS_RPC_PASSWORD", os.Getenv("ELEMENTS_RPC_PASS")), "RPC password (env: ELEMENTS_RPC_PASSWORD)")
	flag.StringVar(&cfg.listenAddr, "web.listen-address", envOrDefault("ELEMENTS_LISTEN_ADDRESS", ":9101"), "Address to serve metrics (env: ELEMENTS_LISTEN_ADDRESS)")
	flag.StringVar(&cfg.metricsPath, "web.telemetry-path", envOrDefault("ELEMENTS_METRICS_PATH", "/metrics"), "Metrics endpoint path (env: ELEMENTS_METRICS_PATH)")
	flag.DurationVar(&cfg.cacheRefresh, "cache.refresh", parseDurationOrDefault("ELEMENTS_CACHE_REFRESH", 15*time.Second), "Min pause between RPC calls per method (env: ELEMENTS_CACHE_REFRESH)")
	flag.DurationVar(&cfg.cacheTTL, "cache.ttl", parseDurationOrDefault("ELEMENTS_CACHE_TTL", 60*time.Second), "Max age of cached data before metrics are omitted (env: ELEMENTS_CACHE_TTL)")
	flag.Parse()

	return cfg
}

func main() {
	cfg := parseConfig()

	if cfg.showVersion {
		fmt.Printf("elements-exporter %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	if cfg.rpcUser == "" || cfg.rpcPass == "" {
		slog.Error("RPC credentials required: use flags -rpc.user/-rpc.password or env vars ELEMENTS_RPC_USER/ELEMENTS_RPC_PASSWORD")
		os.Exit(1)
	}

	rpc := NewRPCClientWithCache(cfg.rpcURL, cfg.rpcUser, cfg.rpcPass, cfg.cacheRefresh, cfg.cacheTTL)
	col := NewElementsCollector(rpc)
	prometheus.MustRegister(col)

	mux := http.NewServeMux()
	mux.Handle(cfg.metricsPath, promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, err := rpc.Call("", "getblockchaininfo")
		if err != nil {
			http.Error(w, "node unreachable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><head><title>Elements Exporter</title></head><body>
<h1>Elements Prometheus Exporter</h1>
<ul>
  <li><a href="%s">Metrics</a></li>
  <li><a href="/healthz">Health check</a></li>
</ul>
</body></html>`, cfg.metricsPath)
	})

	slog.Info("elements-exporter started",
		"version", version,
		"commit", commit,
		"built", date,
		"listen", cfg.listenAddr,
		"metrics", cfg.metricsPath,
	)
	if err := http.ListenAndServe(cfg.listenAddr, mux); err != nil {
		slog.Error("http server failed", "error", err)
		os.Exit(1)
	}
}
