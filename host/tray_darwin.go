package main

import (
	"context"
	"fmt"
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"log"
	"runtime"
	"time"
)

// Cocoa owns the original process thread, including callbacks and cleanup.
func init() { runtime.LockOSThread() }

var macMainJobs = make(chan func(), 32)
var macDispatch func(uintptr, uintptr, uintptr)
var macQueue, macCallback uintptr
var macApp objc.ID
var macTrayError error

func macSend(id objc.ID, selector string, args ...any) objc.ID {
	return id.Send(objc.RegisterName(selector), args...)
}
func macString(s string) objc.ID {
	return macSend(objc.ID(objc.GetClass("NSString")), "stringWithUTF8String:", s)
}
func macOnMain(fn func()) { macMainJobs <- fn; macDispatch(macQueue, 0, macCallback) }
func platformMain(run func()) {
	_, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		macTrayError = err
		run()
		return
	}
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		macTrayError = err
		run()
		return
	}
	macQueue, err = purego.Dlsym(lib, "_dispatch_main_q")
	if err != nil {
		macTrayError = err
		run()
		return
	}
	purego.RegisterLibFunc(&macDispatch, lib, "dispatch_async_f")
	macCallback = purego.NewCallback(func(_ uintptr) { (<-macMainJobs)() })
	pool := macSend(objc.ID(objc.GetClass("NSAutoreleasePool")), "new")
	defer macSend(pool, "drain")
	macApp = macSend(objc.ID(objc.GetClass("NSApplication")), "sharedApplication")
	macSend(macApp, "setActivationPolicy:", int64(1))
	go func() {
		run()
		macOnMain(func() {
			macSend(macApp, "stop:", objc.ID(0))
			// stop: takes effect after an event; wake an otherwise idle loop.
			type point struct{ X, Y float64 }
			event := macSend(objc.ID(objc.GetClass("NSEvent")), "otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:",
				uint64(15), point{}, uint64(0), float64(0), int64(0), uintptr(0), int16(0), int64(0), int64(0))
			macSend(macApp, "postEvent:atStart:", event, true)
		})
	}()
	macSend(macApp, "run")
}
func startTray(ctx context.Context, quit context.CancelFunc, url string, reconnect func(), enableSensors func()) bool {
	if macTrayError != nil {
		log.Print("Menu bar unavailable: ", macTrayError)
		return false
	}
	ready := make(chan bool, 1)
	macOnMain(func() {
		statusBar := macSend(objc.ID(objc.GetClass("NSStatusBar")), "systemStatusBar")
		item := macSend(statusBar, "statusItemWithLength:", float64(-1))
		if item == 0 {
			ready <- false
			return
		}
		macSend(item, "retain")
		menu := macSend(objc.ID(objc.GetClass("NSMenu")), "new")
		macSend(menu, "setAutoenablesItems:", false)
		var startupItem objc.ID
		update := func() {
			state := int64(0)
			if startupEnabled() {
				state = 1
			}
			macSend(startupItem, "setState:", state)
		}
		class, err := objc.RegisterClass("SystemCompanionMenuTarget", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
			{Cmd: objc.RegisterName("performAction:"), Fn: func(_ objc.ID, _ objc.SEL, sender objc.ID) {
				switch int64(macSend(sender, "tag")) {
				case 1:
					go openBrowser(url)
				case 2:
					go reconnect()
				case 3:
					macSend(startupItem, "setEnabled:", false)
					go func() {
						err := setStartup(!startupEnabled())
						macOnMain(func() {
							update()
							macSend(startupItem, "setEnabled:", true)
							if err != nil {
								log.Print("startup: ", err)
								alert := macSend(objc.ID(objc.GetClass("NSAlert")), "new")
								macSend(alert, "setMessageText:", macString("Startup setting could not be saved"))
								macSend(alert, "setInformativeText:", macString(err.Error()))
								macSend(alert, "runModal")
								macSend(alert, "release")
							}
						})
					}()
				case 4:
					quit()
				}
			}},
			{Cmd: objc.RegisterName("menuNeedsUpdate:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) { update() }},
		})
		if err != nil {
			macSend(statusBar, "removeStatusItem:", item)
			macSend(item, "release")
			macSend(menu, "release")
			log.Print(fmt.Errorf("menu bar: %w", err))
			ready <- false
			return
		}
		target := macSend(objc.ID(class), "new")
		for i, label := range []string{"Open dashboard", "Reconnect display", "Run on startup (when display is connected)", "Exit System Companion"} {
			if i == 3 {
				macSend(menu, "addItem:", macSend(objc.ID(objc.GetClass("NSMenuItem")), "separatorItem"))
			}
			entry := macSend(macSend(objc.ID(objc.GetClass("NSMenuItem")), "alloc"), "initWithTitle:action:keyEquivalent:",
				macString(label), objc.RegisterName("performAction:"), macString(""))
			macSend(entry, "setTarget:", target)
			macSend(entry, "setTag:", int64(i+1))
			macSend(menu, "addItem:", entry)
			if i == 2 {
				startupItem = entry
			}
			macSend(entry, "release")
		}
		update()
		macSend(menu, "setDelegate:", target)
		macSend(item, "setMenu:", menu)
		button := macSend(item, "button")
		image := macSend(objc.ID(objc.GetClass("NSImage")), "imageWithSystemSymbolName:accessibilityDescription:",
			macString("gauge"), macString("System Companion"))
		if image != 0 {
			macSend(image, "setTemplate:", true)
			macSend(button, "setImage:", image)
		} else {
			macSend(button, "setTitle:", macString("SC"))
		}
		macSend(button, "setToolTip:", macString("System Companion"))
		ready <- true
		go func() {
			<-ctx.Done()
			macOnMain(func() {
				macSend(statusBar, "removeStatusItem:", item)
				macSend(menu, "setDelegate:", objc.ID(0))
				macSend(item, "release")
				macSend(menu, "release")
				macSend(target, "release")
			})
		}()
	})
	select {
	case ok := <-ready:
		return ok
	case <-time.After(3 * time.Second):
		return false
	}
}
