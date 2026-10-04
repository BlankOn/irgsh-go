package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
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
	// uploadClient is used for submission uploads only. It deliberately has
	// no Client.Timeout: that is a deadline on the whole exchange, and a
	// submission tarball of a few hundred MB on a maintainer's uplink takes
	// longer than any fixed value worth setting. Stalls are caught by the
	// transport's dial/handshake/response-header deadlines instead, and the
	// caller's context still bounds the whole operation.
	uploadClient *http.Client
}

func NewHTTPChiefClient(configStore configLoader) *HTTPChiefClient {
	return &HTTPChiefClient{
		configStore: configStore,
		httpClient:  &http.Client{Timeout: 60 * time.Second},
		uploadClient: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   15 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				// Applies only after the body has been written, so it caps a
				// chief that accepted the upload and then went silent.
				ResponseHeaderTimeout: 10 * time.Minute,
			},
		},
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

// decodeJSON decodes a chief response, turning a non-JSON body into an error
// that says what actually came back.
//
// Chief serves its dashboard from a catch-all route, so an older server
// answers an endpoint it does not know with HTML and HTTP 200. Decoding that
// as JSON otherwise fails with a bare "invalid character '<'".
func decodeJSON(resp *http.Response, endpoint string, v any) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("failed to read the response from %s: %w", endpoint, err)
	}

	if err := json.Unmarshal(body, v); err != nil {
		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "html") || bytes.HasPrefix(bytes.TrimSpace(body), []byte("<")) {
			return fmt.Errorf("chief returned a web page instead of JSON for %s (HTTP %d). "+
				"This irgsh-chief is most likely older than your irgsh-cli and does not have that endpoint; "+
				"upgrade irgsh-chief, or check that %s points at the right server",
				endpoint, resp.StatusCode, resp.Request.URL.Host)
		}
		return fmt.Errorf("chief returned an unreadable response for %s (HTTP %d, %s): %w",
			endpoint, resp.StatusCode, contentType, err)
	}
	return nil
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
	if err := decodeJSON(resp, "/api/v1/version", &v); err != nil {
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

// countingWriter counts bytes written and discards them.
type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

// uploadPart is one file field of the submission upload form.
type uploadPart struct {
	field string
	path  string
	size  int64
}

// multipartOverhead returns the exact number of bytes a multipart body with
// the given boundary and parts costs on top of the part contents themselves,
// by writing the same form with empty contents to a counter. Knowing it lets
// the upload set a real Content-Length while streaming the parts from disk.
func multipartOverhead(boundary string, parts []uploadPart) (int64, error) {
	cw := &countingWriter{}
	w := multipart.NewWriter(cw)
	if err := w.SetBoundary(boundary); err != nil {
		return 0, err
	}
	for _, p := range parts {
		if _, err := w.CreateFormFile(p.field, path.Base(p.path)); err != nil {
			return 0, err
		}
	}
	if err := w.Close(); err != nil {
		return 0, err
	}
	return cw.n, nil
}

// uploadAttempts is how many times a submission upload is tried before giving
// up. A dropped connection partway through a large tarball is common enough on
// the links maintainers use that one attempt is not a fair test.
const uploadAttempts = 3

func (c *HTTPChiefClient) UploadSubmission(ctx context.Context, blobPath, tokenPath string, onProgress func(uploaded, total int64)) (domain.UploadResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.UploadResponse{}, err
	}

	parts := []uploadPart{{field: "blob", path: blobPath}, {field: "token", path: tokenPath}}
	for i := range parts {
		info, err := os.Stat(parts[i].path)
		if err != nil {
			return domain.UploadResponse{}, fmt.Errorf("failed to stat %s file: %w", parts[i].field, err)
		}
		parts[i].size = info.Size()
	}

	var lastErr error
	for attempt := 1; attempt <= uploadAttempts; attempt++ {
		upload, retryable, err := c.uploadOnce(ctx, base, parts, onProgress)
		if err == nil {
			return upload, nil
		}
		lastErr = err
		if !retryable || attempt == uploadAttempts || ctx.Err() != nil {
			break
		}
		backoff := time.Duration(attempt) * 5 * time.Second
		fmt.Printf("\nUpload attempt %d/%d failed (%v), retrying in %s...\n",
			attempt, uploadAttempts, err, backoff)
		select {
		case <-ctx.Done():
			return domain.UploadResponse{}, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return domain.UploadResponse{}, lastErr
}

// uploadOnce performs a single upload attempt. The multipart body is streamed
// from disk through a pipe rather than buffered, so memory use does not scale
// with the tarball. It reports whether the failure is worth retrying.
func (c *HTTPChiefClient) uploadOnce(ctx context.Context, base string, parts []uploadPart, onProgress func(uploaded, total int64)) (domain.UploadResponse, bool, error) {
	boundary := multipart.NewWriter(io.Discard).Boundary()

	overhead, err := multipartOverhead(boundary, parts)
	if err != nil {
		return domain.UploadResponse{}, false, fmt.Errorf("failed to size multipart form: %w", err)
	}
	totalSize := overhead
	for _, p := range parts {
		totalSize += p.size
	}

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	if err := writer.SetBoundary(boundary); err != nil {
		return domain.UploadResponse{}, false, err
	}

	go func() {
		pw.CloseWithError(func() error {
			for _, p := range parts {
				f, err := os.Open(p.path)
				if err != nil {
					return fmt.Errorf("failed to open %s file: %w", p.field, err)
				}
				part, err := writer.CreateFormFile(p.field, path.Base(p.path))
				if err != nil {
					f.Close()
					return fmt.Errorf("failed to create %s form field: %w", p.field, err)
				}
				if _, err := io.Copy(part, f); err != nil {
					f.Close()
					return fmt.Errorf("failed to copy %s file: %w", p.field, err)
				}
				f.Close()
			}
			return writer.Close()
		}())
	}()
	defer pr.Close()

	progressReader := io.Reader(pr)
	if onProgress != nil {
		progressReader = io.TeeReader(pr, &progressWriter{total: totalSize, onProgress: onProgress})
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/submission-upload", progressReader)
	if err != nil {
		return domain.UploadResponse{}, false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.ContentLength = totalSize

	resp, err := c.uploadClient.Do(req)
	if err != nil {
		// A connection dropped or reset mid-body is exactly the case a retry
		// exists for; a cancelled context is not.
		return domain.UploadResponse{}, ctx.Err() == nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		io.Copy(io.Discard, resp.Body) // drain remainder for connection reuse
		// 5xx and 408/429 are chief-side or transient; a 4xx means this
		// submission will be rejected the same way every time.
		retryable := resp.StatusCode >= 500 ||
			resp.StatusCode == http.StatusRequestTimeout ||
			resp.StatusCode == http.StatusTooManyRequests
		return domain.UploadResponse{}, retryable,
			fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var upload domain.UploadResponse
	if err := decodeJSON(resp, "/api/v1/submission-upload", &upload); err != nil {
		return domain.UploadResponse{}, false, err
	}
	return upload, false, nil
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
	if err := decodeJSON(resp, "/api/v1/submit", &sr); err != nil {
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
	if err := decodeJSON(resp, "/api/v1/build-iso", &sr); err != nil {
		return domain.SubmitResponse{}, err
	}
	return sr, nil
}

func (c *HTTPChiefClient) SubmitImport(ctx context.Context, submission domain.ImportSubmission) (domain.SubmitResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.SubmitResponse{}, err
	}

	jsonBytes, err := json.Marshal(submission)
	if err != nil {
		return domain.SubmitResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/import", bytes.NewReader(jsonBytes))
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
	if err := decodeJSON(resp, "/api/v1/import", &sr); err != nil {
		return domain.SubmitResponse{}, err
	}
	return sr, nil
}

func (c *HTTPChiefClient) GetRepoInfo(ctx context.Context, dist string) (domain.RepoInfo, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.RepoInfo{}, err
	}

	reqURL := base + "/api/v1/repo-info?dist=" + url.QueryEscape(dist)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return domain.RepoInfo{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.RepoInfo{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return domain.RepoInfo{}, err
	}

	var info domain.RepoInfo
	if err := decodeJSON(resp, "/api/v1/repo-info", &info); err != nil {
		return domain.RepoInfo{}, err
	}
	return info, nil
}

func (c *HTTPChiefClient) GetImportStatus(ctx context.Context, pipelineID string) (domain.ImportStatus, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.ImportStatus{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/import-status?uuid="+url.QueryEscape(pipelineID), nil)
	if err != nil {
		return domain.ImportStatus{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.ImportStatus{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return domain.ImportStatus{}, err
	}

	var is domain.ImportStatus
	if err := decodeJSON(resp, "/api/v1/import-status", &is); err != nil {
		return domain.ImportStatus{}, err
	}
	return is, nil
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
	if err := decodeJSON(resp, "/api/v1/status", &ps); err != nil {
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
	if err := decodeJSON(resp, "/api/v1/iso-status", &is); err != nil {
		return domain.ISOStatus{}, err
	}
	return is, nil
}

func (c *HTTPChiefClient) Cancel(ctx context.Context, pipelineID string) (domain.CancelResponse, error) {
	base, err := c.baseURL()
	if err != nil {
		return domain.CancelResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/cancel?uuid="+url.QueryEscape(pipelineID), nil)
	if err != nil {
		return domain.CancelResponse{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.CancelResponse{}, err
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return domain.CancelResponse{}, err
	}

	var cr domain.CancelResponse
	if err := decodeJSON(resp, "/api/v1/cancel", &cr); err != nil {
		return domain.CancelResponse{}, err
	}
	return cr, nil
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
	if err := decodeJSON(resp, "/api/v1/retry", &rr); err != nil {
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
