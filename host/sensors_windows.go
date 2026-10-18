package main

import (
	"context"
	"github.com/yusufpapurcu/wmi"
	"strings"
)

type hardwareSensor struct {
	Name, SensorType, Identifier string
	Value                        float32
}
type publishedHardware struct{ Name, Identifier string }

func platformSensors(ctx context.Context, s Sample) {
	var sensors []hardwareSensor
	var hardware []publishedHardware
	source := ""
	for _, ns := range []string{"root\\LibreHardwareMonitor", "root\\OpenHardwareMonitor"} {
		if ctx.Err() != nil {
			return
		}
		if wmi.QueryNamespace("SELECT Name,SensorType,Value,Identifier FROM Sensor", &sensors, ns) == nil {
			source = ns
			wmi.QueryNamespace("SELECT Name,Identifier FROM Hardware", &hardware, ns)
			break
		}
	}
	names := map[string]string{}
	for _, h := range hardware {
		names[strings.ToLower(h.Identifier)] = h.Name
	}
	gpus := map[string]Sample{}
	for _, v := range sensors {
		id := strings.ToLower(v.Identifier)
		name := strings.ToLower(v.Name)
		n := float64(v.Value)
		cpu := strings.Contains(id, "/cpu/") || strings.Contains(id, "/amdcpu/") || strings.Contains(id, "/intelcpu/")
		if cpu {
			key := ""
			switch v.SensorType {
			case "Temperature":
				if usableTemperature(n) && (strings.Contains(name, "package") || strings.Contains(name, "tdie") || strings.Contains(name, "tctl")) {
					old, ok := metricNumber(s["cpu_temp"])
					if !ok || n > old {
						key = "cpu_temp"
						s["cpu_temp_source"] = "Existing " + source + " " + v.Name
						s["cpu_temp_kind"] = "cpu_sensor"
					}
				}
			case "Power":
				if strings.Contains(name, "package") {
					key = "cpu_power"
				}
			case "Clock":
				if strings.Contains(name, "core #1") {
					key = "cpu_clock"
				}
			case "Voltage":
				if strings.Contains(name, "core") {
					key = "cpu_voltage"
				}
			}
			if key != "" && n >= 0 {
				s[key] = finite(n)
			}
		}
		if !strings.Contains(id, "/gpu-") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(id, "/"), "/")
		if len(parts) < 2 {
			continue
		}
		group := "/" + parts[0] + "/" + parts[1]
		product := names[group]
		if product == "" {
			continue
		} // Do not attach an anonymous source to the wrong physical GPU.
		g := gpus[group]
		if g == nil {
			g = Sample{"id": "wmi:" + group, "name": product, "vendor": gpuVendor(product), "source": "Existing " + source}
			gpus[group] = g
		}
		key := ""
		switch v.SensorType {
		case "Temperature":
			if strings.Contains(name, "hot spot") || strings.Contains(name, "hotspot") {
				key = "hotspot"
			} else if strings.Contains(name, "core") {
				key = "temp"
			}
		case "Load":
			if strings.Contains(name, "core") {
				key = "load"
			}
		case "Power":
			if strings.Contains(name, "package") || strings.Contains(name, "gpu power") {
				key = "power"
			}
		case "Clock":
			if strings.Contains(name, "memory") {
				key = "mem_clock"
			} else if strings.Contains(name, "core") {
				key = "clock"
			}
		case "Fan":
			if n > 0 {
				g["fan_rpm"] = finite(n)
			}
		case "Data", "SmallData":
			if strings.Contains(name, "memory used") {
				key = "vram"
			} else if strings.Contains(name, "memory total") {
				key = "vram_total"
			}
			if v.SensorType == "SmallData" {
				n /= 1024
			}
		}
		if key != "" && n >= 0 {
			g[key] = finite(n)
		}
	}
	rows := []Sample{}
	for _, g := range gpus {
		rows = append(rows, g)
	}
	if len(rows) > 0 {
		s["_gpu_wmi"] = rows
	}
}
