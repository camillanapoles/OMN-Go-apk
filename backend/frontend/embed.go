// Package frontend holds the files that the binary embeds. It imports no
// package of this module.
package frontend

import "embed"

// Static holds html/ and md/. The app extracts each file of html/ into
// StorageDir/html, and a person can edit it there with ?edit=true. md/ holds
// the bundled notes. test/ is not in Static, thus no test file reaches a
// device.
//
//go:embed html md
var Static embed.FS

// Templates holds the page fragments that the server renders. It is apart
// from Static on purpose. A template is render logic, and a person must not
// damage it. The app never extracts a template to disk.
//
//go:embed templates
var Templates embed.FS
