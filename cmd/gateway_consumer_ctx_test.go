package cmd

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

func TestApplyInboundContextLocale(t *testing.T) {
	tenant := uuid.Must(uuid.NewV7())

	t.Run("locale metadata propagates", func(t *testing.T) {
		ctx := applyInboundContext(context.Background(), bus.InboundMessage{
			TenantID: tenant,
			Metadata: map[string]string{tools.MetaUserLocale: " vi-VN "},
		})
		if got := store.TenantIDFromContext(ctx); got != tenant {
			t.Errorf("tenant: got %v, want %v", got, tenant)
		}
		if got := store.ExplicitLocaleFromContext(ctx); got != "vi-VN" {
			t.Errorf("locale: got %q, want vi-VN (trimmed, raw)", got)
		}
	})

	t.Run("no locale metadata leaves context unpinned", func(t *testing.T) {
		ctx := applyInboundContext(context.Background(), bus.InboundMessage{TenantID: tenant})
		if got := store.ExplicitLocaleFromContext(ctx); got != "" {
			t.Errorf("locale: got %q, want empty", got)
		}
	})

	t.Run("empty locale metadata leaves context unpinned", func(t *testing.T) {
		ctx := applyInboundContext(context.Background(), bus.InboundMessage{
			TenantID: tenant,
			Metadata: map[string]string{tools.MetaUserLocale: "  "},
		})
		if got := store.ExplicitLocaleFromContext(ctx); got != "" {
			t.Errorf("locale: got %q, want empty", got)
		}
	})

	t.Run("nil tenant falls back to master scope", func(t *testing.T) {
		ctx := applyInboundContext(context.Background(), bus.InboundMessage{})
		if got := store.TenantIDFromContext(ctx); got != store.MasterTenantID {
			t.Errorf("tenant: got %v, want master %v", got, store.MasterTenantID)
		}
	})
}
