package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

const sensorPreferenceKey = `Software\SystemCompanion`
const sensorTaskPrefix = "SystemCompanion-CPU-"

var sensorRememberAction func(bool) error
var sensorStateAction func() Sample

func sensorAccessState() Sample {
	if sensorStateAction != nil {
		return sensorStateAction()
	}
	return Sample{"available": true, "enabled": sensorAccessRemembered(), "pending": false}
}

func sensorSID() (string, error) {
	u, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return "", e
	}
	return u.User.Sid.String(), nil
}
func sensorAccessAvailable() bool { return true }
func sensorAccessRemembered() bool {
	k, e := registry.OpenKey(registry.CURRENT_USER, sensorPreferenceKey, registry.QUERY_VALUE)
	if e != nil {
		return false
	}
	defer k.Close()
	v, _, e := k.GetIntegerValue("RememberCPUAccess")
	return e == nil && v == 1
}
func writeSensorRemembered(enabled bool) error {
	k, _, e := registry.CreateKey(registry.CURRENT_USER, sensorPreferenceKey, registry.SET_VALUE)
	if e != nil {
		return e
	}
	defer k.Close()
	v := uint32(0)
	if enabled {
		v = 1
	}
	return k.SetDWordValue("RememberCPUAccess", v)
}
func setSensorAccessRemembered(enabled bool) error {
	if sensorRememberAction == nil {
		return errors.New("CPU sensor helper unavailable")
	}
	return sensorRememberAction(enabled)
}

type sensorTaskDefinition struct {
	Principal struct {
		UserID    string `xml:"UserId"`
		RunLevel  string `xml:"RunLevel"`
		LogonType string `xml:"LogonType"`
	} `xml:"Principals>Principal"`
	Actions struct {
		Exec []struct {
			Command          string `xml:"Command"`
			Arguments        string `xml:"Arguments"`
			WorkingDirectory string `xml:"WorkingDirectory"`
		} `xml:"Exec"`
		Handler []struct{} `xml:"ComHandler"`
	} `xml:"Actions"`
}

func validSensorTask(raw, sid, programFiles string) bool {
	// COM already converted the BSTR to a UTF-8 Go string; Task Scheduler's
	// retained UTF-16 declaration must not trigger a second decoding.
	raw = strings.Replace(raw, `encoding="UTF-16"`, `encoding="UTF-8"`, 1)
	var d sensorTaskDefinition
	if xml.Unmarshal([]byte(raw), &d) != nil {
		return false
	}
	if len(d.Actions.Exec) != 1 || len(d.Actions.Handler) != 0 || d.Principal.UserID != "S-1-5-18" || d.Principal.RunLevel != "HighestAvailable" || (d.Principal.LogonType != "" && d.Principal.LogonType != "ServiceAccount") {
		return false
	}
	e := d.Actions.Exec[0]
	folder := filepath.Join(programFiles, "System Companion", "Sensors", sid)
	return strings.EqualFold(filepath.Clean(e.Command), filepath.Join(folder, "Sensors.exe")) && e.Arguments == "--persistent --sid "+sid && strings.EqualFold(filepath.Clean(e.WorkingDirectory), folder)
}
func rememberedSensorTask(sid, operation string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if e := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); e != nil {
		if oe, ok := e.(*ole.OleError); !ok || oe.Code() != 1 {
			return e
		}
	}
	defer ole.CoUninitialize()
	unknown, e := oleutil.CreateObject("Schedule.Service")
	if e != nil {
		return e
	}
	defer unknown.Release()
	service, e := unknown.QueryInterface(ole.IID_IDispatch)
	if e != nil {
		return e
	}
	defer service.Release()
	v, e := oleutil.CallMethod(service, "Connect")
	if e != nil {
		return e
	}
	v.Clear()
	folder, e := oleutil.CallMethod(service, "GetFolder", `\`)
	if e != nil {
		return e
	}
	defer folder.Clear()
	task, e := oleutil.CallMethod(folder.ToIDispatch(), "GetTask", sensorTaskPrefix+sid)
	if e != nil {
		return e
	}
	defer task.Clear()
	definition, e := oleutil.GetProperty(task.ToIDispatch(), "Xml")
	if e != nil {
		return e
	}
	defer definition.Clear()
	pf, e := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if e != nil {
		return e
	}
	if !validSensorTask(definition.ToString(), sid, pf) {
		return errors.New("saved sensor task does not match the protected helper")
	}
	switch operation {
	case "run":
		v, e = oleutil.CallMethod(task.ToIDispatch(), "Run", nil)
	case "stop":
		v, e = oleutil.CallMethod(task.ToIDispatch(), "Stop", int32(0))
	default:
		return nil
	}
	if v != nil {
		v.Clear()
	}
	return e
}
func persistentSensorRead(sid string) ([]byte, error) {
	if sid == "" {
		return nil, errors.New("Windows identity unavailable")
	}
	name := wide(`\\.\pipe\` + sensorTaskPrefix + sid)
	// Request individual write rights: GENERIC_WRITE would also request the
	// right to create a server instance, which the CPU-only pipe never grants.
	handle, e := windows.CreateFile(name, windows.GENERIC_READ|windows.FILE_WRITE_DATA|windows.FILE_WRITE_ATTRIBUTES, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED|0x00110000, 0)
	if e != nil {
		return nil, e
	}
	defer windows.CloseHandle(handle)
	mode := uint32(2)
	ok, _, e := kernel32.NewProc("SetNamedPipeHandleState").Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)), 0, 0)
	if ok == 0 {
		return nil, e
	}
	event, e := windows.CreateEvent(nil, 1, 0, nil)
	if e != nil {
		return nil, e
	}
	defer windows.CloseHandle(event)
	overlapped := windows.Overlapped{HEvent: event}
	command := [4]byte{'R', 'E', 'A', 'D'}
	var output [4096]byte
	var length uint32
	ok, _, e = kernel32.NewProc("TransactNamedPipe").Call(uintptr(handle), uintptr(unsafe.Pointer(&command[0])), 4, uintptr(unsafe.Pointer(&output[0])), 4096, uintptr(unsafe.Pointer(&length)), uintptr(unsafe.Pointer(&overlapped)))
	if ok == 0 {
		if e != windows.ERROR_IO_PENDING {
			return nil, e
		}
		state, waitErr := windows.WaitForSingleObject(event, 1500)
		if waitErr != nil || state != uint32(windows.WAIT_OBJECT_0) {
			windows.CancelIoEx(handle, &overlapped)
			windows.GetOverlappedResult(handle, &overlapped, &length, true)
			return nil, errors.New("CPU sensor response timed out")
		}
		if e = windows.GetOverlappedResult(handle, &overlapped, &length, false); e != nil {
			return nil, e
		}
	}

	if length == 0 || length > 4096 || !json.Valid(output[:length]) {
		return nil, fmt.Errorf("invalid CPU sensor response")
	}
	return output[:length], nil
}
