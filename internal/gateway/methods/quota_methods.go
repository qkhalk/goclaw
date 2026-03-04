package methods

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// QuotaMethods handles quota.usage — returns per-user quota consumption for the dashboard.
// Nil-safe: returns {enabled: false} when quotaChecker is nil (standalone mode).
type QuotaMethods struct {
	checker *channels.QuotaChecker
}

func NewQuotaMethods(checker *channels.QuotaChecker) *QuotaMethods {
	return &QuotaMethods{checker: checker}
}

func (m *QuotaMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodQuotaUsage, m.handleUsage)
}

func (m *QuotaMethods) handleUsage(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if m.checker == nil {
		client.SendResponse(protocol.NewOKResponse(req.ID, channels.QuotaUsageResult{
			Enabled: false,
			Entries: []channels.QuotaUsageEntry{},
		}))
		return
	}

	result := m.checker.Usage(ctx)
	client.SendResponse(protocol.NewOKResponse(req.ID, result))
}
