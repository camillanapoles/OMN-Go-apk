package gitsync

import (
	"os"
	"time"

	"github.com/go-git/go-billy/v5"
)

// ----------------------------------------------------------------------
// Filesystem workarounds for Android
// ----------------------------------------------------------------------
//
// See the banner of repo.go for what each git file holds.
//
// go-git speaks to the worktree and to the object store through a
// go-billy filesystem. The two wrappers below change what that
// filesystem answers, and each one repairs a real fault on Android.
// ----------------------------------------------------------------------
// Android filesystem workarounds
// ----------------------------------------------------------------------

// WorktreeFS wraps baseFS with both workarounds of this file. The sync and
// the Status page use it, thus both see one worktree.
func WorktreeFS(baseFS billy.Filesystem) billy.Filesystem {
	return &noLockFS{&stableMtimeFS{baseFS}}
}

type noLockFS struct {
	billy.Filesystem
}

func (fs *noLockFS) Create(filename string) (billy.File, error) {
	f, err := fs.Filesystem.Create(filename)
	if err != nil {
		return nil, err
	}
	return &noLockFile{f}, nil
}

func (fs *noLockFS) Open(filename string) (billy.File, error) {
	f, err := fs.Filesystem.Open(filename)
	if err != nil {
		return nil, err
	}
	return &noLockFile{f}, nil
}

func (fs *noLockFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	f, err := fs.Filesystem.OpenFile(filename, flag, perm)
	if err != nil {
		return nil, err
	}
	return &noLockFile{f}, nil
}

func (fs *noLockFS) TempFile(dir, prefix string) (billy.File, error) {
	f, err := fs.Filesystem.TempFile(dir, prefix)
	if err != nil {
		return nil, err
	}
	return &noLockFile{f}, nil
}

func (fs *noLockFS) Chroot(path string) (billy.Filesystem, error) {
	c, err := fs.Filesystem.Chroot(path)
	if err != nil {
		return nil, err
	}
	return &noLockFS{c}, nil
}

type noLockFile struct {
	billy.File
}

func (f *noLockFile) Lock() error   { return nil }
func (f *noLockFile) Unlock() error { return nil }

// ---------------------------------------------------------------
// Stable mtime wrapper (forces content‑hash based status check)
// ---------------------------------------------------------------

type stableMtimeFS struct {
	billy.Filesystem
}

func (fs *stableMtimeFS) Stat(path string) (os.FileInfo, error) {
	fi, err := fs.Filesystem.Stat(path)
	if err != nil {
		return nil, err
	}
	return &stableFileInfo{fi}, nil
}

func (fs *stableMtimeFS) Lstat(path string) (os.FileInfo, error) {
	fi, err := fs.Filesystem.Lstat(path)
	if err != nil {
		return nil, err
	}
	return &stableFileInfo{fi}, nil
}

type stableFileInfo struct {
	os.FileInfo
}

func (fi *stableFileInfo) ModTime() time.Time {
	return time.Unix(0, 0)
}
