package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// FileStatus is one changed file. IndexStatus and WorktreeStatus are the X and Y codes of git status --short, with
// ' ' for unchanged.
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

type Status struct {
	// Branch is empty when HEAD is detached.
	Branch string
	// Upstream is empty when the branch tracks nothing, and then Ahead and Behind are 0.
	Upstream string
	Ahead    int
	Behind   int
	// HasCommits is false on an unborn branch, before the first commit.
	HasCommits bool
	Files      []FileStatus
}

// ParseStatus reads the NUL-separated output of git status --porcelain=v2 --branch -z.
func ParseStatus(output []byte) (Status, error) {
	if len(output) == 0 {
		return Status{}, errors.New("parse git status: no output, want at least the branch headers")
	}
	status := Status{Files: []FileStatus{}}
	fields := bytes.Split(bytes.TrimSuffix(output, []byte{0}), []byte{0})
	for i := 0; i < len(fields); i++ {
		entry := string(fields[i])
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
				if i == len(fields) {
					return Status{}, fmt.Errorf("parse git status entry %q: rename has no original path", entry)
				}
				file.OriginalPath = string(fields[i])
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
	return status, nil
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

// parseAheadBehind reads a branch.ab value such as "+2 -0".
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

// fieldsBeforePath counts the space-separated fields, XY first, that precede the path in each kind of changed entry.
var fieldsBeforePath = map[string]int{"1": 7, "2": 8, "u": 9}

// parseChangedEntry reads an ordinary, renamed or unmerged entry. The path is the rest of the entry, so it can hold
// spaces.
func parseChangedEntry(kind string, rest string) (FileStatus, error) {
	count := fieldsBeforePath[kind]
	parts := strings.SplitN(rest, " ", count+1)
	if len(parts) != count+1 || len(parts[0]) != 2 || parts[count] == "" {
		return FileStatus{}, fmt.Errorf("want %d fields and a path", count)
	}
	return FileStatus{
		Path:           parts[count],
		IndexStatus:    shortStatusCode(parts[0][0]),
		WorktreeStatus: shortStatusCode(parts[0][1]),
	}, nil
}

// shortStatusCode maps porcelain v2's '.' for unchanged to the ' ' of git status --short.
func shortStatusCode(code byte) byte {
	if code == '.' {
		return ' '
	}
	return code
}
