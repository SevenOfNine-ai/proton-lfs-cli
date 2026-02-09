package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPostJSONSendsBodyAndContentType(t *testing.T) {
	var gotBody string
	var gotContentType string

	client := NewSDKClient("http://sdk.local")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		gotBody = string(body)
		gotContentType = r.Header.Get("Content-Type")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Header:     make(http.Header),
		}, nil
	})}

	resp, err := client.postJSON("/upload", []byte(`{"key":"value"}`))
	if err != nil {
		t.Fatalf("postJSON returned error: %v", err)
	}
	defer resp.Body.Close()

	if gotBody != `{"key":"value"}` {
		t.Fatalf("unexpected body: %q", gotBody)
	}
	if gotContentType != "application/json" {
		t.Fatalf("unexpected content type: %q", gotContentType)
	}
}

func TestInitializeSessionParsesToken(t *testing.T) {
	client := NewSDKClient("http://sdk.local")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/init" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := json.Marshal(map[string]string{"token": "abc"})
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(body))),
			Header:     make(http.Header),
		}, nil
	})}

	token, err := client.InitializeSession("user", "pass")
	if err != nil {
		t.Fatalf("InitializeSession returned error: %v", err)
	}
	if token != "abc" {
		t.Fatalf("unexpected token: %s", token)
	}
}

func TestDownloadFileSendsOutputPath(t *testing.T) {
	var payload map[string]string

	client := NewSDKClient("http://sdk.local")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/download" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Header:     make(http.Header),
		}, nil
	})}

	if err := client.DownloadFile("token", validOID, "/tmp/out.bin"); err != nil {
		t.Fatalf("DownloadFile returned error: %v", err)
	}

	if payload["outputPath"] != "/tmp/out.bin" {
		t.Fatalf("expected outputPath, got %#v", payload)
	}
}
