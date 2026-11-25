//go:build windows

package focus

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Run("loadConfigAcceptsACompleteConfig", loadConfigAcceptsACompleteConfig)
	t.Run("loadConfigRefusesAMisspelledKey", loadConfigRefusesAMisspelledKey)
	t.Run("loadConfigRefusesARelativePath", loadConfigRefusesARelativePath)
	t.Run("loadConfigRefusesAMissingAccount", loadConfigRefusesAMissingAccount)
	t.Run("loadConfigAcceptsAnAgentHostConfigWithoutAccount", loadConfigAcceptsAnAgentHostConfigWithoutAccount)
	t.Run("loadConfigRefusesAnAgentHostSshWouldReadAsAnOption", loadConfigRefusesAnAgentHostSshWouldReadAsAnOption)
	t.Run("loadConfigAcceptsAnOwnerStateFileOnTheAgentHost", loadConfigAcceptsAnOwnerStateFileOnTheAgentHost)
	t.Run("loadConfigRefusesARelativeOwnerStateFile", loadConfigRefusesARelativeOwnerStateFile)
	t.Run("loadConfigRefusesAnOwnerStateFileWithoutAgentHost", loadConfigRefusesAnOwnerStateFileWithoutAgentHost)
}

func loadConfigAcceptsACompleteConfig(t *testing.T) {
	path := configFileHolding(t, `{"agentAccount": "claude", "focusFile": "C:\\Users\\claude\\dev\\.herdr-focus",
		"agentDir": "C:\\Users\\claude\\dev", "ownerDir": "C:\\Users\\Chris\\dev"}`)
	_, err := LoadConfig(path)
	require.NoError(t, err)
}

func loadConfigRefusesAMisspelledKey(t *testing.T) {
	path := configFileHolding(t, `{"agentAccount": "claude", "focusFile": "C:\\f", "agentDir": "C:\\a",
		"ownersDir": "C:\\o"}`)
	_, err := LoadConfig(path)
	require.ErrorIs(t, err, ErrInvalidConfig)
}

func loadConfigRefusesARelativePath(t *testing.T) {
	path := configFileHolding(t, `{"agentAccount": "claude", "focusFile": "C:\\f", "agentDir": "dev",
		"ownerDir": "C:\\o"}`)
	_, err := LoadConfig(path)
	require.ErrorIs(t, err, ErrInvalidConfig)
}

func loadConfigRefusesAMissingAccount(t *testing.T) {
	path := configFileHolding(t, `{"focusFile": "C:\\f", "agentDir": "C:\\a", "ownerDir": "C:\\o"}`)
	_, err := LoadConfig(path)
	require.ErrorIs(t, err, ErrInvalidConfig)
}

func loadConfigAcceptsAnAgentHostConfigWithoutAccount(t *testing.T) {
	path := configFileHolding(t, `{"focusFile": "C:\\f", "agentDir": "C:\\a", "ownerDir": "C:\\o",
		"agentHost": "agent-vm"}`)
	_, err := LoadConfig(path)
	require.NoError(t, err)
}

func loadConfigRefusesAnAgentHostSshWouldReadAsAnOption(t *testing.T) {
	path := configFileHolding(t, `{"agentAccount": "claude", "focusFile": "C:\\f",
		"agentDir": "C:\\a", "ownerDir": "C:\\o", "agentHost": "-oProxyCommand=calc"}`)
	_, err := LoadConfig(path)
	require.ErrorIs(t, err, ErrInvalidConfig)
}

func loadConfigAcceptsAnOwnerStateFileOnTheAgentHost(t *testing.T) {
	path := configFileHolding(t, `{"focusFile": "C:\\f", "agentDir": "C:\\a", "ownerDir": "C:\\o",
		"agentHost": "agent-vm", "ownerStateFile": "C:\\a\\owner-state.json"}`)
	_, err := LoadConfig(path)
	require.NoError(t, err)
}

func loadConfigRefusesARelativeOwnerStateFile(t *testing.T) {
	path := configFileHolding(t, `{"focusFile": "C:\\f", "agentDir": "C:\\a", "ownerDir": "C:\\o",
		"agentHost": "agent-vm", "ownerStateFile": "owner-state.json"}`)
	_, err := LoadConfig(path)
	require.ErrorIs(t, err, ErrInvalidConfig)
}

func loadConfigRefusesAnOwnerStateFileWithoutAgentHost(t *testing.T) {
	path := configFileHolding(t, `{"agentAccount": "claude", "focusFile": "C:\\f",
		"agentDir": "C:\\a", "ownerDir": "C:\\o", "ownerStateFile": "C:\\a\\owner-state.json"}`)
	_, err := LoadConfig(path)
	require.ErrorIs(t, err, ErrInvalidConfig)
}

func configFileHolding(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	err := os.WriteFile(path, []byte(content), 0o600)
	require.NoError(t, err)
	return path
}
