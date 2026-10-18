package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type NvidiaProvider struct{ cachedProvider }

func nvidiaSample(line string) Sample {
	values, err := csv.NewReader(strings.NewReader(line)).Read()
	if err != nil || len(values) != 9 {
		return nil
	}
	s := Sample{"gpu_name": strings.TrimSpace(values[0])}
	keys := []string{"gpu_temp", "gpu_load", "gpu_vram", "gpu_vram_total", "gpu_power", "gpu_clock", "gpu_mem_clock", "gpu_fan"}
	for i, key := range keys {
		if n, e := strconv.ParseFloat(strings.TrimSpace(values[i+1]), 64); e == nil && finite(n) != nil && n >= 0 {
			if key == "gpu_vram" || key == "gpu_vram_total" {
				n /= 1024
			}
			s[key] = finite(n)
		}
	}
	if len(s) == 1 {
		return nil
	}
	return s
}
func nvidiaGPU(line string) Sample {
	v, e := csv.NewReader(strings.NewReader(line)).Read()
	if e != nil || len(v) != 12 {
		return nil
	}
	row := Sample{"id": "nvidia:" + strings.TrimSpace(v[1]), "name": strings.TrimSpace(v[3]), "vendor": "nvidia", "source": "NVIDIA driver"}
	if pci := canonicalPCI(v[2]); pci != "" {
		row["pci"] = pci
	}
	for i, k := range []string{"temp", "load", "vram", "vram_total", "power", "clock", "mem_clock", "fan"} {
		n, e := strconv.ParseFloat(strings.TrimSpace(v[i+4]), 64)
		if e != nil || finite(n) == nil || n < 0 {
			continue
		}
		if k == "vram" || k == "vram_total" {
			n /= 1024
		}
		row[k] = finite(n)
	}
	return row
}
func startNvidia(ctx context.Context, period time.Duration) *NvidiaProvider {
	p := &NvidiaProvider{cachedProvider: cachedProvider{maxAge: 3 * period, done: make(chan struct{})}}
	go func() {
		defer close(p.done)
		for ctx.Err() == nil {
			p.stream(ctx, period)
			timer := time.NewTimer(30 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return p
}
func (p *NvidiaProvider) stream(ctx context.Context, period time.Duration) {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(child, "nvidia-smi", "--query-gpu=index,uuid,pci.bus_id,name,temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw,clocks.gr,clocks.mem,fan.speed", "--format=csv,noheader,nounits", "--loop-ms="+strconv.FormatInt(period.Milliseconds(), 10))
	hideCommand(cmd)
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		return
	}
	if cmd.Start() != nil {
		stdout.Close()
		return
	}
	watchdog := time.AfterFunc(4*period+2*time.Second, cancel)
	rows := map[string]Sample{}
	seen := map[string]time.Time{}
	scan := bufio.NewScanner(stdout)
	scan.Buffer(make([]byte, 1024), 8192)
	for scan.Scan() {
		if row := nvidiaGPU(scan.Text()); row != nil {
			id, _ := row["id"].(string)
			now := time.Now()
			rows[id] = row
			seen[id] = now
			snapshot := []Sample{}
			for key, g := range rows {
				if now.Sub(seen[key]) <= 3*period {
					snapshot = append(snapshot, g)
				} else {
					delete(rows, key)
					delete(seen, key)
				}
			}
			p.mu.Lock()
			p.data = Sample{"_gpu_nvidia": snapshot}
			p.at = now
			p.mu.Unlock()
			watchdog.Reset(4*period + 2*time.Second)
		}
	}
	watchdog.Stop()
	cancel()
	cmd.Wait()
}
