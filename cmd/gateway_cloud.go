package cmd

import (
	"log/slog"
	"os"

	"github.com/nextlevelbuilder/goclaw/internal/cloud"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/edition"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	httpapi "github.com/nextlevelbuilder/goclaw/internal/http"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// newCloudManager builds the shared cloud Manager for the instance (nil when
// the edition or the config kill-switch disables the Cloud surface). Google
// credentials resolve dynamically (web-UI setup form > env/config), so the
// manager exists even before credentials are configured.
func newCloudManager(cfg *config.Config, stores *store.Stores) *cloud.Manager {
	if !edition.Current().CloudAccountsEnabled || !cfg.Cloud.KillSwitchOn() {
		return nil
	}
	if stores == nil || stores.CloudAccounts == nil {
		return nil
	}
	// Same key source as the store layer (gateway_stores_pg.go): it decrypts
	// tokens inside the store and signs OAuth states with a derived key.
	manager := cloud.NewManager(cloud.CloudProviderConfig{
		GoogleClientID:     cfg.Cloud.Google.ClientID,
		GoogleClientSecret: cfg.Cloud.Google.ClientSecret,
	}, stores.CloudAccounts, os.Getenv("GOCLAW_ENCRYPTION_KEY"))
	if stores.ConfigSecrets != nil {
		manager.SetSecretsStore(stores.ConfigSecrets)
	}
	return manager
}

// wireCloud attaches the Cloud handler to the gateway. Wiring is
// unconditional in Standard editions so /v1/cloud/status can answer
// "disabled" (with reasons) instead of 404; the handler gates per request.
func wireCloud(server *gateway.Server, cfg *config.Config, stores *store.Stores) {
	if !edition.Current().CloudAccountsEnabled {
		slog.Debug("cloud: disabled by edition, handler not wired")
		return
	}
	if stores == nil || stores.CloudAccounts == nil {
		slog.Warn("cloud: account store unavailable, handler not wired")
		return
	}
	manager := newCloudManager(cfg, stores)
	enabled := manager != nil
	server.SetCloudHandler(httpapi.NewCloudHandler(
		manager, stores.CloudAccounts, enabled, cfg.Cloud.RedirectBaseURL))
}

// wireCloudTools registers the cloud agent tools (cloud_accounts, mail_*,
// cloud_*) when the edition allows. Credentials and connected accounts are
// resolved at call time, so tools work immediately after the admin saves the
// OAuth client from the web-UI setup form — no restart. Returns a cleanup
// func that stops the rclone supervisor (safe to defer).
func wireCloudTools(cfg *config.Config, stores *store.Stores, toolsReg *tools.Registry, workspace, dataDir string) func() {
	manager := newCloudManager(cfg, stores)
	if manager == nil {
		return func() {}
	}

	mailSvc := cloud.NewMailService(manager, cfg.Cloud.MailRate())
	for _, tool := range tools.NewCloudMailTools(mailSvc, cfg.Cloud.MailReadCap()).Tools() {
		toolsReg.Register(tool)
	}

	// Storage tools (rclone-backed): registered alongside mail so the tool set
	// is coherent; they degrade with a clear error when rclone is missing.
	storageSvc := cloud.NewStorageService(manager, dataDir+"/cloud")
	manager.SetStorageService(storageSvc)
	for _, tool := range tools.NewCloudStorageTools(storageSvc, workspace, int64(cfg.Cloud.FetchCapMB())).Tools() {
		toolsReg.Register(tool)
	}

	slog.Info("cloud tools registered")
	return storageSvc.Shutdown
}
