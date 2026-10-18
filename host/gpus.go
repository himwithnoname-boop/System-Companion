package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

var gpuMetrics = []string{"temp", "load", "hotspot", "power", "clock", "mem_clock", "fan", "vram", "vram_total"}

func metricNumber(v any) (float64, bool) {
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case float32:
		n = float64(x)
	case int:
		n = float64(x)
	case uint64:
		n = float64(x)
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}
func usableTemperature(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0 && v < 130
}
func gpuVendor(name string) string {
	s := strings.ToLower(name)
	for _, v := range []string{"nvidia", "intel", "amd", "aspeed", "apple", "qualcomm"} {
		if strings.Contains(s, v) {
			return v
		}
	}
	if strings.Contains(s, "radeon") {
		return "amd"
	}
	return ""
}
func canonicalPCI(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s, "pci:")))
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return ""
	}
	domain, e1 := strconv.ParseUint(parts[0], 16, 32)
	bus, e2 := strconv.ParseUint(parts[1], 16, 8)
	df := strings.Split(parts[2], ".")
	if len(df) != 2 {
		return ""
	}
	dev, e3 := strconv.ParseUint(df[0], 16, 8)
	fn, e4 := strconv.ParseUint(df[1], 16, 8)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return ""
	}
	return fmt.Sprintf("pci:%04x:%02x:%02x.%x", domain, bus, dev, fn)
}
func gpuName(g Sample) string {
	v, _ := g["name"].(string)
	return strings.ToLower(strings.TrimSpace(v))
}
func gpuIdentityConflict(a, b Sample) bool {
	for _, key := range []string{"pci", "luid"} {
		av, _ := a[key].(string)
		bv, _ := b[key].(string)
		if av != "" && bv != "" && av != bv {
			return true
		}
	}
	return false
}

func sameGPU(a, b Sample) bool {
	if gpuIdentityConflict(a, b) {
		return false
	}
	for _, k := range []string{"id", "pci", "luid"} {
		av, _ := a[k].(string)
		bv, _ := b[k].(string)
		if av != "" && av == bv {
			return true
		}
	}
	return false
}

// Inventories establish identity. A repeated model name is not a physical ID.
func mergeGPUs(s Sample) {
	var result []Sample
	for _, key := range []string{"_gpu_platform", "_gpu_wmi", "_gpu_nvidia"} {
		reports, _ := s[key].([]Sample)
		delete(s, key)
		counts := map[string]int{}
		for _, g := range reports {
			counts[gpuName(g)]++
		}
		used := map[int]bool{}
		for _, g := range reports {
			match := -1
			for i, old := range result {
				if !used[i] && sameGPU(old, g) {
					match = i
					break
				}
			}
			if match < 0 && counts[gpuName(g)] == 1 {
				candidate := -1
				for i, old := range result {
					if !used[i] && !gpuIdentityConflict(old, g) && gpuName(old) == gpuName(g) && gpuName(g) != "" {
						if candidate >= 0 {
							candidate = -2
							break
						}
						candidate = i
					}
				}
				if candidate >= 0 {
					match = candidate
				}
			}
			if match < 0 {
				copy := Sample{}
				for k, v := range g {
					copy[k] = v
				}
				result = append(result, copy)
				match = len(result) - 1
			} else {
				for k, v := range g {
					if v != nil {
						result[match][k] = v
					}
				}
			}
			used[match] = true
		}
	}
	if len(result) == 0 {
		return
	}
	sort.SliceStable(result, func(i, j int) bool {
		if ki, kj := gpuKindRank(result[i]), gpuKindRank(result[j]); ki != kj {
			return ki < kj
		}
		pi, _ := result[i]["pci"].(string)
		pj, _ := result[j]["pci"].(string)
		if pi != "" && pj != "" && pi != pj {
			return pi < pj
		}
		return fmt.Sprint(result[i]["id"]) < fmt.Sprint(result[j]["id"])
	})
	names := map[string]int{}
	for _, g := range result {
		names[gpuName(g)]++
	}
	firstIndex := 1
	if gpuKindRank(result[0]) == 0 {
		firstIndex = 0
	}
	for index, g := range result {
		g["index"] = firstIndex + index
		g["display_key"] = gpuDisplayKey(g, names)
		delete(g, "_display_identity")
		for _, k := range gpuMetrics {
			n, ok := metricNumber(g[k])
			if !ok || n < 0 || ((k == "temp" || k == "hotspot") && !usableTemperature(n)) {
				delete(g, k)
				continue
			}
			if k == "load" || k == "fan" {
				n = math.Min(100, n)
			}
			g[k] = finite(n)
		}
		if _, ok := g["temp"]; !ok {
			g["temperature_status"] = "not exposed by this driver or permitted sensor source"
		}
	}
	s["gpus"] = result
	s["gpu_count"] = len(result)
	for _, k := range gpuMetrics {
		delete(s, "gpu_"+k)
	}
	for _, k := range []string{"temp", "hotspot", "fan", "clock", "mem_clock"} {
		highest := -1.0
		for _, g := range result {
			if n, ok := metricNumber(g[k]); ok && n > highest {
				highest = n
			}
		}
		if highest >= 0 {
			s["gpu_"+k] = highest
		}
	}
	sumLoad, reporting := 0.0, 0
	for _, g := range result {
		if n, ok := metricNumber(g["load"]); ok {
			sumLoad += n
			reporting++
		}
	}
	s["gpu_load_available"] = reporting
	s["gpu_load_method"] = "mean_available"
	if reporting > 0 {
		s["gpu_load"] = finite(sumLoad / float64(reporting))
	}
	for _, k := range []string{"vram", "vram_total", "power"} {
		sum := 0.0
		found := false
		for _, g := range result {
			if n, ok := metricNumber(g[k]); ok {
				sum += n
				found = true
			}
		}
		if found {
			s["gpu_"+k] = finite(sum)
		}
	}
	if len(result) == 1 {
		s["gpu_name"] = result[0]["name"]
	} else {
		s["gpu_name"] = fmt.Sprintf("%d graphics adapters", len(result))
	}
}
