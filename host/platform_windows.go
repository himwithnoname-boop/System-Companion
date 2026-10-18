package main

import (
	"context"
	"fmt"
	"golang.org/x/sys/windows"
	"math"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var pdhDLL = windows.NewLazySystemDLL("pdh.dll")
var dxgiDLL = windows.NewLazySystemDLL("dxgi.dll")
var dxcoreDLL = windows.NewLazySystemDLL("dxcore.dll")
var gdiDLL = windows.NewLazySystemDLL("gdi32.dll")

type adapterLUID struct {
	Low  uint32
	High int32
}
type adapterDesc struct {
	Description                                   [128]uint16
	Vendor, Device, SubSystem, Revision           uint32
	DedicatedVideo, DedicatedSystem, SharedSystem uintptr
	LUID                                          adapterLUID
	Flags                                         uint32
}
type adapterOpen struct {
	LUID   adapterLUID
	Handle uint32
}
type adapterQuery struct {
	Handle, Kind uint32
	Data         uintptr
	Size         uint32
}
type adapterPerf struct {
	Physical                                           uint32
	MemoryHz, MaxMemoryHz, OCMemoryHz, Bandwidth, PCIE uint64
	FanRPM, PowerPercent, Temperature                  uint32
	PowerState                                         byte
}
type windowsPlatform struct {
	mu          sync.Mutex
	query       uintptr
	counters    map[string]uintptr
	adapters    []Sample
	inventoryAt time.Time
}

func newPlatformProvider() Provider { return &windowsPlatform{} }

type comObject struct{ vtable *[32]uintptr }

func comMethod(object *comObject, index int) uintptr { return object.vtable[index] }
func luidName(l adapterLUID) string                  { return fmt.Sprintf("0x%08x_0x%08x", uint32(l.High), l.Low) }
func openAdapter(l adapterLUID) uint32 {
	call := gdiDLL.NewProc("D3DKMTOpenAdapterFromLuid")
	if call.Find() != nil {
		return 0
	}
	a := adapterOpen{LUID: l}
	r, _, _ := call.Call(uintptr(unsafe.Pointer(&a)))
	if r != 0 {
		return 0
	}
	return a.Handle
}
func queryAdapter(h, kind uint32, data unsafe.Pointer, size uintptr) bool {
	call := gdiDLL.NewProc("D3DKMTQueryAdapterInfo")
	if h == 0 || call.Find() != nil {
		return false
	}
	q := adapterQuery{Handle: h, Kind: kind, Data: uintptr(data), Size: uint32(size)}
	r, _, _ := call.Call(uintptr(unsafe.Pointer(&q)))
	runtime.KeepAlive(data)
	return r == 0
}
func closeAdapter(h uint32) {
	call := gdiDLL.NewProc("D3DKMTCloseAdapter")
	if h != 0 && call.Find() == nil {
		call.Call(uintptr(unsafe.Pointer(&h)))
	}
}
func adapterPCI(l adapterLUID) string {
	h := openAdapter(l)
	if h == 0 {
		return ""
	}
	defer closeAdapter(h)
	var a struct{ Bus, Device, Function uint32 }
	if !queryAdapter(h, 6, unsafe.Pointer(&a), unsafe.Sizeof(a)) || a.Bus > 255 || a.Device > 31 || a.Function > 7 {
		return ""
	}
	return fmt.Sprintf("pci:0000:%02x:%02x.%x", a.Bus, a.Device, a.Function)
}
func adapterThermals(l adapterLUID, g Sample) {
	h := openAdapter(l)
	if h == 0 {
		return
	}
	defer closeAdapter(h)
	var perf adapterPerf
	// KMTQAITYPE_ADAPTERPERFDATA: WDDM 2.4+ read-only performance data.
	if !queryAdapter(h, 62, unsafe.Pointer(&perf), unsafe.Sizeof(perf)) {
		return
	}
	if t := float64(perf.Temperature) / 10; usableTemperature(t) {
		g["temp"] = finite(t)
		g["temperature_source"] = "Windows display driver"
	}
	if perf.FanRPM > 0 && perf.FanRPM < 100000 {
		g["fan_rpm"] = int(perf.FanRPM)
	}
	if perf.MemoryHz > 0 && perf.MemoryHz < 1e11 {
		g["mem_clock"] = finite(float64(perf.MemoryHz) / 1e6)
	}
	// Power is a percentage, not watts. Do not publish it as gpu_power.
	if perf.PowerPercent > 0 && perf.PowerPercent <= 1000 {
		g["power_percent"] = float64(perf.PowerPercent) / 10
	}
}

// DXCore reports iGPU status explicitly; memory size and enumeration order do not.
func dxCoreGPUFactory() *comObject {
	create := dxcoreDLL.NewProc("DXCoreCreateAdapterFactory")
	if create.Find() != nil {
		return nil
	}
	iid := windows.GUID{Data1: 0x78ee5945, Data2: 0xc36e, Data3: 0x4b13, Data4: [8]byte{0xa6, 0x69, 0, 0x5d, 0xd1, 0x1c, 0x0f, 6}}
	var factory *comObject
	hr, _, _ := create.Call(uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&factory)))
	if hr != 0 {
		return nil
	}
	return factory
}
func dxCoreGPUKind(factory *comObject, luid adapterLUID) (string, bool) {
	if factory == nil {
		return "unknown", true
	}
	iid := windows.GUID{Data1: 0xf0db4c7f, Data2: 0xfe5a, Data3: 0x42a2, Data4: [8]byte{0xbd, 0x62, 0xf2, 0xa6, 0xcf, 0x6f, 0xc8, 0x3e}}
	var adapter *comObject
	hr, _, _ := syscall.SyscallN(comMethod(factory, 4), uintptr(unsafe.Pointer(factory)), uintptr(unsafe.Pointer(&luid)), uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&adapter)))
	if hr != 0 || adapter == nil {
		return "unknown", true
	}
	defer syscall.SyscallN(comMethod(adapter, 2), uintptr(unsafe.Pointer(adapter)))
	var hardware byte
	hr, _, _ = syscall.SyscallN(comMethod(adapter, 6), uintptr(unsafe.Pointer(adapter)), 11, 1, uintptr(unsafe.Pointer(&hardware)))
	if hr == 0 && hardware == 0 {
		return "software", false
	}
	var integrated byte
	hr, _, _ = syscall.SyscallN(comMethod(adapter, 6), uintptr(unsafe.Pointer(adapter)), 12, 1, uintptr(unsafe.Pointer(&integrated)))
	if hr != 0 {
		return "unknown", true
	}
	if integrated != 0 {
		return "integrated", true
	}
	return "discrete", true
}
func enumerateWindowsGPUs() []Sample {
	var factory *comObject
	iid := windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
	create := dxgiDLL.NewProc("CreateDXGIFactory1")
	if create.Find() != nil {
		return nil
	}
	r, _, _ := create.Call(uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&factory)))
	if r != 0 || factory == nil {
		return nil
	}
	defer syscall.SyscallN(comMethod(factory, 2), uintptr(unsafe.Pointer(factory)))
	core := dxCoreGPUFactory()
	if core != nil {
		defer syscall.SyscallN(comMethod(core, 2), uintptr(unsafe.Pointer(core)))
	}
	rows := []Sample{}
	for i := 0; i < 64; i++ {
		var adapter *comObject
		r, _, _ := syscall.SyscallN(comMethod(factory, 12), uintptr(unsafe.Pointer(factory)), uintptr(i), uintptr(unsafe.Pointer(&adapter)))
		if r != 0 || adapter == nil {
			break
		}
		var d adapterDesc
		r, _, _ = syscall.SyscallN(comMethod(adapter, 10), uintptr(unsafe.Pointer(adapter)), uintptr(unsafe.Pointer(&d)))
		syscall.SyscallN(comMethod(adapter, 2), uintptr(unsafe.Pointer(adapter)))
		if r != 0 || d.Flags&2 != 0 {
			continue
		}
		kind, hardware := dxCoreGPUKind(core, d.LUID)
		if !hardware {
			continue
		}
		name := windows.UTF16ToString(d.Description[:])
		if strings.EqualFold(name, "Microsoft Basic Render Driver") {
			continue
		} // WARP software rasterizer.
		luid := luidName(d.LUID)
		g := Sample{"id": "windows:" + luid, "luid": luid, "name": name, "vendor": gpuVendor(name), "source": "Windows display driver", "_luid": d.LUID, "kind": kind}
		if pci := adapterPCI(d.LUID); pci != "" {
			g["pci"] = pci
			g["_display_identity"] = fmt.Sprintf("%s:%04x:%04x", pci, d.Vendor, d.Device)
		}
		if d.DedicatedVideo > 0 {
			g["vram_total"] = gib(uint64(d.DedicatedVideo))
		}
		rows = append(rows, g)
	}
	return rows
}
func (p *windowsPlatform) initCounters() {
	p.counters = map[string]uintptr{}
	var q uintptr
	r, _, _ := pdhDLL.NewProc("PdhOpenQueryW").Call(0, 0, uintptr(unsafe.Pointer(&q)))
	if r != 0 {
		return
	}
	p.query = q
	paths := map[string]string{
		"gpu_load":        "\\GPU Engine(*)\\Utilization Percentage",
		"gpu_memory":      "\\GPU Adapter Memory(*)\\Dedicated Usage",
		"thermal":         "\\Thermal Zone Information(*)\\Temperature",
		"thermal_precise": "\\Thermal Zone Information(*)\\High Precision Temperature",
	}
	for key, path := range paths {
		wide, _ := windows.UTF16PtrFromString(path)
		var counter uintptr
		r, _, _ := pdhDLL.NewProc("PdhAddEnglishCounterW").Call(q, uintptr(unsafe.Pointer(wide)), 0, uintptr(unsafe.Pointer(&counter)))
		if r == 0 {
			p.counters[key] = counter
		}
	}
}

type pdhValue struct {
	Status, Pad uint32
	Value       float64
}
type pdhItem struct {
	Name  *uint16
	Value pdhValue
}

func readPDHArray(h uintptr) map[string]float64 {
	out := map[string]float64{}
	if h == 0 {
		return out
	}
	get := pdhDLL.NewProc("PdhGetFormattedCounterArrayW")
	var size, count uint32
	r, _, _ := get.Call(h, 0x200, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != 0x800007d2 || size == 0 || size > 2<<20 {
		return out
	}
	buffer := make([]byte, size)
	r, _, _ = get.Call(h, 0x200, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buffer[0])))
	itemSize := unsafe.Sizeof(pdhItem{})
	if r != 0 || uint64(count)*uint64(itemSize) > uint64(len(buffer)) {
		return out
	}
	start := uintptr(unsafe.Pointer(&buffer[0]))
	end := start + uintptr(len(buffer))
	for i := uint32(0); i < count; i++ {
		item := (*pdhItem)(unsafe.Pointer(&buffer[uintptr(i)*itemSize]))
		if item.Value.Status > 1 || math.IsNaN(item.Value.Value) || math.IsInf(item.Value.Value, 0) || uintptr(unsafe.Pointer(item.Name)) < start || uintptr(unsafe.Pointer(item.Name)) >= end {
			continue
		}
		limit := (end - uintptr(unsafe.Pointer(item.Name))) / 2
		if limit > 512 {
			limit = 512
		}
		name := windows.UTF16ToString(unsafe.Slice(item.Name, int(limit)))
		out[name] = item.Value.Value
	}
	runtime.KeepAlive(buffer)
	return out
}
func (p *windowsPlatform) Collect(ctx context.Context, s Sample) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	if p.counters == nil {
		p.initCounters()
	}
	if p.query != 0 {
		pdhDLL.NewProc("PdhCollectQueryData").Call(p.query)
	}
	if time.Since(p.inventoryAt) > 30*time.Second {
		p.adapters = enumerateWindowsGPUs()
		p.inventoryAt = time.Now()
	}
	loads := readPDHArray(p.counters["gpu_load"])
	memory := readPDHArray(p.counters["gpu_memory"])
	rows := []Sample{}
	for _, base := range p.adapters {
		g := Sample{}
		for k, v := range base {
			if k != "_luid" {
				g[k] = v
			}
		}
		l := base["_luid"].(adapterLUID)
		needle := "luid_" + luidName(l) + "_"
		engines := map[string]float64{}
		for name, load := range loads {
			if strings.Contains(strings.ToLower(name), needle) && load >= 0 {
				idx := strings.Index(name, "_eng_")
				if idx >= 0 {
					engines[name[idx:]] += load
				}
			}
		}
		highest := -1.0
		for _, v := range engines {
			highest = math.Max(highest, v)
		}
		if highest >= 0 {
			g["load"] = finite(math.Min(100, highest))
		}
		for name, n := range memory {
			if strings.Contains(strings.ToLower(name), needle) && n >= 0 {
				g["vram"] = finite(n / (1 << 30))
				break
			}
		}
		adapterThermals(l, g)
		rows = append(rows, g)
	}
	if len(rows) > 0 {
		s["_gpu_platform"] = rows
	}
	precise := readPDHArray(p.counters["thermal_precise"])
	whole := readPDHArray(p.counters["thermal"])
	zones := []Sample{}
	hottest := -1.0
	zoneName := ""
	for name, k := range whole {
		c := k - 273.15
		if high := precise[name]; high > 0 {
			c = high/10 - 273.15
		}
		if !usableTemperature(c) {
			continue
		}
		zones = append(zones, Sample{"name": name, "temperature": finite(c), "source": "Windows thermal zone"})
		if c > hottest {
			hottest = c
			zoneName = name
		}
		lower := strings.ToLower(name)
		if strings.Contains(lower, "cpu") || strings.Contains(lower, "processor") {
			if old, ok := metricNumber(s["cpu_temp"]); !ok || c > old {
				s["cpu_temp"] = finite(c)
				s["cpu_temp_source"] = "Windows thermal zone " + name
				s["cpu_temp_kind"] = "cpu_zone"
			}
		}
	}
	if len(zones) > 0 {
		s["thermal_zones"] = zones
		s["thermal_temp"] = finite(hottest)
		s["thermal_name"] = zoneName
	}
}
