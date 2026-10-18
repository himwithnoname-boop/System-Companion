package main

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// Each collector has exactly one worker. A slow or stuck native call cannot
// delay telemetry or accumulate more workers. Published maps are immutable.
type cachedProvider struct {
	mu     sync.RWMutex
	data   Sample
	at     time.Time
	maxAge time.Duration
	done   chan struct{}
}

func startCached(ctx context.Context, source Provider, interval, maxAge, emptyRetry time.Duration) *cachedProvider {
	p := &cachedProvider{maxAge: maxAge, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		timer := time.NewTimer(0)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			data := Sample{}
			child, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			source.Collect(child, data)
			cancel()
			at := time.Now()
			if expires, ok := data["_expires_at"].(time.Time); ok {
				// A file-backed sensor's own timestamp can expire before the
				// worker cache. Never extend the life of that reading.
				at = minTime(at, expires.Add(-maxAge))
				delete(data, "_expires_at")
			}
			p.mu.Lock()
			p.data, p.at = data, at
			p.mu.Unlock()
			delay := interval
			if len(data) == 0 && emptyRetry > delay {
				delay = emptyRetry
			}
			timer.Reset(delay)
		}
	}()
	return p
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (p *cachedProvider) Collect(_ context.Context, s Sample) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if time.Since(p.at) > p.maxAge {
		return
	}
	for key, value := range p.data {
		s[key] = value
	}
}

func startProviders(parent context.Context, interval time.Duration) ([]Provider, func()) {
	ctx, cancel := context.WithCancel(parent)
	workers := []*cachedProvider{}
	providers := []Provider{}
	add := func(source Provider, period, age, absent time.Duration) {
		worker := startCached(ctx, source, period, age, absent)
		workers = append(workers, worker)
		providers = append(providers, worker)
	}
	add(&OSProvider{}, interval, 3*interval+time.Second, interval)
	gpuPeriod := max(interval, time.Second)
	gpu := startNvidia(ctx, gpuPeriod)
	providers = append(providers, gpu)
	add(newPlatformProvider(), 2*time.Second, 6*time.Second, 30*time.Second)
	add(&StorageProvider{}, max(interval, 2*time.Second), 12*time.Second, 30*time.Second)
	discovery := &aiDiscovery{}
	add(&ProcessProvider{ai: discovery}, 5*time.Second, 15*time.Second, 15*time.Second)
	if runtime.GOOS == "windows" {
		add(HardwareProvider{}, 2*time.Second, 6*time.Second, 30*time.Second)
	}
	add(newAIProvider(discovery), 5*time.Second, 12*time.Second, 30*time.Second)
	return providers, func() {
		cancel()
		// GPU cancellation kills and reaps the single child before returning.
		<-gpu.done
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		for _, worker := range workers {
			select {
			case <-worker.done:
			case <-timer.C:
				return
			}
		}
	}
}
