package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// dipperUploadURL is the Dipper endpoint that receives the image archive.
const dipperUploadURL = "https://dipper.perconatest.com/mizar/pmm"

// executeDipperSync archives the images in the supplied directory and uploads
// the resulting .tar.gz to Dipper.
func executeDipperSync(cfg *LumoConfig) {

	validateDipperSyncFlags(cfg)

	zap.S().Infof("Archiving images from '%s'...", cfg.SyncDir)

	archive, count, err := createImageArchive(cfg.SyncDir)
	if err != nil {
		zap.S().Fatalf("error creating archive: %v", err)
	}

	zap.S().Infof("Compressed %d image(s) (%d bytes). Uploading to Dipper...", count, len(archive))

	if err := uploadArchive(cfg, archive); err != nil {
		zap.S().Fatalf("error uploading archive: %v", err)
	}

	zap.S().Infof("Successfully uploaded %d image(s) for project '%s' (host '%s')", count, cfg.DipperProjectID, cfg.Hostname)
}

// validateDipperSyncFlags ensures all required flags and the positional
// argument are present and correctly formatted.
func validateDipperSyncFlags(cfg *LumoConfig) {

	if cfg.DipperToken == "" {
		zap.S().Fatalf("error: -token %v", ErrFlagRequired)
	}

	if cfg.DipperProjectID == "" {
		zap.S().Fatalf("error: -projectid %v", ErrFlagRequired)
	}

	if cfg.Hostname == "" {
		zap.S().Fatalf("error: -hostname %v", ErrFlagRequired)
	}

	if cfg.SyncDir == "" {
		zap.S().Fatalf("error: %v", ErrSyncDirRequired)
	}

	if !dipperProjectIDRe.MatchString(cfg.DipperProjectID) {
		zap.S().Fatalf("error: %v (got %q)", ErrInvalidDipperProjectID, cfg.DipperProjectID)
	}

	// Deliberately avoid logging the token value itself
	if !dipperTokenRe.MatchString(cfg.DipperToken) {
		zap.S().Fatalf("error: %v", ErrInvalidDipperToken)
	}

	info, err := os.Stat(cfg.SyncDir)
	if err != nil {
		zap.S().Fatalf("error: cannot access directory '%s': %v", cfg.SyncDir, err)
	}

	if !info.IsDir() {
		zap.S().Fatalf("error: %v: '%s'", ErrNotADirectory, cfg.SyncDir)
	}
}

// createImageArchive compresses every image file in dir into a .tar.gz,
// returning the archive bytes and the number of files included.
func createImageArchive(dir string) ([]byte, int, error) {

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrReadingDir, err)
	}

	var buf bytes.Buffer

	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	count := 0

	for _, entry := range entries {

		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".png") {
			continue
		}

		if err := addFileToArchive(tw, dir, entry.Name()); err != nil {
			return nil, 0, err
		}

		count++
	}

	// The tar and gzip writers must be closed before the buffer is read
	if err := tw.Close(); err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrArchive, err)
	}

	if err := gw.Close(); err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrArchive, err)
	}

	if count == 0 {
		return nil, 0, ErrNoImages
	}

	return buf.Bytes(), count, nil
}

// addFileToArchive writes a single file into the tar writer.
func addFileToArchive(tw *tar.Writer, dir, name string) error {

	data, err := os.ReadFile(filepath.Join(dir, name)) // #nosec
	if err != nil {
		return fmt.Errorf("%w: %w", ErrReadingFile, err)
	}

	hdr := &tar.Header{
		Name:    name,
		Mode:    0o600,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("%w: %w", ErrArchive, err)
	}

	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("%w: %w", ErrArchive, err)
	}

	return nil
}

// uploadArchive POSTs the archive to Dipper as multipart/form-data along with
// the project_id and hostname fields and the X-Dipper-Auth header.
func uploadArchive(cfg *LumoConfig, archive []byte) error {

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("project_id", cfg.DipperProjectID); err != nil {
		return fmt.Errorf("%w: %w", ErrArchive, err)
	}

	if err := writer.WriteField("hostname", cfg.Hostname); err != nil {
		return fmt.Errorf("%w: %w", ErrArchive, err)
	}

	part, err := writer.CreateFormFile("file", cfg.Hostname+".tar.gz")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrArchive, err)
	}

	if _, err := part.Write(archive); err != nil {
		return fmt.Errorf("%w: %w", ErrArchive, err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("%w: %w", ErrArchive, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dipperUploadURL, &body)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCreateRequest, err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Dipper-Auth", cfg.DipperToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrExecRequest, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, _ := io.ReadAll(resp.Body)

		return fmt.Errorf("%w: HTTP %d: %s", ErrUploadFailed, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}
