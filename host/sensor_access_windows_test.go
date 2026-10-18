package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRememberedSensorTaskRejectsMutableActions(t *testing.T) {
	sid := "S-1-5-21-1-2-3-1001"
	pf := `C:\Program Files`
	folder := filepath.Join(pf, "System Companion", "Sensors", sid)
	task := fmt.Sprintf(`<Task><Principals><Principal><UserId>S-1-5-18</UserId><RunLevel>HighestAvailable</RunLevel><LogonType>ServiceAccount</LogonType></Principal></Principals><Actions><Exec><Command>%s</Command><Arguments>--persistent --sid %s</Arguments><WorkingDirectory>%s</WorkingDirectory></Exec></Actions></Task>`, filepath.Join(folder, "Sensors.exe"), sid, folder)
	if !validSensorTask(task, sid, pf) {
		t.Fatal("protected task rejected")
	}
	if !validSensorTask(strings.Replace(task, "<LogonType>ServiceAccount</LogonType>", "", 1), sid, pf) {
		t.Fatal("Windows omits LogonType for SYSTEM; valid task rejected")
	}
	for _, change := range []struct{ old, replacement string }{
		{filepath.Join(folder, "Sensors.exe"), `C:\Users\User\AppData\Sensors.exe`},
		{"--persistent --sid " + sid, "--out C:\\windows\\injected"},
		{"HighestAvailable", "LeastPrivilege"},
		{"ServiceAccount", "InteractiveToken"},
		{"</Actions>", "<ComHandler/></Actions>"},
		{"</Actions>", "<Exec/></Actions>"},
	} {
		bad := strings.Replace(task, change.old, change.replacement, 1)
		if validSensorTask(bad, sid, pf) {
			t.Fatal("unsafe task accepted", change)
		}
	}
}

func TestRegisteredWindowsSensorTask(t *testing.T) {
	path := os.Getenv("COMPANION_SENSOR_TASK_XML")
	if path == "" {
		t.Skip("real Windows task XML fixture required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := sensorSID()
	if err != nil {
		t.Fatal(err)
	}
	if !validSensorTask(string(raw), sid, `C:\Program Files`) {
		t.Fatal("actual SYSTEM task export rejected")
	}
	if err = rememberedSensorTask(sid, "check"); err != nil {
		t.Fatal("approved task lookup failed", err)
	}
}
