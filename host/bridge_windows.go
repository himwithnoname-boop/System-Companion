package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var sensorActionMu sync.Mutex

type sensorBridge struct {
	path, sid             string
	mu                    sync.Mutex
	status                string
	lastRead              time.Time
	values                map[string]any
	pending, taskStarting bool
	taskAttempt           time.Time
}

type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            uintptr
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance, IDList                  uintptr
	Class                             *uint16
	ClassKey                          uintptr
	HotKey                            uint32
	Icon                              uintptr
	Process                           windows.Handle
}

// ShellExecute on a background thread must finish its launch before the
// argument buffers and thread are released. The returned handle is ours.
func launchSensorHelper(verb, path, arguments string) (windows.Handle, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if oe, ok := err.(*ole.OleError); !ok || oe.Code() != 1 {
			return 0, err
		}
	}
	defer ole.CoUninitialize()
	info := shellExecuteInfo{Mask: 0x40 | 0x100 | 0x400, Verb: wide(verb), File: wide(path), Parameters: wide(arguments), Directory: wide(filepath.Dir(path))}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, err := shell32.NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(&info)
	if ok == 0 {
		return 0, err
	}
	if info.Process == 0 {
		return 0, fmt.Errorf("sensor launch returned no process handle")
	}
	return info.Process, nil
}

func sensorProvider(ctx context.Context) (Provider, func() error) {
	exe, _ := os.Executable()
	b := &sensorBridge{path: filepath.Join(sensorDirectory(exe), "Sensors.exe")}
	b.sid, _ = sensorSID()
	b.status = "CPU temperatures are off. Enable CPU temperatures once to save access on this PC."
	if sensorAccessRemembered() {
		b.status = "Starting CPU temperature reader..."
	}
	sensorRememberAction = b.remember
	sensorStateAction = func() Sample {
		b.mu.Lock()
		defer b.mu.Unlock()
		return Sample{"available": true, "enabled": sensorAccessRemembered(), "pending": b.pending, "status": b.status}
	}
	go func() {
		<-ctx.Done()
		if sensorAccessRemembered() {
			rememberedSensorTask(b.sid, "stop")
		}
	}()
	return b, func() error { return b.remember(true) }
}
func (b *sensorBridge) Collect(ctx context.Context, s Sample) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.status != "" {
		s["sensor_status"] = b.status
	}
	if b.pending || !sensorAccessRemembered() {
		return
	}
	if time.Since(b.lastRead) >= 2*time.Second {
		b.lastRead = time.Now()
		data, err := persistentSensorRead(b.sid)
		if err == nil {
			var values map[string]any
			if json.Unmarshal(data, &values) == nil {
				b.values = values
			}
		} else if !b.taskStarting && time.Since(b.taskAttempt) >= 30*time.Second {
			b.taskStarting = true
			b.taskAttempt = time.Now()
			go func() {
				err := rememberedSensorTask(b.sid, "run")
				b.mu.Lock()
				defer b.mu.Unlock()
				b.taskStarting = false
				if err != nil {
					b.status = "Saved CPU sensor access is unavailable. Turn CPU temperatures off and on to repair it."
				} else {
					b.status = "Starting CPU temperature reader..."
				}
			}()
		}
	}
	stamp, ok := b.values["time"].(float64)
	if !ok || time.Now().Unix()-int64(stamp) > 6 || int64(stamp) > time.Now().Unix()+1 {
		return
	}
	if status, ok := b.values["sensor_status"].(string); ok {
		s["sensor_status"] = status
		b.status = status
	}
	for _, key := range []string{"cpu_temp", "cpu_power", "cpu_clock", "cpu_voltage"} {
		if value, ok := b.values[key].(float64); ok && value > 0 {
			s[key] = finite(value)
			if key == "cpu_temp" {
				s["cpu_temp_source"] = "Approved CPU sensor helper"
				s["cpu_temp_kind"] = "cpu_sensor"
			}
		}
	}
	s["_expires_at"] = time.Unix(int64(stamp)+6, 0)
	if _, ok := s["cpu_temp"]; ok {
		delete(s, "sensor_status")
		b.status = ""
	}
}
func (b *sensorBridge) remember(enabled bool) error {
	sensorActionMu.Lock()
	defer sensorActionMu.Unlock()
	b.mu.Lock()
	if b.pending {
		b.mu.Unlock()
		return fmt.Errorf("CPU setup is already in progress")
	}
	b.mu.Unlock()
	if !enabled {
		if err := writeSensorRemembered(false); err != nil {
			return err
		}
		rememberedSensorTask(b.sid, "stop")
		b.mu.Lock()
		b.values = nil
		b.lastRead = time.Time{}
		b.status = "CPU temperatures are off."
		b.mu.Unlock()
		return nil
	}
	if err := rememberedSensorTask(b.sid, "check"); err == nil {
		if err = rememberedSensorTask(b.sid, "run"); err != nil {
			return err
		}
		if err = writeSensorRemembered(true); err != nil {
			return err
		}
		b.mu.Lock()
		b.values = nil
		b.lastRead = time.Time{}
		b.taskAttempt = time.Now()
		b.status = "Starting CPU temperature reader..."
		b.mu.Unlock()
		return nil
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\PawnIO`, registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("no compatible CPU sensor driver is installed; Monitor installs no drivers")
	}
	key.Close()
	if _, err = os.Stat(b.path); err != nil {
		return fmt.Errorf("optional CPU sensor helper unavailable: %w", err)
	}
	// A dedicated setup filename also identifies its mode if a Windows elevation
	// handler drops command arguments. A missing argument never starts a reader.
	setup := filepath.Join(filepath.Dir(b.path), ".setup", b.sid, "Monitor CPU Setup.exe")
	if err = os.MkdirAll(filepath.Dir(setup), 0700); err != nil {
		return err
	}
	payload, err := os.ReadFile(b.path)
	if err != nil {
		return err
	}
	if err = os.WriteFile(setup, payload, 0600); err != nil {
		return err
	}
	os.Remove(filepath.Join(filepath.Dir(b.path), "cpu-setup.log"))
	b.mu.Lock()
	b.pending = true
	b.status = "Waiting for one-time Windows approval for CPU temperatures..."
	b.mu.Unlock()
	go func() {
		process, launchErr := launchSensorHelper("runas", setup, "--install-persistent --sid "+syscall.EscapeArg(b.sid))
		var code uint32 = 1
		if launchErr == nil {
			windows.WaitForSingleObject(process, windows.INFINITE)
			windows.GetExitCodeProcess(process, &code)
			windows.CloseHandle(process)
		}
		success := launchErr == nil && code == 0
		if success {
			if err := rememberedSensorTask(b.sid, "check"); err != nil {
				launchErr = err
				success = false
			}
		}
		if success {
			if err := writeSensorRemembered(true); err != nil {
				launchErr = err
				success = false
			}
		}
		os.Remove(setup)
		os.Remove(filepath.Dir(setup))
		os.Remove(filepath.Dir(filepath.Dir(setup)))
		b.mu.Lock()
		defer b.mu.Unlock()
		b.pending = false
		b.values = nil
		b.lastRead = time.Time{}
		if success {
			b.status = "Starting CPU temperature reader..."
			b.taskAttempt = time.Now()
		} else {
			b.status = fmt.Sprintf("CPU setup failed (exit %d). CPU temperatures remain off.", code)
			if launchErr != nil {
				b.status = "CPU setup was not completed: " + launchErr.Error()
			}
			if data, err := os.ReadFile(filepath.Join(filepath.Dir(b.path), "cpu-setup.log")); err == nil && len(data) < 2048 {
				b.status += " " + strings.TrimSpace(string(data))
			}
		}
	}()
	return nil
}
