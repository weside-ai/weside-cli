package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// secretInput keeps legacy argv inputs and optional stored-key reuse intact.
// Stdin is opt-in, never echoed, and removes only one final LF or CRLF so pipes
// from password managers work without trimming meaningful secret whitespace.
func secretInput(cmd *cobra.Command, stdinFlag, value string, supplied bool) (secret string, present bool, err error) {
	fromStdin, err := cmd.Flags().GetBool(stdinFlag)
	if err != nil {
		return "", false, err
	}
	if !fromStdin {
		return value, supplied, nil
	}
	if supplied {
		return "", false, fmt.Errorf("--%s cannot be combined with a secret argument or flag", stdinFlag)
	}
	const maxSecretBytes = 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxSecretBytes+1))
	if err != nil {
		return "", false, fmt.Errorf("reading secret from stdin: %w", err)
	}
	if len(data) > maxSecretBytes {
		return "", false, fmt.Errorf("secret on stdin exceeds 1 MiB")
	}
	value = strings.TrimSuffix(string(data), "\n")
	if len(value) != len(data) {
		value = strings.TrimSuffix(value, "\r")
	}
	if value == "" {
		return "", false, fmt.Errorf("secret on stdin is empty")
	}
	return value, true, nil
}

func addSecretStdinFlag(cmd *cobra.Command, name string) {
	cmd.Flags().Bool(name, false, "read secret from stdin instead of argv (removes one final LF/CRLF)")
}
