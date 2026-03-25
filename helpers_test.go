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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestBoolToFloat(t *testing.T) {
	if v := boolToFloat(true); v != 1 {
		t.Errorf("boolToFloat(true) = %f, want 1", v)
	}
	if v := boolToFloat(false); v != 0 {
		t.Errorf("boolToFloat(false) = %f, want 0", v)
	}
}

func TestGauge(t *testing.T) {
	desc := prometheus.NewDesc("test_gauge", "test", nil, nil)
	ch := make(chan prometheus.Metric, 1)
	gauge(ch, desc, 42.5)

	m := <-ch
	var d dto.Metric
	if err := m.Write(&d); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	if v := d.GetGauge().GetValue(); v != 42.5 {
		t.Errorf("gauge value = %f, want 42.5", v)
	}
}

func TestGaugeL(t *testing.T) {
	desc := prometheus.NewDesc("test_gauge_labels", "test", []string{"wallet", "asset"}, nil)
	ch := make(chan prometheus.Metric, 1)
	gaugeL(ch, desc, 1.23, "w1", "bitcoin")

	m := <-ch
	var d dto.Metric
	if err := m.Write(&d); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	if v := d.GetGauge().GetValue(); v != 1.23 {
		t.Errorf("gauge value = %f, want 1.23", v)
	}
	labels := d.GetLabel()
	if len(labels) != 2 {
		t.Fatalf("label count = %d, want 2", len(labels))
	}
	// Labels are sorted alphabetically by prometheus
	found := map[string]string{}
	for _, l := range labels {
		found[l.GetName()] = l.GetValue()
	}
	if found["wallet"] != "w1" {
		t.Errorf("wallet label = %q, want w1", found["wallet"])
	}
	if found["asset"] != "bitcoin" {
		t.Errorf("asset label = %q, want bitcoin", found["asset"])
	}
}

func TestScrapeHealth(t *testing.T) {
	descOK := prometheus.NewDesc("test_ok", "test", []string{"collector"}, nil)
	descDur := prometheus.NewDesc("test_dur", "test", []string{"collector"}, nil)
	ch := make(chan prometheus.Metric, 2)

	start := time.Now().Add(-100 * time.Millisecond)
	scrapeHealth(ch, descOK, descDur, "blockchain", 1.0, start)

	// First metric: ok value
	m1 := <-ch
	var d1 dto.Metric
	m1.Write(&d1)
	if v := d1.GetGauge().GetValue(); v != 1.0 {
		t.Errorf("ok value = %f, want 1.0", v)
	}
	if l := d1.GetLabel()[0].GetValue(); l != "blockchain" {
		t.Errorf("collector label = %q, want blockchain", l)
	}

	// Second metric: duration
	m2 := <-ch
	var d2 dto.Metric
	m2.Write(&d2)
	dur := d2.GetGauge().GetValue()
	if dur < 0.05 {
		t.Errorf("duration = %f, expected >= 0.05s", dur)
	}
}
