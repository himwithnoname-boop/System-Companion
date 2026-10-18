package main

import (
	"bytes"
	"encoding/json"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentSensorPipeWithRealHelper(t *testing.T) {
	path := os.Getenv("COMPANION_SENSOR_HELPER")
	if path == "" {
		t.Skip("set COMPANION_SENSOR_HELPER to exercise the built helper without elevation")
	}
	if sensorAccessRemembered() {
		t.Skip("do not disturb an installed sensor helper")
	}
	sid, e := sensorSID()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(path, "--persistent", "--sid", sid, "--diagnostic")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	hideCommand(cmd)
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}()
	var b []byte
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		b, e = persistentSensorRead(sid)
		if e == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if e != nil {
		select {
		case childErr := <-done:
			t.Fatal("sensor helper exited", childErr, stderr.String())
		default:
			t.Fatal("native sensor pipe failed", e)
		}
	}
	var s Sample
	if e = json.Unmarshal(b, &s); e != nil {
		t.Fatal(e)
	}
	stamp, ok := s["time"].(float64)
	if !ok || stamp < float64(time.Now().Unix()-3) {
		t.Fatal("stale or invalid helper reading", s)
	}
	if _, ok = s["cpu_temp"]; !ok {
		if _, ok = s["sensor_status"].(string); !ok {
			t.Fatal("missing unavailable status", s)
		}
	}
}

func TestSensorShellLaunchPreservesArguments(t *testing.T) {
	path := os.Getenv("COMPANION_SENSOR_HELPER")
	if path == "" {
		t.Skip("built Windows helper required")
	}
	sid, err := sensorSID()
	if err != nil {
		t.Fatal(err)
	}
	handle, err := launchSensorHelper("open", path, "--install-persistent --sid "+sid)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 4000)
	if err != nil || state != uint32(windows.WAIT_OBJECT_0) {
		windows.TerminateProcess(handle, 1)
		t.Fatal("setup arguments were lost or installer hung", state, err)
	}
	var code uint32
	windows.GetExitCodeProcess(handle, &code)
	if code != 3 {
		t.Fatal("unelevated install must refuse before creating protected files", code)
	}
}
func TestSensorSetupModeSurvivesMissingArguments(t *testing.T) {
	path := os.Getenv("COMPANION_SENSOR_HELPER")
	if path == "" {
		t.Skip("built Windows helper required")
	}
	sid, err := sensorSID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), ".setup", sid, "Monitor CPU Setup.exe")
	if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(dest, b, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(dest)
	hideCommand(cmd)
	err = cmd.Run()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 3 {
		t.Fatal("setup mode must reject unelevated installation even without arguments", err)
	}
}
