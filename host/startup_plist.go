package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

const macStartupLabel = "local.systemcompanion.monitor"

func macStartupPlist(executable string) []byte {
	var path bytes.Buffer
	_ = xml.EscapeText(&path, []byte(executable))
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>--startup</string><string>--no-browser</string></array>
<key>RunAtLoad</key><true/>
<key>ProcessType</key><string>Background</string>
<key>LimitLoadToSessionType</key><string>Aqua</string>
</dict></plist>
`, macStartupLabel, path.String()))
}
