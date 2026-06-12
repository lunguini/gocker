package fsutil

import (
	"fmt"
	"os"
)

var osExit = os.Exit

// HomeDir returns the user's home directory. All gocker state lives under
// ~/.gocker, so there is no sane fallback when the home directory cannot be
// determined — fail fast with a clear message instead of silently using
// paths rooted at "/" (which is what `home, _ := os.UserHomeDir()` produces).
func HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		fmt.Fprintf(os.Stderr, "gocker: cannot determine home directory (is $HOME set?): %v\n", err)
		osExit(1)
	}
	return home
}
