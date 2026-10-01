package config

import (
	"strconv"
	"strings"

	"net.basov.omngo/backend/internal/logx"
)

// PageValue is one placeholder of the Config page and its value.
type PageValue struct {
	Name  string
	Value string
	Text  bool // a value that a person typed, which the page escapes
}

// PageValues answers the placeholders of the Config page for a normalized
// copy of c. A Secret row gives none. Section 3 of CLAUDE.md gives the names.
func PageValues(c Config) []PageValue {
	normalizeConfig(&c)
	var out []PageValue
	for _, f := range configFields {
		if f.Secret {
			continue
		}
		name := PlaceholderName(f.Key)
		switch {
		case f.Options != nil:
			chosen := map[string]bool{}
			if f.Kind == cfList {
				for _, v := range *f.List(&c) {
					chosen[v] = true
				}
			} else {
				chosen[*f.String(&c)] = true
			}
			for _, o := range f.Options {
				mark := ""
				if chosen[o] {
					mark = f.Mark
				}
				out = append(out, PageValue{Name: name + "_" + PlaceholderName(o), Value: mark})
			}
		case f.Kind == cfBool:
			mark := ""
			if *f.Bool(&c) {
				mark = "checked"
			}
			out = append(out, PageValue{Name: name + "_CHECKED", Value: mark})
		case f.Kind == cfInt:
			out = append(out, PageValue{Name: name, Value: strconv.Itoa(*f.Int(&c))})
		case f.Kind == cfString:
			out = append(out, PageValue{Name: name, Value: *f.String(&c), Text: true})
		}
	}
	return out
}

// PlaceholderName answers s in upper case, with "_" for each other character.
func PlaceholderName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		}
		return '_'
	}, s)
}

// logTagNames answers the tags of logx.AllTags as strings, in their order.
func logTagNames() []string {
	names := make([]string, len(logx.AllTags))
	for i, t := range logx.AllTags {
		names[i] = string(t)
	}
	return names
}

// StatusValue is one value of the config section of /api/status.
type StatusValue struct {
	Key   string
	Value any // a bool, an int, a string or a []string
}

// StatusValues answers the rows with a Status key for a normalized copy of c.
func StatusValues(c Config) []StatusValue {
	normalizeConfig(&c)
	var out []StatusValue
	for _, f := range configFields {
		if f.Status == "" {
			continue
		}
		var v any
		switch f.Kind {
		case cfBool:
			v = *f.Bool(&c)
		case cfInt:
			v = *f.Int(&c)
		case cfString:
			v = *f.String(&c)
		case cfList:
			v = append([]string{}, *f.List(&c)...)
		}
		out = append(out, StatusValue{Key: f.Status, Value: v})
	}
	return out
}
