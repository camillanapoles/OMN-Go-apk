package storage

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// --- Note templates ---

// Template is one note template of the editor. Name is the file name
// without the .md suffix. Builtin marks a template that ships with the app,
// and a file of the same name in the templates directory shadows it.
type Template struct {
	Name    string
	Content string
	Builtin bool
}

// builtinTemplates are the templates of a fresh install. Each one starts
// with the header block that the renderer reads. None carries a date,
// because the person fills the date when the note starts. The directory is
// an open drop-in, and no format of the fork locks the person in. See
// doc/decisions/0032-open-note-templates.md.
var builtinTemplates = []Template{
	{
		Name:    "Meeting notes",
		Builtin: true,
		Content: `Title: Meeting
Category: Meetings

# Meeting notes

Attendees:

-

## Decisions

-

## Action items

- [ ] First action
`,
	},
	{
		Name:    "Daily log",
		Builtin: true,
		Content: `Title: Daily log
Category: Log

# Daily log

## What happened

-

## Follow up

- [ ] First follow up
`,
	},
	{
		Name:    "Task list",
		Builtin: true,
		Content: `Title: Task list
Category: Tasks

# Task list

- [ ] First task
- [ ] Second task
`,
	},
	{
		Name:    "Book review",
		Builtin: true,
		Content: `Title: Book review
Category: Reviews

# Book review

Author:
Rating:
Pages:

## What the book says

-

## What I take with me

-
`,
	},
}

// ListTemplates answers the templates of dir, with the built-ins in the
// list. A .md file in dir shadows a built-in of the same name, thus a
// person can replace each built-in text with an own copy. A file that the
// read fails to open leaves the list alone, and the answer stays stable.
//
// The directory is an open drop-in. Each .md file from any editor or tool
// becomes a template, and the person leaves no format behind.
func ListTemplates(dir string) []Template {
	var out []Template
	seen := map[string]bool{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		seen[name] = true
		out = append(out, Template{Name: name, Content: string(data)})
	}
	for _, t := range builtinTemplates {
		if !seen[t.Name] {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
