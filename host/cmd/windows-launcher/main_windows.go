//go:build windows

package main

// Small GUI architecture selector. The monitoring process itself is native x64/ARM64.
import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

func fail(message string) {
	text, _ := syscall.UTF16PtrFromString(message)
	title, _ := syscall.UTF16PtrFromString("System Companion")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
	os.Exit(1)
}
func main() {
	var info [64]byte
	syscall.NewLazyDLL("kernel32.dll").NewProc("GetNativeSystemInfo").Call(uintptr(unsafe.Pointer(&info[0])))
	arch := ""
	switch *(*uint16)(unsafe.Pointer(&info[0])) {
	case 9:
		arch = "amd64"
	case 12:
		arch = "arm64"
	default:
		fail("This package requires 64-bit Windows (x64 or ARM64).")
	}
	exe, e := os.Executable()
	if e != nil {
		fail(e.Error())
	}
	target := filepath.Join(filepath.Dir(exe), arch, "Monitor.exe")
	child := exec.Command(target, os.Args[1:]...)
	child.Dir = filepath.Dir(exe)
	child.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	// GUI launches may have no valid standard handles. Leave those streams
	// disconnected unless a caller deliberately supplied a usable handle.
	if os.Stdout != nil {
		if _, err := os.Stdout.Stat(); err == nil {
			child.Stdout = os.Stdout
		}
	}
	if os.Stderr != nil {
		if _, err := os.Stderr.Stat(); err == nil {
			child.Stderr = os.Stderr
		}
	}
	if e = child.Run(); e != nil {
		if exit, ok := e.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		fail("Unable to start Monitor: " + e.Error())
	}
}
