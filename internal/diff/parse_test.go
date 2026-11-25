package diff

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseUnified(t *testing.T) {
	t.Run("pairsRemovedWithAddedAndPadsTheShorterSide", pairsRemovedWithAddedAndPadsTheShorterSide)
	t.Run("numbersLinesFromEachHunkStart", numbersLinesFromEachHunkStart)
	t.Run("startsANewPairingWhenRemovalsFollowAdditions", startsANewPairingWhenRemovalsFollowAdditions)
	t.Run("rejectsUnknownLinePrefix", rejectsUnknownLinePrefix)
}

func TestParseFiles(t *testing.T) {
	t.Run("namesEachFileOfACommitAndWhatHappenedToIt", namesEachFileOfACommitAndWhatHappenedToIt)
	t.Run("readsPathsGitQuotedOrEndedWithATab", readsPathsGitQuotedOrEndedWithATab)
}

func TestParseFile(t *testing.T) {
	text := strings.Join([]string{
		"diff --git a/a.txt b/a.txt",
		"deleted file mode 100644",
		"--- a/a.txt",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-old",
		"diff --git a/b.txt b/b.txt",
		"new file mode 100644",
		"--- /dev/null",
		"+++ b/b.txt",
		"@@ -0,0 +1 @@",
		"+new",
		"",
	}, "\n")

	fileDiff, err := ParseFile(text)
	require.NoError(t, err)

	require.Equal(t, FileModified, fileDiff.Change)
	require.Len(t, fileDiff.Hunks, 2)
	require.Equal(t, "old", fileDiff.Hunks[0].Rows[0].Old.Text)
	require.Equal(t, "new", fileDiff.Hunks[1].Rows[0].New.Text)
}

func readsPathsGitQuotedOrEndedWithATab(t *testing.T) {
	text := strings.Join([]string{
		"diff --git a/old name.txt b/old name.txt",
		"--- a/old name.txt\t",
		"+++ b/old name.txt\t",
		"@@ -1 +1 @@",
		"-a",
		"+b",
		"diff --git a/gone file.go b/gone file.go",
		"deleted file mode 100644",
		"--- a/gone file.go\t",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-package y",
		`diff --git "a/caf\303\251.txt" "b/caf\303\251.txt"`,
		`--- "a/caf\303\251.txt"`,
		`+++ "b/caf\303\251.txt"`,
		"@@ -1 +1 @@",
		"-a",
		"+b",
		`diff --git "a/caf\303\251.txt" "b/th\303\251 \"x\".txt"`,
		"similarity index 100%",
		`rename from "caf\303\251.txt"`,
		`rename to "th\303\251 \"x\".txt"`,
		`diff --git "a/\303\251 b.png" "b/\303\251 b.png"`,
		`Binary files "a/\303\251 b.png" and "b/\303\251 b.png" differ`,
	}, "\n")

	sections, err := ParseFiles(text)
	require.NoError(t, err)

	paths := []string{}
	for _, section := range sections {
		paths = append(paths, section.Path)
	}
	wantPaths := []string{"old name.txt", "gone file.go", "café.txt", `thé "x".txt`, "é b.png"}
	require.Equal(t, wantPaths, paths)
}

func namesEachFileOfACommitAndWhatHappenedToIt(t *testing.T) {
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
	require.NoError(t, err)

	paths := []string{}
	changes := []FileChange{}
	for _, section := range sections {
		paths = append(paths, section.Path)
		changes = append(changes, section.FileDiff.Change)
	}
	wantPaths := []string{"new.go", "gone.go", "new name.go", "logo b.png"}
	wantChanges := []FileChange{FileAdded, FileDeleted, FileRenamed, FileModified}
	require.Equal(t, wantPaths, paths)
	require.Equal(t, wantChanges, changes)
	require.True(t, sections[3].FileDiff.Binary)
	require.Len(t, sections[0].FileDiff.Hunks, 1)
}

func TestChangedSpan(t *testing.T) {
	t.Run("marksWhatLiesBetweenTheCommonPrefixAndSuffix", marksWhatLiesBetweenTheCommonPrefixAndSuffix)
	t.Run("neverSplitsACharacter", neverSplitsACharacter)
	t.Run("marksNothingInAnUnevenBlockOrBetweenUnrelatedLines", marksNothingInAnUnevenBlockOrBetweenUnrelatedLines)
}

func marksWhatLiesBetweenTheCommonPrefixAndSuffix(t *testing.T) {
	row := onlyRow(t, "@@ -1 +1 @@\n-\tcall(a, b)\n+\tcall(a, bc)\n")

	require.Equal(t, Span{Start: 10, End: 10}, row.Old.Changed)
	require.Equal(t, Span{Start: 10, End: 11}, row.New.Changed)
}

func neverSplitsACharacter(t *testing.T) {
	row := onlyRow(t, "@@ -1 +1 @@\n-café\n+cafè\n")

	require.Equal(t, Span{Start: 3, End: 5}, row.Old.Changed)
	require.Equal(t, Span{Start: 3, End: 5}, row.New.Changed)
}

func marksNothingInAnUnevenBlockOrBetweenUnrelatedLines(t *testing.T) {
	uneven, err := parseUnified("@@ -1,2 +1 @@\n-same a\n-same b\n+same c\n")
	require.NoError(t, err)
	unrelated := onlyRow(t, "@@ -1 +1 @@\n-abc\n+xyz\n")

	rows := append(uneven.Hunks[0].Rows, unrelated)

	for _, row := range rows {
		require.Zero(t, row.Old.Changed)
		require.Zero(t, row.New.Changed)
	}
}

func onlyRow(t *testing.T, text string) Row {
	t.Helper()
	fileDiff, err := parseUnified(text)
	require.NoError(t, err)
	return fileDiff.Hunks[0].Rows[0]
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

	fileDiff, err := parseUnified(text)
	require.NoError(t, err)

	want := FileDiff{Hunks: []Hunk{{
		Header: "@@ -10,4 +10,3 @@ func f() {",
		Rows: []Row{
			{Old: Line{Kind: LineContext, Number: 10, Text: "keep"}, New: Line{Kind: LineContext, Number: 10, Text: "keep"}},
			{Old: Line{Kind: LineRemoved, Number: 11, Text: "old one"}, New: Line{Kind: LineAdded, Number: 11, Text: "new one"}},
			{Old: Line{Kind: LineRemoved, Number: 12, Text: "old two"}, New: Line{Kind: LineAdded, Number: 12, Text: "new two"}},
			{Old: Line{Kind: LineRemoved, Number: 13, Text: "old three"}, New: Line{Kind: LineBlank}},
		},
	}}}
	require.Equal(t, want, fileDiff)
}

func numbersLinesFromEachHunkStart(t *testing.T) {
	text := "@@ -1 +1,2 @@\n a\n+b\n@@ -40,2 +41,1 @@\n-c\n d\n"

	fileDiff, err := parseUnified(text)
	require.NoError(t, err)

	second := fileDiff.Hunks[1].Rows
	want := []Row{
		{Old: Line{Kind: LineRemoved, Number: 40, Text: "c"}, New: Line{Kind: LineBlank}},
		{Old: Line{Kind: LineContext, Number: 41, Text: "d"}, New: Line{Kind: LineContext, Number: 41, Text: "d"}},
	}
	require.Equal(t, want, second)
}

func startsANewPairingWhenRemovalsFollowAdditions(t *testing.T) {
	text := "@@ -1,2 +1,2 @@\n-a\n+b\n-c\n+d\n"

	fileDiff, err := parseUnified(text)
	require.NoError(t, err)

	want := []Row{
		{Old: Line{Kind: LineRemoved, Number: 1, Text: "a"}, New: Line{Kind: LineAdded, Number: 1, Text: "b"}},
		{Old: Line{Kind: LineRemoved, Number: 2, Text: "c"}, New: Line{Kind: LineAdded, Number: 2, Text: "d"}},
	}
	require.Equal(t, want, fileDiff.Hunks[0].Rows)
}

func rejectsUnknownLinePrefix(t *testing.T) {
	_, err := parseUnified("@@ -1 +1 @@\n*a\n")

	require.ErrorContains(t, err, "line 2")
}
