package cloud

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/store/pg"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestRealDBResolverProbe is a manually-gated integration probe: with
// RESOLVER_PROBE_DSN set it connects to a live database and verifies that a
// Telegram user resolves the tenant-shared OneDrive account through
// Manager.ResolveAccount (the exact path the agent tools take).
func TestRealDBResolverProbe(t *testing.T) {
	dsn := os.Getenv("RESOLVER_PROBE_DSN")
	if dsn == "" {
		t.Skip("RESOLVER_PROBE_DSN not set — manual probe only")
	}
	encKey := os.Getenv("GOCLAW_ENCRYPTION_KEY")
	tenantStr := os.Getenv("RESOLVER_PROBE_TENANT")
	userID := os.Getenv("RESOLVER_PROBE_USER")
	if tenantStr == "" || userID == "" {
		t.Fatal("RESOLVER_PROBE_TENANT and RESOLVER_PROBE_USER are required")
	}

	db, err := pg.OpenDB(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()
	accounts := pg.NewPGCloudAccountStore(db, encKey)

	tenantID, err := uuid.Parse(tenantStr)
	if err != nil {
		t.Fatalf("parse tenant: %v", err)
	}
	ctx := store.WithUserID(store.WithTenantID(context.Background(), tenantID), userID)

	own, err := accounts.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	shared, err := accounts.ListShared(ctx)
	if err != nil {
		t.Fatalf("ListShared: %v", err)
	}
	t.Logf("own=%d shared=%d", len(own), len(shared))

	m := NewManager(CloudProviderConfig{}, accounts, encKey)
	m.SetBindingStore(accounts)
	m.SetSecretsStore(nil)

	acct, err := m.ResolveAccount(ctx, "", []string{GoogleProvider, MicrosoftProvider}, func(a *store.CloudAccount) bool {
		return a.Provider == GoogleProvider || a.Provider == MicrosoftProvider
	})
	if err != nil {
		t.Fatalf("ResolveAccount: %v", err)
	}
	t.Logf("resolved: provider=%s email=%s shared=%v owner=%s", acct.Provider, acct.Email, acct.Shared, acct.UserID)

	// The token source must also resolve for this user (shared account).
	if _, err := m.TokenSource(ctx, acct.ID); err != nil {
		t.Fatalf("TokenSource for %s on shared account: %v", userID, err)
	}
	t.Log("TokenSource OK (refresh path works for the telegram user)")
}
