package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type LineKind int

const (
	// LineBlank fills the side of a row that has no line, opposite an addition or removal.
	LineBlank LineKind = iota
	LineContext
	LineRemoved
	LineAdded
)

// Changed is the part of Text that differs from the line paired with it, empty when none is marked. Tokens are
// sorted and cover only the colored parts of Text.
type Line struct {
	Kind    LineKind
	Number  int
	Text    string
	Changed Span
	Tokens  []Token
}

// Color is a #rrggbb foreground.
type Token struct {
	Span  Span
	Color string
}

// Span is a byte range of a line's Text, End excluded.
type Span struct {
	Start int
	End   int
}

type Row struct {
	Left  Line
	Right Line
}

type Hunk struct {
	Header string
	Rows   []Row
}

type FileChange int

const (
	FileModified FileChange = iota
	FileAdded
	FileDeleted
	FileRenamed
)

type FileDiff struct {
	Hunks  []Hunk
	Binary bool
	Change FileChange
}

type FileSection struct {
	Path     string
	FileDiff FileDiff
}

type LineCounts struct {
	Added   int
	Removed int
}

func CountLines(fileDiff FileDiff) LineCounts {
	counts := LineCounts{}
	for _, hunk := range fileDiff.Hunks {
		for _, row := range hunk.Rows {
			if row.Left.Kind == LineRemoved {
				counts.Removed++
			}
			if row.Right.Kind == LineAdded {
				counts.Added++
			}
		}
	}
	return counts
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

const fileHeaderPrefix = "diff --git "

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
	path, err := sectionPath(lines)
	if err != nil {
		return FileSection{}, err
	}
	return FileSection{Path: path, FileDiff: fileDiff}, nil
}

// A binary file has no ---/+++ lines, so its path comes from the diff --git header.
func sectionPath(lines []string) (string, error) {
	oldPath := ""
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			break
		}
		switch {
		case strings.HasPrefix(line, "rename to "):
			return unquotePath(strings.TrimPrefix(line, "rename to "))
		case strings.HasPrefix(line, "+++ "):
			path, err := unquotePath(strings.TrimPrefix(line, "+++ "))
			if err != nil {
				return "", err
			}
			if strings.HasPrefix(path, "b/") {
				return strings.TrimPrefix(path, "b/"), nil
			}
		case strings.HasPrefix(line, "--- "):
			path, err := unquotePath(strings.TrimPrefix(line, "--- "))
			if err != nil {
				return "", err
			}
			if strings.HasPrefix(path, "a/") {
				oldPath = strings.TrimPrefix(path, "a/")
			}
		}
	}
	if oldPath != "" {
		return oldPath, nil
	}
	return headerPath(strings.TrimPrefix(lines[0], fileHeaderPrefix))
}

// Both names are quoted alike unless the file was renamed, and a rename has its own rename to line.
func headerPath(paths string) (string, error) {
	if strings.HasPrefix(paths, `"`) {
		quoted, err := strconv.QuotedPrefix(paths)
		if err != nil {
			return "", fmt.Errorf("find the quoted path in %q: %w", paths, err)
		}
		path, err := unquotePath(quoted)
		if err != nil {
			return "", err
		}
		return strings.TrimPrefix(path, "a/"), nil
	}
	half := (len(paths) - 1) / 2
	if len(paths)%2 == 1 && strings.HasPrefix(paths, "a/") && paths[half:half+3] == " b/" {
		return paths[2:half], nil
	}
	return paths, nil
}

// git ends a path holding a space with a tab and C-quotes special bytes (core.quotePath) in escapes Go also reads.
func unquotePath(field string) (string, error) {
	path := strings.TrimSuffix(field, "\t")
	if !strings.HasPrefix(path, `"`) {
		return path, nil
	}
	unquoted, err := strconv.Unquote(path)
	if err != nil {
		return "", fmt.Errorf("unquote path %s: %w", path, err)
	}
	return unquoted, nil
}

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

type changeBlock struct {
	removed []Line
	added   []Line
}

// Only a block with as many removals as additions has its changes marked, since otherwise the pairs are a guess.
func (b changeBlock) pairRows() []Row {
	rows := []Row{}
	for i := 0; i < max(len(b.removed), len(b.added)); i++ {
		left := lineOrBlank(b.removed, i)
		right := lineOrBlank(b.added, i)
		if len(b.removed) == len(b.added) {
			rows = append(rows, markedRow(left, right))
		} else {
			rows = append(rows, Row{Left: left, Right: right})
		}
	}
	return rows
}

// Marks what lies between the lines' common prefix and suffix, and nothing when they share neither.
func markedRow(removed Line, added Line) Row {
	prefix := commonPrefixLength(removed.Text, added.Text)
	suffix := commonSuffixLength(removed.Text[prefix:], added.Text[prefix:])
	if prefix+suffix == 0 {
		return Row{Left: removed, Right: added}
	}
	return Row{
		Left:  withChanged(removed, Span{Start: prefix, End: len(removed.Text) - suffix}),
		Right: withChanged(added, Span{Start: prefix, End: len(added.Text) - suffix}),
	}
}

func withChanged(line Line, changed Span) Line {
	return Line{Kind: line.Kind, Number: line.Number, Text: line.Text, Changed: changed}
}

// Backs off to a rune start, so a span never splits a character.
func commonPrefixLength(first string, second string) int {
	length := 0
	for length < len(first) && length < len(second) && first[length] == second[length] {
		length++
	}
	for length > 0 && (splitsRune(first, length) || splitsRune(second, length)) {
		length--
	}
	return length
}

func commonSuffixLength(first string, second string) int {
	length := 0
	for length < len(first) && length < len(second) && first[len(first)-1-length] == second[len(second)-1-length] {
		length++
	}
	for length > 0 && splitsRune(first, len(first)-length) {
		length--
	}
	return length
}

func splitsRune(text string, index int) bool {
	return index < len(text) && !utf8.RuneStart(text[index])
}

func lineOrBlank(lines []Line, index int) Line {
	if index < len(lines) {
		return lines[index]
	}
	return Line{Kind: LineBlank}
}
