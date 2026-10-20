package gitrepo

import (
	"bytes"
	"fmt"
)

// FileStatus is one entry of git status --porcelain=v1. IndexStatus and WorktreeStatus are its X and Y codes.
type FileStatus struct {
	Path string
	// OriginalPath is the source of a rename or copy, empty otherwise.
	OriginalPath   string
	IndexStatus    byte
	WorktreeStatus byte
}

func (f FileStatus) IsUntracked() bool {
	return f.IndexStatus == '?'
}

func (f FileStatus) HasStagedChanges() bool {
	return f.IndexStatus != ' ' && f.IndexStatus != '?'
}

// HasUnstagedChanges is true for an untracked file too, since staging it is still pending.
func (f FileStatus) HasUnstagedChanges() bool {
	return f.WorktreeStatus != ' '
}

// ParseStatus reads the NUL-separated output of git status --porcelain=v1 -z. A rename or copy entry is followed by
// a second field holding its original path.
func ParseStatus(output []byte) ([]FileStatus, error) {
	fields := bytes.Split(bytes.TrimSuffix(output, []byte{0}), []byte{0})
	files := []FileStatus{}
	if len(output) == 0 {
		return files, nil
	}
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 || entry[2] != ' ' {
			return nil, fmt.Errorf("parse git status entry %d: unexpected %q", i, entry)
		}
		file := FileStatus{Path: string(entry[3:]), IndexStatus: entry[0], WorktreeStatus: entry[1]}
		if file.IndexStatus == 'R' || file.IndexStatus == 'C' {
			i++
			if i == len(fields) {
				return nil, fmt.Errorf("parse git status entry %q: rename has no original path", entry)
			}
			file.OriginalPath = string(fields[i])
		}
		files = append(files, file)
	}
	return files, nil
}
