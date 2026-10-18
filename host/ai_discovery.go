package main

import (
	"context"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type aiEngine struct {
	kind, model        string
	port               int
	pid                int32
	cpu, ram           float64
	cpuKnown, ramKnown bool
}
type aiIdentity struct {
	created int64
	engine  aiEngine
	checked time.Time
}
type aiDiscovery struct {
	mu         sync.RWMutex
	engines    []aiEngine
	at         time.Time
	identities map[int32]aiIdentity // owned by the existing process collector
}

func (d *aiDiscovery) snapshot() []aiEngine {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if time.Since(d.at) > 15*time.Second {
		return nil
	}
	return append([]aiEngine(nil), d.engines...)
}
func aiArg(args []string, keys ...string) string {
	for i, a := range args {
		for _, k := range keys {
			if a == k && i+1 < len(args) {
				return args[i+1]
			}
			if strings.HasPrefix(a, k+"=") {
				return strings.TrimPrefix(a, k+"=")
			}
		}
	}
	return ""
}
func aiClassify(name string, args []string) aiEngine {
	name = strings.ToLower(strings.TrimSuffix(name, ".exe"))
	joined := strings.ToLower(strings.Join(args, " "))
	e := aiEngine{}
	module := aiArg(args, "-m")
	command := ""
	if len(args) > 1 {
		command = strings.ToLower(filepath.Base(strings.ReplaceAll(args[1], "\\", "/")))
	}
	switch {
	case name == "llama-server":
		e.kind = "LLAMA.CPP"
		e.port = 8080
	case name == "llama-cli":
		e.kind = "LLAMA.CLI"
	case name == "ollama" && strings.Contains(joined, "serve"):
		e.kind = "OLLAMA"
		e.port = 11434
	case strings.HasPrefix(name, "koboldcpp"):
		e.kind = "KOBOLDCPP"
		e.port = 5001
	case name == "llmster":
		e.kind = "LM STUDIO"
		e.port = 1234
	case name == "text-generation-launcher":
		e.kind = "TGI"
		e.port = 8000
	case name == "vllm" || command == "vllm" || module == "vllm" || strings.HasPrefix(module, "vllm."):
		e.kind = "VLLM"
		e.port = 8000
	case strings.HasPrefix(module, "sglang.") || command == "sglang":
		e.kind = "SGLANG"
		e.port = 30000
	case module == "mlx_lm.server" || name == "mlx_lm.server" || command == "mlx_lm.server":
		e.kind = "MLX LM"
		e.port = 8080
	case strings.Contains(joined, "maestro") && (strings.Contains(joined, "portable/backend.py") || strings.Contains(joined, "portable\\backend.py")):
		e.kind = "MAESTRO"
		e.port = 7860
	}
	if e.kind == "" {
		return e
	}
	if port := aiArg(args, "--port", "-p"); port != "" {
		n, err := strconv.Atoi(port)
		if err == nil && n > 0 && n <= 65535 {
			e.port = n
		}
	}
	e.model = aiArg(args, "--alias", "--served-model-name")
	if e.model == "" {
		e.model = aiArg(args, "--model", "--model-path", "--model-id")
		if e.model == "" && (e.kind == "LLAMA.CPP" || e.kind == "LLAMA.CLI" || e.kind == "KOBOLDCPP") {
			e.model = aiArg(args, "-m")
		}
		if strings.Contains(e.model, "\\") || strings.HasPrefix(e.model, "/") {
			e.model = filepath.Base(strings.ReplaceAll(e.model, "\\", "/"))
		}
	}
	if e.kind == "VLLM" && e.model == "" {
		for i, a := range args {
			if a == "serve" && i+1 < len(args) {
				e.model = args[i+1]
			}
		}
	}
	// Only these summarized fields survive discovery; raw command lines do not.
	return e
}
func aiMaybe(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "python") || strings.HasPrefix(n, "llama-") || strings.HasPrefix(n, "koboldcpp") || n == "ollama" || n == "ollama.exe" || n == "llmster" || n == "llmster.exe" || n == "vllm" || n == "vllm.exe" || n == "text-generation-launcher" || n == "mlx_lm.server" || n == "sglang"
}
func (d *aiDiscovery) identify(ctx context.Context, p *process.Process) aiEngine {
	if d == nil {
		return aiEngine{}
	}
	if d.identities == nil {
		d.identities = map[int32]aiIdentity{}
	}
	created, _ := p.CreateTimeWithContext(ctx)
	if old, ok := d.identities[p.Pid]; ok && old.created == created && time.Since(old.checked) < 30*time.Second {
		return old.engine
	}
	name, _ := p.NameWithContext(ctx)
	engine := aiEngine{}
	if aiMaybe(name) {
		args, _ := p.CmdlineSliceWithContext(ctx)
		engine = aiClassify(name, args)
		// A native socket lookup discovers an embedded engine's ephemeral port or
		// a launcher using environment configuration. Never shell out to netstat.
		if engine.kind != "" && engine.port > 0 {
			if conns, err := gnet.ConnectionsPidWithContext(ctx, "tcp", p.Pid); err == nil {
				ports := []int{}
				for _, c := range conns {
					if c.Status == "LISTEN" && c.Laddr.Port > 0 && (c.Laddr.IP == "127.0.0.1" || c.Laddr.IP == "0.0.0.0" || c.Laddr.IP == "::1" || c.Laddr.IP == "::") {
						ports = append(ports, int(c.Laddr.Port))
					}
				}
				if len(ports) == 1 {
					engine.port = ports[0]
				}
			}
		}
	}
	engine.pid = p.Pid
	d.identities[p.Pid] = aiIdentity{created, engine, time.Now()}
	return engine
}
func (d *aiDiscovery) publish(engines []aiEngine, live map[int32]bool) {
	if d == nil {
		return
	}
	for pid := range d.identities {
		if !live[pid] {
			delete(d.identities, pid)
		}
	}
	d.mu.Lock()
	d.engines, d.at = engines, time.Now()
	d.mu.Unlock()
}
