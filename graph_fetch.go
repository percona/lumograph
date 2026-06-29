package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// fetchSeries is responsible for constructing the HTTP request to the PMM Victoria Metrics API,
// fetching the series data, marshaling, and returning the response
func fetchSeries(lumoConfig *LumoConfig, expr, legend string) (*VMResponse, error) {

	// Handle trailing slash in endpoint URL
	urlPath, err := url.JoinPath(strings.TrimRight(lumoConfig.Endpoint, "/"), "victoriametrics/prometheus/api/v1/query_range")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateRequest, err)
	}

	// Construct the query parameters
	q := url.Values{}
	q.Set("query", interpolateGraphConfig(expr, lumoConfig))
	q.Set("step", lumoConfig.Interval)
	q.Set("start", strconv.FormatInt(lumoConfig.Start.Unix(), 10))
	q.Set("end", strconv.FormatInt(lumoConfig.End.Unix(), 10))

	urlPath += "?" + q.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlPath, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateRequest, err)
	}

	// Set the Authorization header
	req.Header.Set("Authorization", "Bearer "+lumoConfig.Token)

	if lumoConfig.Debug {
		dump, err := httputil.DumpRequestOut(req, true)
		if err == nil {
			zap.S().Debugf("--- DEBUG: HTTP Request (%s) ---\n%s\n---------------------------", legend, dump)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrExecRequest, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if lumoConfig.Debug {
		dump, err := httputil.DumpResponse(resp, true)
		if err == nil {
			zap.S().Debugf("--- DEBUG: HTTP Response (%s) ---\n%s\n----------------------------", legend, dump)
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedHTTPStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReadResponse, err)
	}

	// Unmarshal the response body into a VMResponse struct
	var vmResp VMResponse

	if err := json.Unmarshal(body, &vmResp); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParsingJSON, err)
	}

	if vmResp.Status != "success" {
		return nil, fmt.Errorf("%w: %s", ErrAPIStatus, vmResp.Status)
	}

	return &vmResp, nil
}
