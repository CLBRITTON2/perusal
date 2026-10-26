//go:build windows

package focus

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The bits every .git must deny the agent account: icacls (OI)(CI)(WD,AD,WEA,WA,DE,DC).
const (
	deniedWriteMask = windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA |
		windows.FILE_WRITE_ATTRIBUTES | windows.DELETE | 0x40 // FILE_DELETE_CHILD, which x/sys/windows lacks
	grantedWriteMask = deniedWriteMask | windows.GENERIC_WRITE | windows.GENERIC_ALL
)

// verifyGitDir checks every entry, since one the agent owns, that grants writes ahead of the inherited deny, or that
// lets the agent or a group other than SYSTEM and Administrators rewrite its ACL or owner, is writable through it.
func verifyGitDir(gitDir string, agent *windows.SID) error {
	info, err := os.Lstat(gitDir)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", gitDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a plain directory (mode %s)", ErrGitDirUnprotected, gitDir, info.Mode())
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("build the SYSTEM SID: %w", err)
	}
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return fmt.Errorf("build the Administrators SID: %w", err)
	}
	sidTypes := map[string]uint32{}
	return filepath.WalkDir(gitDir, func(path string, entry fs.DirEntry, listErr error) error {
		if listErr != nil {
			return fmt.Errorf("list %s: %w", path, listErr)
		}
		// A junction or link leads out of .git, past its deny.
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("%w: %s is not a plain file or directory (mode %s)", ErrGitDirUnprotected, path, entry.Type())
		}
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
		denied := false
		// Windows applies the first ACE that matches each bit, so a grant ahead of the deny wins.
		for index := uint16(0); index < dacl.AceCount; index++ {
			var ace *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
				return fmt.Errorf("read ACE %d of %s: %w", index, path, err)
			}
			// The SID starts at SidStart and runs past the end of the Go struct, as in the Win32 ACCESS_DENIED_ACE.
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			switch {
			case ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0: // applies to children only, not to path
			case ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE:
				denied = denied || uint32(ace.Mask)&deniedWriteMask == deniedWriteMask && sid.Equals(agent)
			case !denied && uint32(ace.Mask)&grantedWriteMask != 0:
				return fmt.Errorf("%w: ACE %d of %s grants writes ahead of the deny for %s, "+
					"icacls %s /reset /T restores the inherited ACL", ErrGitDirUnprotected, index, path, agent, path)
			// The deny does not cover rewriting the ACL or the owner, and a group the agent is in could hold either.
			case uint32(ace.Mask)&(windows.WRITE_DAC|windows.WRITE_OWNER|windows.GENERIC_ALL) != 0 &&
				!sid.Equals(system) && !sid.Equals(administrators):
				key := sid.String()
				if _, cached := sidTypes[key]; !cached {
					_, _, sidType, err := sid.LookupAccount("")
					if err != nil {
						return fmt.Errorf("look up %s, which ACE %d of %s lets rewrite its ACL: %w", key, index, path, err)
					}
					sidTypes[key] = sidType
				}
				if sid.Equals(agent) || sidTypes[key] != windows.SidTypeUser {
					return fmt.Errorf("%w: ACE %d of %s lets %s rewrite its ACL or owner, which only SYSTEM, "+
						"Administrators and user accounts other than %s may", ErrGitDirUnprotected, index, path, key, agent)
				}
			}
		}
		if !denied {
			return fmt.Errorf("%w: %s has no (WD,AD,WEA,WA,DE,DC) deny for %s, "+
				"add it as the README's Follow mode section shows", ErrGitDirUnprotected, path, agent)
		}
		return nil
	})
}

// heldGitDirs keeps a handle open in each checked git directory, keyed by its final path. Windows refuses to rename a
// folder with a handle open inside it, so the agent cannot move a work tree aside and plant its own .git at the
// checked path.
type heldGitDirs struct {
	mutex   sync.Mutex
	handles map[string]windows.Handle
}

// hold opens path before verify runs on its final path, and keeps it open only when verify passes. The final path has
// the junctions and links on the way to path resolved, since the agent could re-point one after the check.
func (h *heldGitDirs) hold(path string, verify func(final string) error) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", fmt.Errorf("encode path %s: %w", path, err)
	}
	// Backup semantics opens a directory, access 0 is enough to read its name, and a reparse point at path is held
	// itself, not its target.
	handle, err := windows.CreateFile(name, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	// Flags 0 are FILE_NAME_NORMALIZED and VOLUME_NAME_DOS, which x/sys/windows lacks.
	length, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
	resolved := windows.UTF16ToString(buffer[:min(int(length), len(buffer))])
	final := strings.TrimPrefix(resolved, `\\?\`)
	var refusal error
	switch {
	case err != nil:
		refusal = fmt.Errorf("resolve %s: %w", path, err)
	case int(length) >= len(buffer):
		refusal = fmt.Errorf("resolve %s: the final path is over %d characters", path, len(buffer))
	case strings.HasPrefix(resolved, `\\?\UNC\`):
		refusal = fmt.Errorf("%w: %s resolves to the network path %s", ErrGitDirUnprotected, path, resolved)
	default:
		refusal = verify(final)
	}
	if refusal != nil {
		return "", errors.Join(refusal, windows.CloseHandle(handle))
	}
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if _, held := h.handles[final]; held {
		if err := windows.CloseHandle(handle); err != nil {
			return "", fmt.Errorf("close the second handle on %s: %w", final, err)
		}
		return final, nil
	}
	h.handles[final] = handle
	return final, nil
}

func (h *heldGitDirs) release() error {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	errs := []error{}
	for final, handle := range h.handles {
		if err := windows.CloseHandle(handle); err != nil {
			errs = append(errs, fmt.Errorf("close the handle on %s: %w", final, err))
		}
	}
	h.handles = map[string]windows.Handle{}
	return errors.Join(errs...)
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
