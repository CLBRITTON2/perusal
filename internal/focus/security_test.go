package focus

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// guestsSID stands in for the agent account: a SID that is never the test's own user.
const guestsSID = "S-1-5-32-546"

func TestVerifyGitDir(t *testing.T) {
	t.Run("acceptsTheSetupDeny", acceptsTheSetupDeny)
	t.Run("refusesADirectoryWithoutTheDeny", refusesADirectoryWithoutTheDeny)
	t.Run("refusesADenyThatIsMissingDelete", refusesADenyThatIsMissingDelete)
	t.Run("refusesADirectoryTheAgentOwns", refusesADirectoryTheAgentOwns)
	t.Run("refusesAGitFile", refusesAGitFile)
}

func acceptsTheSetupDeny(t *testing.T) {
	gitDir := deniedGitDir(t, deniedWriteMask)

	if err := VerifyGitDir(gitDir, guestsSID); err != nil {
		t.Fatal(err)
	}
}

func refusesADirectoryWithoutTheDeny(t *testing.T) {
	gitDir := filepath.Join(t.TempDir(), ".git")
	mustMkdir(t, gitDir)

	err := VerifyGitDir(gitDir, guestsSID)

	if !errors.Is(err, ErrGitDirUnprotected) {
		t.Fatalf("got %v, want %v", err, ErrGitDirUnprotected)
	}
}

func refusesADenyThatIsMissingDelete(t *testing.T) {
	gitDir := deniedGitDir(t, deniedWriteMask&^windows.DELETE)

	err := VerifyGitDir(gitDir, guestsSID)

	if !errors.Is(err, ErrGitDirUnprotected) {
		t.Fatalf("got %v, want %v", err, ErrGitDirUnprotected)
	}
}

func refusesADirectoryTheAgentOwns(t *testing.T) {
	gitDir := deniedGitDir(t, deniedWriteMask)
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}

	err = VerifyGitDir(gitDir, user.User.Sid.String())

	if !errors.Is(err, ErrGitDirUnprotected) {
		t.Fatalf("got %v, want %v", err, ErrGitDirUnprotected)
	}
}

func refusesAGitFile(t *testing.T) {
	gitFile := filepath.Join(t.TempDir(), ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: ../elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := VerifyGitDir(gitFile, guestsSID)

	if !errors.Is(err, ErrGitDirUnprotected) {
		t.Fatalf("got %v, want %v", err, ErrGitDirUnprotected)
	}
}

func deniedGitDir(t *testing.T, mask uint32) string {
	t.Helper()
	gitDir := filepath.Join(t.TempDir(), ".git")
	mustMkdir(t, gitDir)
	denyGuests(t, gitDir, mask)
	return gitDir
}

func denyGuests(t *testing.T, gitDir string, mask uint32) {
	t.Helper()
	guests, err := windows.StringToSid(guestsSID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := windows.GetNamedSecurityInfo(gitDir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	currentDACL, _, err := current.DACL()
	if err != nil {
		t.Fatal(err)
	}
	deny := windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.ACCESS_MASK(mask),
		AccessMode:        windows.DENY_ACCESS,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(guests),
		},
	}
	dacl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{deny}, currentDACL)
	if err != nil {
		t.Fatal(err)
	}
	err = windows.SetNamedSecurityInfo(gitDir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	if err != nil {
		t.Fatal(err)
	}
}
