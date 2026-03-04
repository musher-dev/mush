package client

import (
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func requireLocalListener(t *testing.T) {
	t.Helper()

	var lc net.ListenConfig

	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local listener not available in this environment: %v", err)
		return
	}

	_ = ln.Close()
}

func TestProbeHealth_Reachable(t *testing.T) {
	requireLocalListener(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result := ProbeHealth(t.Context(), srv.URL)

	if !result.Reachable {
		t.Fatalf("expected reachable, got error: %s", result.Error)
	}

	if result.Latency <= 0 {
		t.Fatalf("expected positive latency, got %v", result.Latency)
	}

	if result.Error != "" {
		t.Fatalf("expected no error, got %q", result.Error)
	}
}

func TestProbeHealth_Reachable4xx(t *testing.T) {
	requireLocalListener(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	result := ProbeHealth(t.Context(), srv.URL)

	if !result.Reachable {
		t.Fatalf("expected reachable even for 401, got error: %s", result.Error)
	}
}

func TestProbeHealth_Reachable5xx(t *testing.T) {
	requireLocalListener(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	result := ProbeHealth(t.Context(), srv.URL)

	if !result.Reachable {
		t.Fatalf("expected reachable even for 500, got error: %s", result.Error)
	}
}

func TestProbeHealth_Unreachable(t *testing.T) {
	// Use a port that's almost certainly not listening.
	result := ProbeHealth(t.Context(), "http://127.0.0.1:1")

	if result.Reachable {
		t.Fatal("expected unreachable for closed port")
	}

	if result.Error == "" {
		t.Fatal("expected error message for unreachable host")
	}

	if result.Host != "127.0.0.1" {
		t.Fatalf("expected host 127.0.0.1, got %q", result.Host)
	}
}

func TestProbeHealth_InvalidURL(t *testing.T) {
	result := ProbeHealth(t.Context(), "://bad")

	if result.Reachable {
		t.Fatal("expected unreachable for invalid URL")
	}

	if result.Error != "invalid URL" {
		t.Fatalf("expected 'invalid URL', got %q", result.Error)
	}
}

func TestProbeHealth_EmptyHost(t *testing.T) {
	result := ProbeHealth(t.Context(), "http://")

	if result.Reachable {
		t.Fatal("expected unreachable for empty host")
	}

	if result.Error != "invalid URL" {
		t.Fatalf("expected 'invalid URL', got %q", result.Error)
	}
}

func TestSummarizeNetworkError_Nil(t *testing.T) {
	if got := summarizeNetworkError(nil); got != "" {
		t.Fatalf("expected empty string for nil, got %q", got)
	}
}

func TestSummarizeNetworkError_ConnectionRefused(t *testing.T) {
	err := fmt.Errorf("dial tcp 127.0.0.1:1: connection refused")
	if got := summarizeNetworkError(err); got != "connection refused" {
		t.Fatalf("expected 'connection refused', got %q", got)
	}
}

func TestSummarizeNetworkError_DNS(t *testing.T) {
	err := &net.DNSError{Err: "no such host", Name: "bad.invalid"}
	if got := summarizeNetworkError(err); got != "DNS resolution failed" {
		t.Fatalf("expected 'DNS resolution failed', got %q", got)
	}
}

func TestSummarizeNetworkError_Timeout(t *testing.T) {
	err := fmt.Errorf("context deadline exceeded")
	if got := summarizeNetworkError(err); got != "connection timed out" {
		t.Fatalf("expected 'connection timed out', got %q", got)
	}
}

func TestSummarizeNetworkError_TLSCertTyped(t *testing.T) {
	err := &x509.UnknownAuthorityError{}
	if got := summarizeNetworkError(err); got != "TLS certificate error" {
		t.Fatalf("expected 'TLS certificate error', got %q", got)
	}
}

func TestSummarizeNetworkError_TLSHostnameTyped(t *testing.T) {
	err := &x509.HostnameError{Host: "example.com"}
	if got := summarizeNetworkError(err); got != "TLS certificate error" {
		t.Fatalf("expected 'TLS certificate error', got %q", got)
	}
}

func TestSummarizeNetworkError_CertificateString(t *testing.T) {
	err := fmt.Errorf("tls: failed to verify certificate")
	if got := summarizeNetworkError(err); got != "TLS certificate error" {
		t.Fatalf("expected 'TLS certificate error', got %q", got)
	}
}

func TestSummarizeNetworkError_Truncation(t *testing.T) {
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'a'
	}

	err := fmt.Errorf("%s", string(long))
	got := summarizeNetworkError(err)

	if len(got) != 120 {
		t.Fatalf("expected truncation to 120 chars, got %d", len(got))
	}
}
