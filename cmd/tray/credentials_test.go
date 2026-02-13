package main

import (
	"testing"

	"proton-git-lfs/internal/config"
)

func TestPassCliLoginItemDisplayName(t *testing.T) {
	cases := []struct {
		name     string
		item     passCliLoginItem
		expected string
	}{
		{"email only", passCliLoginItem{Email: "user@proton.me"}, "user@proton.me"},
		{"username only", passCliLoginItem{Username: "agent.smith"}, "agent.smith"},
		{"both prefers email", passCliLoginItem{Email: "user@proton.me", Username: "agent.smith"}, "user@proton.me"},
		{"neither", passCliLoginItem{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.item.DisplayName()
			if got != tc.expected {
				t.Fatalf("DisplayName() = %q, expected %q", got, tc.expected)
			}
		})
	}
}

func TestProtonCredentialHost(t *testing.T) {
	// Verify the shared constant is what we expect
	if config.ProtonCredentialHost != "proton.me" {
		t.Fatalf("config.ProtonCredentialHost = %q, expected %q", config.ProtonCredentialHost, "proton.me")
	}
}
