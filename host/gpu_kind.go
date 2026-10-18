package main

import "encoding/binary"

var linuxDRMGPUKind = func(card, driver string) string { return "unknown" }

func gpuKindRank(g Sample) int {
	switch g["kind"] {
	case "integrated":
		return 0
	case "discrete":
		return 1
	}
	return 2
}
func amdGPUKind(data []byte) string {
	if len(data) < 144 {
		return "unknown"
	}
	if binary.LittleEndian.Uint64(data[136:144])&1 != 0 {
		return "integrated"
	}
	return "discrete"
}
func intelGPUKind(driver string, data []byte) string {
	if driver == "xe" {
		if len(data) < 24 || binary.LittleEndian.Uint32(data[:4]) < 2 {
			return "unknown"
		}
		if binary.LittleEndian.Uint64(data[16:24])&1 != 0 {
			return "discrete"
		}
		return "integrated"
	}
	if len(data) < 16 {
		return "unknown"
	}
	n := int(binary.LittleEndian.Uint32(data[:4]))
	if n < 1 || n > 64 || len(data) < 16+n*88 {
		return "unknown"
	}
	for i := 0; i < n; i++ {
		if binary.LittleEndian.Uint16(data[16+i*88:]) == 1 {
			return "discrete"
		}
	}
	return "integrated"
}
