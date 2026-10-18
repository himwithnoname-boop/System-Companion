package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const notifierInterface = "org.kde.StatusNotifierItem"
const menuInterface = "com.canonical.dbusmenu"
const notifierPath dbus.ObjectPath = "/StatusNotifierItem"
const menuPath dbus.ObjectPath = "/Menu"

type trayPixmap struct {
	Width, Height int32
	Data          []byte
}
type trayTooltip struct {
	Icon               string
	Pixmaps            []trayPixmap
	Title, Description string
}
type menuLayout struct {
	ID         int32
	Properties map[string]dbus.Variant
	Children   []dbus.Variant
}
type menuProperties struct {
	ID         int32
	Properties map[string]dbus.Variant
}
type menuEvent struct {
	ID        int32
	Event     string
	Data      dbus.Variant
	Timestamp uint32
}
type linuxTray struct {
	conn      *dbus.Conn
	ctx       context.Context
	quit      context.CancelFunc
	url       string
	reconnect func()
	mu        sync.Mutex
	revision  uint32
}
type trayMethods struct{ tray *linuxTray }

func (m *trayMethods) Activate(x, y int32) *dbus.Error                    { go openBrowser(m.tray.url); return nil }
func (m *trayMethods) SecondaryActivate(x, y int32) *dbus.Error           { return m.Activate(x, y) }
func (m *trayMethods) ContextMenu(x, y int32) *dbus.Error                 { return nil }
func (m *trayMethods) Scroll(delta int32, orientation string) *dbus.Error { return nil }

func trayPixels() []trayPixmap {
	pixels := make([]byte, 32*32*4)
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			r := math.Hypot(float64(x)-15.5, float64(y)-15.5)
			color := [4]byte{255, 7, 16, 24}
			switch {
			case r > 15.5:
				color = [4]byte{}
			case r >= 12 && r <= 14:
				color = [4]byte{255, 0, 229, 255}
			case r >= 8 && r <= 10:
				color = [4]byte{255, 197, 129, 255}
			case r >= 4 && r <= 6:
				color = [4]byte{255, 185, 243, 107}
			}
			copy(pixels[(y*32+x)*4:], color[:])
		}
	}
	return []trayPixmap{{32, 32, pixels}}
}
func filterMenuProperties(all map[string]dbus.Variant, names []string) map[string]dbus.Variant {
	if len(names) == 0 {
		return all
	}
	result := make(map[string]dbus.Variant)
	for _, name := range names {
		if value, ok := all[name]; ok {
			result[name] = value
		}
	}
	return result
}
func (t *linuxTray) properties(id int32) map[string]dbus.Variant {
	values := map[string]dbus.Variant{"enabled": dbus.MakeVariant(true), "visible": dbus.MakeVariant(true)}
	switch id {
	case 0:
		values["children-display"] = dbus.MakeVariant("submenu")
	case 1:
		values["label"] = dbus.MakeVariant("Open dashboard")
	case 2:
		values["label"] = dbus.MakeVariant("Reconnect display")
	case 3:
		values["label"] = dbus.MakeVariant("Run on startup (when display is connected)")
		values["toggle-type"] = dbus.MakeVariant("checkmark")
		checked := int32(0)
		if startupEnabled() {
			checked = 1
		}
		values["toggle-state"] = dbus.MakeVariant(checked)
	case 4:
		values["type"] = dbus.MakeVariant("separator")
	case 5:
		values["label"] = dbus.MakeVariant("Exit")
	}
	return values
}
func (t *linuxTray) GetLayout(parent, depth int32, names []string) (uint32, menuLayout, *dbus.Error) {
	if parent < 0 || parent > 5 {
		return 0, menuLayout{}, dbus.MakeFailedError(fmt.Errorf("unknown menu item"))
	}
	layout := menuLayout{parent, filterMenuProperties(t.properties(parent), names), []dbus.Variant{}}
	if parent == 0 && depth != 0 {
		for id := int32(1); id <= 5; id++ {
			layout.Children = append(layout.Children, dbus.MakeVariant(menuLayout{id, filterMenuProperties(t.properties(id), names), []dbus.Variant{}}))
		}
	}
	t.mu.Lock()
	revision := t.revision
	t.mu.Unlock()
	return revision, layout, nil
}
func (t *linuxTray) GetGroupProperties(ids []int32, names []string) ([]menuProperties, *dbus.Error) {
	if len(ids) == 0 {
		ids = []int32{0, 1, 2, 3, 4, 5}
	}
	result := []menuProperties{}
	for _, id := range ids {
		if id >= 0 && id <= 5 {
			result = append(result, menuProperties{id, filterMenuProperties(t.properties(id), names)})
		}
	}
	return result, nil
}
func (t *linuxTray) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	value, ok := t.properties(id)[name]
	if !ok {
		return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("unknown menu property"))
	}
	return value, nil
}
func (t *linuxTray) changed() {
	t.mu.Lock()
	t.revision++
	revision := t.revision
	t.mu.Unlock()
	t.conn.Emit(menuPath, menuInterface+".LayoutUpdated", revision, int32(0))
}
func (t *linuxTray) Event(id int32, event string, data dbus.Variant, timestamp uint32) *dbus.Error {
	if event != "clicked" {
		return nil
	}
	switch id {
	case 1:
		go openBrowser(t.url)
	case 2:
		t.reconnect()
	case 3:
		go func() {
			if err := setStartup(!startupEnabled()); err != nil {
				log.Print("startup: ", err)
				timeout, cancel := context.WithTimeout(t.ctx, 2*time.Second)
				defer cancel()
				t.conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications").CallWithContext(timeout, "org.freedesktop.Notifications.Notify", 0, "System Companion", uint32(0), "", "Startup setting could not be saved", err.Error(), []string{}, map[string]dbus.Variant{}, int32(7000))
			}
			t.changed()
		}()
	case 5:
		t.quit()
	}
	return nil
}
func (t *linuxTray) EventGroup(events []menuEvent) ([]int32, *dbus.Error) {
	failed := []int32{}
	for _, event := range events {
		if event.ID < 1 || event.ID > 5 {
			failed = append(failed, event.ID)
		} else {
			t.Event(event.ID, event.Event, event.Data, event.Timestamp)
		}
	}
	return failed, nil
}
func (t *linuxTray) AboutToShow(id int32) (bool, *dbus.Error) { return true, nil }
func (t *linuxTray) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	return ids, []int32{}, nil
}
func startTray(ctx context.Context, quit context.CancelFunc, url string, reconnect func(), enableSensors func()) bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Print("Tray unavailable: ", err)
		return false
	}
	t := &linuxTray{conn: conn, ctx: ctx, quit: quit, url: url, reconnect: reconnect, revision: 1}
	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	result, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil || result != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return false
	}
	item := &trayMethods{t}
	if err = conn.Export(item, notifierPath, notifierInterface); err != nil {
		conn.Close()
		return false
	}
	if err = conn.Export(t, menuPath, menuInterface); err != nil {
		conn.Close()
		return false
	}
	pixmaps := trayPixels()
	readonly := func(value any) *prop.Prop { return &prop.Prop{Value: value, Writable: false, Emit: prop.EmitFalse} }
	prop.Export(conn, notifierPath, map[string]map[string]*prop.Prop{notifierInterface: {
		"Category": readonly("ApplicationStatus"), "Id": readonly("system-companion"), "Title": readonly("System Companion"), "Status": readonly("Active"),
		"WindowId": readonly(uint32(0)), "IconName": readonly(""), "IconPixmap": readonly(pixmaps), "IconThemePath": readonly(""),
		"AttentionIconName": readonly(""), "AttentionIconPixmap": readonly([]trayPixmap{}), "OverlayIconName": readonly(""), "OverlayIconPixmap": readonly([]trayPixmap{}),
		"ToolTip": readonly(trayTooltip{"", pixmaps, "System Companion", "Open dashboard or use the menu to reconnect and exit"}),
		"Menu":    readonly(menuPath), "ItemIsMenu": readonly(true),
	}})
	prop.Export(conn, menuPath, map[string]map[string]*prop.Prop{menuInterface: {"Version": readonly(uint32(3)), "TextDirection": readonly("ltr"), "Status": readonly("normal"), "IconThemePath": readonly([]string{})}})
	conn.Export(introspect.NewIntrospectable(&introspect.Node{Interfaces: []introspect.Interface{introspect.IntrospectData, prop.IntrospectData, {Name: notifierInterface, Methods: introspect.Methods(item)}}}), notifierPath, "org.freedesktop.DBus.Introspectable")
	conn.Export(introspect.NewIntrospectable(&introspect.Node{Interfaces: []introspect.Interface{introspect.IntrospectData, prop.IntrospectData, {Name: menuInterface, Methods: introspect.Methods(t)}}}), menuPath, "org.freedesktop.DBus.Introspectable")
	register := func() error {
		timeout, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher").CallWithContext(timeout, "org.kde.StatusNotifierWatcher.RegisterStatusNotifierItem", 0, name).Err
	}
	signals := make(chan *dbus.Signal, 8)
	conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, "org.kde.StatusNotifierWatcher"))
	conn.Signal(signals)
	registered := register() == nil
	if !registered {
		log.Print("Tray host unavailable; use dashboard Exit")
	}
	go func() {
		defer conn.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case signal := <-signals:
				if signal == nil {
					return
				}
				if len(signal.Body) == 3 && signal.Body[2] != "" {
					register()
				}
			}
		}
	}()
	return registered
}
