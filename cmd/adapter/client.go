package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SDKClient communicates with the Proton Drive SDK service
type SDKClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewSDKClient creates a new SDK service client
func NewSDKClient(baseURL string) *SDKClient {
	return &SDKClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// InitializeSession establishes a session with Proton Drive
// Phase 4: Implement actual authentication flow
func (c *SDKClient) InitializeSession(username, password string) (string, error) {
	req := map[string]string{
		"username": username,
		"password": password,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.postJSON("/init", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("init failed: %s", string(respBody))
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	token, ok := result["token"]
	if !ok {
		return "", fmt.Errorf("no token in response")
	}

	return token, nil
}

// UploadFile uploads a file to Proton Drive
// Phase 4: Implement actual file upload with encryption
func (c *SDKClient) UploadFile(token, oid string, filePath string) error {
	req := map[string]string{
		"token": token,
		"oid":   oid,
		"path":  filePath,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.postJSON("/upload", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: %s", string(respBody))
	}

	return nil
}

// DownloadFile downloads a file from Proton Drive
// Phase 4: Implement actual file download with decryption
func (c *SDKClient) DownloadFile(token, oid string, outputPath string) error {
	req := map[string]string{
		"token":      token,
		"oid":        oid,
		"outputPath": outputPath,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.postJSON("/download", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed: %s", string(respBody))
	}

	return nil
}

// postJSON sends a JSON POST request to the SDK service
func (c *SDKClient) postJSON(endpoint string, body []byte) (*http.Response, error) {
	url := c.baseURL + endpoint
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

// Health checks if the SDK service is available
func (c *SDKClient) Health() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SDK service unhealthy: status %d", resp.StatusCode)
	}

	return nil
}
