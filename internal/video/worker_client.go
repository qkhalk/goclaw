package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// WorkerClient is an HTTP client for communicating with the videoworker sidecar.
// Pattern: RCClient.do() at internal/cloud/storage/rc_client.go.
type WorkerClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewWorkerClient creates a WorkerClient with the given worker URL and bearer
// token. submitTimeout controls POST /v1/jobs; pollTimeout controls GET/POST.
func NewWorkerClient(baseURL, token string, submitTimeout, pollTimeout time.Duration) *WorkerClient {
	if submitTimeout <= 0 {
		submitTimeout = 10 * time.Second
	}
	if pollTimeout <= 0 {
		pollTimeout = 5 * time.Second
	}
	// The main client uses submitTimeout; poll operations override per-call.
	return &WorkerClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: submitTimeout,
		},
	}
}

// SubmitJob sends a render job to the worker and returns the response.
func (c *WorkerClient) SubmitJob(ctx context.Context, job *SubmitJob) (*SubmitJobResponse, error) {
	var resp SubmitJobResponse
	if err := c.do(ctx, http.MethodPost, "/v1/jobs", job, &resp, 10*time.Second); err != nil {
		return nil, fmt.Errorf("submit job: %w", err)
	}
	return &resp, nil
}

// GetJobStatus polls the worker for a job's current state.
func (c *WorkerClient) GetJobStatus(ctx context.Context, jobID string) (*JobState, error) {
	var state JobState
	if err := c.do(ctx, http.MethodGet, "/v1/jobs/"+jobID, nil, &state, 5*time.Second); err != nil {
		return nil, fmt.Errorf("get job status: %w", err)
	}
	return &state, nil
}

// CancelJob requests cancellation of a running/queued job on the worker.
func (c *WorkerClient) CancelJob(ctx context.Context, jobID string) (*CancelResponse, error) {
	var resp CancelResponse
	if err := c.do(ctx, http.MethodPost, "/v1/jobs/"+jobID+"/cancel", nil, &resp, 5*time.Second); err != nil {
		return nil, fmt.Errorf("cancel job: %w", err)
	}
	return &resp, nil
}

// HealthCheck pings the worker's /health endpoint.
func (c *WorkerClient) HealthCheck(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/health", nil, nil, 2*time.Second)
}

// do is the shared HTTP helper — builds the request, sets auth, and decodes
// the response into out (nil for void endpoints).
func (c *WorkerClient) do(ctx context.Context, method, path string, body any, out any, timeout time.Duration) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	client := c.httpClient
	if timeout > 0 {
		client = &http.Client{Timeout: timeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		slog.Warn("video.worker_client: error response",
			"method", method, "path", path, "status", resp.StatusCode, "body", string(bodyBytes))
		return fmt.Errorf("worker returned %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
