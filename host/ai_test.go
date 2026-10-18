package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func aiFixture(engines []aiEngine, responses map[string]string) *AIProvider {
	d := &aiDiscovery{engines: engines, at: time.Now()}
	return &AIProvider{discovery: d, client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		body, ok := responses[r.URL.Host+r.URL.Path]
		code := 404
		if ok {
			code = 200
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
}
func TestAIDiscoveryKnownEnginesAndCustomPorts(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		kind  string
		port  int
		model string
	}{
		{"llama-server.exe", []string{"llama-server", "--port", "52791", "-m", `D:\Models\qwen.gguf`}, "LLAMA.CPP", 52791, "qwen.gguf"},
		{"python.exe", []string{"python", `D:\Maestro\portable\backend.py`}, "MAESTRO", 7860, ""},
		{"python", []string{"python", "-m", "vllm.entrypoints.openai.api_server", "--model", "org/model", "--port=8089"}, "VLLM", 8089, "org/model"},
		{"vllm", []string{"vllm", "serve", "org/model"}, "VLLM", 8000, "org/model"},
		{"python", []string{"python", "/opt/bin/vllm", "serve", "org/model"}, "VLLM", 8000, "org/model"},
		{"python", []string{"python", "-m", "sglang.launch_server", "--model-path", "org/qwen"}, "SGLANG", 30000, "org/qwen"},
		{"python3", []string{"python3", "-m", "mlx_lm.server", "--model", "org/mlx"}, "MLX LM", 8080, "org/mlx"},
		{"koboldcpp_cuda.exe", []string{"koboldcpp_cuda", "--port", "5002"}, "KOBOLDCPP", 5002, ""},
		{"text-generation-launcher", nil, "TGI", 8000, ""},
		{"python.exe", []string{"python", "unrelated.py"}, "", 0, ""},
		{"Codex.exe", []string{"Codex"}, "", 0, ""},
	}
	for _, tt := range tests {
		e := aiClassify(tt.name, tt.args)
		if e.kind != tt.kind || e.port != tt.port || e.model != tt.model {
			t.Errorf("%s: %#v", tt.name, e)
		}
	}
}
func TestMaestroGenerationWithUnloadedWriter(t *testing.T) {
	p := aiFixture([]aiEngine{{kind: "MAESTRO", pid: 1, port: 7860, ram: 6, cpu: 3, ramKnown: true, cpuKnown: true}}, map[string]string{
		"127.0.0.1:7860/api/v1/system-stats": `{"model":{"name":"Local video model","loaded":true},"gpu_memory_used":99}`,
		"127.0.0.1:7860/api/v1/jobs":         `{"jobs":[{"kind":"generation","status":"running"},{"kind":"generation","status":"queued"}]}`,
		"127.0.0.1:11434/api/ps":             `{"models":[{"name":"Resident Ollama","size_vram":123}]}`,
	})
	s := Sample{}
	p.Collect(context.Background(), s)
	if s["ai_state"] != "ACTIVE" || s["ai_provider"] != "MAESTRO" || s["ai_model"] != "Local video model" || s["ai_queue"] != 1 || s["ai_ram"] != float64(6) {
		t.Fatal(s)
	}
	if _, ok := s["ai_vram"]; ok {
		t.Fatal("global GPU memory attributed to Maestro")
	}
	if _, ok := s["ai_tps"]; ok {
		t.Fatal("token rate invented for image generation")
	}
}
func TestRemoteMaestroWriterExcluded(t *testing.T) {
	p := aiFixture([]aiEngine{{kind: "MAESTRO", pid: 1, port: 7860}}, map[string]string{
		"127.0.0.1:7860/api/v1/system-stats": `{"model":{"loaded":false}}`,
		"127.0.0.1:7860/api/v1/jobs":         `{"jobs":[]}`,
		"127.0.0.1:7860/api/v1/llm/status":   `{"provider":"openai","loaded":true,"model_id":"cloud-model"}`,
	})
	s := Sample{}
	p.Collect(context.Background(), s)
	if s["ai_state"] != "IDLE" || s["ai_model"] != nil {
		t.Fatal(s)
	}
}
func TestLlamaCustomPortActiveIdleAndMissingSlots(t *testing.T) {
	replies := map[string]string{"127.0.0.1:52981/slots": `[{"id":0,"id_task":9,"n_ctx":4096,"is_processing":true,"next_token":{"n_decoded":50}}]`}
	p := aiFixture([]aiEngine{{kind: "LLAMA.CPP", pid: 1, port: 52981, model: "qwen.gguf"}}, replies)
	p.counters = map[string]aiCounter{"http://127.0.0.1:52981/slot/0/task/9": {value: 30, at: time.Now().Add(-time.Second)}}
	s := Sample{}
	p.Collect(context.Background(), s)
	if s["ai_state"] != "ACTIVE" || s["ai_model"] != "qwen.gguf" || s["ai_tps"].(float64) < 19 {
		t.Fatal(s)
	}
	if _, ok := s["ai_context"]; ok {
		t.Fatal("decoded output is not KV occupancy")
	}
	replies["127.0.0.1:52981/slots"] = `[{"is_processing":false}]`
	s = Sample{}
	p.Collect(context.Background(), s)
	if s["ai_state"] != "IDLE" || s["ai_tps"] != nil {
		t.Fatal(s)
	}
	delete(replies, "127.0.0.1:52981/slots")
	s = Sample{}
	p.Collect(context.Background(), s)
	if s["ai_state"] != "WORKER" {
		t.Fatal("missing slots must not mean idle", s)
	}
}
func TestAIUnavailableAndStaleDiscovery(t *testing.T) {
	p := aiFixture([]aiEngine{{kind: "LLAMA.CPP", pid: 1, port: 52981}}, nil)
	p.discovery.at = time.Now().Add(-time.Minute)
	s := Sample{}
	p.Collect(context.Background(), s)
	if s["ai_state"] != "NO SOURCE" {
		t.Fatal(s)
	}
}
func TestAIEngineMetricsAndCounterReset(t *testing.T) {
	for _, kind := range []string{"VLLM", "SGLANG"} {
		running, queue, tokens, usage := "vllm:num_requests_running", "vllm:num_requests_waiting", "vllm:generation_tokens_total", "vllm:kv_cache_usage_perc"
		if kind == "SGLANG" {
			running, queue, tokens, usage = "sglang:num_running_reqs", "sglang:num_queue_reqs", "sglang:generation_tokens_total", "sglang:token_usage"
		}
		metricBody := running + "{model_name=\"m\"} 2\n" + queue + " 3\n" + tokens + " 100\n" + usage + " 0.5\ninvalid NaN\nbad +Inf\n"
		replies := map[string]string{"127.0.0.1:8000/metrics": metricBody, "127.0.0.1:8000/v1/models": `{"data":[{"id":"m"}]}`}
		p := aiFixture([]aiEngine{{kind: kind, pid: 1, port: 8000}}, replies)
		p.counters = map[string]aiCounter{"http://127.0.0.1:8000": {value: 50, at: time.Now().Add(-time.Second)}}
		s := Sample{}
		p.Collect(context.Background(), s)
		if s["ai_state"] != "ACTIVE" || s["ai_context"] != float64(50) || s["ai_queue"] != float64(3) || s["ai_tps"].(float64) < 49 {
			t.Fatal(kind, s)
		}
		replies["127.0.0.1:8000/metrics"] = strings.Replace(metricBody, tokens+" 100", tokens+" 1", 1)
		s = Sample{}
		p.Collect(context.Background(), s)
		if s["ai_tps"] != nil {
			t.Fatal("counter reset created a bogus token rate", s)
		}
	}
}
func TestAILocalHTTPBoundsAndRedirects(t *testing.T) {
	remoteCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { remoteCalls++; w.Write([]byte(`{}`)) }))
	defer target.Close()
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big" {
			w.Write([]byte(strings.Repeat(" ", 65537)))
			return
		}
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer local.Close()
	p := newAIProvider(nil)
	for _, endpoint := range []string{local.URL, local.URL + "/big", "http://example.com/status"} {
		if _, ok := p.read(context.Background(), endpoint); ok {
			t.Fatal("unsafe response accepted", endpoint)
		}
	}
	if remoteCalls != 0 {
		t.Fatal("followed a redirect")
	}
	if tr := p.client.Transport.(*http.Transport); tr.Proxy != nil {
		t.Fatal("local telemetry inherited a proxy")
	}
	if n, _ := strconv.Atoi(strings.TrimPrefix(local.URL, "http://127.0.0.1:")); n == 0 {
		t.Fatal("fixture not on loopback")
	}
}
