package main

import (
	"encoding/json"
	"io"
	"strings"
)

// TestHelpers provides utilities for testing the adapter

// MockReader creates a reader from a sequence of JSON messages
func MockReader(messages ...string) io.Reader {
	content := strings.Join(messages, "\n") + "\n"
	return strings.NewReader(content)
}

// MockWriter captures output from the adapter
type MockWriter struct {
	messages []OutboundMessage
	raw      strings.Builder
}

// Write appends data to the mock writer
func (mw *MockWriter) Write(p []byte) (n int, err error) {
	n, err = mw.raw.Write(p)
	if err == nil {
		// Try to decode as JSON message
		lines := strings.Split(strings.TrimSpace(mw.raw.String()), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			var msg OutboundMessage
			if err := json.Unmarshal([]byte(line), &msg); err == nil {
				mw.messages = append(mw.messages, msg)
			}
		}
	}
	return
}

// Messages returns all decoded messages
func (mw *MockWriter) Messages() []OutboundMessage {
	return mw.messages
}

// LastMessage returns the most recent message
func (mw *MockWriter) LastMessage() *OutboundMessage {
	if len(mw.messages) == 0 {
		return nil
	}
	return &mw.messages[len(mw.messages)-1]
}

// RawOutput returns the raw written data
func (mw *MockWriter) RawOutput() string {
	return mw.raw.String()
}

// AssertLastEvent checks if the last message has the expected event
func (mw *MockWriter) AssertLastEvent(expectedEvent string) bool {
	msg := mw.LastMessage()
	return msg != nil && msg.Event == expectedEvent
}

// AssertLastOID checks if the last message has the expected OID
func (mw *MockWriter) AssertLastOID(expectedOID string) bool {
	msg := mw.LastMessage()
	return msg != nil && msg.OID == expectedOID
}

// AssertLastError checks if the last message has an error with expected code
func (mw *MockWriter) AssertLastError(expectedCode int) bool {
	msg := mw.LastMessage()
	return msg != nil && msg.Error != nil && msg.Error.Code == expectedCode
}

// AssertMessageCount checks if expected number of messages were written
func (mw *MockWriter) AssertMessageCount(expected int) bool {
	return len(mw.messages) == expected
}
