package main

import (
	"crypto/tls"
	"net/http"

	"go.uber.org/zap"
)

// httpClient is the shared HTTP client used for all PMM API requests.
var httpClient = http.DefaultClient

// configureHTTPClient configures the shared HTTP client. When insecure is true,
// TLS certificate verification is disabled to support endpoints that present
// self-signed certificates (the -insecure-tls flag).
func configureHTTPClient(insecure bool) {

	if !insecure {
		return
	}

	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		zap.S().Fatal("error: unexpected default HTTP transport type")
	}

	clone := transport.Clone()
	clone.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // -insecure-tls explicitly disables verification for self-signed certs

	httpClient = &http.Client{Transport: clone}
}
