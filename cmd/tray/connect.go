package main

import (
	"fmt"
	"os/exec"

	"proton-git-lfs/internal/config"
)

// connectToProton runs the unified tray Connect flow for any credential provider:
//  1. Verify credentials exist via proton-drive-cli credential verify --provider
//  2. If missing → open terminal for interactive credential store
//  3. If present → log in silently via proton-drive-cli login --credential-provider
func connectToProton() {
	driveCLI := discoverDriveCLIBinary()
	if driveCLI == "" {
		sendNotification("Error: CLI not found")
		return
	}

	prefs := config.LoadPrefs()
	provider := prefs.CredentialProvider

	if !credentialVerify(provider) {
		// No credentials stored — open terminal for interactive store
		script := fmt.Sprintf("'%s' credential store --provider %s; echo; printf 'Press Enter to close... ' && read", driveCLI, provider)
		cmd := terminalCommand(script)
		if cmd != nil {
			_ = cmd.Start()
		}
		sendNotification("Complete setup in Terminal")
		return
	}

	// Credentials exist — log in silently
	sendNotification("Connecting…")
	go func() {
		if err := protonDriveLogin(driveCLI, "--credential-provider", provider); err != nil {
			sendNotification("Login failed")
			return
		}
		sendNotification("Connected to Proton")
		applyConnectStatus(true)
	}()
}

// protonDriveLogin runs proton-drive-cli login with the given extra args.
func protonDriveLogin(driveCLI string, args ...string) error {
	cmdArgs := append([]string{"login"}, args...)
	cmdArgs = append(cmdArgs, "-q")
	return exec.Command(driveCLI, cmdArgs...).Run()
}
