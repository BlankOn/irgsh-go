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
	"time"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/pkg/httputil"
)

// configLoader is the subset of ConfigStore needed by HTTPChiefClient.
type configLoader interface {
	Load() (domain.Config, error)
}

// HTTPChiefClient implements usecase.ChiefAPI using net/http.
type HTTPChiefClient struct {
	configStore configLoader
	httpClient  *http.Client
}

func NewHTTPChiefClient(configStore configLoader) *HTTPChiefClient {
	return &HTTPChiefClient{
		configStore: configStore,
		httpClient:  &http.Client{Timeout: 60 * time.Second},
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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	io.Copy(io.Discard, resp.Body) // drain remainder for connection reuse
	return httputil.HTTPStatusError{StatusCode: resp.StatusCode, Body: string(body)}
}

func (c *HTTPChiefClient) GetVersion(ctx context.Context) (domain.VersionResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.VersionResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/version", nil)
	if err != nil {
		return domain.VersionResponse{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.VersionResponse{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return domain.VersionResponse{}, err
	}

	var v domain.VersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return domain.VersionResponse{}, err
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

func (c *HTTPChiefClient) UploadSubmission(ctx context.Context, blobPath, tokenPath string, onProgress func(uploaded, total int64)) (domain.UploadResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.UploadResponse{}, err
	}

	blobFile, err := os.Open(blobPath)
	if err != nil {
		return domain.UploadResponse{}, fmt.Errorf("failed to open blob file: %w", err)
	}
	defer blobFile.Close()

	tokenFile, err := os.Open(tokenPath)
	if err != nil {
		return domain.UploadResponse{}, fmt.Errorf("failed to open token file: %w", err)
	}
	defer tokenFile.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	blobPart, err := writer.CreateFormFile("blob", path.Base(blobPath))
	if err != nil {
		return domain.UploadResponse{}, fmt.Errorf("failed to create blob form field: %w", err)
	}
	if _, err := io.Copy(blobPart, blobFile); err != nil {
		return domain.UploadResponse{}, fmt.Errorf("failed to copy blob file: %w", err)
	}

	tokenPart, err := writer.CreateFormFile("token", path.Base(tokenPath))
	if err != nil {
		return domain.UploadResponse{}, fmt.Errorf("failed to create token form field: %w", err)
	}
	if _, err := io.Copy(tokenPart, tokenFile); err != nil {
		return domain.UploadResponse{}, fmt.Errorf("failed to copy token file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return domain.UploadResponse{}, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	totalSize := int64(body.Len())
	pw := &progressWriter{total: totalSize, onProgress: onProgress}
	progressReader := io.TeeReader(body, pw)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/submission-upload", progressReader)
	if err != nil {
		return domain.UploadResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.ContentLength = totalSize

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.UploadResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		io.Copy(io.Discard, resp.Body) // drain remainder for connection reuse
		return domain.UploadResponse{}, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var upload domain.UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&upload); err != nil {
		return domain.UploadResponse{}, fmt.Errorf("failed to decode upload response: %w", err)
	}
	return upload, nil
}

func (c *HTTPChiefClient) SubmitPackage(ctx context.Context, submission domain.Submission) (domain.SubmitResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.SubmitResponse{}, err
	}

	jsonBytes, err := json.Marshal(submission)
	if err != nil {
		return domain.SubmitResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/submit", bytes.NewReader(jsonBytes))
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return domain.SubmitResponse{}, err
	}

	var sr domain.SubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return domain.SubmitResponse{}, err
	}
	return sr, nil
}

func (c *HTTPChiefClient) SubmitISO(ctx context.Context, submission domain.ISOSubmission) (domain.SubmitResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.SubmitResponse{}, err
	}

	jsonBytes, err := json.Marshal(submission)
	if err != nil {
		return domain.SubmitResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/build-iso", bytes.NewReader(jsonBytes))
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return domain.SubmitResponse{}, err
	}

	var sr domain.SubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return domain.SubmitResponse{}, err
	}
	return sr, nil
}

func (c *HTTPChiefClient) GetPackageStatus(ctx context.Context, pipelineID string) (domain.PackageStatus, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.PackageStatus{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/status?uuid="+url.QueryEscape(pipelineID), nil)
	if err != nil {
		return domain.PackageStatus{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.PackageStatus{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return domain.PackageStatus{}, err
	}

	var ps domain.PackageStatus
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		return domain.PackageStatus{}, err
	}
	return ps, nil
}

func (c *HTTPChiefClient) GetISOStatus(ctx context.Context, pipelineID string) (domain.ISOStatus, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.ISOStatus{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/iso-status?uuid="+url.QueryEscape(pipelineID), nil)
	if err != nil {
		return domain.ISOStatus{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.ISOStatus{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return domain.ISOStatus{}, err
	}

	var is domain.ISOStatus
	if err := json.NewDecoder(resp.Body).Decode(&is); err != nil {
		return domain.ISOStatus{}, err
	}
	return is, nil
}

func (c *HTTPChiefClient) Retry(ctx context.Context, pipelineID string) (domain.RetryResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.RetryResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/retry?uuid="+url.QueryEscape(pipelineID), nil)
	if err != nil {
		return domain.RetryResponse{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.RetryResponse{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return domain.RetryResponse{}, err
	}

	var rr domain.RetryResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return domain.RetryResponse{}, err
	}
	return rr, nil
}

func (c *HTTPChiefClient) FetchLog(ctx context.Context, logPath string) (string, error) {
	base, err := c.baseURL()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/logs/"+url.PathEscape(logPath), nil)
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
	io.Copy(io.Discard, resp.Body) // drain remainder for connection reuse
	return string(body), nil
}
