package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shirou/gopsutil/v4/host"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

var hostKeyOnce sync.Once
var hostKeyValue string

func shortIdentity(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:16])
}
func clientHostKey() string {
	hostKeyOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		id, _ := host.HostIDWithContext(ctx)
		if id == "" {
			id, _ = os.Hostname()
			id = runtime.GOOS + ":" + id
		}
		hostKeyValue = shortIdentity(id)
	})
	return hostKeyValue
}
func gpuDisplayKey(g Sample, names map[string]int) string {
	if native, ok := g["_display_identity"].(string); ok && native != "" {
		return shortIdentity(native)
	}
	id, _ := g["id"].(string)
	pci, _ := g["pci"].(string)
	identity := id
	switch {
	case pci != "":
		identity = pci
	case strings.Contains(strings.ToLower(id), "gpu-"): // NVIDIA hardware UUID when PCI identity is unavailable.
	case names[gpuName(g)] == 1:
		identity = "model:" + gpuName(g)
	}
	return shortIdentity(identity + "|" + gpuName(g))
}
func validGPUKeys(keys []string) bool {
	if len(keys) > 16 {
		return false
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if len(key) != 32 || seen[key] {
			return false
		}
		if _, e := hex.DecodeString(key); e != nil || strings.ToLower(key) != key {
			return false
		}
		seen[key] = true
	}
	return true
}

// Display activity follows the selected GPUs; collection retains every adapter.
func displayTelemetry(v any, prefs *DisplaySettings) any {
	s, ok := v.(Sample)
	if !ok || s["type"] != "telemetry" {
		return v
	}
	rows, ok := s["gpus"].([]Sample)
	if !ok {
		return v
	}
	hidden := map[string]bool{}
	if prefs != nil && prefs.GPUSelection && prefs.HostKey == clientHostKey() {
		for _, key := range prefs.GPUHidden {
			hidden[key] = true
		}
	}
	visible := make([]Sample, 0, len(rows))
	for _, g := range rows {
		key, _ := g["display_key"].(string)
		if !hidden[key] {
			visible = append(visible, g)
		}
	}
	result := Sample{}
	for key, value := range s {
		result[key] = value
	}
	result["gpus"] = visible
	result["gpu_detected"] = len(rows)
	result["gpu_selected"] = len(visible)
	sum, reporting := 0.0, 0
	for _, gpu := range visible {
		if load, ok := metricNumber(gpu["load"]); ok {
			sum += load
			reporting++
		}
	}
	result["gpu_load"] = nil
	result["gpu_load_available"] = reporting
	result["gpu_load_method"] = "mean_selected_available"
	if reporting > 0 {
		result["gpu_load"] = sum / float64(reporting)
	}
	return result
}
func settingsMatch(s *DisplaySettings, changes Sample) bool {
	if s == nil {
		return false
	}
	data, _ := json.Marshal(s)
	var got map[string]json.RawMessage
	json.Unmarshal(data, &got)
	for key, value := range changes {
		if key == "type" || key == "v" {
			continue
		}
		want, _ := json.Marshal(value)
		if string(got[key]) != string(want) {
			return false
		}
	}
	return true
}
func (d *Device) settingsSignalLocked() {
	if d.settingsChanged != nil {
		close(d.settingsChanged)
	}
	d.settingsChanged = make(chan struct{})
	d.settingsRevision++
}

// Wait for the board to report the committed preferences, not just a USB write.
func (d *Device) ApplySettings(ctx context.Context, changes Sample) error {
	d.settingsMu.Lock()
	defer d.settingsMu.Unlock()
	d.mu.Lock()
	revision := d.settingsRevision
	d.lastSettingsError = ""
	d.mu.Unlock()
	if e := d.Send(changes); e != nil {
		return e
	}
	for {
		d.mu.Lock()
		if d.port == nil {
			d.mu.Unlock()
			return errors.New("display disconnected")
		}
		if d.settingsRevision > revision {
			if d.lastSettingsError != "" {
				e := errors.New(d.lastSettingsError)
				d.mu.Unlock()
				return e
			}
			if settingsMatch(d.settings, changes) {
				d.mu.Unlock()
				return nil
			}
		}
		changed := d.settingsChanged
		d.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return fmt.Errorf("display did not confirm settings: %w", ctx.Err())
		}
	}
}
