package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var startupMu sync.Mutex

func startupAvailable() bool { return true }
func macStartupPaths() (entry, executable string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", macStartupLabel+".plist"),
		filepath.Join(home, "Library", "Application Support", "System Companion", "Monitor"), nil
}
func startupEnabled() bool {
	entry, executable, err := macStartupPaths()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(entry)
	return err == nil && string(data) == string(macStartupPlist(executable))
}
func macAtomicFile(target string, write func(io.Writer) error, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".companion-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = write(f)
	if err == nil {
		err = f.Chmod(mode)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), target)
}
func setStartup(enabled bool) error {
	startupMu.Lock()
	defer startupMu.Unlock()
	entry, target, err := macStartupPaths()
	if err != nil {
		return err
	}
	if !enabled {
		err = os.Remove(entry)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		return err
	}
	source, err := os.Executable()
	if err != nil {
		return err
	}
	if filepath.Clean(source) != target {
		in, err := os.Open(source)
		if err != nil {
			return err
		}
		defer in.Close()
		if err = macAtomicFile(target, func(w io.Writer) error { _, e := io.Copy(w, in); return e }, 0700); err != nil {
			return err
		}
	}
	data := macStartupPlist(target)
	if err = macAtomicFile(entry, func(w io.Writer) error { _, e := w.Write(data); return e }, 0600); err != nil {
		return err
	}
	// Enable for future logins, without launching a second instance now.
	// --startup exits when the display is absent; no KeepAlive or polling.
	command := exec.Command("/bin/launchctl", "enable", fmt.Sprintf("gui/%d/%s", os.Getuid(), macStartupLabel))
	if output, e := command.CombinedOutput(); e != nil {
		_ = os.Remove(entry)
		return fmt.Errorf("login item: %v: %s", e, strings.TrimSpace(string(output)))
	}
	return nil
}
