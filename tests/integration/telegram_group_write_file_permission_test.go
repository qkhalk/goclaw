//go:build integration

// Regression coverage for issue #915 — Telegram group write_file permission flow.
//
// Design invariant: in a Telegram group context:
//
//	UserID   = "group:telegram:<chatID>"  (scope / memory namespace)
//	SenderID = "<numeric>"                (acting principal)
//
// CheckFileWriterPermission must evaluate the grant against the SENDER, not
// the group principal. This test mirrors gateway_consumer_normal.go:84-99
// context-build and commands_writers.go:80-93 grant shape exactly, to
// guarantee the harness matches production ingress.
package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/store/pg"
)

// Fixture A.1 — granted sender in a Telegram group can write_file.
// Scope string + UserID format copied from commands_writers.go:44,80,93.
func TestTelegramGroupWriteFilePermission_GrantedSender(t *testing.T) {
	db := testDB(t)
	pg.InitSqlx(db)
	tenantID, agentID := seedTenantAgent(t, db)

	permStore := pg.NewPGConfigPermissionStore(db)

	const (
		chatID       = "-100987654321"
		senderNumID  = "42"
		scopeFromCmd = "group:telegram:-100987654321" // commands_writers.go:44 shape
	)

	// Grant mirrors commands_writers.go:80-93 — UserID is numeric sender ID.
	ctxGrant := tenantCtx(tenantID)
	if err := permStore.Grant(ctxGrant, &store.ConfigPermission{
		AgentID:    agentID,
		Scope:      scopeFromCmd,
		ConfigType: store.ConfigTypeFileWriter,
		UserID:     senderNumID,
		Permission: "allow",
		GrantedBy:  strPtr("test-admin"),
	}); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	// Build ctx matching gateway_consumer_normal.go:84-99 group branch.
	ctx := store.WithTenantID(context.Background(), tenantID)
	ctx = store.WithUserID(ctx, "group:telegram:"+chatID)
	ctx = store.WithSenderID(ctx, senderNumID)
	ctx = store.WithAgentID(ctx, agentID)

	if err := store.CheckFileWriterPermission(ctx, permStore); err != nil {
		t.Errorf("granted sender expected nil, got: %v", err)
	}
}

// Fixture A.2 — sender without a grant hits permission denied.
func TestTelegramGroupWriteFilePermission_UngrantedSender(t *testing.T) {
	db := testDB(t)
	pg.InitSqlx(db)
	tenantID, agentID := seedTenantAgent(t, db)

	permStore := pg.NewPGConfigPermissionStore(db)
	_ = permStore // no grant

	const (
		chatID       = "-100987654321"
		uninvitedNum = "99" // never granted
	)

	ctx := store.WithTenantID(context.Background(), tenantID)
	ctx = store.WithUserID(ctx, "group:telegram:"+chatID)
	ctx = store.WithSenderID(ctx, uninvitedNum)
	ctx = store.WithAgentID(ctx, agentID)

	err := store.CheckFileWriterPermission(ctx, permStore)
	if err == nil {
		t.Fatalf("expected permission denied, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("expected permission denied error, got: %v", err)
	}
}

// Fixture A.3 — fail-open distinguisher. When agentID is missing, the
// function returns nil by design (config_permission_store.go:61-62). This
// test makes explicit that a "nil" result is ambiguous unless the test
// pins WHY (granted vs fail-open). Without this test, a silent regression
// that strips agentID upstream would look like "granted".
func TestTelegramGroupWriteFilePermission_NoAgent_FailOpen(t *testing.T) {
	db := testDB(t)
	pg.InitSqlx(db)
	tenantID, _ := seedTenantAgent(t, db)

	permStore := pg.NewPGConfigPermissionStore(db)

	ctx := store.WithTenantID(context.Background(), tenantID)
	ctx = store.WithUserID(ctx, "group:telegram:-100987654321")
	ctx = store.WithSenderID(ctx, "42")
	// Deliberately NO WithAgentID — exercises the fail-open branch.

	if err := store.CheckFileWriterPermission(ctx, permStore); err != nil {
		t.Errorf("fail-open path expected nil (no agent in ctx), got: %v", err)
	}
	// Note: a production-grade assertion here would require a log-capture
	// hook to distinguish "allowed" from "fail-open nil". Current surface
	// returns plain error/nil; documenting the ambiguity is the mitigation.
}

// Fixture A.4 — DM context (no "group:" prefix) is a no-op; always nil.
// Ensures the DM path is untouched by the permission flow.
func TestTelegramGroupWriteFilePermission_DMContextPasses(t *testing.T) {
	db := testDB(t)
	pg.InitSqlx(db)
	_, agentID := seedTenantAgent(t, db)

	permStore := pg.NewPGConfigPermissionStore(db)

	ctx := store.WithUserID(context.Background(), "user-private-42")
	ctx = store.WithSenderID(ctx, "42")
	ctx = store.WithAgentID(ctx, agentID)

	if err := store.CheckFileWriterPermission(ctx, permStore); err != nil {
		t.Errorf("DM context expected nil, got: %v", err)
	}
}

// Fixture A.5 — delimited sender ("42|name") — ingress tokens sometimes
// carry a "|" suffix. CheckFileWriterPermission splits on "|" at line 68
// (config_permission_store.go). This test pins that behavior.
func TestTelegramGroupWriteFilePermission_DelimitedSender(t *testing.T) {
	db := testDB(t)
	pg.InitSqlx(db)
	tenantID, agentID := seedTenantAgent(t, db)

	permStore := pg.NewPGConfigPermissionStore(db)

	const (
		chatID      = "-100987654321"
		senderNumID = "42"
	)

	ctxGrant := tenantCtx(tenantID)
	if err := permStore.Grant(ctxGrant, &store.ConfigPermission{
		AgentID:    agentID,
		Scope:      "group:telegram:" + chatID,
		ConfigType: store.ConfigTypeFileWriter,
		UserID:     senderNumID,
		Permission: "allow",
		GrantedBy:  strPtr("test-admin"),
	}); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	ctx := store.WithTenantID(context.Background(), tenantID)
	ctx = store.WithUserID(ctx, "group:telegram:"+chatID)
	ctx = store.WithSenderID(ctx, senderNumID+"|displayname")
	ctx = store.WithAgentID(ctx, agentID)

	if err := store.CheckFileWriterPermission(ctx, permStore); err != nil {
		t.Errorf("delimited sender expected nil after split, got: %v", err)
	}
}
