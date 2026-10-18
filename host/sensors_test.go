package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureFile(t *testing.T, root, path, value string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(path))
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(value), 0644); e != nil {
		t.Fatal(e)
	}
}
func TestLinuxCPUAndThermalProvenance(t *testing.T) {
	root := t.TempDir()
	for path, value := range map[string]string{"class/hwmon/hwmon0/name": "k10temp", "class/hwmon/hwmon0/temp1_label": "Tctl", "class/hwmon/hwmon0/temp1_input": "85000", "class/hwmon/hwmon0/temp2_label": "Tdie", "class/hwmon/hwmon0/temp2_input": "72000", "class/thermal/thermal_zone0/type": "qcom-tsens", "class/thermal/thermal_zone0/temp": "88000", "class/thermal/thermal_zone1/type": "cpu-thermal", "class/thermal/thermal_zone1/temp": "69000"} {
		fixtureFile(t, root, path, value)
	}
	s := Sample{}
	(&linuxPlatform{root: root}).Collect(context.Background(), s)
	if s["cpu_temp"] != float64(72) || s["thermal_temp"] != float64(88) || !strings.Contains(s["cpu_temp_source"].(string), "Tdie") {
		t.Fatalf("incorrect sensor meaning: %v", s)
	}
}
func TestGenericARMZoneIsNotFabricatedCPU(t *testing.T) {
	root := t.TempDir()
	fixtureFile(t, root, "class/thermal/thermal_zone0/type", "soc-thermal")
	fixtureFile(t, root, "class/thermal/thermal_zone0/temp", "47000")
	s := Sample{}
	(&linuxPlatform{root: root}).Collect(context.Background(), s)
	if s["cpu_temp"] != nil || s["thermal_temp"] != float64(47) {
		t.Fatalf("zone mislabeled as CPU: %v", s)
	}
}
func TestLinuxMissingAndInactiveSensors(t *testing.T) {
	root := t.TempDir()
	fixtureFile(t, root, "class/hwmon/hwmon0/name", "coretemp")
	fixtureFile(t, root, "class/hwmon/hwmon0/temp1_input", "0")
	s := Sample{}
	(&linuxPlatform{root: root}).Collect(context.Background(), s)
	if s["cpu_temp"] != nil {
		t.Fatal("inactive sensor published")
	}
}
func TestSerialGPUBoundsAndFullDashboardPreserved(t *testing.T) {
	rows := []Sample{}
	for i := 0; i < 12; i++ {
		rows = append(rows, Sample{"id": strings.Repeat("x", 120), "name": strings.Repeat("GPU", 50), "temp": float64(50), "load": float64(20), "source": "full source metadata", "pci": "pci:0000:01:00.0"})
	}
	s := Sample{"type": "telemetry", "seq": uint64(8), "gpus": rows, "gpu_count": 12, "thermal_zones": []Sample{{"name": "zone"}}}
	wire := serialTelemetry(s).(Sample)
	out := wire["gpus"].([]Sample)
	if len(out) != 4 || wire["gpu_offset"] != 8 || len(s["gpus"].([]Sample)) != 12 || s["thermal_zones"] == nil || wire["thermal_zones"] != nil {
		t.Fatalf("wire pagination changed original: %v", wire)
	}
	b, e := json.Marshal(wire)
	if e != nil || len(b) >= 8191 {
		t.Fatalf("unbounded GPU frame: %d %v", len(b), e)
	}
}
