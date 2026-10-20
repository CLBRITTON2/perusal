package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type LineKind int

const (
	// LineBlank fills the side of a row that has no line, opposite an addition or removal.
	LineBlank LineKind = iota
	LineContext
	LineRemoved
	LineAdded
)

type Line struct {
	Kind   LineKind
	Number int
	Text   string
}

// Row is one side-by-side line: Left from the old file, Right from the new one.
type Row struct {
	Left  Line
	Right Line
}

type Hunk struct {
	Header string
	Rows   []Row
}

// FileChange is what happened to a file as a whole, read from the extended header lines above its first hunk.
type FileChange int

const (
	FileModified FileChange = iota
	FileAdded
	FileDeleted
	FileRenamed
)

type FileDiff struct {
	Hunks []Hunk
	// Binary is set when git reports the file as binary, which has no hunks.
	Binary bool
	Change FileChange
}

// FileSection is one file of a diff that spans several, such as git show prints for a commit.
type FileSection struct {
	Path     string
	FileDiff FileDiff
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

const fileHeaderPrefix = "diff --git "

// ParseFiles splits a diff of several files at each "diff --git" line and parses every file.
func ParseFiles(text string) ([]FileSection, error) {
	sections := []FileSection{}
	lines := strings.Split(text, "\n")
	start := -1
	for index := 0; index <= len(lines); index++ {
		if index < len(lines) && !strings.HasPrefix(lines[index], fileHeaderPrefix) {
			continue
		}
		if start >= 0 {
			section, err := parseFileSection(lines[start:index])
			if err != nil {
				return nil, fmt.Errorf("parse diff of the file starting at line %d: %w", start+1, err)
			}
			sections = append(sections, section)
		}
		start = index
	}
	return sections, nil
}

func parseFileSection(lines []string) (FileSection, error) {
	fileDiff, err := ParseUnified(strings.Join(lines, "\n"))
	if err != nil {
		return FileSection{}, err
	}
	return FileSection{Path: sectionPath(lines), FileDiff: fileDiff}, nil
}

// sectionPath finds the new path of a file section, or the old one for a deletion. A binary file has no ---/+++
// lines, so its path comes from the "diff --git a/<path> b/<path>" header, which is ambiguous only for a rename.
func sectionPath(lines []string) string {
	oldPath := ""
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			break
		}
		switch {
		case strings.HasPrefix(line, "rename to "):
			return strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "+++ b/"):
			return strings.TrimPrefix(line, "+++ b/")
		case strings.HasPrefix(line, "--- a/"):
			oldPath = strings.TrimPrefix(line, "--- a/")
		}
	}
	if oldPath != "" {
		return oldPath
	}
	return headerPath(strings.TrimPrefix(lines[0], fileHeaderPrefix))
}

// headerPath reads "a/<path> b/<path>", where both halves name the same path.
func headerPath(paths string) string {
	half := (len(paths) - 1) / 2
	if len(paths)%2 == 1 && strings.HasPrefix(paths, "a/") && paths[half:half+3] == " b/" {
		return paths[2:half]
	}
	return paths
}

// ParseUnified pairs the lines of a git unified diff for side-by-side display. Within a change, the nth removed line
// sits beside the nth added line, and the longer side's extra lines sit beside blanks.
func ParseUnified(text string) (FileDiff, error) {
	fileDiff := FileDiff{}
	var hunk *Hunk
	pending := changeBlock{}
	oldNumber := 0
	newNumber := 0
	for index, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSuffix(rawLine, "\r")
		if strings.HasPrefix(line, "@@") {
			if hunk != nil {
				hunk.Rows = append(hunk.Rows, pending.pairRows()...)
				pending = changeBlock{}
			}
			oldStart, newStart, err := parseHunkStarts(line)
			if err != nil {
				return FileDiff{}, fmt.Errorf("parse diff line %d: %w", index+1, err)
			}
			oldNumber = oldStart
			newNumber = newStart
			fileDiff.Hunks = append(fileDiff.Hunks, Hunk{Header: line})
			hunk = &fileDiff.Hunks[len(fileDiff.Hunks)-1]
			continue
		}
		if hunk == nil {
			switch {
			case strings.HasPrefix(line, "Binary files "):
				fileDiff.Binary = true
			case strings.HasPrefix(line, "new file mode "):
				fileDiff.Change = FileAdded
			case strings.HasPrefix(line, "deleted file mode "):
				fileDiff.Change = FileDeleted
			case strings.HasPrefix(line, "rename from "):
				fileDiff.Change = FileRenamed
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, `\`) {
			continue
		}
		switch line[0] {
		case ' ':
			hunk.Rows = append(hunk.Rows, pending.pairRows()...)
			pending = changeBlock{}
			hunk.Rows = append(hunk.Rows, Row{
				Left:  Line{Kind: LineContext, Number: oldNumber, Text: line[1:]},
				Right: Line{Kind: LineContext, Number: newNumber, Text: line[1:]},
			})
			oldNumber++
			newNumber++
		case '-':
			if len(pending.added) > 0 {
				hunk.Rows = append(hunk.Rows, pending.pairRows()...)
				pending = changeBlock{}
			}
			pending.removed = append(pending.removed, Line{Kind: LineRemoved, Number: oldNumber, Text: line[1:]})
			oldNumber++
		case '+':
			pending.added = append(pending.added, Line{Kind: LineAdded, Number: newNumber, Text: line[1:]})
			newNumber++
		default:
			return FileDiff{}, fmt.Errorf("parse diff line %d: unexpected %q inside a hunk", index+1, line)
		}
	}
	if hunk != nil {
		hunk.Rows = append(hunk.Rows, pending.pairRows()...)
	}
	return fileDiff, nil
}

// parseHunkStarts returns the first old and new line numbers of a header such as "@@ -134,7 +134,6 @@ func".
func parseHunkStarts(header string) (int, int, error) {
	match := hunkHeader.FindStringSubmatch(header)
	if match == nil {
		return 0, 0, fmt.Errorf("malformed hunk header %q", header)
	}
	oldStart, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse hunk header %q: %w", header, err)
	}
	newStart, err := strconv.Atoi(match[2])
	if err != nil {
		return 0, 0, fmt.Errorf("parse hunk header %q: %w", header, err)
	}
	return oldStart, newStart, nil
}

// changeBlock holds a run of removed lines and the added lines that replace them.
type changeBlock struct {
	removed []Line
	added   []Line
}

func (b changeBlock) pairRows() []Row {
	rows := []Row{}
	for i := 0; i < max(len(b.removed), len(b.added)); i++ {
		rows = append(rows, Row{Left: lineOrBlank(b.removed, i), Right: lineOrBlank(b.added, i)})
	}
	return rows
}

func lineOrBlank(lines []Line, index int) Line {
	if index < len(lines) {
		return lines[index]
	}
	return Line{Kind: LineBlank}
}
