package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewInstrumentedHTTPClient_Default(t *testing.T) {
	httpClient, err := NewInstrumentedHTTPClient("")
	if err != nil {
		t.Fatalf("NewInstrumentedHTTPClient() error = %v", err)
	}

	if httpClient == nil {
		t.Fatal("NewInstrumentedHTTPClient() returned nil client")
	}

	if httpClient.Transport == nil {
		t.Fatal("expected transport to be configured")
	}
}

func TestNewInstrumentedHTTPClient_InvalidCAPath(t *testing.T) {
	_, err := NewInstrumentedHTTPClient("/does/not/exist.pem")
	if err == nil {
		t.Fatal("expected error for missing CA cert file")
	}
}

func TestNewInstrumentedHTTPClient_InvalidPEM(t *testing.T) {
	certPath := filepath.Join(t.TempDir(), "invalid.pem")
	if err := os.WriteFile(certPath, []byte("not-a-pem"), 0o600); err != nil {
		t.Fatalf("write invalid cert file: %v", err)
	}

	_, err := NewInstrumentedHTTPClient(certPath)
	if err == nil {
		t.Fatal("expected error for invalid CA cert file")
	}
}
