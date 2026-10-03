package repocheck

import (
	"regexp"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------
// Each time is UTC
// ----------------------------------------------------------------------
//
// The desktop wrote each time in the zone of the computer, and Android
// wrote UTC. See doc/decisions/0023-write-each-time-in-utc.md. The tests of
// package app move the local zone and read the result of each handler.
// This file reads the source, thus a NEW place that writes a time cannot
// use the zone of the computer.

// timeFormatCallRe finds a call of the method Format.
var timeFormatCallRe = regexp.MustCompile(`\.Format\(`)

// timeLocalRe finds code that asks for the zone of the computer by name.
var timeLocalRe = regexp.MustCompile(`time\.Local\b|\.Local\(\)|\.In\(`)

// A time becomes text only after .UTC(), on the same line.
// noteheader.Stamp holds that call for a time in a note.
func TestEachFormattedTimeIsUTC(t *testing.T) {
	calls := 0
	for _, rel := range productionGoFiles(t) {
		raw, err := readBackendFile(rel)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			code := line
			if at := strings.Index(code, "//"); at >= 0 {
				code = code[:at]
			}
			if timeLocalRe.MatchString(code) {
				t.Errorf("%s:%d asks for a time zone: %s\nEach time of the application is UTC.",
					rel, i+1, strings.TrimSpace(line))
			}
			if !timeFormatCallRe.MatchString(code) {
				continue
			}
			calls++
			if !strings.Contains(code, ".UTC().Format(") {
				t.Errorf("%s:%d makes text from a time with no .UTC(): %s\n"+
					"The desktop then writes the time of the computer, and Android writes "+
					"UTC. Use noteheader.Stamp for a time in a note, or .UTC().Format.",
					rel, i+1, strings.TrimSpace(line))
			}
		}
	}
	if calls == 0 {
		t.Fatal("the scan found no call of Format, thus this test reads nothing")
	}
}

// The lines of the standard log package go to the same page as the lines
// of Logger.emit. log.LUTC makes them UTC.
func TestStandardLogIsUTC(t *testing.T) {
	raw, err := readBackendFile("internal/app/log_app.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "log.SetFlags(log.LstdFlags | log.LUTC)") {
		t.Error("initLogger no longer sets log.LUTC. The lines of the standard log " +
			"package then have the time of the computer.")
	}
}

// The one layout of a time in a note is noteheader.StampLayout. A second
// copy of the text is a second place that can forget .UTC().
func TestNoteStampLayoutHasOneAuthority(t *testing.T) {
	for _, rel := range productionGoFiles(t) {
		if rel == "internal/noteheader/noteheader.go" {
			continue
		}
		raw, err := readBackendFile(rel)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"2006-01-02 15:04:05"`) {
			t.Errorf("%s holds the layout of a note time. Use noteheader.Stamp.", rel)
		}
	}
}
