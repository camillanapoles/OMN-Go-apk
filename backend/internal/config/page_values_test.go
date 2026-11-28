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
		"search_scope", "log_debug", "log_info", "log_tags", "admin_password_default"}
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

// The first-run marks stand while the install holds the default password
// or the default author, and both go away after real values are set. See
// doc/decisions/0034.
func TestPageValuesCarryTheFirstRunMarks(t *testing.T) {
	collect := func(c Config) map[string]string {
		out := map[string]string{}
		for _, v := range PageValues(c) {
			out[v.Name] = v.Value
		}
		return out
	}

	fresh := collect(Config{AdminPassword: DefaultAdminPassword, Author: DefaultAuthor})
	if fresh["DEFAULT_PASSWORD_MARK"] != "true" {
		t.Errorf("fresh DEFAULT_PASSWORD_MARK = %q, want true", fresh["DEFAULT_PASSWORD_MARK"])
	}
	if fresh["AUTHOR_DEFAULT"] != "true" {
		t.Errorf("fresh AUTHOR_DEFAULT = %q, want true", fresh["AUTHOR_DEFAULT"])
	}
	if !strings.Contains(fresh["FIRST_RUN_WARNING"], "default admin password") {
		t.Error("the warning block of a fresh install does not name the default password")
	}
	if !strings.Contains(fresh["FIRST_RUN_WARNING"], "author") {
		t.Error("the warning block of a fresh install does not name the author")
	}

	own := collect(Config{AdminPassword: "own long password", Author: "Ana"})
	for _, name := range []string{"DEFAULT_PASSWORD_MARK", "AUTHOR_DEFAULT", "FIRST_RUN_WARNING"} {
		if own[name] != "" {
			t.Errorf("%s = %q after real values, want empty", name, own[name])
		}
	}
}

// The config section of /api/status marks the default password, and the
// mark clears after a real password. See doc/decisions/0034.
func TestStatusValuesMarkTheDefaultPassword(t *testing.T) {
	find := func(c Config) (bool, bool) {
		found, set := false, false
		for _, v := range StatusValues(c) {
			if v.Key == "admin_password_default" {
				found = true
				b, _ := v.Value.(bool)
				set = b
			}
		}
		return found, set
	}

	found, set := find(Config{AdminPassword: DefaultAdminPassword})
	if !found {
		t.Fatal("the status holds no admin_password_default row")
	}
	if !set {
		t.Error("the status does not mark the default password")
	}
	found, set = find(Config{AdminPassword: "own long password"})
	if !found || set {
		t.Errorf("after a real password the mark is found=%v set=%v, want found=true set=false", found, set)
	}
}
