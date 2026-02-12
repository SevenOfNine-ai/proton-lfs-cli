package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// discoverAdapterBinary finds the bundled git-lfs-proton-adapter binary
// relative to the running tray executable.
func discoverAdapterBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	exe, _ = filepath.EvalSymlinks(exe)
	dir := filepath.Dir(exe)

	candidates := adapterCandidates(dir)
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

// discoverDriveCLIBinary finds the bundled proton-drive-cli binary.
func discoverDriveCLIBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	exe, _ = filepath.EvalSymlinks(exe)
	dir := filepath.Dir(exe)

	candidates := driveCLICandidates(dir)
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

func adapterCandidates(exeDir string) []string {
	name := "git-lfs-proton-adapter"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := []string{
		filepath.Join(exeDir, name), // same directory (Linux/Windows)
	}
	if runtime.GOOS == "darwin" {
		// macOS .app bundle: Contents/MacOS/tray → Contents/Helpers/adapter
		candidates = append(candidates, filepath.Join(exeDir, "..", "Helpers", name))
	}
	return candidates
}

func driveCLICandidates(exeDir string) []string {
	name := "proton-drive-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := []string{
		filepath.Join(exeDir, name),
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, filepath.Join(exeDir, "..", "Helpers", name))
	}
	return candidates
}
