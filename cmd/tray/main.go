// Proton Git LFS system tray application.
package main

import (
	"os"
	"os/exec"
	"strings"

	"fyne.io/systray"
)

// Version is stamped at build time via -ldflags.
var Version = "dev"

func main() {
	augmentPath()
	systray.Run(onReady, onExit)
}

func onReady() {
	setupMenu()
	startStatusWatcher()
}

// augmentPath inherits the user's full shell PATH so that binaries
// installed via Homebrew, nvm, ~/.local/bin, etc. are available.
// macOS apps launched from Finder/Spotlight get a minimal PATH that
// excludes most user-installed tools.
func augmentPath() {
	out, err := exec.Command("zsh", "-lc", "echo $PATH").Output()
	if err != nil {
		return
	}
	shellPath := strings.TrimSpace(string(out))
	if shellPath != "" {
		_ = os.Setenv("PATH", shellPath)
	}
}

func onExit() {
	stopStatusWatcher()
}
