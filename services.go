package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
)

//nolint:tagliatelle // JSON is returned by Grafana
type PMMService struct {
	ServiceName    string `json:"service_name"`
	ServiceType    string `json:"service_type"`
	NodeName       string `json:"node_name"`
	ClusterName    string `json:"cluster"`
	ReplicationSet string `json:"replication_set"`
}

func getPmmServices(endpoint, token string, debug bool) ([]PMMService, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Handle trailing slash in endpoint URL
	urlPath, err := url.JoinPath(strings.TrimRight(endpoint, "/"), "v1/management/services")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateRequest, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateRequest, err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	if debug {
		dump, err := httputil.DumpRequestOut(req, true)
		if err == nil {
			zap.S().Debugf("--- DEBUG: HTTP Request ---\n%s\n---------------------------", dump)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFetchServices, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if debug {
		dump, err := httputil.DumpResponse(resp, true)
		if err == nil {
			zap.S().Debugf("--- DEBUG: HTTP Response ---\n%s\n---------------------------", dump)
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedHTTPStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReadResponse, err)
	}

	var response struct {
		Services []PMMService `json:"services"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParsingJSON, err)
	}

	return response.Services, nil
}

// Query PMM to get a list of all services, and display the result grouped by
// service type, with each group sorted by service name.
func listServices(endpoint, token string, debug bool) {

	services, err := getPmmServices(endpoint, token, debug)
	if err != nil {
		zap.S().Fatalf("Failed to retrieve services: %v", err)
	}

	// Group the services by their type
	byType := make(map[string][]string)
	for _, service := range services {
		if service.ServiceName == "" {
			continue
		}

		byType[service.ServiceType] = append(byType[service.ServiceType], service.ServiceName)
	}

	// Sort the service types so the groups are emitted deterministically
	types := slices.Collect(maps.Keys(byType))
	sort.Strings(types)

	for _, sType := range types {
		names := byType[sType]
		sort.Strings(names)

		_, _ = fmt.Fprintf(os.Stdout, "%s:\n", sType)

		for _, name := range names {
			_, _ = fmt.Fprintf(os.Stdout, "  - %s\n", name)
		}
	}
}

func getServiceByName(endpoint, token, serviceName string) (PMMService, error) {

	services, err := getPmmServices(endpoint, token, false)
	if err != nil {
		return PMMService{}, fmt.Errorf("failed to fetch services: %w", err)
	}

	for _, s := range services {
		if s.ServiceName == serviceName {
			return s, nil
		}
	}

	return PMMService{}, fmt.Errorf("%w: '%s'", ErrServiceNotFound, serviceName)
}
