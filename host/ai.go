package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type AIProvider struct {
	client    *http.Client
	discovery *aiDiscovery
	nextProbe map[string]time.Time
	counters  map[string]aiCounter
}
type aiCounter struct {
	value float64
	at    time.Time
}

func newAIProvider(d *aiDiscovery) *AIProvider {
	// Local telemetry never uses an HTTP proxy or follows a redirect.
	return &AIProvider{discovery: d, client: &http.Client{Timeout: 300 * time.Millisecond,
		Transport:     &http.Transport{Proxy: nil, MaxIdleConns: 8, MaxIdleConnsPerHost: 1, IdleConnTimeout: time.Minute},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (p *AIProvider) read(ctx context.Context, endpoint string) ([]byte, bool) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil {
		return nil, false
	}
	req, e := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if e != nil {
		return nil, false
	}
	r, e := p.client.Do(req)
	if e != nil {
		return nil, false
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, false
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 65537))
	return b, e == nil && len(b) <= 65536
}
func (p *AIProvider) get(ctx context.Context, endpoint string, out any) bool {
	b, ok := p.read(ctx, endpoint)
	return ok && json.Unmarshal(b, out) == nil
}
func aiRank(s Sample) int {
	switch s["ai_state"] {
	case "ACTIVE":
		return 5
	case "QUEUED":
		return 4
	case "LOADED":
		return 3
	case "WORKER":
		return 2
	case "IDLE":
		return 1
	}
	return 0
}
func (p *AIProvider) Collect(ctx context.Context, s Sample) {
	if p.nextProbe == nil {
		p.nextProbe = map[string]time.Time{}
	}
	if p.counters == nil {
		p.counters = map[string]aiCounter{}
	}
	candidates := p.discovery.snapshot()
	// Common defaults also cover servers hidden inside a container. Custom ports
	// come from recognized local workers, never from a port-range scan.
	candidates = append(candidates, aiEngine{kind: "OLLAMA", port: 11434}, aiEngine{kind: "LM STUDIO", port: 1234}, aiEngine{kind: "KOBOLDCPP", port: 5001})
	best := Sample{"ai_state": "NO SOURCE", "ai_note": "No local AI source detected"}
	seen := map[string]bool{}
	for _, engine := range candidates {
		if ctx.Err() != nil {
			break
		}
		base := "http://127.0.0.1:" + strconv.Itoa(engine.port)
		key := engine.kind + base
		if seen[key] {
			continue
		}
		seen[key] = true
		if engine.pid == 0 && time.Now().Before(p.nextProbe[key]) {
			continue
		}
		data := Sample{"ai_provider": engine.kind, "ai_state": "WORKER", "ai_note": "Activity unavailable"}
		if engine.model != "" {
			data["ai_model"] = engine.model
		}
		if engine.ramKnown {
			data["ai_ram"] = engine.ram
		}
		if engine.cpuKnown {
			data["ai_cpu"] = finite(engine.cpu)
		}
		available := false
		switch engine.kind {
		case "LLAMA.CPP":
			available = p.llama(ctx, base, data)
		case "VLLM", "SGLANG", "TGI":
			available = p.metrics(ctx, base, engine.kind, data)
		case "MAESTRO":
			available = p.maestro(ctx, base, data)
		case "OLLAMA":
			var body struct {
				Models []struct {
					Name     string `json:"name"`
					SizeVRAM uint64 `json:"size_vram"`
				} `json:"models"`
			}
			available = p.get(ctx, base+"/api/ps", &body) && body.Models != nil
			if available {
				data["ai_state"] = "IDLE"
				data["ai_note"] = "No loaded models"
				if len(body.Models) > 0 {
					data["ai_model"] = body.Models[0].Name
					data["ai_vram"] = gib(body.Models[0].SizeVRAM)
					data["ai_state"] = "LOADED"
					data["ai_note"] = "Resident; activity unavailable"
				}
			}
		case "LM STUDIO":
			var body struct {
				Data []struct {
					ID    string `json:"id"`
					State string `json:"state"`
				} `json:"data"`
			}
			available = p.get(ctx, base+"/api/v0/models", &body) && body.Data != nil
			if available {
				data["ai_state"] = "IDLE"
				data["ai_note"] = "No loaded models"
				for _, m := range body.Data {
					if m.State == "loaded" {
						data["ai_model"] = m.ID
						data["ai_state"] = "LOADED"
						data["ai_note"] = "Loaded; activity unavailable"
						break
					}
				}
			}
		case "KOBOLDCPP":
			var body struct {
				Result string `json:"result"`
			}
			available = p.get(ctx, base+"/api/v1/model", &body) && body.Result != ""
			if available && body.Result != "" {
				data["ai_model"] = body.Result
				data["ai_state"] = "LOADED"
				data["ai_note"] = "Loaded; activity unavailable"
			}
		case "MLX LM":
			available = p.models(ctx, base, data)
			if available {
				data["ai_state"] = "LOADED"
				data["ai_note"] = "Loaded; activity unavailable"
			}
		}
		if !available && engine.pid == 0 {
			p.nextProbe[key] = time.Now().Add(30 * time.Second)
			continue
		}
		if aiRank(data) > aiRank(best) {
			best = data
		}
	}
	// Bound housekeeping even if a launcher creates a new ephemeral port per job.
	for k, at := range p.nextProbe {
		if time.Since(at) > time.Minute {
			delete(p.nextProbe, k)
		}
	}
	for k, v := range p.counters {
		if time.Since(v.at) > time.Minute {
			delete(p.counters, k)
		}
	}
	for k, v := range best {
		if text, ok := v.(string); ok {
			text = strings.NewReplacer("—", "-", "–", "-", "’", "'").Replace(text)
			if len(text) > 128 {
				text = string([]rune(text)[:min(len([]rune(text)), 80)])
			}
			v = text
		}
		s[k] = v
	}
}
func (p *AIProvider) models(ctx context.Context, base string, s Sample) bool {
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if !p.get(ctx, base+"/v1/models", &body) || len(body.Data) == 0 {
		return false
	}
	s["ai_model"] = body.Data[0].ID
	return true
}
func (p *AIProvider) llama(ctx context.Context, base string, s Sample) bool {
	var slots []struct {
		ID         int     `json:"id"`
		Task       *int    `json:"id_task"`
		Processing *bool   `json:"is_processing"`
		Context    float64 `json:"n_ctx"`
		Next       struct {
			Decoded *float64 `json:"n_decoded"`
		} `json:"next_token"`
	}
	ok := p.get(ctx, base+"/slots", &slots)
	known, active := false, false
	for _, slot := range slots {
		if slot.Processing == nil {
			continue
		}
		known = true
		if *slot.Processing {
			active = true
			if slot.Task == nil || slot.Next.Decoded == nil {
				continue
			}
			// llama.cpp reports generated tokens, not total prompt/context occupancy.
			// Do not turn n_decoded/n_ctx into a fabricated context percentage.
			key := base + "/slot/" + strconv.Itoa(slot.ID) + "/task/" + strconv.Itoa(*slot.Task)
			now := time.Now()
			old, exists := p.counters[key]
			p.counters[key] = aiCounter{*slot.Next.Decoded, now}
			if exists && *slot.Next.Decoded >= old.value && now.Sub(old.at) > 100*time.Millisecond {
				rate := (*slot.Next.Decoded - old.value) / now.Sub(old.at).Seconds()
				if current, exists := s["ai_tps"].(float64); exists {
					rate += current
				}
				s["ai_tps"] = rate
			}
		}
	}
	if known {
		s["ai_state"] = "IDLE"
		s["ai_note"] = "No inference requests"
		if active {
			s["ai_state"] = "ACTIVE"
			s["ai_note"] = "Local inference"
		}
	}
	if _, has := s["ai_model"]; !has {
		p.models(ctx, base, s)
	}
	return ok && known
}
func (p *AIProvider) maestro(ctx context.Context, base string, s Sample) bool {
	var stats struct {
		Model struct {
			Name   string `json:"name"`
			Loaded bool   `json:"loaded"`
		} `json:"model"`
	}
	if !p.get(ctx, base+"/api/v1/system-stats", &stats) {
		return false
	}
	if stats.Model.Loaded {
		s["ai_model"] = stats.Model.Name
		s["ai_state"] = "LOADED"
		s["ai_note"] = "Local generation model"
	}
	var jobs struct {
		Jobs []struct {
			Kind   string `json:"kind"`
			Status string `json:"status"`
		} `json:"jobs"`
	}
	jobsOK := p.get(ctx, base+"/api/v1/jobs", &jobs)
	queued, running := 0, 0
	if jobsOK {
		for _, j := range jobs.Jobs {
			if j.Kind != "generation" {
				continue
			}
			if j.Status == "running" {
				running++
			}
			if j.Status == "queued" {
				queued++
			}
		}
		s["ai_queue"] = queued
		if running > 0 {
			s["ai_state"] = "ACTIVE"
			s["ai_note"] = "Local image/video generation"
			return true
		}
		if queued > 0 {
			s["ai_state"] = "QUEUED"
			s["ai_note"] = "Local generation queued"
			return true
		}
	}
	var llm struct {
		Provider string `json:"provider"`
		Loaded   bool   `json:"loaded"`
		Model    string `json:"model_id"`
	}
	llmOK := p.get(ctx, base+"/api/v1/llm/status", &llm)
	if llmOK && llm.Provider == "local" && llm.Loaded {
		s["ai_model"] = llm.Model
		s["ai_state"] = "LOADED"
		s["ai_note"] = "Local writer; activity unavailable"
		var stream struct {
			Done *bool `json:"done"`
		}
		if p.get(ctx, base+"/api/v1/llm/stream-status", &stream) && stream.Done != nil && !*stream.Done {
			s["ai_state"] = "ACTIVE"
			s["ai_note"] = "Local writer inference"
		}
	} else if !stats.Model.Loaded && jobsOK && llmOK {
		s["ai_state"] = "IDLE"
		s["ai_note"] = "No local generation running"
	}
	return true
}

// Parse only numeric monitoring series; prompts and generated text never enter telemetry.
func aiMetrics(b []byte) map[string]float64 {
	out := map[string]float64{}
	scan := bufio.NewScanner(strings.NewReader(string(b)))
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		end := strings.IndexAny(line, "{ \t")
		if end < 0 {
			continue
		}
		name := line[:end]
		if brace := strings.IndexByte(line, '}'); brace >= 0 {
			line = line[brace+1:]
		} else {
			line = line[end:]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		v, e := strconv.ParseFloat(fields[0], 64)
		if e == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 {
			if strings.Contains(name, "usage") {
				out[name] = math.Max(out[name], v)
			} else {
				out[name] += v
			}
		}
	}
	return out
}
func (p *AIProvider) metrics(ctx context.Context, base, kind string, s Sample) bool {
	b, ok := p.read(ctx, base+"/metrics")
	if !ok {
		if p.models(ctx, base, s) {
			s["ai_state"] = "LOADED"
			s["ai_note"] = "Loaded; activity unavailable"
			return true
		}
		return false
	}
	m := aiMetrics(b)
	running, waiting, tokens, usage := "vllm:num_requests_running", "vllm:num_requests_waiting", "vllm:generation_tokens_total", "vllm:kv_cache_usage_perc"
	if kind == "SGLANG" {
		running, waiting, tokens, usage = "sglang:num_running_reqs", "sglang:num_queue_reqs", "sglang:generation_tokens_total", "sglang:token_usage"
	}
	if kind == "TGI" {
		running, waiting, tokens, usage = "tgi_batch_current_size", "tgi_queue_size", "", ""
	}
	if v, exists := m[running]; exists {
		s["ai_state"] = "IDLE"
		s["ai_note"] = "No inference requests"
		if v > 0 {
			s["ai_state"] = "ACTIVE"
			s["ai_note"] = "Local inference"
		}
	}
	if v, exists := m[waiting]; exists {
		s["ai_queue"] = v
		if v > 0 && s["ai_state"] != "ACTIVE" {
			s["ai_state"] = "QUEUED"
			s["ai_note"] = "Local inference queued"
		}
	}
	if v, exists := m[usage]; exists && v <= 1 {
		s["ai_context"] = v * 100
	}
	if v, exists := m[tokens]; exists {
		now := time.Now()
		old, has := p.counters[base]
		p.counters[base] = aiCounter{v, now}
		if has && v >= old.value && now.Sub(old.at) > 100*time.Millisecond {
			s["ai_tps"] = (v - old.value) / now.Sub(old.at).Seconds()
		}
	}
	p.models(ctx, base, s)
	return len(m) > 0
}
