package main

import (
	"context"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Sample map[string]any
type Provider interface{ Collect(context.Context, Sample) }
type OSProvider struct {
	last     time.Time
	network  map[string]gnet.IOCountersStat
	hostname string
	names    string
	clock    float64
}

type StorageProvider struct {
	last       time.Time
	disk       map[string]disk.IOCountersStat
	drives     []disk.PartitionStat
	capacity   []Sample
	capacityAt time.Time
}
type ProcessProvider struct {
	processTime map[int32]float64
	processAt   time.Time
	ai          *aiDiscovery
}

func gib(n uint64) float64 { return float64(n) / (1 << 30) }
func finite(n float64) any {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return math.Round(n*100) / 100
}
func (p *OSProvider) Collect(ctx context.Context, s Sample) {
	now := time.Now()
	elapsed := now.Sub(p.last).Seconds()
	if p.hostname == "" {
		p.hostname, _ = os.Hostname()
	}
	s["host_name"] = p.hostname
	if p.names == "" {
		if info, e := cpu.InfoWithContext(ctx); e == nil && len(info) > 0 {
			p.names = info[0].ModelName
			p.clock = info[0].Mhz
		}
	}
	s["cpu_name"] = p.names
	if p.clock > 0 {
		s["cpu_clock"] = finite(p.clock)
	}
	if loads, e := cpu.PercentWithContext(ctx, 0, true); e == nil && len(loads) > 0 {
		sum := 0.0
		cores := make([]Sample, 0, 32)
		for i, v := range loads {
			sum += v
			if i < 32 {
				cores = append(cores, Sample{"load": finite(v)})
			}
		}
		s["cpu_load"] = finite(sum / float64(len(loads)))
		s["cores"] = cores
	}
	if v, e := mem.VirtualMemoryWithContext(ctx); e == nil {
		s["ram_used"] = gib(v.Total - v.Available)
		s["ram_total"] = gib(v.Total)
		s["ram_available"] = gib(v.Available)
		s["ram_cached"] = gib(v.Cached)
	}
	if v, e := mem.SwapMemoryWithContext(ctx); e == nil {
		s["swap_used"] = gib(v.Used)
	}
	if ns, e := gnet.IOCountersWithContext(ctx, true); e == nil {
		next := map[string]gnet.IOCountersStat{}
		down, up, peak := 0.0, 0.0, 0.0
		adapter := ""
		for _, v := range ns {
			next[v.Name] = v
			low := strings.ToLower(v.Name)
			if strings.HasPrefix(low, "lo") || strings.Contains(low, "loopback") {
				continue
			}
			if old, ok := p.network[v.Name]; ok && elapsed > 0 && v.BytesRecv >= old.BytesRecv && v.BytesSent >= old.BytesSent {
				rx := float64(v.BytesRecv-old.BytesRecv) * 8 / elapsed / 1e6
				tx := float64(v.BytesSent-old.BytesSent) * 8 / elapsed / 1e6
				down += rx
				up += tx
				if rx+tx > peak {
					peak = rx + tx
					adapter = v.Name
				}
			}
		}
		if !p.last.IsZero() {
			s["net_down"] = finite(down)
			s["net_up"] = finite(up)
		}
		s["net_adapter"] = adapter
		p.network = next
	}
	p.last = now
}

func (p *StorageProvider) Collect(ctx context.Context, s Sample) {
	now := time.Now()
	elapsed := now.Sub(p.last).Seconds()
	if now.Sub(p.capacityAt) >= 30*time.Second {
		p.drives, _ = disk.PartitionsWithContext(ctx, false)
		p.capacity = nil
		for _, part := range p.drives {
			if len(p.capacity) >= 6 {
				break
			}
			if strings.HasPrefix(part.Mountpoint, "/snap/") || part.Fstype == "tmpfs" || part.Fstype == "squashfs" {
				continue
			}
			usage, e := disk.UsageWithContext(ctx, part.Mountpoint)
			if e == nil && usage.Total > 0 {
				p.capacity = append(p.capacity, Sample{"name": part.Mountpoint, "used": gib(usage.Used), "total": gib(usage.Total), "device": part.Device})
			}
		}
		p.capacityAt = now
	}
	ioStats, _ := disk.IOCountersWithContext(ctx)
	drives := make([]Sample, 0, 6)
	for _, capacity := range p.capacity {
		d := Sample{"name": capacity["name"], "used": capacity["used"], "total": capacity["total"]}
		device := capacity["device"].(string)
		key := strings.TrimPrefix(device, "/dev/")
		stat, ok := ioStats[key]
		if !ok && runtime.GOOS == "windows" {
			for k, v := range ioStats {
				if strings.EqualFold(v.Name, device) || strings.HasSuffix(k, strings.TrimSuffix(device, ":")) {
					stat = v
					key = k
					ok = true
					break
				}
			}
		}
		if old, exists := p.disk[key]; ok && exists && elapsed > 0 && stat.ReadBytes >= old.ReadBytes && stat.WriteBytes >= old.WriteBytes {
			d["read"] = float64(stat.ReadBytes-old.ReadBytes) / elapsed / 1e6
			d["write"] = float64(stat.WriteBytes-old.WriteBytes) / elapsed / 1e6
		}
		drives = append(drives, d)
	}
	s["drives"] = drives
	p.disk = ioStats
	p.last = now
}
func (p *ProcessProvider) Collect(ctx context.Context, s Sample) {
	now := time.Now()
	ps, e := process.ProcessesWithContext(ctx)
	if e != nil {
		return
	}
	next := map[int32]float64{}
	live := map[int32]bool{}
	engines := []aiEngine{}
	type row struct {
		proc *process.Process
		cpu  float64
		ram  uint64
	}
	items := make([]row, 0, len(ps))
	elapsed := now.Sub(p.processAt).Seconds()
	for _, proc := range ps {
		if ctx.Err() != nil {
			break
		}
		live[proc.Pid] = true
		engine := p.ai.identify(ctx, proc)
		m, e := proc.MemoryInfoWithContext(ctx)
		if e != nil {
			continue
		}
		load := 0.0
		if t, e := proc.TimesWithContext(ctx); e == nil {
			total := t.User + t.System
			next[proc.Pid] = total
			if old, ok := p.processTime[proc.Pid]; ok && elapsed > 0 && total >= old {
				load = math.Min(100, (total-old)*100/elapsed/float64(runtime.NumCPU()))
			}
		}
		if engine.kind != "" && len(engines) < 8 {
			engine.cpu, engine.ram = load, gib(m.RSS)
			_, engine.cpuKnown = p.processTime[proc.Pid]
			engine.ramKnown = true
			engines = append(engines, engine)
		}
		items = append(items, row{proc, load, m.RSS})
	}
	// Resolve names only for the ten displayed candidates, rather than every process.
	sort.Slice(items, func(i, j int) bool { return items[i].cpu > items[j].cpu })
	chosen := make([]row, 0, 10)
	seen := map[int32]bool{}
	for i := 0; i < len(items) && i < 5; i++ {
		chosen = append(chosen, items[i])
		seen[items[i].proc.Pid] = true
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ram > items[j].ram })
	for i := 0; i < len(items) && i < 5; i++ {
		if !seen[items[i].proc.Pid] {
			chosen = append(chosen, items[i])
		}
	}
	result := make([]Sample, 0, len(chosen))
	for _, item := range chosen {
		if name, e := item.proc.NameWithContext(ctx); e == nil {
			result = append(result, Sample{"name": name, "cpu": finite(item.cpu), "ram": gib(item.ram), "pid": item.proc.Pid})
		}
	}
	s["processes"] = result
	if ctx.Err() == nil {
		p.ai.publish(engines, live)
	}
	p.processTime = next
	p.processAt = now
}

type HardwareProvider struct{}

func (HardwareProvider) Collect(ctx context.Context, s Sample) { platformSensors(ctx, s) }

type TemperatureProvider struct{}

func (TemperatureProvider) Collect(ctx context.Context, s Sample) { linuxTemperatures(ctx, s) }

func collect(ctx context.Context, providers []Provider, seq uint64) Sample {
	s := Sample{"type": "telemetry", "v": 1, "seq": seq, "time": time.Now().Unix() + int64(zoneOffset())}
	for _, p := range providers {
		child, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		p.Collect(child, s)
		cancel()
	}
	mergeGPUs(s)
	return s
}
func zoneOffset() int { _, offset := time.Now().Zone(); return offset }
