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
	Old Line
	New Line
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
			if row.Old.Kind == LineRemoved {
				counts.Removed++
			}
			if row.New.Kind == LineAdded {
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

// git splits one file into a deletion and an addition when a rename or type change leaves too little in common.
func ParseFile(text string) (FileDiff, error) {
	sections, err := ParseFiles(text)
	if err != nil {
		return FileDiff{}, err
	}
	if len(sections) == 1 {
		return sections[0].FileDiff, nil
	}
	joined := FileDiff{Hunks: []Hunk{}, Change: FileModified}
	for _, section := range sections {
		joined.Hunks = append(joined.Hunks, section.FileDiff.Hunks...)
		joined.Binary = joined.Binary || section.FileDiff.Binary
	}
	return joined, nil
}

func parseFileSection(lines []string) (FileSection, error) {
	fileDiff, err := parseUnified(strings.Join(lines, "\n"))
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
	// Both names are quoted alike unless the file was renamed, and a rename has its own rename to line.
	paths := strings.TrimPrefix(lines[0], fileHeaderPrefix)
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
	separatorIndex := (len(paths) - 1) / 2
	if len(paths)%2 == 1 && strings.HasPrefix(paths, "a/") && paths[separatorIndex:separatorIndex+3] == " b/" {
		return paths[2:separatorIndex], nil
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

func parseUnified(text string) (FileDiff, error) {
	parser := unifiedParser{}
	for index, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSuffix(rawLine, "\r")
		if strings.HasPrefix(line, "@@") {
			if parser.inHunk() {
				parser.flush()
			}
			oldStart, newStart, err := parseHunkStarts(line)
			if err != nil {
				return FileDiff{}, fmt.Errorf("parse diff line %d: %w", index+1, err)
			}
			parser.oldNumber = oldStart
			parser.newNumber = newStart
			parser.fileDiff.Hunks = append(parser.fileDiff.Hunks, Hunk{Header: line})
			continue
		}
		if !parser.inHunk() {
			switch {
			case strings.HasPrefix(line, "Binary files "):
				parser.fileDiff.Binary = true
			case strings.HasPrefix(line, "new file mode "):
				parser.fileDiff.Change = FileAdded
			case strings.HasPrefix(line, "deleted file mode "):
				parser.fileDiff.Change = FileDeleted
			case strings.HasPrefix(line, "rename from "):
				parser.fileDiff.Change = FileRenamed
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, `\`) {
			continue
		}
		switch line[0] {
		case ' ':
			parser.flush()
			parser.appendRows(Row{
				Old: Line{Kind: LineContext, Number: parser.oldNumber, Text: line[1:]},
				New: Line{Kind: LineContext, Number: parser.newNumber, Text: line[1:]},
			})
			parser.oldNumber++
			parser.newNumber++
		case '-':
			if len(parser.pending.added) > 0 {
				parser.flush()
			}
			parser.pending.removed = append(parser.pending.removed,
				Line{Kind: LineRemoved, Number: parser.oldNumber, Text: line[1:]})
			parser.oldNumber++
		case '+':
			parser.pending.added = append(parser.pending.added, Line{Kind: LineAdded, Number: parser.newNumber, Text: line[1:]})
			parser.newNumber++
		default:
			return FileDiff{}, fmt.Errorf("parse diff line %d: unexpected %q inside a hunk", index+1, line)
		}
	}
	if parser.inHunk() {
		parser.flush()
	}
	return parser.fileDiff, nil
}

// pending holds the removals and additions not yet paired into rows of the last hunk.
type unifiedParser struct {
	fileDiff  FileDiff
	pending   changeBlock
	oldNumber int
	newNumber int
}

func (p *unifiedParser) inHunk() bool {
	return len(p.fileDiff.Hunks) > 0
}

func (p *unifiedParser) flush() {
	p.appendRows(p.pending.pairRows()...)
	p.pending = changeBlock{}
}

func (p *unifiedParser) appendRows(rows ...Row) {
	last := len(p.fileDiff.Hunks) - 1
	p.fileDiff.Hunks[last].Rows = append(p.fileDiff.Hunks[last].Rows, rows...)
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
		removed := lineOrBlank(b.removed, i)
		added := lineOrBlank(b.added, i)
		if len(b.removed) == len(b.added) {
			prefix := 0
			for prefix < len(removed.Text) && prefix < len(added.Text) && removed.Text[prefix] == added.Text[prefix] {
				prefix++
			}
			// Backs off to a rune start, so a span never splits a character.
			for prefix > 0 && (splitsRune(removed.Text, prefix) || splitsRune(added.Text, prefix)) {
				prefix--
			}
			oldRest := removed.Text[prefix:]
			newRest := added.Text[prefix:]
			suffix := 0
			for suffix < len(oldRest) && suffix < len(newRest) &&
				oldRest[len(oldRest)-1-suffix] == newRest[len(newRest)-1-suffix] {
				suffix++
			}
			for suffix > 0 && splitsRune(oldRest, len(oldRest)-suffix) {
				suffix--
			}
			if prefix+suffix > 0 {
				rows = append(rows, Row{
					Old: lineWithChanged(removed, Span{Start: prefix, End: len(removed.Text) - suffix}),
					New: lineWithChanged(added, Span{Start: prefix, End: len(added.Text) - suffix}),
				})
				continue
			}
		}
		rows = append(rows, Row{Old: removed, New: added})
	}
	return rows
}

func lineWithChanged(line Line, changed Span) Line {
	marked := line
	marked.Changed = changed
	return marked
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
