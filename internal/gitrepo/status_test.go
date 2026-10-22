package gitrepo

import (
	"reflect"
	"testing"
)

func TestParseStatus(t *testing.T) {
	t.Run("readsBranchHeadersAndEveryEntryKind", readsBranchHeadersAndEveryEntryKind)
	t.Run("readsDetachedUnbornHeadWithNoUpstream", readsDetachedUnbornHeadWithNoUpstream)
	t.Run("rejectsARenameWithNoOriginalPath", rejectsARenameWithNoOriginalPath)
}

func readsBranchHeadersAndEveryEntryKind(t *testing.T) {
	output := []byte("# branch.oid 72164ac5a6f1bcbca714a97261c5bf3627aeadd7\x00# branch.head master\x00" +
		"# branch.upstream origin/master\x00# branch.ab +2 -1\x00" +
		"1 .M N... 100644 100644 100644 63dff5c 63dff5c a.go\x00" +
		"2 R. N... 100644 100644 100644 1a2b3c4 1a2b3c4 R100 new name.go\x00old name.go\x00" +
		"u UU N... 100644 100644 100644 100644 1a2b3c4 5d6e7f8 9a0b1c2 conflict.go\x00" +
		"? dir/new.txt\x00")

	status, err := parseStatus(output)
	if err != nil {
		t.Fatal(err)
	}

	want := Status{
		Branch:     "master",
		Tracking:   Tracking{Upstream: "origin/master", Ahead: 2, Behind: 1},
		HasCommits: true,
		Files: []FileStatus{
			{Path: "a.go", IndexStatus: ' ', WorktreeStatus: 'M'},
			{Path: "new name.go", OriginalPath: "old name.go", IndexStatus: 'R', WorktreeStatus: ' '},
			{Path: "conflict.go", IndexStatus: 'U', WorktreeStatus: 'U'},
			{Path: "dir/new.txt", IndexStatus: '?', WorktreeStatus: '?'},
		},
	}
	if !reflect.DeepEqual(status, want) {
		t.Fatalf("status:\n%+v\nwant:\n%+v", status, want)
	}
}

func readsDetachedUnbornHeadWithNoUpstream(t *testing.T) {
	status, err := parseStatus([]byte("# branch.oid (initial)\x00# branch.head (detached)\x00"))
	if err != nil {
		t.Fatal(err)
	}

	want := Status{Files: []FileStatus{}}
	if !reflect.DeepEqual(status, want) {
		t.Fatalf("status:\n%+v\nwant:\n%+v", status, want)
	}
}

func rejectsARenameWithNoOriginalPath(t *testing.T) {
	_, err := parseStatus([]byte("2 R. N... 100644 100644 100644 1a2b3c4 1a2b3c4 R100 new.go\x00"))

	if err == nil {
		t.Fatal("got no error for a rename with no original path")
	}
}
