package gitrepo

import (
	"reflect"
	"testing"
)

func TestParseStatus(t *testing.T) {
	t.Run("readsRenameOriginalPathFromTheNextField", readsRenameOriginalPathFromTheNextField)
	t.Run("returnsNoFilesForACleanTree", returnsNoFilesForACleanTree)
	t.Run("rejectsAnEntryWithNoPath", rejectsAnEntryWithNoPath)
	t.Run("rejectsARenameWithNoOriginalPath", rejectsARenameWithNoOriginalPath)
}

func TestFileStatus(t *testing.T) {
	t.Run("partlyStagedFileHasBothKindsOfChanges", partlyStagedFileHasBothKindsOfChanges)
	t.Run("untrackedFileIsUnstagedOnly", untrackedFileIsUnstagedOnly)
}

func readsRenameOriginalPathFromTheNextField(t *testing.T) {
	output := []byte(" M a.go\x00R  new name.go\x00old name.go\x00?? dir/new.txt\x00")

	files, err := ParseStatus(output)
	if err != nil {
		t.Fatal(err)
	}

	want := []FileStatus{
		{Path: "a.go", IndexStatus: ' ', WorktreeStatus: 'M'},
		{Path: "new name.go", OriginalPath: "old name.go", IndexStatus: 'R', WorktreeStatus: ' '},
		{Path: "dir/new.txt", IndexStatus: '?', WorktreeStatus: '?'},
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files:\n%+v\nwant:\n%+v", files, want)
	}
}

func returnsNoFilesForACleanTree(t *testing.T) {
	files, err := ParseStatus([]byte{})
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 0 {
		t.Fatalf("got %+v, want no files", files)
	}
}

func rejectsAnEntryWithNoPath(t *testing.T) {
	_, err := ParseStatus([]byte(" M\x00"))

	if err == nil {
		t.Fatal("got no error for an entry with no path")
	}
}

func rejectsARenameWithNoOriginalPath(t *testing.T) {
	_, err := ParseStatus([]byte("R  new.go\x00"))

	if err == nil {
		t.Fatal("got no error for a rename with no original path")
	}
}

func partlyStagedFileHasBothKindsOfChanges(t *testing.T) {
	file := FileStatus{Path: "a.go", IndexStatus: 'M', WorktreeStatus: 'M'}

	if !file.HasStagedChanges() || !file.HasUnstagedChanges() || file.IsUntracked() {
		t.Fatalf("%+v: want staged and unstaged changes, tracked", file)
	}
}

func untrackedFileIsUnstagedOnly(t *testing.T) {
	file := FileStatus{Path: "a.go", IndexStatus: '?', WorktreeStatus: '?'}

	if file.HasStagedChanges() || !file.HasUnstagedChanges() || !file.IsUntracked() {
		t.Fatalf("%+v: want untracked with unstaged changes only", file)
	}
}
