package main

import (
	"context"
	"golang.org/x/sys/windows"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsNativeLayout(t *testing.T) {
	if unsafe.Sizeof(adapterDesc{}) != 312 || unsafe.Sizeof(adapterQuery{}) != 24 || unsafe.Sizeof(adapterPerf{}) != 64 || unsafe.Sizeof(pdhItem{}) != 24 {
		t.Fatal("native ABI structure size mismatch")
	}
}
func TestWindowsNativeReadOnly(t *testing.T) {
	t.Logf("Windows token elevated: %v", windows.GetCurrentProcessToken().IsElevated())
	p := &windowsPlatform{}
	s := Sample{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p.Collect(ctx, s)
	rows, _ := s["_gpu_platform"].([]Sample)
	if len(rows) == 0 {
		t.Skip("No readable DXGI adapters in this environment")
	}
	for _, g := range rows {
		t.Logf("adapter %s: name=%s pci=%s temperature=%v", g["id"], g["name"], g["pci"], g["temp"])
	}
}
