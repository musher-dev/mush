package client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// NewInstrumentedHTTPClient creates an HTTP client with OpenTelemetry transport
// and optional custom CA bundle support.
func NewInstrumentedHTTPClient(caCertFile string) (*http.Client, error) {
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default transport type %T is not *http.Transport", http.DefaultTransport)
	}

	transport := baseTransport.Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}

	customCAPath := strings.TrimSpace(caCertFile)
	if customCAPath != "" {
		pemData, err := os.ReadFile(customCAPath) //nolint:gosec // path is user-provided config
		if err != nil {
			return nil, fmt.Errorf("read CA cert file %q: %w", customCAPath, err)
		}

		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}

		if ok := pool.AppendCertsFromPEM(pemData); !ok {
			return nil, fmt.Errorf("parse CA cert file %q: no certificates found", customCAPath)
		}

		transport.TLSClientConfig.RootCAs = pool
	}

	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: otelhttp.NewTransport(transport),
	}, nil
}
