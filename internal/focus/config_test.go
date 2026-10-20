//go:build windows

package focus

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		wantErr error
	}{
		{"loadsACompleteConfig", `{"agentAccount": "claude", "focusFile": "C:\\Users\\claude\\dev\\.herdr-focus",
			"agentDir": "C:\\Users\\claude\\dev", "ownerDir": "C:\\Users\\Chris\\dev"}`, nil},
		{"refusesAMisspelledKey", `{"agentAccount": "claude", "focusFile": "C:\\f", "agentDir": "C:\\a",
			"ownersDir": "C:\\o"}`, ErrInvalidConfig},
		{"refusesARelativePath", `{"agentAccount": "claude", "focusFile": "C:\\f", "agentDir": "dev",
			"ownerDir": "C:\\o"}`, ErrInvalidConfig},
		{"refusesAMissingAccount", `{"focusFile": "C:\\f", "agentDir": "C:\\a", "ownerDir": "C:\\o"}`, ErrInvalidConfig},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := LoadConfig(path)

			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("got %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
