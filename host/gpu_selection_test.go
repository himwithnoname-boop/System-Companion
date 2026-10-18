package main

import (
	"strings"
	"testing"
)

func TestGPUBarFollowsSelectionAndExcludesMissing(t *testing.T) {
	s := Sample{"type": "telemetry", "_gpu_platform": []Sample{
		{"id": "a", "name": "Idle GPU", "kind": "integrated", "load": float64(0)},
		{"id": "b", "name": "Busy GPU", "load": float64(100)},
		{"id": "c", "name": "No load sensor", "temp": float64(40)},
	}}
	mergeGPUs(s)
	if s["gpu_load"] != float64(50) || s["gpu_load_available"] != 2 {
		t.Fatalf("All-GPU average wrong: %v", s)
	}
	rows := s["gpus"].([]Sample)
	busyKey := rows[1]["display_key"].(string)
	prefs := &DisplaySettings{GPUSelection: true, HostKey: clientHostKey(), GPUHidden: []string{busyKey}}
	wire := serialTelemetry(displayTelemetry(s, prefs)).(Sample)
	if wire["gpu_load"] != float64(0) || wire["gpu_load_available"] != 1 || wire["gpu_detected"] != 3 || wire["gpu_count"] != 2 {
		t.Fatalf("Selected idle GPU bar wrong: %v", wire)
	}
	visible := wire["gpus"].([]Sample)
	if visible[0]["index"] != 0 || visible[1]["index"] != 2 {
		t.Fatalf("GPU labels were renumbered: %v", visible)
	}
	if len(s["gpus"].([]Sample)) != 3 {
		t.Fatal("Full monitoring inventory was mutated")
	}
	prefs.GPUHidden = []string{rows[0]["display_key"].(string)}
	busyOnly := serialTelemetry(displayTelemetry(s, prefs)).(Sample)
	if busyOnly["gpu_load"] != float64(100) || busyOnly["gpu_load_available"] != 1 {
		t.Fatal("GPU1-only bar must show 100%", busyOnly)
	}
	if s["gpu_load"] != float64(50) {
		t.Fatal("Selection mutated full inventory sample")
	}
	prefs.HostKey = shortIdentity("another computer")
	if len(displayTelemetry(s, prefs).(Sample)["gpus"].([]Sample)) != 3 {
		t.Fatal("Another computer inherited exclusions")
	}
	prefs.HostKey = clientHostKey()
	prefs.GPUHidden = []string{rows[0]["display_key"].(string), busyKey, rows[2]["display_key"].(string)}
	allHidden := serialTelemetry(displayTelemetry(s, prefs)).(Sample)
	if len(allHidden["gpus"].([]Sample)) != 0 || allHidden["gpu_load"] != nil || allHidden["gpu_load_available"] != 0 {
		t.Fatal("All-hidden view must have no GPU activity reading")
	}
}
func TestGPUSelectionIdentitySurvivesReorderingAndKeepsTwinsDistinct(t *testing.T) {
	a := Sample{"id": "driver-a", "pci": "pci:0000:01:00.0", "name": "Twin"}
	b := Sample{"id": "driver-b", "pci": "pci:0000:02:00.0", "name": "Twin"}
	names := map[string]int{"twin": 2}
	if gpuDisplayKey(a, names) == gpuDisplayKey(b, names) {
		t.Fatal("Identical models collapsed")
	}
	changed := Sample{"id": "new-session-id", "pci": a["pci"], "name": "Twin"}
	if gpuDisplayKey(a, names) != gpuDisplayKey(changed, names) {
		t.Fatal("Session ID changed PCI selection identity")
	}
	apple := Sample{"id": "ioreg:123", "name": "Apple M4"}
	appleNext := Sample{"id": "ioreg:456", "name": "Apple M4"}
	if gpuDisplayKey(apple, map[string]int{"apple m4": 1}) != gpuDisplayKey(appleNext, map[string]int{"apple m4": 1}) {
		t.Fatal("Unique Apple GPU lost selection across registry sessions")
	}
}
func TestGPUPreferenceBounds(t *testing.T) {
	if !validGPUKeys([]string{}) || !validGPUKeys([]string{shortIdentity("a")}) {
		t.Fatal("Valid GPU preferences rejected")
	}
	for _, keys := range [][]string{{"../settings"}, {strings.Repeat("x", 32)}, {shortIdentity("a"), shortIdentity("a")}, make([]string, 17)} {
		if validGPUKeys(keys) {
			t.Fatal("Invalid GPU preference accepted", keys)
		}
	}
}

func TestSelectionStableWhileDriverEnrichmentArrives(t *testing.T) {
	native := Sample{"id": "windows-session", "pci": "pci:0000:01:00.0", "name": "NVIDIA native name", "_display_identity": "pci:0000:01:00.0:10de:1234"}
	first := Sample{"_gpu_platform": []Sample{native}}
	mergeGPUs(first)
	enriched := Sample{"_gpu_platform": []Sample{native}, "_gpu_nvidia": []Sample{{"id": "nvidia:GPU-1234", "pci": "pci:0000:01:00.0", "name": "NVIDIA detailed driver name", "load": float64(40)}}}
	mergeGPUs(enriched)
	a := first["gpus"].([]Sample)[0]
	b := enriched["gpus"].([]Sample)[0]
	if a["display_key"] != b["display_key"] {
		t.Fatal("A late driver reading changed the saved GPU choice")
	}
	if a["_display_identity"] != nil || b["_display_identity"] != nil {
		t.Fatal("Internal provider data leaked into the public GPU record")
	}
}
