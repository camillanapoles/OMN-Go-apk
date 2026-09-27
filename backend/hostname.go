package backend

import (
	"os"
	"regexp"
	"strings"
)

// hostnameUnsafeRe strips anything that cannot appear in the filename of a
// database backup. See db_backup.go. The hostname is embedded verbatim in
// <timestamp>_<hostname>.jsonl, thus it shares the same [A-Za-z0-9_-]
// alphabet that the database names already use.
var hostnameUnsafeRe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// sanitizeHostname maps an arbitrary user-supplied device label to a
// filename-safe string, capped at 64 chars. Empty input stays empty so
// callers can detect "not set" and fall back to defaultHostname.
func sanitizeHostname(s string) string {
	s = hostnameUnsafeRe.ReplaceAllString(strings.TrimSpace(s), "_")
	if len(s) > 64 {
		s = s[:64]
	}
	return strings.Trim(s, "_")
}

// defaultHostname derives a device label from the OS hostname. On Android
// that is usually a useless "localhost". The Hostname field of the Config
// page exists exactly so the user can set a meaningful label, such as
// "pixel7". A device needs that one time.
func defaultHostname() string {
	h, err := os.Hostname()
	if err != nil || sanitizeHostname(h) == "" || strings.EqualFold(h, "localhost") {
		return "device"
	}
	return sanitizeHostname(h)
}

// normalizeHostname repairs the device label of a configuration.
//
// It returns the label that the operating system gives when the stored
// value is empty or holds no usable character. loadConfig calls it, and
// so does the hostname row of configFields. A cleared box on the Config
// page therefore resets the label.
//
// A request that does not carry "hostname" does not change the label. The
// name of each database backup carries the label, thus a save of an
// unrelated setting must not rename the device.
//
// IT REPAIRS AT LOAD TIME. config.json, the Config page and db_backup.go
// thus read one value.
func normalizeHostname(h string) string {
	if s := sanitizeHostname(h); s != "" {
		return s
	}
	return defaultHostname()
}

// normalizePruneDepth repairs the count of backups that one database
// keeps. A count of zero or less keeps no backup at all, which no person
// asks for, thus it becomes the default of three.
//
// It also repairs at load time, the same as normalizeHostname above. The
// Config page thus shows the value that config.json holds.
func normalizePruneDepth(d int) int {
	if d <= 0 {
		return 3
	}
	return d
}
