package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
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
	entries := bytes.Split(bytes.TrimSuffix(output, []byte{0}), []byte{0})
	for i := 0; i < len(entries); i++ {
		entry := string(entries[i])
		kind, rest, _ := strings.Cut(entry, " ")
		switch kind {
		case "#":
			withHeader, err := statusWithHeader(status, rest)
			if err != nil {
				return Status{}, fmt.Errorf("parse git status entry %q: %w", entry, err)
			}
			status = withHeader
		case "1", "2", "u":
			file, err := parseChangedEntry(kind, rest)
			if err != nil {
				return Status{}, fmt.Errorf("parse git status entry %q: %w", entry, err)
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
	status.UpstreamGone = status.Upstream != "" && !hasAheadBehind(entries)
	return status, nil
}

// git omits the branch.ab header when the upstream ref is missing.
func hasAheadBehind(entries [][]byte) bool {
	return slices.ContainsFunc(entries, func(entry []byte) bool { return bytes.HasPrefix(entry, []byte("# branch.ab ")) })
}

func statusWithHeader(status Status, header string) (Status, error) {
	name, value, _ := strings.Cut(header, " ")
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
		ahead, behind, err := parseAheadBehind(value)
		if err != nil {
			return Status{}, err
		}
		status.Ahead = ahead
		status.Behind = behind
	}
	return status, nil
}

func parseAheadBehind(value string) (int, int, error) {
	aheadText, behindText, found := strings.Cut(value, " ")
	if !found || !strings.HasPrefix(aheadText, "+") || !strings.HasPrefix(behindText, "-") {
		return 0, 0, fmt.Errorf("malformed ahead and behind counts %q", value)
	}
	ahead, err := strconv.Atoi(aheadText[1:])
	if err != nil {
		return 0, 0, fmt.Errorf("parse ahead count %q: %w", value, err)
	}
	behind, err := strconv.Atoi(behindText[1:])
	if err != nil {
		return 0, 0, fmt.Errorf("parse behind count %q: %w", value, err)
	}
	return ahead, behind, nil
}

// Counts XY and every field before the path, per porcelain v2 entry kind.
var fieldsBeforePath = map[string]int{"1": 7, "2": 8, "u": 9}

// The path is the rest of the entry, so it can hold spaces.
func parseChangedEntry(kind string, rest string) (FileStatus, error) {
	pathIndex := fieldsBeforePath[kind]
	parts := strings.SplitN(rest, " ", pathIndex+1)
	if len(parts) != pathIndex+1 || len(parts[0]) != 2 || parts[pathIndex] == "" {
		return FileStatus{}, fmt.Errorf("want %d fields and a path", pathIndex)
	}
	return FileStatus{
		Path:           parts[pathIndex],
		IndexStatus:    shortStatusCode(parts[0][0]),
		WorktreeStatus: shortStatusCode(parts[0][1]),
	}, nil
}

func shortStatusCode(code byte) byte {
	if code == '.' {
		return ' '
	}
	return code
}
