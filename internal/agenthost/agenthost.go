// Package agenthost runs commands on the machine the agent works in, over ssh.
package agenthost

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Run runs script in host's ssh login shell, which must be PowerShell (5.1 or 7). sshd hands the command to its -c, so
// script must be one line without a double quote. The error leaves script out, so callers name the step.
func Run(ctx context.Context, host string, script string) ([]byte, error) {
	// BatchMode makes ssh fail instead of asking for a passphrase or a host key over the TUI.
	command := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", host, script)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return output, fmt.Errorf("ssh %s: %w: %s", host, err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

// QuotedString quotes text as a PowerShell single-quoted string.
func QuotedString(text string) string {
	return "'" + strings.ReplaceAll(text, "'", "''") + "'"
}
