package backend

import (
	"os"
	"regexp"
	"strings"
)

// hostnameUnsafeRe finds each character that cannot be in the file name of a
// database backup, <timestamp>_<hostname>.jsonl. See db_backup.go. The
// allowed set is [A-Za-z0-9_-], the same as for a database name.
var hostnameUnsafeRe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// sanitizeHostname makes a device label safe for a file name, with at most 64
// characters. An empty input stays empty, thus the caller can use
// defaultHostname.
func sanitizeHostname(s string) string {
	s = hostnameUnsafeRe.ReplaceAllString(strings.TrimSpace(s), "_")
	if len(s) > 64 {
		s = s[:64]
	}
	return strings.Trim(s, "_")
}

// defaultHostname answers a device label from the hostname of the system. On
// Android that is often "localhost", thus a person can set a label such as
// "pixel7" on the Config page.
func defaultHostname() string {
	h, err := os.Hostname()
	if err != nil || sanitizeHostname(h) == "" || strings.EqualFold(h, "localhost") {
		return "device"
	}
	return sanitizeHostname(h)
}

// normalizeHostname repairs the device label. It answers the label of the
// system when the stored value is empty or has no usable character.
// loadConfig and the hostname row of configFields call it, thus a clear box
// resets the label.
//
// A request that does not carry "hostname" does not change the label. The
// name of each backup file holds the label, thus a save of another setting
// must not rename the device.
func normalizeHostname(h string) string {
	if s := sanitizeHostname(h); s != "" {
		return s
	}
	return defaultHostname()
}

// normalizePruneDepth repairs the number of backups that each database keeps.
// Zero or less keeps no backup, and nobody asks for that, thus it becomes the
// default of three.
func normalizePruneDepth(d int) int {
	if d <= 0 {
		return 3
	}
	return d
}
