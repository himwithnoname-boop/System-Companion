package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const startupKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const startupValue = "SystemCompanion"

var startupMu sync.Mutex

func startupAvailable() bool { return true }
func startupEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, startupKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetStringValue(startupValue)
	return err == nil && value != ""
}

func copyStartupFile(source, target string) error {
	if strings.EqualFold(filepath.Clean(source), filepath.Clean(target)) {
		return nil
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(target), ".update-*")
	if err != nil {
		return err
	}
	name := out.Name()
	defer os.Remove(name)
	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	// Windows cannot replace a currently running executable. Report the error
	// before changing the Run entry, leaving its working installation intact.
	return os.Rename(name, target)
}

func setStartup(enabled bool) error {
	startupMu.Lock()
	defer startupMu.Unlock()
	if !enabled {
		key, err := registry.OpenKey(registry.CURRENT_USER, startupKey, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer key.Close()
		err = key.DeleteValue(startupValue)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(cache, "SystemCompanion")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "Monitor.exe")
	if err = copyStartupFile(exe, target); err != nil {
		return fmt.Errorf("startup copy could not be updated: %w", err)
	}
	sensorDir := sensorDirectory(exe)
	if files, e := os.ReadDir(sensorDir); e == nil {
		destination := filepath.Join(dir, "Sensors")
		if err = os.MkdirAll(destination, 0700); err != nil {
			return err
		}
		for _, file := range files {
			ext := strings.ToLower(filepath.Ext(file.Name()))
			if !file.IsDir() && (ext == ".dll" || file.Name() == "Sensors.exe") {
				if err = copyStartupFile(filepath.Join(sensorDir, file.Name()), filepath.Join(destination, file.Name())); err != nil {
					return err
				}
			}
		}
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, startupKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(startupValue, syscall.EscapeArg(target)+" --startup --no-browser")
}
