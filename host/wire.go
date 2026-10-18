package main

// serialTelemetry bounds display-only data without changing the full dashboard sample.
func serialTelemetry(v any) any {
	s, ok := v.(Sample)
	if !ok || s["type"] != "telemetry" {
		return v
	}
	wire := Sample{}
	for k, value := range s {
		wire[k] = value
	}
	delete(wire, "thermal_zones") // Generic zone details remain available in the dashboard.
	rows, ok := s["gpus"].([]Sample)
	if !ok {
		return wire
	}
	offset := 0
	if len(rows) > 8 {
		seq, _ := s["seq"].(uint64)
		offset = int((seq/8)%uint64((len(rows)+7)/8)) * 8
	}
	end := min(offset+8, len(rows))
	compact := make([]Sample, 0, end-offset)
	for _, g := range rows[offset:end] {
		item := Sample{}
		for _, key := range []string{"id", "name", "vendor", "index", "temp", "load", "power", "vram", "vram_total", "hotspot", "fan", "fan_rpm", "clock", "mem_clock"} {
			if value, ok := g[key]; ok {
				if text, yes := value.(string); yes {
					r := []rune(text)
					if len(r) > 64 {
						value = string(r[:64])
					}
				}
				item[key] = value
			}
		}
		compact = append(compact, item)
	}
	wire["gpus"] = compact
	wire["gpu_count"] = len(rows)
	wire["gpu_offset"] = offset
	return wire
}
