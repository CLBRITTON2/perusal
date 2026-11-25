//go:build windows

package focus

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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
	t.Run("refusesAnEntryThatGrantsWritesAheadOfTheDeny", refusesAnEntryThatGrantsWritesAheadOfTheDeny)
	t.Run("refusesAJunctionInside", refusesAJunctionInside)
	t.Run("refusesAWriteDacGrantThatCanLiftTheDeny", refusesAWriteDacGrantThatCanLiftTheDeny)
	t.Run("refusesAWriteOwnerGrantThatCanLiftTheDeny", refusesAWriteOwnerGrantThatCanLiftTheDeny)
}

func refusesAWriteDacGrantThatCanLiftTheDeny(t *testing.T) {
	gitDir := deniedGitDir(t, deniedWriteMask)
	addGuestsACE(t, gitDir, windows.WRITE_DAC, windows.GRANT_ACCESS)
	agentSID := parsedGuestsSID(t)

	err := verifyGitDir(gitDir, agentSID)

	require.ErrorIs(t, err, ErrGitDirUnprotected)
}

func refusesAWriteOwnerGrantThatCanLiftTheDeny(t *testing.T) {
	gitDir := deniedGitDir(t, deniedWriteMask)
	addGuestsACE(t, gitDir, windows.WRITE_OWNER, windows.GRANT_ACCESS)
	agentSID := parsedGuestsSID(t)

	err := verifyGitDir(gitDir, agentSID)

	require.ErrorIs(t, err, ErrGitDirUnprotected)
}

func acceptsTheSetupDeny(t *testing.T) {
	gitDir := deniedGitDir(t, deniedWriteMask)
	mustMkdir(t, filepath.Join(gitDir, "hooks"))
	err := os.WriteFile(filepath.Join(gitDir, "hooks", "pre-commit"), []byte("exit 0\n"), 0o600)
	require.NoError(t, err)
	agentSID := parsedGuestsSID(t)

	err = verifyGitDir(gitDir, agentSID)

	require.NoError(t, err)
}

func refusesADirectoryWithoutTheDeny(t *testing.T) {
	gitDir := filepath.Join(t.TempDir(), ".git")
	mustMkdir(t, gitDir)
	agentSID := parsedGuestsSID(t)

	err := verifyGitDir(gitDir, agentSID)

	require.ErrorIs(t, err, ErrGitDirUnprotected)
}

func refusesADenyThatIsMissingDelete(t *testing.T) {
	gitDir := deniedGitDir(t, deniedWriteMask&^windows.DELETE)
	agentSID := parsedGuestsSID(t)

	err := verifyGitDir(gitDir, agentSID)

	require.ErrorIs(t, err, ErrGitDirUnprotected)
}

func refusesADirectoryTheAgentOwns(t *testing.T) {
	gitDir := deniedGitDir(t, deniedWriteMask)
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	require.NoError(t, err)

	err = verifyGitDir(gitDir, user.User.Sid)

	require.ErrorIs(t, err, ErrGitDirUnprotected)
}

func refusesAnEntryThatGrantsWritesAheadOfTheDeny(t *testing.T) {
	gitDir := deniedGitDir(t, deniedWriteMask)
	hooks := filepath.Join(gitDir, "hooks")
	mustMkdir(t, hooks)
	addGuestsACE(t, hooks, windows.FILE_WRITE_DATA, windows.GRANT_ACCESS)
	agentSID := parsedGuestsSID(t)

	err := verifyGitDir(gitDir, agentSID)

	require.ErrorIs(t, err, ErrGitDirUnprotected)
}

func refusesAJunctionInside(t *testing.T) {
	gitDir := deniedGitDir(t, deniedWriteMask)
	target := filepath.Join(t.TempDir(), "hooks")
	mustMkdir(t, target)
	junction := filepath.Join(gitDir, "hooks")
	makeJunction(t, junction, target)
	agentSID := parsedGuestsSID(t)

	err := verifyGitDir(gitDir, agentSID)

	require.ErrorIs(t, err, ErrGitDirUnprotected)
}

func refusesAGitFile(t *testing.T) {
	gitFile := filepath.Join(t.TempDir(), ".git")
	err := os.WriteFile(gitFile, []byte("gitdir: ../elsewhere\n"), 0o600)
	require.NoError(t, err)
	agentSID := parsedGuestsSID(t)

	err = verifyGitDir(gitFile, agentSID)

	require.ErrorIs(t, err, ErrGitDirUnprotected)
}

func makeJunction(t *testing.T, junction string, target string) {
	t.Helper()
	command := exec.Command("cmd", "/c", "mklink", "/J", junction, target)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "mklink: %s", output)
}

func deniedGitDir(t *testing.T, mask uint32) string {
	t.Helper()
	gitDir := filepath.Join(t.TempDir(), ".git")
	mustMkdir(t, gitDir)
	denyGuests(t, gitDir, mask)
	return gitDir
}

func parsedGuestsSID(t *testing.T) *windows.SID {
	t.Helper()
	sid, err := windows.StringToSid(guestsSID)
	require.NoError(t, err)
	return sid
}

func denyGuests(t *testing.T, path string, mask uint32) {
	t.Helper()
	addGuestsACE(t, path, mask, windows.DENY_ACCESS)
}

func addGuestsACE(t *testing.T, path string, mask uint32, mode windows.ACCESS_MODE) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	currentDACL, _, err := descriptor.DACL()
	require.NoError(t, err)
	guests := parsedGuestsSID(t)
	entry := windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.ACCESS_MASK(mask),
		AccessMode:        mode,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(guests),
		},
	}
	dacl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{entry}, currentDACL)
	require.NoError(t, err)
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
	require.NoError(t, err)
}
