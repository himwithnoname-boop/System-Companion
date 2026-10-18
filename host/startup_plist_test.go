package main

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestMacStartupPlist(t *testing.T) {
	data := macStartupPlist("/Users/A & B/<test>/Monitor")
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	found := map[string]bool{}
	for {
		tok, err := decoder.Token()
		if err != nil {
			if err != io.EOF {
				t.Fatal(err)
			}
			break
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "string" {
			var s string
			if err := decoder.DecodeElement(&s, &start); err != nil {
				t.Fatal(err)
			}
			found[s] = true
		}
	}
	for _, s := range []string{macStartupLabel, "/Users/A & B/<test>/Monitor", "--startup", "--no-browser", "Aqua"} {
		if !found[s] {
			t.Fatal("missing argument", s)
		}
	}
	if strings.Contains(string(data), "KeepAlive") {
		t.Fatal("must not poll or respawn")
	}
}
