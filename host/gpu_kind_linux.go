package main

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

func init() { linuxDRMGPUKind = queryLinuxGPUKind }
func drmInfo(fd uintptr, request uintptr, arg unsafe.Pointer) bool {
	_, _, e := unix.Syscall(unix.SYS_IOCTL, fd, request, uintptr(arg))
	runtime.KeepAlive(arg)
	return e == 0
}
func queryLinuxGPUKind(card, driver string) string {
	resolved, _ := filepath.EvalSymlinks(filepath.Join(card, "device"))
	if (driver == "nouveau" || driver == "nvidia") && !strings.Contains(resolved, "/platform/") {
		return "discrete"
	}
	nodes, _ := filepath.Glob(filepath.Join(card, "device/drm/renderD*"))
	nodes = append(nodes, card)
	for _, node := range nodes {
		file, e := os.OpenFile(filepath.Join("/dev/dri", filepath.Base(node)), os.O_RDONLY|unix.O_CLOEXEC, 0)
		if e != nil {
			continue
		}
		kind := queryDRMGPUKind(file.Fd(), driver)
		file.Close()
		if kind != "unknown" {
			return kind
		}
	}
	return "unknown"
}
func queryDRMGPUKind(fd uintptr, driver string) string {
	// Query buffers and nested UAPI pointers must remain pinned during ioctl.
	var pin runtime.Pinner
	defer pin.Unpin()
	switch driver {
	case "amdgpu":
		var data [144]byte
		pin.Pin(&data[0])
		q := struct {
			Pointer     uint64
			Size, Query uint32
			Reserved    [16]byte
		}{Pointer: uint64(uintptr(unsafe.Pointer(&data[0]))), Size: 144, Query: 0x16}
		ok := drmInfo(fd, 0x40206445, unsafe.Pointer(&q))
		if ok {
			return amdGPUKind(data[:])
		}
	case "i915":
		item := struct {
			ID      uint64
			Length  int32
			Flags   uint32
			Pointer uint64
		}{ID: 4}
		pin.Pin(&item)
		q := struct {
			Count, Flags uint32
			Pointer      uint64
		}{Count: 1, Pointer: uint64(uintptr(unsafe.Pointer(&item)))}
		if !drmInfo(fd, 0xc0106479, unsafe.Pointer(&q)) || item.Length < 16 || item.Length > 65536 {
			return "unknown"
		}
		data := make([]byte, item.Length)
		pin.Pin(&data[0])
		item.Pointer = uint64(uintptr(unsafe.Pointer(&data[0])))
		if drmInfo(fd, 0xc0106479, unsafe.Pointer(&q)) && item.Length > 0 {
			return intelGPUKind(driver, data)
		}
	case "xe":
		q := struct {
			Extensions  uint64
			Query, Size uint32
			Pointer     uint64
			Reserved    [2]uint64
		}{Query: 2}
		if !drmInfo(fd, 0xc0286440, unsafe.Pointer(&q)) || q.Size < 24 || q.Size > 65536 {
			return "unknown"
		}
		data := make([]byte, q.Size)
		pin.Pin(&data[0])
		q.Pointer = uint64(uintptr(unsafe.Pointer(&data[0])))
		if drmInfo(fd, 0xc0286440, unsafe.Pointer(&q)) {
			return intelGPUKind(driver, data)
		}
	}
	return "unknown"
}
