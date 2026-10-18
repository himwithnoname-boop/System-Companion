package main

import (
	"context"
	"log"
	"math"
	"os/exec"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var shell32 = syscall.NewLazyDLL("shell32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var registerClass = user32.NewProc("RegisterClassExW")
var createWindow = user32.NewProc("CreateWindowExW")
var defWindow = user32.NewProc("DefWindowProcW")
var destroyWindow = user32.NewProc("DestroyWindow")
var postMessage = user32.NewProc("PostMessageW")
var getMessage = user32.NewProc("GetMessageW")
var translateMessage = user32.NewProc("TranslateMessage")
var dispatchMessage = user32.NewProc("DispatchMessageW")
var notifyIcon = shell32.NewProc("Shell_NotifyIconW")

type windowClass struct {
	Size, Style                        uint32
	Procedure                          uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}
type notifyData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Timeout             uint32
	Title               [64]uint16
	InfoFlags           uint32
	GUID                [16]byte
	BalloonIcon         uintptr
}
type winPoint struct{ X, Y int32 }
type winMessage struct {
	Window  uintptr
	ID      uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   winPoint
	Private uint32
}

func wide(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func hideCommand(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func companionIcon(module uintptr) uintptr {
	var mask [128]byte
	var pixels [1024]uint32
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			r := math.Hypot(float64(x)-15.5, float64(y)-15.5)
			color := uint32(0xff071018)
			switch {
			case r > 15.5:
				mask[y*4+x/8] |= 0x80 >> uint(x%8)
				color = 0
			case r >= 12 && r <= 14:
				color = 0xff00e5ff
			case r >= 8 && r <= 10:
				color = 0xffc581ff
			case r >= 4 && r <= 6:
				color = 0xffb9f36b
			}
			pixels[y*32+x] = color
		}
	}
	icon, _, _ := user32.NewProc("CreateIcon").Call(module, 32, 32, 1, 32, uintptr(unsafe.Pointer(&mask[0])), uintptr(unsafe.Pointer(&pixels[0])))
	return icon
}
func startTray(ctx context.Context, quit context.CancelFunc, url string, reconnect func(), enableSensors func()) bool {
	ready := make(chan bool, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		module, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
		className := wide("SystemCompanionTray")
		icon := companionIcon(module)
		if icon == 0 {
			icon, _, _ = user32.NewProc("LoadIconW").Call(0, 32512)
		} else {
			defer user32.NewProc("DestroyIcon").Call(icon)
		}
		taskbarMessage, _, _ := user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(wide("TaskbarCreated"))))
		var data notifyData
		procedure := syscall.NewCallback(func(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
			switch message {
			case 0x8001:
				if lparam == 0x0202 {
					go openBrowser(url)
				}
				if lparam == 0x0205 {
					menu, _, _ := user32.NewProc("CreatePopupMenu").Call()
					appendMenu := user32.NewProc("AppendMenuW")
					appendMenu.Call(menu, 0, 1, uintptr(unsafe.Pointer(wide("Open dashboard"))))
					appendMenu.Call(menu, 0, 2, uintptr(unsafe.Pointer(wide("Reconnect display"))))
					sensorFlags := uintptr(0)
					if sensorAccessRemembered() {
						sensorFlags = 8
					}
					state := sensorAccessState()
					label := "CPU temperatures"
					if state["pending"] == true {
						sensorFlags |= 1
						label = "CPU temperatures (setting up...)"
					}
					appendMenu.Call(menu, sensorFlags, 6, uintptr(unsafe.Pointer(wide(label))))
					startupFlags := uintptr(0)
					if startupEnabled() {
						startupFlags = 8
					}
					appendMenu.Call(menu, startupFlags, 5, uintptr(unsafe.Pointer(wide("Run on startup (when display is connected)"))))
					appendMenu.Call(menu, 0x800, 0, 0)
					appendMenu.Call(menu, 0, 3, uintptr(unsafe.Pointer(wide("Exit System Companion"))))
					var point winPoint
					user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&point)))
					user32.NewProc("SetForegroundWindow").Call(hwnd)
					command, _, _ := user32.NewProc("TrackPopupMenu").Call(menu, 0x0100|0x0080, uintptr(point.X), uintptr(point.Y), 0, hwnd, 0)
					user32.NewProc("DestroyMenu").Call(menu)
					postMessage.Call(hwnd, 0, 0, 0)
					switch command {
					case 1:
						go openBrowser(url)
					case 2:
						go reconnect()
					case 3:
						quit()
					case 6:
						go func() {
							if err := setSensorAccessRemembered(!sensorAccessRemembered()); err != nil {
								log.Print(err)
							}
						}()
					case 5:
						go func() {
							if err := setStartup(!startupEnabled()); err != nil {
								log.Print(err)
							}
						}()
					}
				}
				return 0
			case 0x0010:
				quit()
				destroyWindow.Call(hwnd)
				return 0
			case 0x0002:
				notifyIcon.Call(2, uintptr(unsafe.Pointer(&data)))
				user32.NewProc("PostQuitMessage").Call(0)
				return 0
			}
			if message == uint32(taskbarMessage) {
				notifyIcon.Call(0, uintptr(unsafe.Pointer(&data)))
				return 0
			}
			result, _, _ := defWindow.Call(hwnd, uintptr(message), wparam, lparam)
			return result
		})
		class := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), Procedure: procedure, Instance: module, ClassName: className, Icon: icon}
		if ok, _, _ := registerClass.Call(uintptr(unsafe.Pointer(&class))); ok == 0 {
			ready <- false
			return
		}
		window, _, _ := createWindow.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(wide("System Companion"))), 0, 0, 0, 0, 0, 0, 0, module, 0)
		if window == 0 {
			ready <- false
			return
		}
		data = notifyData{Size: uint32(unsafe.Sizeof(notifyData{})), Window: window, ID: 1, Flags: 1 | 2 | 4, Callback: 0x8001, Icon: icon}
		copy(data.Tip[:], syscall.StringToUTF16("System Companion Â· right-click to exit"))
		ok, _, _ := notifyIcon.Call(0, uintptr(unsafe.Pointer(&data)))
		ready <- ok != 0
		go func() { <-ctx.Done(); postMessage.Call(window, 0x0010, 0, 0) }()
		var message winMessage
		for {
			result, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
			if int32(result) <= 0 {
				break
			}
			translateMessage.Call(uintptr(unsafe.Pointer(&message)))
			dispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
		}
	}()
	select {
	case ok := <-ready:
		return ok
	case <-time.After(3 * time.Second):
		return false
	}
}
