package main

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestInvalidSensorsRemainUnavailable(t *testing.T) {
	for _, n := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if finite(n) != nil {
			t.Fatal("invalid sensor escaped into JSON")
		}
	}
	b, e := json.Marshal(Sample{"temperature": finite(math.NaN())})
	if e != nil || string(b) != "{\"temperature\":null}" {
		t.Fatalf("bad unavailable value: %s %v", b, e)
	}
}
func TestFrameLimitBeforeSerialWrite(t *testing.T) {
	d := &Device{}
	if e := d.Send(Sample{"payload": strings.Repeat("x", 8192)}); e == nil || !strings.Contains(e.Error(), "frame limit") {
		t.Fatalf("oversized packet accepted: %v", e)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestAIProviderOnlyReportsLoadedModels(t *testing.T) {
	calls := 0
	p := AIProvider{client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() != "127.0.0.1" {
			t.Fatal("AI provider attempted an external request")
		}
		calls++
		body := "{}"
		if r.URL.Path == "/api/v0/models" {
			body = `{"data":[{"id":"unloaded-model","state":"not-loaded"},{"id":"active-model","state":"loaded"}]}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	s := Sample{}
	p.Collect(context.Background(), s)
	if calls != 3 || s["ai_model"] != "active-model" || s["ai_provider"] != "LM STUDIO" {
		t.Fatalf("incorrect loaded model: %#v (%d calls)", s, calls)
	}
	if _, ok := s["ai_tps"]; ok {
		t.Fatal("throughput must not be fabricated")
	}
}
