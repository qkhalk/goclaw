package config

import "time"

// VideoConfig controls the video render pipeline (worker sidecar + gateway
// dispatcher). Secret fields (WorkerToken) should come from env overlay
// (GOCLAW_VIDEO_WORKER_TOKEN) — never stored in config.json.
type VideoConfig struct {
	// Enabled gates the entire video surface (HTTP API, dispatcher, agent
	// tool). Default: true (kill-switch on).
	Enabled *bool `json:"enabled,omitempty"`
	// WorkerURL is the base URL of the videoworker sidecar
	// (e.g. "http://127.0.0.1:18791"). Required when enabled.
	WorkerURL string `json:"worker_url,omitempty"`
	// WorkerToken is the shared Bearer secret for authenticating with the
	// worker. Injected via GOCLAW_VIDEO_WORKER_TOKEN env var.
	WorkerToken string `json:"worker_token,omitempty"`
	// JobTimeoutSec is how long the gateway waits for a worker job before
	// marking it failed (default 3600 = 1 hour).
	JobTimeoutSec int `json:"job_timeout_sec,omitempty"`
	// PollIntervalSec is how often the dispatcher polls the worker for
	// progress (default 5).
	PollIntervalSec int `json:"poll_interval_sec,omitempty"`
}

// VideoEnabled reports whether the video surface is allowed to run.
func (c VideoConfig) VideoEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// EffectiveJobTimeout returns the job timeout (default 1h).
func (c VideoConfig) EffectiveJobTimeout() time.Duration {
	if c.JobTimeoutSec > 0 {
		return time.Duration(c.JobTimeoutSec) * time.Second
	}
	return time.Hour
}

// EffectivePollInterval returns the poll interval (default 5s).
func (c VideoConfig) EffectivePollInterval() time.Duration {
	if c.PollIntervalSec > 0 {
		return time.Duration(c.PollIntervalSec) * time.Second
	}
	return 5 * time.Second
}
