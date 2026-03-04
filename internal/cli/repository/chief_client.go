package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/blankon/irgsh-go/internal/cli/entity"
)

// configLoader is the subset of ConfigStore needed by HTTPChiefClient.
type configLoader interface {
	Load() (entity.Config, error)
}

// HTTPChiefClient implements usecase.ChiefAPI using net/http.
type HTTPChiefClient struct {
	configStore configLoader
	httpClient  *http.Client
}

func NewHTTPChiefClient(configStore configLoader) *HTTPChiefClient {
	return &HTTPChiefClient{
		configStore: configStore,
		httpClient:  &http.Client{},
	}
}

func (c *HTTPChiefClient) baseURL() (string, error) {
	cfg, err := c.configStore.Load()
	if err != nil {
		return "", err
	}
	return cfg.ChiefAddress, nil
}

func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
}

func (c *HTTPChiefClient) GetVersion(ctx context.Context) (entity.VersionResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return entity.VersionResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/version", nil)
	if err != nil {
		return entity.VersionResponse{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return entity.VersionResponse{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return entity.VersionResponse{}, err
	}

	var v entity.VersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return entity.VersionResponse{}, err
	}
	return v, nil
}

// progressWriter tracks upload progress.
type progressWriter struct {
	total      int64
	uploaded   int64
	onProgress func(uploaded, total int64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.uploaded += int64(n)
	if pw.onProgress != nil {
		pw.onProgress(pw.uploaded, pw.total)
	}
	return n, nil
}

func (c *HTTPChiefClient) UploadSubmission(ctx context.Context, blobPath, tokenPath string, onProgress func(uploaded, total int64)) (entity.UploadResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return entity.UploadResponse{}, err
	}

	blobFile, err := os.Open(blobPath)
	if err != nil {
		return entity.UploadResponse{}, fmt.Errorf("failed to open blob file: %w", err)
	}
	defer blobFile.Close()

	tokenFile, err := os.Open(tokenPath)
	if err != nil {
		return entity.UploadResponse{}, fmt.Errorf("failed to open token file: %w", err)
	}
	defer tokenFile.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	blobPart, err := writer.CreateFormFile("blob", path.Base(blobPath))
	if err != nil {
		return entity.UploadResponse{}, fmt.Errorf("failed to create blob form field: %w", err)
	}
	if _, err := io.Copy(blobPart, blobFile); err != nil {
		return entity.UploadResponse{}, fmt.Errorf("failed to copy blob file: %w", err)
	}

	tokenPart, err := writer.CreateFormFile("token", path.Base(tokenPath))
	if err != nil {
		return entity.UploadResponse{}, fmt.Errorf("failed to create token form field: %w", err)
	}
	if _, err := io.Copy(tokenPart, tokenFile); err != nil {
		return entity.UploadResponse{}, fmt.Errorf("failed to copy token file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return entity.UploadResponse{}, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	totalSize := int64(body.Len())
	pw := &progressWriter{total: totalSize, onProgress: onProgress}
	progressReader := io.TeeReader(body, pw)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/submission-upload", progressReader)
	if err != nil {
		return entity.UploadResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.ContentLength = totalSize

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return entity.UploadResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return entity.UploadResponse{}, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var upload entity.UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&upload); err != nil {
		return entity.UploadResponse{}, fmt.Errorf("failed to decode upload response: %w", err)
	}
	return upload, nil
}

func (c *HTTPChiefClient) SubmitPackage(ctx context.Context, submission entity.Submission) (entity.SubmitResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return entity.SubmitResponse{}, err
	}

	jsonBytes, err := json.Marshal(submission)
	if err != nil {
		return entity.SubmitResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/submit", bytes.NewReader(jsonBytes))
	if err != nil {
		return entity.SubmitResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return entity.SubmitResponse{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return entity.SubmitResponse{}, err
	}

	var sr entity.SubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return entity.SubmitResponse{}, err
	}
	return sr, nil
}

func (c *HTTPChiefClient) SubmitISO(ctx context.Context, submission entity.ISOSubmission) (entity.SubmitResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return entity.SubmitResponse{}, err
	}

	jsonBytes, err := json.Marshal(submission)
	if err != nil {
		return entity.SubmitResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/build-iso", bytes.NewReader(jsonBytes))
	if err != nil {
		return entity.SubmitResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return entity.SubmitResponse{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return entity.SubmitResponse{}, err
	}

	var sr entity.SubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return entity.SubmitResponse{}, err
	}
	return sr, nil
}

func (c *HTTPChiefClient) GetPackageStatus(ctx context.Context, pipelineID string) (entity.PackageStatus, error) {
	base, err := c.baseURL()
	if err != nil {
		return entity.PackageStatus{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/status?uuid="+url.QueryEscape(pipelineID), nil)
	if err != nil {
		return entity.PackageStatus{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return entity.PackageStatus{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return entity.PackageStatus{}, err
	}

	var ps entity.PackageStatus
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		return entity.PackageStatus{}, err
	}
	return ps, nil
}

func (c *HTTPChiefClient) GetISOStatus(ctx context.Context, pipelineID string) (entity.ISOStatus, error) {
	base, err := c.baseURL()
	if err != nil {
		return entity.ISOStatus{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/iso-status?uuid="+url.QueryEscape(pipelineID), nil)
	if err != nil {
		return entity.ISOStatus{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return entity.ISOStatus{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return entity.ISOStatus{}, err
	}

	var is entity.ISOStatus
	if err := json.NewDecoder(resp.Body).Decode(&is); err != nil {
		return entity.ISOStatus{}, err
	}
	return is, nil
}

func (c *HTTPChiefClient) Retry(ctx context.Context, pipelineID string) (entity.RetryResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return entity.RetryResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/retry?uuid="+url.QueryEscape(pipelineID), nil)
	if err != nil {
		return entity.RetryResponse{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return entity.RetryResponse{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return entity.RetryResponse{}, err
	}

	var rr entity.RetryResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return entity.RetryResponse{}, err
	}
	return rr, nil
}

func (c *HTTPChiefClient) FetchLog(ctx context.Context, logPath string) (string, error) {
	base, err := c.baseURL()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/logs/"+logPath, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return "", err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}
