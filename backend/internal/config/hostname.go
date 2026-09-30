package config

import (
	"os"
	"regexp"
	"strings"
)

// hostnameUnsafeRe finds each character that cannot be in the file name of a
// database backup, <timestamp>_<hostname>.jsonl. See internal/db/backup.go. The
// allowed set is [A-Za-z0-9_-], the same as for a database name.
var hostnameUnsafeRe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// SanitizeHostname makes a device label safe for a file name, with at most 64
// characters. An empty input stays empty, thus the caller can use
// DefaultHostname.
func SanitizeHostname(s string) string {
	s = hostnameUnsafeRe.ReplaceAllString(strings.TrimSpace(s), "_")
	if len(s) > 64 {
		s = s[:64]
	}
	return strings.Trim(s, "_")
}

// DefaultHostname answers a device label from the hostname of the system. On
// Android that is often "localhost", thus a person can set a label such as
// "pixel7" on the Config page.
func DefaultHostname() string {
	h, err := os.Hostname()
	if err != nil || SanitizeHostname(h) == "" || strings.EqualFold(h, "localhost") {
		return "device"
	}
	return SanitizeHostname(h)
}

// NormalizeHostname repairs the device label. It answers the label of the
// system when the stored value is empty or has no usable character.
// loadConfig and the hostname row of configFields call it, thus a clear box
// resets the label.
//
// A request that does not carry "hostname" does not change the label. The
// name of each backup file holds the label, thus a save of another setting
// must not rename the device.
func NormalizeHostname(h string) string {
	if s := SanitizeHostname(h); s != "" {
		return s
	}
	return DefaultHostname()
}

// NormalizePruneDepth repairs the number of backups that each database keeps.
// Zero or less keeps no backup, and nobody asks for that, thus it becomes the
// default of three.
func NormalizePruneDepth(d int) int {
	if d <= 0 {
		return 3
	}
	return d
}
