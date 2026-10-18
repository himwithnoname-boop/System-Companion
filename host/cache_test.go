package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type providerFunc func(context.Context, Sample)

func (f providerFunc) Collect(ctx context.Context, s Sample) { f(ctx, s) }

func TestStuckCollectorDoesNotBlockSamplingOrMultiplyWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	blocked, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	p := startCached(ctx, providerFunc(func(_ context.Context, s Sample) {
		if calls.Add(1) == 1 {
			s["cpu_temp"] = 42.0
			return
		}
		close(blocked)
		<-release // Deliberately emulate a native call ignoring cancellation.
	}), 10*time.Millisecond, 40*time.Millisecond, 10*time.Millisecond)
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("collector did not start")
	}
	for i := 0; i < 100; i++ {
		complete := make(chan Sample, 1)
		go func() { s := collect(ctx, []Provider{p}, uint64(i)); complete <- s }()
		select {
		case <-complete:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("sampling waited for the blocked collector")
		}
	}
	time.Sleep(50 * time.Millisecond)
	s := collect(ctx, []Provider{p}, 101)
	if _, ok := s["cpu_temp"]; ok {
		t.Fatal("expired temperature stayed visible")
	}
	if calls.Load() != 2 {
		t.Fatal("a stuck collector spawned additional work")
	}
	cancel()
	close(release)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
}

func TestAbsentProviderBacksOff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	var calls atomic.Int32
	p := startCached(ctx, providerFunc(func(_ context.Context, _ Sample) {
		calls.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
	}), 5*time.Millisecond, time.Second, 100*time.Millisecond)
	<-started
	time.Sleep(30 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatal("absent backend was polled at the live telemetry rate")
	}
	cancel()
	<-p.done
}

func TestGPUCSVPreservesUnavailableReadings(t *testing.T) {
	s := nvidiaSample(`GPU, 42, 0, 2048, 8192, N/A, 500, NaN, [Not Supported]`)
	if s["gpu_temp"] != 42.0 || s["gpu_load"] != 0.0 || s["gpu_vram"] != 2.0 {
		t.Fatalf("valid readings lost: %v", s)
	}
	for _, key := range []string{"gpu_power", "gpu_mem_clock", "gpu_fan"} {
		if _, ok := s[key]; ok {
			t.Fatalf("unavailable %s was fabricated", key)
		}
	}
	for _, line := range []string{"NVIDIA driver error", "GPU,N/A,N/A,N/A,N/A,N/A,N/A,N/A,N/A"} {
		if nvidiaSample(line) != nil {
			t.Fatal("diagnostic output was accepted as telemetry")
		}
	}
}
