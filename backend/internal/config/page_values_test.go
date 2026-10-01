package config

import (
	"strings"
	"testing"
)

// A Secret row gives no value to the Config page. See
// doc/decisions/0005-keep-each-secret-out-of-the-config-page.md.
func TestPageValuesGiveNoSecret(t *testing.T) {
	const secret = "s3cret-value"
	c := Config{AdminPassword: secret}
	for _, pv := range PageValues(c) {
		if strings.Contains(pv.Value, secret) {
			t.Errorf("%s carries the admin password", pv.Name)
		}
		if strings.HasPrefix(pv.Name, "ADMIN_PASSWORD") {
			t.Errorf("the Secret row admin_password gives the value %s", pv.Name)
		}
	}
}

// Each option of a row gives one placeholder, and only the chosen option
// carries the mark. The values come from a normalized copy, thus an unknown
// theme marks auto.
func TestPageValuesMarkTheChosenOption(t *testing.T) {
	values := map[string]string{}
	for _, pv := range PageValues(Config{Theme: "purple", SearchKinds: []string{SearchKindJS}}) {
		values[pv.Name] = pv.Value
	}
	for name, want := range map[string]string{
		"THEME_AUTO":         "selected",
		"THEME_LIGHT":        "",
		"THEME_DARK":         "",
		"SEARCH_KINDS_JS":    "checked",
		"SEARCH_KINDS_MD":    "",
		"LOG_TAGS_DB_BACKUP": "checked",
	} {
		got, ok := values[name]
		if !ok {
			t.Errorf("no placeholder %s", name)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// The keys of the config section of /api/status are an API. doc/API.md names
// them in this order. A change here needs a change there.
func TestStatusValuesKeepTheStatusKeys(t *testing.T) {
	want := []string{"author", "internal_editor", "theme", "hostname",
		"backup_prune_depth", "max_upload_mb", "intent_uri", "termux_intent",
		"android_fullscreen", "search_enabled", "search_bundled", "search_kinds",
		"search_scope", "log_debug", "log_info", "log_tags"}
	var got []string
	for _, v := range StatusValues(Config{AdminPassword: "s3cret"}) {
		got = append(got, v.Key)
		if v.Value == "s3cret" {
			t.Errorf("%s carries the admin password", v.Key)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the status keys are %v, want %v", got, want)
	}
}
