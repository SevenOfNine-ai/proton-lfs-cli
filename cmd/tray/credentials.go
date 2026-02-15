package main

import (
	"os/exec"
)

// credentialVerify checks whether credentials exist for the given provider
// by running proton-drive-cli credential verify --provider <provider>.
// GIT_TERMINAL_PROMPT=0 and GCM_INTERACTIVE=never suppress interactive
// prompts so the check is truly silent.
func credentialVerify(provider string) bool {
	driveCLI := discoverDriveCLIBinary()
	if driveCLI == "" {
		return false
	}
	cmd := exec.Command(driveCLI, "credential", "verify", "--provider", provider, "-q")
	cmd.Env = append(cmd.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	)
	return cmd.Run() == nil
}
