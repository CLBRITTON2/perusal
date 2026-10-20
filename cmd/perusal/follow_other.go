//go:build !windows

package main

import (
	"errors"
)

const followUsage = ""

// Follow mode trusts a .git only after checking its Windows ACL, and that check has no Unix port yet.
func runFollow(args []string) error {
	return errors.New("perusal follow runs only on Windows, see the README")
}
