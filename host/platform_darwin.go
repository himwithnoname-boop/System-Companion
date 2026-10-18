package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/ebitengine/purego"
	"runtime"
	"strings"
	"unsafe"
)

type appleAPIs struct {
	matching      func(string) uintptr
	getService    func(uint32, uintptr) uint32
	getServices   func(uint32, uintptr, *uint32) int32
	next          func(uint32) uint32
	releaseObject func(uint32) int32
	open          func(uint32, uint32, uint32, *uint32) int32
	close         func(uint32) int32
	call          func(uint32, uint32, uintptr, uintptr, uintptr, *uintptr) int32
	property      func(uint32, uintptr, uintptr, uint32) uintptr
	search        func(uint32, string, uintptr, uintptr, uint32) uintptr
	registryID    func(uint32, *uint64) int32
	cfString      func(uintptr, string, uint32) uintptr
	cfStringCopy  func(uintptr, uintptr, int64, uint32) bool
	cfRelease     func(uintptr)
	cfType        func(uintptr) uint64
	cfStringType  func() uint64
	cfDataType    func() uint64
	cfNumberType  func() uint64
	cfDataBytes   func(uintptr) uintptr
	cfDataLength  func(uintptr) int64
	cfNumber      func(uintptr, int32, uintptr) bool
	cfDictGet     func(uintptr, uintptr) uintptr
	task          uint32
}
type darwinPlatform struct {
	api                *appleAPIs
	conn               uint32
	keys               []string
	keyIndex, keyCount int
	info               map[string][2]uint32
	failed             bool
}

func newPlatformProvider() Provider { return &darwinPlatform{} }
func loadAppleAPIs() (*appleAPIs, error) {
	io, e := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if e != nil {
		return nil, e
	}
	cf, e := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if e != nil {
		return nil, e
	}
	system, e := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if e != nil {
		return nil, e
	}
	task, e := purego.Dlsym(system, "mach_task_self_")
	if e != nil {
		return nil, e
	}
	a := &appleAPIs{task: *(*uint32)(unsafe.Pointer(task))}
	for _, binding := range []struct {
		target any
		lib    uintptr
		name   string
	}{
		{&a.matching, io, "IOServiceMatching"}, {&a.getService, io, "IOServiceGetMatchingService"}, {&a.getServices, io, "IOServiceGetMatchingServices"},
		{&a.next, io, "IOIteratorNext"}, {&a.releaseObject, io, "IOObjectRelease"}, {&a.open, io, "IOServiceOpen"}, {&a.close, io, "IOServiceClose"},
		{&a.call, io, "IOConnectCallStructMethod"}, {&a.property, io, "IORegistryEntryCreateCFProperty"}, {&a.search, io, "IORegistryEntrySearchCFProperty"},
		{&a.registryID, io, "IORegistryEntryGetRegistryEntryID"}, {&a.cfString, cf, "CFStringCreateWithCString"}, {&a.cfStringCopy, cf, "CFStringGetCString"},
		{&a.cfRelease, cf, "CFRelease"}, {&a.cfType, cf, "CFGetTypeID"}, {&a.cfStringType, cf, "CFStringGetTypeID"}, {&a.cfDataType, cf, "CFDataGetTypeID"}, {&a.cfNumberType, cf, "CFNumberGetTypeID"},
		{&a.cfDataBytes, cf, "CFDataGetBytePtr"}, {&a.cfDataLength, cf, "CFDataGetLength"}, {&a.cfNumber, cf, "CFNumberGetValue"}, {&a.cfDictGet, cf, "CFDictionaryGetValue"},
	} {
		purego.RegisterLibFunc(binding.target, binding.lib, binding.name)
	}
	return a, nil
}
func (a *appleAPIs) key(s string) uintptr { return a.cfString(0, s, 0x08000100) }
func (a *appleAPIs) text(v uintptr) string {
	if v == 0 {
		return ""
	}
	kind := a.cfType(v)
	if kind == a.cfStringType() {
		var b [512]byte
		if a.cfStringCopy(v, uintptr(unsafe.Pointer(&b[0])), int64(len(b)), 0x08000100) {
			return strings.TrimRight(string(b[:]), "\x00")
		}
	}
	if kind == a.cfDataType() {
		n := a.cfDataLength(v)
		if n > 0 && n <= 512 {
			p := a.cfDataBytes(v)
			if p != 0 {
				return strings.TrimRight(string(unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n))), "\x00")
			}
		}
	}
	return ""
}
func (a *appleAPIs) number(dict uintptr, key string) (float64, bool) {
	if dict == 0 {
		return 0, false
	}
	k := a.key(key)
	defer a.cfRelease(k)
	v := a.cfDictGet(dict, k)
	if v == 0 || a.cfType(v) != a.cfNumberType() {
		return 0, false
	}
	var n float64
	ok := a.cfNumber(v, 13, uintptr(unsafe.Pointer(&n)))
	return n, ok && finite(n) != nil
}
func (p *darwinPlatform) smcCall(in *[80]byte) ([80]byte, bool) {
	var out [80]byte
	size := uintptr(len(out))
	r := p.api.call(p.conn, 2, uintptr(unsafe.Pointer(&in[0])), uintptr(len(in)), uintptr(unsafe.Pointer(&out[0])), &size)
	runtime.KeepAlive(in)
	return out, r == 0 && size == 80 && out[40] == 0
}
func (p *darwinPlatform) smcRead(key string) (string, []byte, bool) {
	if len(key) != 4 || p.conn == 0 {
		return "", nil, false
	}
	in := [80]byte{}
	binary.LittleEndian.PutUint32(in[0:4], binary.BigEndian.Uint32([]byte(key)))
	info, ok := p.info[key]
	if !ok {
		in[42] = 9
		out, good := p.smcCall(&in)
		if !good {
			return "", nil, false
		}
		info = [2]uint32{binary.LittleEndian.Uint32(out[28:32]), binary.LittleEndian.Uint32(out[32:36])}
		if info[0] == 0 || info[0] > 32 {
			return "", nil, false
		}
		p.info[key] = info
	}
	in[42] = 5
	binary.LittleEndian.PutUint32(in[28:32], info[0])
	out, good := p.smcCall(&in)
	if !good {
		return "", nil, false
	}
	t := [4]byte{}
	binary.BigEndian.PutUint32(t[:], info[1])
	return string(t[:]), out[48 : 48+info[0]], true
}
func (p *darwinPlatform) openSMC() {
	if p.conn != 0 {
		return
	}
	service := p.api.getService(0, p.api.matching("AppleSMC"))
	if service == 0 {
		return
	}
	defer p.api.releaseObject(service)
	var conn uint32
	if p.api.open(service, p.api.task, 0, &conn) != 0 {
		return
	}
	p.conn = conn
	p.info = map[string][2]uint32{}
	if _, v, ok := p.smcRead("#KEY"); ok && len(v) >= 4 {
		p.keyCount = int(binary.BigEndian.Uint32(v))
		if p.keyCount > 4096 {
			p.keyCount = 4096
		}
	}
}
func (p *darwinPlatform) gpuInventory() []Sample {
	a := p.api
	var iterator uint32
	if a.getServices(0, a.matching("IOAccelerator"), &iterator) != 0 {
		return nil
	}
	defer a.releaseObject(iterator)
	rows := []Sample{}
	for i := 0; i < 32; i++ {
		service := a.next(iterator)
		if service == 0 {
			break
		}
		var id uint64
		a.registryID(service, &id)
		modelKey := a.key("model")
		model := a.search(service, "IOService", modelKey, 0, 3)
		a.cfRelease(modelKey)
		name := a.text(model)
		if model != 0 {
			a.cfRelease(model)
		}
		if name == "" {
			name = fmt.Sprintf("macOS GPU %d", i+1)
		}
		g := Sample{"id": fmt.Sprintf("ioreg:%x", id), "name": name, "vendor": gpuVendor(name), "source": "macOS IOAccelerator"}
		if g["vendor"] == "apple" {
			g["kind"] = "integrated"
		}
		statsKey := a.key("PerformanceStatistics")
		stats := a.property(service, statsKey, 0, 0)
		a.cfRelease(statsKey)
		for _, key := range []string{"Device Utilization %", "GPU Core Utilization", "GPU Activity(%)"} {
			if n, ok := a.number(stats, key); ok && n >= 0 && n <= 100 {
				g["load"] = finite(n)
				break
			}
		}
		if n, ok := a.number(stats, "Temperature(C)"); ok && usableTemperature(n) {
			g["temp"] = finite(n)
		}
		if stats != 0 {
			a.cfRelease(stats)
		}
		a.releaseObject(service)
		rows = append(rows, g)
	}
	return rows
}
func (p *darwinPlatform) Collect(ctx context.Context, s Sample) {
	if p.failed {
		return
	}
	if p.api == nil {
		a, e := loadAppleAPIs()
		if e != nil {
			p.failed = true
			s["sensor_status"] = "macOS hardware interface unavailable"
			return
		}
		p.api = a
	}
	p.openSMC()
	// Discover keys once, in bounded batches. Only SMC read/info commands are used.
	for p.conn != 0 && p.keyIndex < p.keyCount && ctx.Err() == nil {
		in := [80]byte{}
		in[42] = 8
		binary.LittleEndian.PutUint32(in[44:48], uint32(p.keyIndex))
		out, ok := p.smcCall(&in)
		p.keyIndex++
		if !ok {
			continue
		}
		var keyBytes [4]byte
		binary.BigEndian.PutUint32(keyBytes[:], binary.LittleEndian.Uint32(out[0:4]))
		key := string(keyBytes[:])
		if strings.HasPrefix(key, "TC") || strings.HasPrefix(key, "Tc") || strings.HasPrefix(key, "Tp") || strings.HasPrefix(key, "Te") || strings.HasPrefix(key, "Tf") || strings.HasPrefix(key, "TG") || strings.HasPrefix(key, "Tg") {
			p.keys = append(p.keys, key)
		}
	}
	keys := p.keys
	if len(keys) == 0 {
		keys = []string{"TC0P", "TC0D", "Tp09", "Tp0T", "TG0D", "TG0P"}
	}
	cpu, gpu := -1.0, -1.0
	for _, key := range keys {
		if ctx.Err() != nil {
			break
		}
		kind, b, ok := p.smcRead(key)
		if !ok {
			continue
		}
		n, ok := decodeSMCTemperature(kind, b)
		if !ok {
			continue
		}
		if strings.HasPrefix(key, "TG") || strings.HasPrefix(key, "Tg") {
			if n > gpu {
				gpu = n
			}
		} else if n > cpu {
			cpu = n
		}
	}
	if cpu > 0 {
		s["cpu_temp"] = finite(cpu)
		s["cpu_temp_source"] = "Apple SMC (hottest CPU sensor)"
		s["cpu_temp_kind"] = "cpu_sensor"
	}
	rows := p.gpuInventory()
	if gpu > 0 {
		if len(rows) == 1 {
			rows[0]["temp"] = finite(gpu)
			rows[0]["temperature_source"] = "Apple SMC"
		}
		if len(rows) == 0 {
			s["gpu_temp"] = finite(gpu)
			s["gpu_name"] = "Apple SMC GPU sensor"
		}
		if len(rows) > 1 {
			s["gpu_sensor_temp"] = finite(gpu)
			s["gpu_sensor_note"] = "SMC GPU sensor cannot be assigned reliably to a specific adapter"
		}
	}
	if len(rows) > 0 {
		s["_gpu_platform"] = rows
	}
	if p.conn == 0 {
		s["sensor_status"] = "Apple SMC temperature access is unavailable to this account"
	}
}
