package main

import (
	"encoding/binary"
	"testing"
)

func TestIntegratedGPUZeroAndDiscreteGPUOne(t *testing.T) {
	s := Sample{"_gpu_platform": []Sample{{"id": "d", "pci": "pci:0000:01:00.0", "kind": "discrete", "name": "PCIe", "load": float64(100)}, {"id": "i", "pci": "pci:0000:60:00.0", "kind": "integrated", "name": "Onboard", "load": float64(0)}}}
	mergeGPUs(s)
	g := s["gpus"].([]Sample)
	if g[0]["name"] != "Onboard" || g[0]["index"] != 0 || g[1]["index"] != 1 || s["gpu_load"] != float64(50) {
		t.Fatal(s)
	}
	key := g[1]["display_key"]
	s = Sample{"_gpu_platform": []Sample{{"id": "d", "pci": "pci:0000:01:00.0", "kind": "discrete", "name": "PCIe"}}}
	mergeGPUs(s)
	g = s["gpus"].([]Sample)
	if g[0]["index"] != 1 || g[0]["display_key"] != key {
		t.Fatal(s)
	}
}
func TestLinuxIntegratedGPUClassification(t *testing.T) {
	a := make([]byte, 144)
	if amdGPUKind(a) != "discrete" {
		t.Fatal("AMD discrete")
	}
	a[136] = 1
	if amdGPUKind(a) != "integrated" {
		t.Fatal("AMD APU")
	}
	x := make([]byte, 24)
	binary.LittleEndian.PutUint32(x, 2)
	if intelGPUKind("xe", x) != "integrated" {
		t.Fatal("Xe UMA")
	}
	x[16] = 1
	if intelGPUKind("xe", x) != "discrete" {
		t.Fatal("Xe VRAM")
	}
	i := make([]byte, 16+88)
	i[0] = 1
	if intelGPUKind("i915", i) != "integrated" {
		t.Fatal("i915 UMA")
	}
	i[16] = 1
	if intelGPUKind("i915", i) != "discrete" {
		t.Fatal("i915 VRAM")
	}
	for _, data := range [][]byte{nil, {1, 2}, make([]byte, 17)} {
		if amdGPUKind(data) != "unknown" || intelGPUKind("i915", data) != "unknown" || intelGPUKind("xe", data) != "unknown" {
			t.Fatal("Invalid ABI data classified")
		}
	}
}
