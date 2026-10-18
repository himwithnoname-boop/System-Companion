package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestLinuxTrayMenuContract(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	tray := &linuxTray{revision: 1, reconnect: func() {}, quit: func() {}}
	revision, layout, err := tray.GetLayout(0, -1, nil)
	if err != nil || revision != 1 || len(layout.Children) != 5 {
		t.Fatalf("menu: %d %+v %v", revision, layout, err)
	}
	if signature := dbus.SignatureOf(layout).String(); signature != "(ia{sv}av)" {
		t.Fatal(signature)
	}
	if tray.properties(3)["toggle-state"].Value() != int32(0) {
		t.Fatal("startup enabled by default")
	}
	// Verify that the two critical menu actions invoke their callbacks.
	reconnects, exits := 0, 0
	tray.reconnect = func() { reconnects++ }
	tray.quit = func() { exits++ }
	tray.Event(2, "clicked", dbus.MakeVariant(int32(0)), 0)
	tray.Event(5, "clicked", dbus.MakeVariant(int32(0)), 0)
	if reconnects != 1 || exits != 1 {
		t.Fatal(reconnects, exits)
	}
	if len(trayPixels()[0].Data) != 32*32*4 {
		t.Fatal("icon pixels")
	}
	_ = context.Background()
}

func TestLinuxStartupIsExplicitAndUsesStableCopy(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache with % and $"))
	if startupEnabled() {
		t.Fatal("startup starts enabled")
	}
	if err := setStartup(true); err != nil {
		t.Fatal(err)
	}
	if !startupEnabled() {
		t.Fatal("startup entry missing")
	}
	entry, _ := startupEntry()
	data, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "--startup --no-browser") || !strings.Contains(text, "Terminal=false") || !strings.Contains(text, "%%") {
		t.Fatal(text)
	}
	target := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "system-companion", "startup", "Monitor")
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal(info, err)
	}
	if err = setStartup(false); err != nil {
		t.Fatal(err)
	}
	if startupEnabled() {
		t.Fatal("startup remained enabled")
	}
}
