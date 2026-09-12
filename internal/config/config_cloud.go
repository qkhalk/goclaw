package config

// CloudConfig enables per-user OAuth connections to cloud providers (Google
// first: Gmail + Drive). The client secret is NEVER stored in config.json —
// it comes from the GOCLAW_CLOUD_GOOGLE_CLIENT_SECRET env var (overlay in
// config_load.go) or .env.local.
type CloudConfig struct {
	// Enabled gates the whole Cloud surface (HTTP endpoints, agent tools, UI
	// section). Defaults to true when a Google client ID is configured.
	Enabled *bool `json:"enabled,omitempty"`
	// RedirectBaseURL is the public origin registered as the OAuth redirect
	// URI base (e.g. "https://goclaw.example.com"). Empty = derive from the
	// incoming request (X-Forwarded-* then r.Host), matching the MCP OAuth
	// callbackURL precedence. Must match the GCP console registration exactly.
	RedirectBaseURL string `json:"redirect_base_url,omitempty"`
	// Google OAuth client credentials (BYO client — each install registers its
	// own GCP OAuth client; testing-mode clients have 7-day refresh tokens).
	Google GoogleCloudConfig `json:"google"`
	// MailRatePerMinute caps Gmail API calls per account (token bucket).
	MailRatePerMinute int `json:"mail_rate_per_minute,omitempty"`
	// MailReadMaxBytes truncates mail_read output (default 8192).
	MailReadMaxBytes int `json:"mail_read_max_bytes,omitempty"`
	// FetchSizeCapMB caps cloud_fetch downloads into the workspace.
	FetchSizeCapMB int `json:"fetch_size_cap_mb,omitempty"`
	// RClonePath is the rclone binary for the storage layer (default "rclone").
	RClonePath string `json:"rclone_path,omitempty"`
}

// GoogleCloudConfig carries the OAuth client registration for Google.
type GoogleCloudConfig struct {
	// ClientID from the GCP console ("Web application" client). Public info.
	ClientID string `json:"client_id,omitempty"`
	// ClientSecret is usually injected via GOCLAW_CLOUD_GOOGLE_CLIENT_SECRET;
	// if set in config it is honored (self-hosted single-tenant installs may
	// prefer file config with restricted permissions).
	ClientSecret string `json:"client_secret,omitempty"`
}

// KillSwitchOn reports whether the cloud surface is allowed to run (the
// explicit enabled:false kill-switch is respected; credentials may come from
// the web-UI setup form at runtime, so their presence is checked dynamically
// by the cloud manager — not here).
func (c CloudConfig) KillSwitchOn() bool {
	return c.Enabled == nil || *c.Enabled
}

// MailRate returns the effective mail rate limit (default 10).
func (c CloudConfig) MailRate() int {
	if c.MailRatePerMinute > 0 {
		return c.MailRatePerMinute
	}
	return 10
}

// MailReadCap returns the effective mail_read truncation (default 8192).
func (c CloudConfig) MailReadCap() int {
	if c.MailReadMaxBytes > 0 {
		return c.MailReadMaxBytes
	}
	return 8192
}

// FetchCapMB returns the effective cloud_fetch cap (default 100 MB).
func (c CloudConfig) FetchCapMB() int {
	if c.FetchSizeCapMB > 0 {
		return c.FetchSizeCapMB
	}
	return 100
}

// RCloneBin returns the effective rclone binary path.
func (c CloudConfig) RCloneBin() string {
	if c.RClonePath != "" {
		return c.RClonePath
	}
	return "rclone"
}
