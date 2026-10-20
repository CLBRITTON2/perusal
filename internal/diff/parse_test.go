package diff

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseUnified(t *testing.T) {
	t.Run("pairsRemovedWithAddedAndPadsTheShorterSide", pairsRemovedWithAddedAndPadsTheShorterSide)
	t.Run("numbersLinesFromEachHunkStart", numbersLinesFromEachHunkStart)
	t.Run("startsANewPairingWhenRemovalsFollowAdditions", startsANewPairingWhenRemovalsFollowAdditions)
	t.Run("marksBinaryFilesWithNoHunks", marksBinaryFilesWithNoHunks)
	t.Run("rejectsMalformedHunkHeader", rejectsMalformedHunkHeader)
	t.Run("rejectsUnknownLinePrefix", rejectsUnknownLinePrefix)
}

func TestParseFiles(t *testing.T) {
	t.Run("namesEachFileOfACommit", namesEachFileOfACommit)
}

func namesEachFileOfACommit(t *testing.T) {
	text := strings.Join([]string{
		"",
		"diff --git a/new.go b/new.go",
		"new file mode 100644",
		"--- /dev/null",
		"+++ b/new.go",
		"@@ -0,0 +1 @@",
		"+package x",
		"diff --git a/gone.go b/gone.go",
		"deleted file mode 100644",
		"--- a/gone.go",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-package y",
		"diff --git a/old name.go b/new name.go",
		"similarity index 100%",
		"rename from old name.go",
		"rename to new name.go",
		"diff --git a/logo b.png b/logo b.png",
		"Binary files a/logo b.png and b/logo b.png differ",
		"",
	}, "\n")

	sections, err := ParseFiles(text)
	if err != nil {
		t.Fatal(err)
	}

	paths := []string{}
	for _, section := range sections {
		paths = append(paths, section.Path)
	}
	want := []string{"new.go", "gone.go", "new name.go", "logo b.png"}
	if !reflect.DeepEqual(paths, want) || !sections[3].FileDiff.Binary || len(sections[0].FileDiff.Hunks) != 1 {
		t.Fatalf("got paths %q, binary %v, %d hunks in the first", paths, sections[3].FileDiff.Binary,
			len(sections[0].FileDiff.Hunks))
	}
}

func pairsRemovedWithAddedAndPadsTheShorterSide(t *testing.T) {
	text := strings.Join([]string{
		"diff --git a/f.go b/f.go",
		"--- a/f.go",
		"+++ b/f.go",
		"@@ -10,4 +10,3 @@ func f() {",
		" keep",
		"-old one",
		"-old two",
		"-old three",
		"+new one",
		"+new two",
		"\\ No newline at end of file",
		"",
	}, "\n")

	fileDiff, err := ParseUnified(text)
	if err != nil {
		t.Fatal(err)
	}

	want := FileDiff{Hunks: []Hunk{{
		Header: "@@ -10,4 +10,3 @@ func f() {",
		Rows: []Row{
			{Left: Line{Kind: LineContext, Number: 10, Text: "keep"}, Right: Line{Kind: LineContext, Number: 10, Text: "keep"}},
			{Left: Line{Kind: LineRemoved, Number: 11, Text: "old one"}, Right: Line{Kind: LineAdded, Number: 11, Text: "new one"}},
			{Left: Line{Kind: LineRemoved, Number: 12, Text: "old two"}, Right: Line{Kind: LineAdded, Number: 12, Text: "new two"}},
			{Left: Line{Kind: LineRemoved, Number: 13, Text: "old three"}, Right: Line{Kind: LineBlank}},
		},
	}}}
	if !reflect.DeepEqual(fileDiff, want) {
		t.Fatalf("diff:\n%+v\nwant:\n%+v", fileDiff, want)
	}
}

func numbersLinesFromEachHunkStart(t *testing.T) {
	text := "@@ -1 +1,2 @@\n a\n+b\n@@ -40,2 +41,1 @@\n-c\n d\n"

	fileDiff, err := ParseUnified(text)
	if err != nil {
		t.Fatal(err)
	}

	second := fileDiff.Hunks[1].Rows
	want := []Row{
		{Left: Line{Kind: LineRemoved, Number: 40, Text: "c"}, Right: Line{Kind: LineBlank}},
		{Left: Line{Kind: LineContext, Number: 41, Text: "d"}, Right: Line{Kind: LineContext, Number: 41, Text: "d"}},
	}
	if !reflect.DeepEqual(second, want) {
		t.Fatalf("second hunk rows:\n%+v\nwant:\n%+v", second, want)
	}
}

func startsANewPairingWhenRemovalsFollowAdditions(t *testing.T) {
	text := "@@ -1,2 +1,2 @@\n-a\n+b\n-c\n+d\n"

	fileDiff, err := ParseUnified(text)
	if err != nil {
		t.Fatal(err)
	}

	want := []Row{
		{Left: Line{Kind: LineRemoved, Number: 1, Text: "a"}, Right: Line{Kind: LineAdded, Number: 1, Text: "b"}},
		{Left: Line{Kind: LineRemoved, Number: 2, Text: "c"}, Right: Line{Kind: LineAdded, Number: 2, Text: "d"}},
	}
	if !reflect.DeepEqual(fileDiff.Hunks[0].Rows, want) {
		t.Fatalf("rows:\n%+v\nwant:\n%+v", fileDiff.Hunks[0].Rows, want)
	}
}

func marksBinaryFilesWithNoHunks(t *testing.T) {
	text := "diff --git a/i.png b/i.png\nindex 1..2 100644\nBinary files a/i.png and b/i.png differ\n"

	fileDiff, err := ParseUnified(text)
	if err != nil {
		t.Fatal(err)
	}

	if !fileDiff.Binary || len(fileDiff.Hunks) != 0 {
		t.Fatalf("got %+v, want binary with no hunks", fileDiff)
	}
}

func rejectsMalformedHunkHeader(t *testing.T) {
	_, err := ParseUnified("@@ -x +1 @@\n a\n")

	if err == nil || !strings.Contains(err.Error(), "malformed hunk header") {
		t.Fatalf("got error %v, want a malformed hunk header error", err)
	}
}

func rejectsUnknownLinePrefix(t *testing.T) {
	_, err := ParseUnified("@@ -1 +1 @@\n*a\n")

	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("got error %v, want an error naming line 2", err)
	}
}
