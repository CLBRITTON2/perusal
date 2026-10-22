//go:build windows

package focus

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fileDeleteChild = 0x40 // x/sys/windows lacks FILE_DELETE_CHILD

// The bits every .git must deny the agent account: icacls (OI)(CI)(WD,AD,WEA,WA,DE,DC).
const (
	deniedWriteMask = windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA |
		windows.FILE_WRITE_ATTRIBUTES | windows.DELETE | fileDeleteChild
	grantedWriteMask = deniedWriteMask | windows.GENERIC_WRITE | windows.GENERIC_ALL
)

// verifyGitDir checks every entry, since one the agent owns, or that grants writes ahead of the inherited deny, is
// writable through it.
func verifyGitDir(gitDir string, agent *windows.SID) error {
	info, err := os.Lstat(gitDir)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", gitDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a plain directory (mode %s)", ErrGitDirUnprotected, gitDir, info.Mode())
	}
	return filepath.WalkDir(gitDir, func(path string, entry fs.DirEntry, listErr error) error {
		if listErr != nil {
			return fmt.Errorf("list %s: %w", path, listErr)
		}
		// A junction or link leads out of .git, past its deny.
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("%w: %s is not a plain file or directory (mode %s)", ErrGitDirUnprotected, path, entry.Type())
		}
		return verifyGitEntry(path, agent)
	})
}

func verifyGitEntry(path string, agent *windows.SID) error {
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read the security descriptor of %s: %w", path, err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return fmt.Errorf("read the owner of %s: %w", path, err)
	}
	// An owner can rewrite the DACL, so a deny on something the agent owns proves nothing.
	if owner.Equals(agent) {
		return fmt.Errorf("%w: the agent account %s owns %s", ErrGitDirUnprotected, agent, path)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return fmt.Errorf("read the DACL of %s: %w", path, err)
	}
	if dacl == nil {
		return fmt.Errorf("%w: %s has a null DACL, which grants everyone full access", ErrGitDirUnprotected, path)
	}
	// Windows applies the first ACE that matches each bit, so a grant ahead of the deny wins.
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return fmt.Errorf("read ACE %d of %s: %w", index, path, err)
		}
		switch {
		case ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0: // applies to children only, not to path
		case deniesAgentWrites(ace, agent):
			return nil
		case ace.Header.AceType != windows.ACCESS_DENIED_ACE_TYPE && uint32(ace.Mask)&grantedWriteMask != 0:
			return fmt.Errorf("%w: ACE %d of %s grants writes ahead of the deny for %s, "+
				"icacls %s /reset /T restores the inherited ACL", ErrGitDirUnprotected, index, path, agent, path)
		}
	}
	return fmt.Errorf("%w: %s has no (WD,AD,WEA,WA,DE,DC) deny for %s, "+
		"add it as the README's Follow mode section shows", ErrGitDirUnprotected, path, agent)
}

func deniesAgentWrites(ace *windows.ACCESS_ALLOWED_ACE, agent *windows.SID) bool {
	if ace.Header.AceType != windows.ACCESS_DENIED_ACE_TYPE || uint32(ace.Mask)&deniedWriteMask != deniedWriteMask {
		return false
	}
	// The SID starts at SidStart and runs past the end of the Go struct, as in the Win32 ACCESS_DENIED_ACE.
	return (*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(agent)
}

// os.Open does not share delete access, which would make the publisher's replace fail.
func openSharingDelete(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("encode path %s: %w", path, err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}
