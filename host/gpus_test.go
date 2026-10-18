package main

import (
	"context"
	"encoding/json"
	"math"
	"testing"
)

func TestAllNvidiaAdaptersAndUnsupportedFields(t *testing.T) {
	a := nvidiaGPU("0, GPU-one, 00000000:01:00.0, Card A, 48, 75, 1024, 8192, 90, 1700, 7000, 35")
	b := nvidiaGPU("1, GPU-two, 00000000:02:00.0, Card B, [N/A], 0, 0, 4096, [Not Supported], 500, 1000, [N/A]")
	if a["pci"] != "pci:0000:01:00.0" || a["vram"] != float64(1) || b["temp"] != nil || b["load"] != float64(0) {
		t.Fatalf("bad driver rows: %v %v", a, b)
	}
	s := Sample{"_gpu_nvidia": []Sample{a, b}}
	mergeGPUs(s)
	if s["gpu_count"] != 2 || s["gpu_temp"] != float64(48) || s["gpu_vram_total"] != float64(12) {
		t.Fatalf("bad aggregate: %v", s)
	}
}
func TestGPUIdentityMergesSameCardAndKeepsIdenticalModels(t *testing.T) {
	platform := []Sample{{"id": "a", "pci": "pci:0000:01:00.0", "name": "Twin GPU"}, {"id": "b", "pci": "pci:0000:02:00.0", "name": "Twin GPU"}}
	driver := []Sample{{"id": "uuid-a", "pci": "pci:0000:01:00.0", "name": "Twin GPU", "temp": float64(45)}, {"id": "uuid-b", "pci": "pci:0000:02:00.0", "name": "Twin GPU", "temp": float64(63)}}
	s := Sample{"_gpu_platform": platform, "_gpu_nvidia": driver}
	mergeGPUs(s)
	rows := s["gpus"].([]Sample)
	if len(rows) != 2 || rows[0]["temp"] != float64(45) || rows[1]["temp"] != float64(63) {
		t.Fatalf("physical identities lost: %v", rows)
	}
	if platform[0]["temp"] != nil {
		t.Fatal("published provider maps were mutated")
	}
}
func TestGPUInvalidMetricsAndZeroLoad(t *testing.T) {
	s := Sample{"_gpu_platform": []Sample{{"id": "a", "name": "A", "temp": float64(0), "load": float64(0), "power": math.Inf(1)}}}
	mergeGPUs(s)
	g := s["gpus"].([]Sample)[0]
	if g["temp"] != nil || g["power"] != nil || g["load"] != float64(0) {
		t.Fatalf("invalid sensor values: %v", g)
	}
	if _, e := json.Marshal(s); e != nil {
		t.Fatal(e)
	}
}
func TestSMCTemperatureFormats(t *testing.T) {
	n, ok := decodeSMCTemperature("sp78", []byte{0x32, 0x80})
	if !ok || n != 50.5 {
		t.Fatalf("bad SMC fixed point: %v", n)
	}
	n, ok = decodeSMCTemperature("flt ", []byte{0, 0, 0x48, 0x42})
	if !ok || n != 50 {
		t.Fatalf("bad SMC float: %v", n)
	}
	if _, ok = decodeSMCTemperature("flt ", []byte{0, 0, 0xc0, 0x7f}); ok {
		t.Fatal("SMC NaN accepted")
	}
	if _, ok = decodeSMCTemperature("sp78", []byte{0, 0}); ok {
		t.Fatal("inactive SMC sensor accepted")
	}
}
func TestCollectMergesGPUs(t *testing.T) {
	p := fixtureProvider{data: Sample{"_gpu_platform": []Sample{{"id": "a", "name": "A", "temp": float64(51)}}}}
	s := collect(context.Background(), []Provider{p}, 1)
	if s["gpu_count"] != 1 || s["_gpu_platform"] != nil {
		t.Fatalf("wire model incomplete: %v", s)
	}
}

type fixtureProvider struct{ data Sample }

func (p fixtureProvider) Collect(_ context.Context, s Sample) {
	for k, v := range p.data {
		s[k] = v
	}
}

func TestPartialInventoriesDoNotMergeDifferentPCIIdentities(t *testing.T) {
	s := Sample{
		"_gpu_platform": []Sample{{"id": "native-a", "pci": "pci:0000:01:00.0", "name": "Twin GPU", "load": float64(10)}},
		"_gpu_nvidia":   []Sample{{"id": "driver-b", "pci": "pci:0000:02:00.0", "name": "Twin GPU", "temp": float64(60)}},
	}
	mergeGPUs(s)
	rows := s["gpus"].([]Sample)
	if len(rows) != 2 || rows[0]["temp"] != nil || rows[1]["load"] != nil {
		t.Fatalf("conflicting physical identities were merged: %v", rows)
	}
}
