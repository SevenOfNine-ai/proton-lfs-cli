package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/systray"

	"proton-git-lfs/internal/config"
)

const pollInterval = 5 * time.Second
const refreshInterval = 15 * time.Minute

var (
	stopCh      chan struct{}
	stopOnce    sync.Once
	lastRefresh time.Time
)

func startStatusWatcher() {
	stopCh = make(chan struct{})
	go watchLoop()
}

func stopStatusWatcher() {
	stopOnce.Do(func() { close(stopCh) })
}

func watchLoop() {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Initial read
	applyStatus()

	for {
		select {
		case <-ticker.C:
			applyStatus()
			maybeRefreshSession()
		case <-stopCh:
			return
		}
	}
}

// maybeRefreshSession proactively refreshes the Proton session token
// every 15 minutes to keep the session alive. This calls POST /auth/v4/refresh
// (NOT a login attempt) — it never triggers CAPTCHA or rate-limiting.
func maybeRefreshSession() {
	if time.Since(lastRefresh) < refreshInterval {
		return
	}

	// Check if a session file exists (no point refreshing if not logged in)
	sessionFile := filepath.Join(os.Getenv("HOME"), ".proton-drive-cli", "session.json")
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		return
	}

	driveCLI := discoverDriveCLIBinary()
	if driveCLI == "" {
		return
	}

	lastRefresh = time.Now()

	// Spawn in background — don't block the status poll loop
	go func() {
		cmd := exec.Command(driveCLI, "session", "refresh")
		_ = cmd.Run()
	}()
}

func applyStatus() {
	report, err := config.ReadStatus()
	if err != nil {
		mStatus.SetTitle("Status: No adapter activity")
		systray.SetIcon(iconIdle)
		systray.SetTemplateIcon(iconIdle, iconIdle)
		return
	}

	switch report.State {
	case config.StateIdle:
		mStatus.SetTitle("Status: Ready")
		systray.SetIcon(iconIdle)
		systray.SetTemplateIcon(iconIdle, iconIdle)
	case config.StateOK:
		mStatus.SetTitle("Status: OK")
		systray.SetIcon(iconOK)
		systray.SetTemplateIcon(iconOK, iconOK)
	case config.StateError:
		msg := "Status: Error"
		if report.Error != "" {
			msg = fmt.Sprintf("Status: Error — %s", truncate(report.Error, 40))
		}
		mStatus.SetTitle(msg)
		systray.SetIcon(iconError)
		systray.SetTemplateIcon(iconError, iconError)
	case config.StateTransferring:
		mStatus.SetTitle("Status: Transferring…")
		systray.SetIcon(iconSyncing)
		systray.SetTemplateIcon(iconSyncing, iconSyncing)
	}

	if !report.Timestamp.IsZero() {
		// Only show "Last Transfer" for actual data operations, not init/terminate
		switch report.LastOp {
		case "upload", "download":
			mLastTransfer.SetTitle(fmt.Sprintf("Last Transfer: %s (%s)", relativeTime(report.Timestamp), report.LastOp))
		default:
			mLastTransfer.SetTitle(fmt.Sprintf("Last Activity: %s", relativeTime(report.Timestamp)))
		}
	}
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit-1] + "…"
}
