//go:build windows

package focus

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fileDeleteChild = 0x40 // x/sys/windows lacks FILE_DELETE_CHILD

// The bits every .git must deny the agent account: icacls (OI)(CI)(WD,AD,WEA,WA,DE,DC).
const (
	deniedWriteMask = windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA |
		windows.FILE_WRITE_ATTRIBUTES | windows.DELETE | fileDeleteChild
	deniedInheritFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
)

func verifyGitDir(gitDir string, agent *windows.SID) error {
	info, err := os.Lstat(gitDir)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", gitDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a plain directory (mode %s)", ErrGitDirUnprotected, gitDir, info.Mode())
	}
	descriptor, err := windows.GetNamedSecurityInfo(gitDir, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read the security descriptor of %s: %w", gitDir, err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return fmt.Errorf("read the owner of %s: %w", gitDir, err)
	}
	// An owner can rewrite the DACL, so a deny the agent set on its own directory proves nothing.
	if owner.Equals(agent) {
		return fmt.Errorf("%w: the agent account %s owns %s", ErrGitDirUnprotected, agent, gitDir)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return fmt.Errorf("read the DACL of %s: %w", gitDir, err)
	}
	if dacl == nil {
		return fmt.Errorf("%w: %s has a null DACL, which grants everyone full access", ErrGitDirUnprotected, gitDir)
	}
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return fmt.Errorf("read ACE %d of %s: %w", index, gitDir, err)
		}
		if deniesAgentWrites(ace, agent) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s has no (OI)(CI)(WD,AD,WEA,WA,DE,DC) deny for %s, "+
		"add it as the README's Follow mode section shows", ErrGitDirUnprotected, gitDir, agent)
}

func deniesAgentWrites(ace *windows.ACCESS_ALLOWED_ACE, agent *windows.SID) bool {
	if ace.Header.AceType != windows.ACCESS_DENIED_ACE_TYPE {
		return false
	}
	if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || ace.Header.AceFlags&deniedInheritFlags != deniedInheritFlags {
		return false
	}
	if uint32(ace.Mask)&deniedWriteMask != deniedWriteMask {
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
