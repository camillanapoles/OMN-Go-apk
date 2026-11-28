package app

import (
	"net/http"
	"os"

	"net.basov.omngo/backend/internal/storage"
)

// handleTemplates answers the note templates of this install. The answer
// names the templates directory, thus a person can drop own .md files
// there. See doc/decisions/0032-open-note-templates.md.
func (a *App) handleTemplates(w http.ResponseWriter, r *http.Request) {
	dir := a.layout().Templates()
	list := storage.ListTemplates(dir)
	type entry struct {
		Name    string `json:"name"`
		Content string `json:"content"`
		Builtin bool   `json:"builtin"`
	}
	_, statErr := os.Stat(dir)
	out := struct {
		Dir       string  `json:"dir"`
		DirExists bool    `json:"dir_exists"`
		Templates []entry `json:"templates"`
	}{Dir: dir, DirExists: statErr == nil}
	for _, t := range list {
		out.Templates = append(out.Templates,
			entry{Name: t.Name, Content: t.Content, Builtin: t.Builtin})
	}
	a.writeJSON(w, http.StatusOK, out)
}
