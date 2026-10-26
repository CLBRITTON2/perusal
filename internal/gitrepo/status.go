package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// IndexStatus and WorktreeStatus are the X and Y codes of git status --short.
type FileStatus struct {
	Path           string
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

// pathspec names a rename's original path too.
func (f FileStatus) pathspec() []string {
	if f.OriginalPath == "" {
		return []string{f.Path}
	}
	return []string{f.Path, f.OriginalPath}
}

// HasUnstagedChanges is true for an untracked file too, since staging it is still pending.
func (f FileStatus) HasUnstagedChanges() bool {
	return f.WorktreeStatus != ' '
}

type Status struct {
	// Branch is empty when HEAD is detached.
	Branch string
	Tracking
	HasCommits bool
	Files      []FileStatus
}

// IsPublished reports whether the branch tracks its own name on origin, the only branch perusal pushes it to.
func (s Status) IsPublished() bool {
	return s.Branch != "" && s.Upstream == "origin/"+s.Branch && !s.UpstreamGone
}

type Tracking struct {
	// Upstream is empty when the branch tracks nothing. Ahead and Behind are 0 then, and when UpstreamGone.
	Upstream string
	// UpstreamGone is true when the ref Upstream names is missing, as after its remote branch was deleted and pruned.
	UpstreamGone bool
	Ahead        int
	Behind       int
}

// parseStatus reads the NUL-separated output of git status --porcelain=v2 --branch -z.
func parseStatus(output []byte) (Status, error) {
	if len(output) == 0 {
		return Status{}, errors.New("parse git status: no output, want at least the branch headers")
	}
	status := Status{Files: []FileStatus{}}
	hasAheadBehind := false
	entries := bytes.Split(bytes.TrimSuffix(output, []byte{0}), []byte{0})
	for i := 0; i < len(entries); i++ {
		entry := string(entries[i])
		kind, rest, _ := strings.Cut(entry, " ")
		switch kind {
		case "#":
			name, value, _ := strings.Cut(rest, " ")
			switch name {
			case "branch.oid":
				status.HasCommits = value != "(initial)"
			case "branch.head":
				if value != "(detached)" {
					status.Branch = value
				}
			case "branch.upstream":
				status.Upstream = value
			case "branch.ab":
				hasAheadBehind = true
				aheadText, behindText, found := strings.Cut(value, " ")
				if !found || !strings.HasPrefix(aheadText, "+") || !strings.HasPrefix(behindText, "-") {
					return Status{}, fmt.Errorf("parse git status entry %q: malformed ahead and behind counts %q", entry,
						value)
				}
				ahead, err := strconv.Atoi(aheadText[1:])
				if err != nil {
					return Status{}, fmt.Errorf("parse git status entry %q: parse ahead count %q: %w", entry, value, err)
				}
				behind, err := strconv.Atoi(behindText[1:])
				if err != nil {
					return Status{}, fmt.Errorf("parse git status entry %q: parse behind count %q: %w", entry, value, err)
				}
				status.Ahead = ahead
				status.Behind = behind
			}
		case "1", "2", "u":
			// Counts XY and every field before the path, which is the rest of the entry, so it can hold spaces.
			pathIndex := map[string]int{"1": 7, "2": 8, "u": 9}[kind]
			parts := strings.SplitN(rest, " ", pathIndex+1)
			if len(parts) != pathIndex+1 || len(parts[0]) != 2 || parts[pathIndex] == "" {
				return Status{}, fmt.Errorf("parse git status entry %q: want %d fields and a path", entry, pathIndex)
			}
			file := FileStatus{
				Path:           parts[pathIndex],
				IndexStatus:    shortStatusCode(parts[0][0]),
				WorktreeStatus: shortStatusCode(parts[0][1]),
			}
			if kind == "2" {
				i++
				if i == len(entries) {
					return Status{}, fmt.Errorf("parse git status entry %q: rename has no original path", entry)
				}
				file.OriginalPath = string(entries[i])
			}
			status.Files = append(status.Files, file)
		case "?":
			if rest == "" {
				return Status{}, fmt.Errorf("parse git status entry %q: untracked entry has no path", entry)
			}
			status.Files = append(status.Files, FileStatus{Path: rest, IndexStatus: '?', WorktreeStatus: '?'})
		default:
			return Status{}, fmt.Errorf("parse git status entry %q: unknown entry kind %q", entry, kind)
		}
	}
	// git omits the branch.ab header when the upstream ref is missing.
	status.UpstreamGone = status.Upstream != "" && !hasAheadBehind
	return status, nil
}

func shortStatusCode(code byte) byte {
	if code == '.' {
		return ' '
	}
	return code
}
