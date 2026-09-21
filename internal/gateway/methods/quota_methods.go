package methods

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// QuotaMethods handles quota.usage — returns per-user quota consumption for the dashboard.
// Nil-safe: returns {enabled: false} when quotaChecker is nil (quota not configured).
// When checker is nil but db is available, still queries today's summary from traces.
type QuotaMethods struct {
	checker *channels.QuotaChecker
	db      *sql.DB
}

func NewQuotaMethods(checker *channels.QuotaChecker, db *sql.DB) *QuotaMethods {
	return &QuotaMethods{checker: checker, db: db}
}

func (m *QuotaMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodQuotaUsage, m.handleUsage)
}

func (m *QuotaMethods) handleUsage(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	// tz carries the client's IANA timezone so "today" starts at the user's
	// local midnight, not UTC midnight (empty → UTC, backwards compatible).
	var params struct {
		TZ string `json:"tz"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
			return
		}
	}

	if m.checker == nil {
		result := channels.QuotaUsageResult{
			Enabled: false,
			Entries: []channels.QuotaUsageEntry{},
		}
		// Still query today's summary from traces when DB is available
		if m.db != nil {
			channels.QueryTodaySummary(ctx, m.db, &result, params.TZ)
		}
		client.SendResponse(protocol.NewOKResponse(req.ID, result))
		return
	}

	result := m.checker.Usage(ctx, params.TZ)
	client.SendResponse(protocol.NewOKResponse(req.ID, result))
}
