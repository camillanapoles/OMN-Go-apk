package backend

import (
	"fmt"
	"net/http"
	"strings"
)

// ----------------------------------------------------------------------
// One descriptor for each setting
// ----------------------------------------------------------------------
//
// A setting touches three places: the POST handler, the loader and the
// checkbox list of the Config page. One table holds the three, thus they
// cannot disagree. The table is the one authority for the form side of a
// setting. Each row tells how a request writes the field, and how the loader
// repairs an old value.
//
// The table drives applyConfigForm (the POST handler), configCheckboxFields
// (the hidden config_fields input) and normalizeConfig (loadConfig). The page
// view and the Status page keep their own typed structs, because a reader can
// follow a struct to the markup or to doc/API.md.
//
// The git server slots and active_git_index stay outside the table. They are
// an array of structs, and 21 rows would read worse than the loop of
// applyGitServerForm. TestEveryConfigFieldIsInTheTable fails for a Config
// field that has no place.

// configFieldKind tells how a request writes one field. An enumeration is a
// cfString row with a Normalize function that allows only known values, for
// example normalizeTheme.
type configFieldKind int

const (
	// cfBool is a checkbox. "true" is on, and each other value is off. A
	// browser sends nothing for a clear box, thus such a row must be in
	// configCheckboxFields.
	cfBool configFieldKind = iota

	// cfInt writes the value only when it parses to a positive number. An
	// empty field, a word or a zero changes nothing.
	cfInt

	// cfString writes the value as it arrives, also an empty value. An empty
	// author name is a valid value.
	cfString

	// cfList is a set of checkboxes with one name. The request carries the
	// whole new set, and an empty set is valid.
	cfList
)

// configField describes one setting. Exactly one of Bool, Int, String and
// List is set, and it agrees with Kind. Each answers a pointer into the
// Config of the caller, thus the apply loop needs no reflection.
type configField struct {
	// Key is the name of the form field and of the JSON key.
	// TestEveryConfigFieldIsInTheTable checks that the two are equal.
	Key  string
	Kind configFieldKind

	// Secret marks a value that the Config page never shows. "Show passwords"
	// reads it from GET /api/config. See
	// doc/decisions/0005-keep-each-secret-out-of-the-config-page.md.
	Secret bool

	Bool   func(*Config) *bool
	Int    func(*Config) *int
	String func(*Config) *string
	List   func(*Config) *[]string

	// Normalize repairs the value. loadConfig calls it for each row, and the
	// apply loop calls it after each write. A request thus cannot store a
	// value that the loader would refuse.
	Normalize func(*Config)
}

// configFields is the table, in the order of the Config page.
// configCheckboxFields uses that order.
var configFields = []configField{
	{
		Key: "server_port", Kind: cfInt,
		Int: func(c *Config) *int { return &c.ServerPort },
		// This row has no Normalize, because the repair needs the fallback
		// port, and that is a field of App. loadConfig holds that one line.
	},
	{
		Key: "admin_password", Kind: cfString, Secret: true,
		String: func(c *Config) *string { return &c.AdminPassword },
	},
	{
		Key: "author", Kind: cfString,
		String: func(c *Config) *string { return &c.Author },
	},
	{
		Key: "use_internal_editor", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.UseInternalEd },
	},
	{
		Key: "desktop_ext_cmd", Kind: cfString,
		String: func(c *Config) *string { return &c.DesktopExtCmd },
	},
	{
		// This is an enumeration. normalizeTheme changes each value other
		// than light or dark to auto.
		Key: "theme", Kind: cfString,
		String:    func(c *Config) *string { return &c.Theme },
		Normalize: func(c *Config) { c.Theme = normalizeTheme(c.Theme) },
	},
	{
		// The socket binds one time at the start, thus a change applies at
		// the next start. handleConfigPost answers "RestartRequired" when the
		// value changes.
		Key: "share_lan", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.ShareLAN },
	},
	{
		// This is the device label in the name of a database backup file. A
		// clear box gives the label of the operating system again. See
		// normalizeHostname.
		Key: "hostname", Kind: cfString,
		String:    func(c *Config) *string { return &c.Hostname },
		Normalize: func(c *Config) { c.Hostname = normalizeHostname(c.Hostname) },
	},
	{
		Key: "backup_prune_depth", Kind: cfInt,
		Int:       func(c *Config) *int { return &c.BackupPruneDepth },
		Normalize: func(c *Config) { c.BackupPruneDepth = normalizePruneDepth(c.BackupPruneDepth) },
	},
	{
		Key: "max_upload_size_mb", Kind: cfInt,
		Int: func(c *Config) *int { return &c.MaxUploadSizeMB },
		Normalize: func(c *Config) {
			if c.MaxUploadSizeMB <= 0 {
				c.MaxUploadSizeMB = defaultMaxUploadSizeMB
			}
		},
	},
	{
		// MainActivity reads this value from config.json at each tap. The
		// desktop ignores it.
		Key: "enable_intent_uri", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.EnableIntentURI },
	},
	{
		Key: "enable_termux_intent", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.EnableTermuxIntent },
	},
	{
		// This is an enumeration, the same as theme. An unknown value becomes
		// FullscreenOn.
		Key: "android_fullscreen", Kind: cfString,
		String:    func(c *Config) *string { return &c.AndroidFullscreen },
		Normalize: func(c *Config) { c.AndroidFullscreen = normalizeFullscreen(c.AndroidFullscreen) },
	},
	{
		Key: "search_enabled", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.SearchEnabled },
	},
	{
		Key: "search_bundled", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.SearchBundled },
	},
	{
		// This is a set of checkboxes. An empty set means "index nothing",
		// and nil means "no answer recorded". normalizeSearchKinds keeps the
		// two apart.
		Key: "search_kinds", Kind: cfList,
		List:      func(c *Config) *[]string { return &c.SearchKinds },
		Normalize: func(c *Config) { c.SearchKinds = normalizeSearchKinds(c.SearchKinds) },
	},
	{
		// This is a pair of radio buttons. A browser always sends one of
		// them, thus this key is not a checkbox key.
		Key: "search_scope", Kind: cfString,
		String:    func(c *Config) *string { return &c.SearchScope },
		Normalize: func(c *Config) { c.SearchScope = normalizeSearchScope(c.SearchScope) },
	},
	{
		Key: "log_debug", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.LogDebug },
	},
	{
		Key: "log_info", Kind: cfBool,
		Bool: func(c *Config) *bool { return &c.LogInfo },
	},
	{
		// This is a set of checkboxes, the same as search_kinds. An empty set
		// means "no debug or info line".
		Key: "log_tags", Kind: cfList,
		List:      func(c *Config) *[]string { return &c.LogTags },
		Normalize: func(c *Config) { c.LogTags = normalizeLogTags(c.LogTags) },
	},
}

// configCheckboxFields answers the keys that the Config page names in its
// hidden config_fields input, joined by commas. A browser sends nothing for a
// clear checkbox, thus "clear" and "not this form" look the same. The form
// declares its fields, and each name in the list counts as sent. See
// configFieldSent in config_handlers.go. The table writes the list, thus a
// new row needs no change of the markup.
func configCheckboxFields() string {
	var keys []string
	for _, f := range configFields {
		if f.Kind == cfBool || f.Kind == cfList {
			keys = append(keys, f.Key)
		}
	}
	return strings.Join(keys, ",")
}

// normalizeConfig repairs each field that has a Normalize function.
// loadConfig calls it one time after it reads config.json.
func normalizeConfig(c *Config) {
	for _, f := range configFields {
		if f.Normalize != nil {
			f.Normalize(c)
		}
	}
}

// applyConfigForm writes each field that the request carries into c. THE
// RULE: a field that the request does not carry stays as it is. sent tells
// whether the request carries a field, and a name in config_fields counts.
// See configFieldSent.
//
// A cfInt row needs no sent test, because a missing or an empty field does
// not parse to a positive number. Normalize runs after the write, thus a
// request cannot store a value that loadConfig would refuse.
func applyConfigForm(c *Config, r *http.Request, sent func(string) bool) {
	for _, f := range configFields {
		if !applyConfigField(f, c, r, sent) {
			continue
		}
		if f.Normalize != nil {
			f.Normalize(c)
		}
	}
}

// applyConfigField writes one field, and it reports whether it wrote it.
func applyConfigField(f configField, c *Config, r *http.Request, sent func(string) bool) bool {
	switch f.Kind {
	case cfBool:
		if !sent(f.Key) {
			return false
		}
		*f.Bool(c) = r.FormValue(f.Key) == "true"
		return true

	case cfInt:
		var n int
		fmt.Sscanf(r.FormValue(f.Key), "%d", &n)
		if n <= 0 {
			return false
		}
		*f.Int(c) = n
		return true

	case cfString:
		if !sent(f.Key) {
			return false
		}
		*f.String(c) = r.FormValue(f.Key)
		return true

	case cfList:
		if !sent(f.Key) {
			return false
		}
		// The empty slice is not nil, and the difference has a meaning. See
		// the search_kinds row.
		values := []string{}
		values = append(values, r.Form[f.Key]...)
		*f.List(c) = values
		return true
	}
	return false
}

// applyGitServerForm writes the git server slots and the active slot. Each
// slot is a struct of four fields with an index in its form names, thus the
// slots stay outside the table.
//
// Each field follows the sent rule of the table. The Config page does not
// carry the SSH key or the key password. A save that changes only the name
// must thus not replace the real key with an empty one. See
// doc/decisions/0005-keep-each-secret-out-of-the-config-page.md.
func applyGitServerForm(c *Config, r *http.Request, sent func(string) bool) {
	// The active slot is an index, and zero is valid, thus it cannot be a
	// cfInt row.
	if idxStr := r.FormValue("active_git_index"); idxStr != "" {
		var idx int
		fmt.Sscanf(idxStr, "%d", &idx)
		if idx >= 0 && idx < len(c.GitServers) {
			c.ActiveGitIndex = idx
		}
	}
	for i := 0; i < maxGitServers && i < len(c.GitServers); i++ {
		if f := fmt.Sprintf("git_name_%d", i); sent(f) {
			c.GitServers[i].Name = r.FormValue(f)
		}
		if f := fmt.Sprintf("git_url_%d", i); sent(f) {
			c.GitServers[i].URL = r.FormValue(f)
		}
		if f := fmt.Sprintf("git_key_%d", i); sent(f) {
			c.GitServers[i].SSHKeyData = r.FormValue(f)
		}
		if f := fmt.Sprintf("git_pass_%d", i); sent(f) {
			c.GitServers[i].Password = r.FormValue(f)
		}
	}
}
