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
		GoogleClientID:        cfg.Cloud.Google.ClientID,
		GoogleClientSecret:    cfg.Cloud.Google.ClientSecret,
		MicrosoftClientID:     cfg.Cloud.Microsoft.ClientID,
		MicrosoftClientSecret: cfg.Cloud.Microsoft.ClientSecret,
	}, stores.CloudAccounts, os.Getenv("GOCLAW_ENCRYPTION_KEY"))
	if stores.ConfigSecrets != nil {
		manager.SetSecretsStore(stores.ConfigSecrets)
	}
	if bs, ok := any(stores.CloudAccounts).(store.CloudBindingStore); ok {
		manager.SetBindingStore(bs)
	}
	return manager
}

// newCloudStack builds the shared cloud Manager + rclone StorageService used
// by BOTH the HTTP handler (account detail views) and the agent tools, so
// there is exactly one rcd supervisor per gateway. Either may be nil when the
// edition/kill-switch disables the surface.
func newCloudStack(cfg *config.Config, stores *store.Stores, dataDir string) (*cloud.Manager, *cloud.StorageService, *cloud.MailService) {
	manager := newCloudManager(cfg, stores)
	if manager == nil {
		return nil, nil, nil
	}
	storageSvc := cloud.NewStorageService(manager, dataDir+"/cloud")
	manager.SetStorageService(storageSvc)
	return manager, storageSvc, cloud.NewMailService(manager, cfg.Cloud.MailRate())
}

// wireCloud attaches the Cloud handler to the gateway. Wiring is
// unconditional in Standard editions so /v1/cloud/status can answer
// "disabled" (with reasons) instead of 404; the handler gates per request.
func wireCloud(server *gateway.Server, cfg *config.Config, stores *store.Stores, manager *cloud.Manager, mailSvc *cloud.MailService) {
	if !edition.Current().CloudAccountsEnabled {
		slog.Debug("cloud: disabled by edition, handler not wired")
		return
	}
	if stores == nil || stores.CloudAccounts == nil {
		slog.Warn("cloud: account store unavailable, handler not wired")
		return
	}
	enabled := manager != nil
	server.SetCloudHandler(httpapi.NewCloudHandler(
		manager, stores.CloudAccounts, stores.Tenants, mailSvc, enabled, cfg.Cloud.RedirectBaseURL))
}

// wireCloudTools registers the cloud agent tools (cloud_accounts, mail_*,
// cloud_*) on the shared manager/stack. Credentials and connected accounts
// are resolved at call time, so tools work immediately after the admin saves
// the OAuth client from the web-UI setup form — no restart. Returns a
// cleanup func that stops the rclone supervisor (safe to defer).
func wireCloudTools(manager *cloud.Manager, storageSvc *cloud.StorageService, cfg *config.Config, toolsReg *tools.Registry, workspace string) func() {
	if manager == nil || storageSvc == nil {
		return func() {}
	}

	mailSvc := cloud.NewMailService(manager, cfg.Cloud.MailRate())
	for _, tool := range tools.NewCloudMailTools(mailSvc, cfg.Cloud.MailReadCap()).Tools() {
		toolsReg.Register(tool)
	}
	for _, tool := range tools.NewCloudStorageTools(storageSvc, workspace, int64(cfg.Cloud.FetchCapMB())).Tools() {
		toolsReg.Register(tool)
	}

	slog.Info("cloud tools registered")
	return storageSvc.Shutdown
}
