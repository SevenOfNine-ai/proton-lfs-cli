package main

import (
	"encoding/json"
	"os/exec"
	"strings"

	"proton-git-lfs/internal/config"
)

// passCliVaults returns the list of vault names from pass-cli.
func passCliVaults() ([]string, error) {
	out, err := exec.Command("pass-cli", "vault", "list", "--output", "json").CombinedOutput()
	if err != nil {
		return nil, err
	}
	var result struct {
		Vaults []struct {
			Name string `json:"name"`
		} `json:"vaults"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}
	names := make([]string, len(result.Vaults))
	for i, v := range result.Vaults {
		names[i] = v.Name
	}
	return names, nil
}

// passCliLoginItem represents the relevant fields of a pass-cli login item.
type passCliLoginItem struct {
	Username string
	Email    string
	Password string
}

// passCliSearchProtonEntry searches all pass-cli vaults for a login entry
// with a URL matching proton.me. Returns the first match.
func passCliSearchProtonEntry() (*passCliLoginItem, error) {
	vaults, err := passCliVaults()
	if err != nil {
		return nil, err
	}
	for _, vault := range vaults {
		item, err := passCliSearchVault(vault)
		if err != nil {
			continue
		}
		if item != nil {
			return item, nil
		}
	}
	return nil, nil
}

// passCliSearchVault searches a single vault for a login entry with proton.me URL.
func passCliSearchVault(vaultName string) (*passCliLoginItem, error) {
	out, err := exec.Command("pass-cli", "item", "list", vaultName,
		"--filter-type", "login", "--output", "json").CombinedOutput()
	if err != nil {
		return nil, err
	}

	var result struct {
		Items []struct {
			State   string `json:"state"`
			Content struct {
				Content struct {
					Login struct {
						Email    string   `json:"email"`
						Username string   `json:"username"`
						Password string   `json:"password"`
						URLs     []string `json:"urls"`
					} `json:"Login"`
				} `json:"content"`
			} `json:"content"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}

	for _, item := range result.Items {
		if item.State != "Active" {
			continue
		}
		for _, u := range item.Content.Content.Login.URLs {
			if strings.Contains(strings.ToLower(u), config.ProtonCredentialHost) {
				return &passCliLoginItem{
					Username: item.Content.Content.Login.Username,
					Email:    item.Content.Content.Login.Email,
					Password: item.Content.Content.Login.Password,
				}, nil
			}
		}
	}
	return nil, nil
}

// passCliDisplayName returns the best display name for the entry.
func (p *passCliLoginItem) DisplayName() string {
	if p.Email != "" {
		return p.Email
	}
	return p.Username
}

// gitCredentialVerify checks whether credentials exist in the git credential
// helper by running proton-drive-cli credential verify.
func gitCredentialVerify() bool {
	driveCLI := discoverDriveCLIBinary()
	if driveCLI == "" {
		return false
	}
	err := exec.Command(driveCLI, "credential", "verify").Run()
	return err == nil
}

// passCliIsLoggedIn checks whether pass-cli is logged in by running pass-cli test.
func passCliIsLoggedIn() bool {
	err := exec.Command("pass-cli", "test").Run()
	return err == nil
}
