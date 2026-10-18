package main

import (
	"os"
	"path/filepath"
	"runtime"
	"unsafe"
)

func sensorDirectory(exe string) string {
	direct := filepath.Join(filepath.Dir(exe), "Sensors")
	if info, e := os.Stat(direct); e == nil && info.IsDir() {
		return direct
	}
	shared := filepath.Join(filepath.Dir(filepath.Dir(exe)), "Sensors")
	if info, e := os.Stat(shared); e == nil && info.IsDir() {
		return shared
	}
	return direct
}
func openBrowser(url string) {
	verb, target := wide("open"), wide(url)
	shell32.NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, 1)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(target)
}
