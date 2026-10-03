package config

// UserFileTree is one tree of files that the user uploads. The binary embeds
// none of these files.
type UserFileTree struct {
	// Dir is the directory below html/. It is also the first segment of the
	// URL, the search kind and the name of the Status group.
	Dir string
	// Upload is the route that writes a file into the tree.
	Upload string
	// Label names the tree in a heading of the search page.
	Label string
	// Exts are the extensions that the upload accepts, in lower case. The
	// first one is the extension of a shared file that has no name.
	Exts []string
}

// UserFileTrees is the one table of the trees of uploaded files. The routes,
// the upload handler, the search index and the Status page read it.
// OmnText.java and omn-go-editor.js hold a copy each, because neither can
// read Go. TestUserFileTreesHaveTheirCopies compares the three.
var UserFileTrees = []UserFileTree{
	{Dir: "user_json", Upload: "/api/upload_json", Label: "Uploaded JSON", Exts: []string{".json", ".jsonl"}},
	{Dir: "user_contacts", Upload: "/api/upload_contacts", Label: "Contacts", Exts: []string{".vcf"}},
	{Dir: "user_calendars", Upload: "/api/upload_calendars", Label: "Calendars", Exts: []string{".ics", ".vcs"}},
}

// userFileKinds answers the search kind of each tree, in the order of the
// table.
func userFileKinds() []string {
	kinds := make([]string, 0, len(UserFileTrees))
	for _, tree := range UserFileTrees {
		kinds = append(kinds, tree.Dir)
	}
	return kinds
}
