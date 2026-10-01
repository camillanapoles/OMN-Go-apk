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
