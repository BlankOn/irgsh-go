package httputil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestResponseJSON(t *testing.T) {
	// null status ok
	handler := func(w http.ResponseWriter, r *http.Request) {
		ResponseJSON(nil, http.StatusOK, w)
	}
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, body, []byte("null"))
	assert.Equal(t, w.Header(), http.Header(http.Header{"Content-Type": []string{"application/json"}}))
	assert.Equal(t, w.Code, 200)

	// interface status 500
	handler = func(w http.ResponseWriter, r *http.Request) {
		ResponseError("Not OK", http.StatusInternalServerError, w)
	}
	w = httptest.NewRecorder()
	handler(w, req)
	resp = w.Result()
	body, _ = io.ReadAll(resp.Body)

	assert.Equal(t, body, []byte(`{"message":"Not OK"}`))
	assert.Equal(t, w.Header(), http.Header(http.Header{"Content-Type": []string{"application/json"}}))
	assert.Equal(t, w.Code, 500)
}


func TestPostJSONWithRetry_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := PostJSONWithRetry(context.Background(), srv.Client(), srv.URL, map[string]string{"k": "v"}, 3, time.Millisecond, nil)
	assert.NoError(t, err)
}

func TestPostJSONWithRetry_RetryOnTransportError(t *testing.T) {
	var attempts int32
	// Server that closes connection on first attempt
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			// Force a connection reset by hijacking
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				conn.Close()
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var errCount int
	err := PostJSONWithRetry(context.Background(), srv.Client(), srv.URL, "payload", 3, time.Millisecond, func(attempt, max int, e error) {
		errCount++
	})
	// Should succeed on retry
	assert.NoError(t, err)
	assert.Equal(t, 1, errCount)
}

func TestPostJSONWithRetry_RetryOnNon2xx(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := PostJSONWithRetry(context.Background(), srv.Client(), srv.URL, "x", 3, time.Millisecond, nil)
	assert.NoError(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
}

func TestPostJSONWithRetry_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after first attempt's error callback
	err := PostJSONWithRetry(ctx, srv.Client(), srv.URL, "x", 5, time.Second, func(attempt, max int, e error) {
		if attempt == 1 {
			cancel()
		}
	})
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestPostJSONWithRetry_MarshalError(t *testing.T) {
	// Channels cannot be marshaled to JSON
	err := PostJSONWithRetry(context.Background(), http.DefaultClient, "http://localhost", make(chan int), 3, time.Millisecond, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marshal payload")
}
