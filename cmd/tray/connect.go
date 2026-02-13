package main

import (
	"bytes"
	"fmt"
	"os/exec"

	"proton-git-lfs/internal/config"
)

// connectToProton dispatches to the correct credential flow based on prefs.
func connectToProton() {
	prefs := config.LoadPrefs()
	if prefs.CredentialProvider == config.CredentialProviderGitCredential {
		connectGitCredential()
	} else {
		connectPassCLI()
	}
}

// connectGitCredential handles the git-credential Connect flow:
//  1. Verify credentials exist (silent subprocess)
//  2. If missing → open terminal for proton-drive credential store (interactive)
//  3. If present → log in silently via proton-drive login --credential-provider git
func connectGitCredential() {
	driveCLI := discoverDriveCLIBinary()
	if driveCLI == "" {
		sendNotification("Error: CLI not found")
		return
	}

	if !gitCredentialVerify() {
		// No credentials stored — open terminal for interactive store
		script := fmt.Sprintf("'%s' credential store; echo; printf 'Press Enter to close... ' && read", driveCLI)
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
		if err := protonDriveLogin(driveCLI, "--credential-provider", "git"); err != nil {
			sendNotification("Login failed")
			return
		}
		sendNotification("Connected to Proton")
		applyConnectStatus(true)
	}()
}

// connectPassCLI handles the Proton Pass Connect flow:
//  1. Check pass-cli is logged in (silent subprocess)
//  2. If not → open terminal for pass-cli login (browser OAuth)
//  3. Search vaults for proton.me entry
//  4. If not found → open terminal for credential creation
//  5. If found → log in silently, piping password via stdin
func connectPassCLI() {
	driveCLI := discoverDriveCLIBinary()
	if driveCLI == "" {
		sendNotification("Error: CLI not found")
		return
	}

	if !passCliIsLoggedIn() {
		// pass-cli not logged in — open terminal for browser OAuth
		script := "pass-cli login; echo; printf 'Press Enter to close... ' && read"
		cmd := terminalCommand(script)
		if cmd != nil {
			_ = cmd.Start()
		}
		sendNotification("Complete login in Terminal")
		return
	}

	// pass-cli is logged in — search for proton.me entry
	item, err := passCliSearchProtonEntry()
	if err != nil || item == nil {
		// No entry found — open terminal to create one
		script := fmt.Sprintf(`printf 'No Proton credentials found in Proton Pass.\n'
printf 'Proton email or username: ' && read account
stty -echo && printf 'Password: ' && read password && stty echo && echo
pass-cli item create login --title "Proton" --email "$account" --password "$password" --url https://%s
echo
printf 'Press Enter to close... ' && read`, config.ProtonCredentialHost)
		cmd := terminalCommand(script)
		if cmd != nil {
			_ = cmd.Start()
		}
		sendNotification("Complete setup in Terminal")
		return
	}

	// Entry found — log in silently, piping password via stdin
	sendNotification("Connecting…")
	username := item.DisplayName()
	password := item.Password

	go func() {
		if err := protonDriveLoginWithPassword(driveCLI, username, password); err != nil {
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

// protonDriveLoginWithPassword runs proton-drive-cli login, piping the
// password via stdin (--password-stdin).
func protonDriveLoginWithPassword(driveCLI, username, password string) error {
	cmd := exec.Command(driveCLI, "login", "-u", username, "--password-stdin", "-q")
	cmd.Stdin = bytes.NewBufferString(password)
	return cmd.Run()
}
