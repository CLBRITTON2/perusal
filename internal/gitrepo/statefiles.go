package gitrepo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileStamp is a file's size and last write, with Exists false for a missing file.
type FileStamp struct {
	Path               string
	Exists             bool
	Size               int64
	ModifiedAtUnixNano int64
}

// StateFileStamps stamps the files holding HEAD, the index, the config and the refs, so a caller can tell this work
// tree's git state changed without starting git. A work tree edit that has not been staged changes none of them.
func (r Repository) StateFileStamps() ([]FileStamp, error) {
	if r.GitDir == "" {
		return []FileStamp{}, fmt.Errorf("stamp the git state of %s: its git directory is not pinned", r.Dir)
	}
	commonDir, err := r.commonDir()
	if err != nil {
		return []FileStamp{}, err
	}
	stamps := []FileStamp{}
	for _, path := range []string{filepath.Join(r.GitDir, "HEAD"), filepath.Join(r.GitDir, "index"),
		filepath.Join(commonDir, "config"), filepath.Join(commonDir, "packed-refs")} {
		info, err := os.Stat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			stamps = append(stamps, FileStamp{Path: path})
		case err != nil:
			return []FileStamp{}, fmt.Errorf("stamp %s: %w", path, err)
		default:
			stamps = append(stamps, FileStamp{Path: path, Exists: true, Size: info.Size(),
				ModifiedAtUnixNano: info.ModTime().UnixNano()})
		}
	}
	err = filepath.WalkDir(filepath.Join(commonDir, "refs"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("stamp %s: %w", path, walkErr)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		switch {
		// A ref deleted since the listing is left out, which already differs from a stamp that held it.
		case errors.Is(err, fs.ErrNotExist):
			return nil
		case err != nil:
			return fmt.Errorf("stamp %s: %w", path, err)
		}
		stamps = append(stamps, FileStamp{Path: path, Exists: true, Size: info.Size(),
			ModifiedAtUnixNano: info.ModTime().UnixNano()})
		return nil
	})
	if err != nil {
		return []FileStamp{}, err
	}
	return stamps, nil
}

// commonDir is the git directory a linked work tree shares with its main one, which holds the refs and config.
func (r Repository) commonDir() (string, error) {
	pointerFile := filepath.Join(r.GitDir, "commondir")
	content, err := os.ReadFile(pointerFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return r.GitDir, nil
	case err != nil:
		return "", fmt.Errorf("read %s: %w", pointerFile, err)
	}
	named := filepath.FromSlash(strings.TrimRight(string(content), "\r\n"))
	if filepath.IsAbs(named) {
		return named, nil
	}
	return filepath.Join(r.GitDir, named), nil
}
