package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type linuxPlatform struct{ root string }

func sysText(path string) string {
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 16384 {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func sysNumber(path string) (float64, bool) {
	v, e := strconv.ParseFloat(sysText(path), 64)
	return v, e == nil && finite(v) != nil
}
func cpuSensorName(name string) bool {
	s := strings.ToLower(name)
	return strings.Contains(s, "cpu") || strings.Contains(s, "coretemp") || strings.Contains(s, "k10temp") || strings.Contains(s, "zenpower") || strings.Contains(s, "x86_pkg") || strings.Contains(s, "peci_cputemp")
}
func (p *linuxPlatform) Collect(ctx context.Context, s Sample) {
	root := p.root
	if root == "" {
		root = "/sys"
	}
	highest := -1.0
	source := ""
	kind := ""
	hwmons, _ := filepath.Glob(filepath.Join(root, "class/hwmon/hwmon*"))
	for _, node := range hwmons {
		if ctx.Err() != nil {
			return
		}
		name := sysText(filepath.Join(node, "name"))
		inputs, _ := filepath.Glob(filepath.Join(node, "temp*_input"))
		dieFound := false
		for _, input := range inputs {
			if strings.EqualFold(sysText(strings.TrimSuffix(input, "_input")+"_label"), "Tdie") {
				if n, ok := sysNumber(input); ok && usableTemperature(n/1000) {
					dieFound = true
				}
			}
		}
		for _, input := range inputs {
			label := sysText(strings.TrimSuffix(input, "_input") + "_label")
			if !cpuSensorName(name) && !cpuSensorName(label) {
				continue
			}
			lower := strings.ToLower(label)
			if strings.Contains(lower, "tctl") && dieFound {
				continue
			}
			if strings.Contains(lower, "tcontrol") || strings.Contains(lower, "tjmax") || strings.Contains(lower, "tthrottle") {
				continue
			}
			n, ok := sysNumber(input)
			n /= 1000
			if !ok || !usableTemperature(n) {
				continue
			}
			if n > highest {
				highest = n
				source = "Linux hwmon " + name + " " + label
				kind = "cpu_sensor"
			}
		}
	}
	zones := []Sample{}
	paths, _ := filepath.Glob(filepath.Join(root, "class/thermal/thermal_zone*"))
	thermalMax := -1.0
	thermalName := ""
	for _, node := range paths {
		if ctx.Err() != nil {
			return
		}
		name := sysText(filepath.Join(node, "type"))
		n, ok := sysNumber(filepath.Join(node, "temp"))
		n /= 1000
		if !ok || !usableTemperature(n) {
			continue
		}
		zones = append(zones, Sample{"name": name, "temperature": finite(n), "source": "Linux thermal"})
		if n > thermalMax {
			thermalMax = n
			thermalName = name
		}
		if cpuSensorName(name) && n > highest {
			highest = n
			source = "Linux thermal " + name
			kind = "cpu_zone"
		}
	}
	if highest > 0 {
		s["cpu_temp"] = finite(highest)
		s["cpu_temp_source"] = strings.TrimSpace(source)
		s["cpu_temp_kind"] = kind
	}
	if len(zones) > 0 {
		s["thermal_zones"] = zones
		s["thermal_temp"] = finite(thermalMax)
		s["thermal_name"] = thermalName
	}
	cards, _ := filepath.Glob(filepath.Join(root, "class/drm/card[0-9]*"))
	gpus := []Sample{}
	seen := map[string]bool{}
	for _, card := range cards {
		if ctx.Err() != nil {
			return
		}
		if strings.Contains(filepath.Base(card), "-") {
			continue
		}
		device := filepath.Join(card, "device")
		resolved, e := filepath.EvalSymlinks(device)
		if e != nil {
			continue
		}
		if seen[resolved] {
			continue
		}
		seen[resolved] = true
		vendorID := strings.ToLower(sysText(filepath.Join(device, "vendor")))
		vendor := map[string]string{"0x10de": "nvidia", "0x1002": "amd", "0x8086": "intel", "0x1a03": "aspeed", "0x17cb": "qualcomm"}[vendorID]
		driverPath, _ := filepath.EvalSymlinks(filepath.Join(device, "driver"))
		driver := filepath.Base(driverPath)
		if driver == "." {
			driver = ""
		}
		if vendor == "" {
			vendor = gpuVendor(driver)
		}
		name := sysText(filepath.Join(device, "product_name"))
		if name == "" {
			name = strings.TrimSpace(strings.ToUpper(vendor) + " " + driver + " " + sysText(filepath.Join(device, "device")))
		}
		if name == "" {
			name = filepath.Base(card)
		}
		g := Sample{"id": "linux:" + resolved, "name": name, "vendor": vendor, "source": "Linux DRM/hwmon"}
		if pci := canonicalPCI(filepath.Base(resolved)); pci != "" {
			g["pci"] = pci
			g["_display_identity"] = pci + ":" + vendorID + ":" + sysText(filepath.Join(device, "device"))
		}
		kind := "unknown"
		if root == "/sys" {
			kind = linuxDRMGPUKind(card, driver)
		}
		if kind == "unknown" {
			switch driver {
			case "msm", "panfrost", "panthor", "v3d", "vc4", "asahi", "tegra", "etnaviv", "lima":
				kind = "integrated"
			}
		}
		g["kind"] = kind
		for _, metric := range []struct {
			key, file string
			div       float64
		}{{"load", "gpu_busy_percent", 1}, {"vram", "mem_info_vram_used", 1 << 30}, {"vram_total", "mem_info_vram_total", 1 << 30}} {
			if n, ok := sysNumber(filepath.Join(device, metric.file)); ok && n >= 0 {
				g[metric.key] = finite(n / metric.div)
			}
		}
		nodes, _ := filepath.Glob(filepath.Join(device, "hwmon/hwmon*"))
		for _, node := range nodes {
			if n, ok := sysNumber(filepath.Join(node, "temp1_input")); ok && usableTemperature(n/1000) {
				g["temp"] = finite(n / 1000)
			}
			label := strings.ToLower(sysText(filepath.Join(node, "temp2_label")))
			if n, ok := sysNumber(filepath.Join(node, "temp2_input")); ok && usableTemperature(n/1000) && (strings.Contains(label, "junction") || strings.Contains(label, "hotspot")) {
				g["hotspot"] = finite(n / 1000)
			}
			if n, ok := sysNumber(filepath.Join(node, "power1_average")); ok && n > 0 {
				g["power"] = finite(n / 1e6)
			}
			if n, ok := sysNumber(filepath.Join(node, "fan1_input")); ok && n > 0 {
				g["fan_rpm"] = finite(n)
			}
		}
		if n, ok := sysNumber(filepath.Join(card, "gt_cur_freq_mhz")); ok && n > 0 {
			g["clock"] = finite(n)
		}
		// Prefer the driver's active DPM state; do not report an advertised boost clock.
		for _, line := range strings.Split(sysText(filepath.Join(device, "pp_dpm_sclk")), "\n") {
			if strings.Contains(line, "*") {
				fields := strings.Fields(line)
				if len(fields) > 1 {
					n, e := strconv.ParseFloat(strings.TrimSuffix(strings.ToLower(fields[1]), "mhz"), 64)
					if e == nil {
						g["clock"] = finite(n)
					}
				}
			}
		}
		if pci, _ := g["pci"].(string); pci != "" {
			g["id"] = pci
		}
		gpus = append(gpus, g)
	}
	if len(gpus) > 0 {
		s["_gpu_platform"] = gpus
	}
}
