package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var startupMu sync.Mutex

func startupAvailable() bool { return true }
func startupEntry() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "system-companion.desktop"), nil
}
func startupEnabled() bool {
	entry, err := startupEntry()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(entry)
	return err == nil && strings.Contains(string(data), "X-SystemCompanion=true")
}
func writeStartupFile(target string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(target), ".companion-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, target)
}
func setStartup(enabled bool) error {
	startupMu.Lock()
	defer startupMu.Unlock()
	entry, err := startupEntry()
	if err != nil {
		return err
	}
	if !enabled {
		err = os.Remove(entry)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(cache, "system-companion", "startup")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "Monitor")
	if filepath.Clean(exe) != target {
		source, err := os.Open(exe)
		if err != nil {
			return err
		}
		defer source.Close()
		tmp, err := os.CreateTemp(dir, ".monitor-*")
		if err != nil {
			return err
		}
		name := tmp.Name()
		defer os.Remove(name)
		_, err = io.Copy(tmp, source)
		if err == nil {
			err = tmp.Chmod(0700)
		}
		closeErr := tmp.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		if err = os.Rename(name, target); err != nil {
			return err
		}
	}
	if err = os.MkdirAll(filepath.Dir(entry), 0700); err != nil {
		return err
	}
	// Desktop Exec parsing is not a shell. Escape both desktop string and
	// reserved Exec characters, including literal percent field codes.
	escaped := strings.NewReplacer("\\", "\\\\\\\\", "\"", "\\\\\"", "$", "\\\\$", "`", "\\\\`", "%", "%%").Replace(target)
	data := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=System Companion\nExec=\"%s\" --startup --no-browser\nTerminal=false\nX-SystemCompanion=true\n", escaped)
	return writeStartupFile(entry, []byte(data), 0600)
}
