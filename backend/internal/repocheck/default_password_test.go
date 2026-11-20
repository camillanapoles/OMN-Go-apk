package repocheck

// ----------------------------------------------------------------------
// The default admin password stays in one file
// ----------------------------------------------------------------------
//
// The fresh install writes one default admin password. The value is
// public, and the network login refuses it while it stands. A second
// copy of the literal would give the value a second home. A change would
// then fix one copy and miss the other. Section 1 rule 7 of CLAUDE.md
// gives the rule: one authority for one decision.
//
// The scan reads each source file of the repository. A document may name
// the password, because a document warns the reader with the value. The
// code reads the constant config.DefaultAdminPassword and never spells
// the value.
//
// The needle below is the value of that constant. Its two string halves
// keep this file free of one full spelling, thus the scan needs no
// exception for this file. See
// doc/decisions/0030-refuse-the-default-admin-password-on-the-network.md.

import (
	"strings"
	"testing"
)

const defaultPasswordNeedle = "admin_secret_" + "changeme"

// defaultPasswordHome is the one file that may hold the literal. It is
// the authority of the value.
const defaultPasswordHome = "backend/internal/config/config.go"

// Each source file outside config.go holds no spelling of the default
// password. A test that needs the value reads the constant.
func TestNoSourceFileSpellsTheDefaultPassword(t *testing.T) {
	read := 0
	for _, rel := range commentStyleFiles(t) {
		src, err := readRepoFile(rel)
		if err != nil {
			t.Fatalf("cannot read %s: %v", rel, err)
		}
		read++
		if rel == defaultPasswordHome {
			continue
		}
		if strings.Contains(src, defaultPasswordNeedle) {
			t.Errorf("%s spells the default admin password."+
				"\n  Write config.DefaultAdminPassword instead."+
				"\n  See doc/decisions/0030-refuse-the-default-admin-password-on-the-network.md.",
				rel)
		}
	}
	if read == 0 {
		t.Fatal("no file was read, thus this test proves nothing")
	}
}
