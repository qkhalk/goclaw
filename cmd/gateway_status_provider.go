package cmd

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels/telegram"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/scheduler"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// gatewayStatusProvider implements telegram.StatusProvider from live gateway
// handles: server uptime, scheduler lane utilization, session facts, and the
// summed trace cost for the session.
type gatewayStatusProvider struct {
	version  string
	server   *gateway.Server
	schedFn  func() *scheduler.Scheduler // lazy: scheduler is built after factory registration
	sessions store.SessionStore
	tracing  store.TracingStore
}

func (p *gatewayStatusProvider) Gateway(ctx context.Context) telegram.StatusGatewayInfo {
	info := telegram.StatusGatewayInfo{Version: p.version}
	if p.server != nil {
		info.StartedAt = p.server.StartedAt()
	}
	info.SystemUptime = systemUptime()
	if p.schedFn != nil {
		if sched := p.schedFn(); sched != nil {
			for _, lane := range sched.LaneStats() {
				if lane.Name == "main" {
					info.LaneName = lane.Name
					info.LaneActive = lane.Active
					info.LaneConcurrency = lane.Concurrency
					info.LanePending = lane.Pending
					break
				}
			}
		}
	}
	return info
}

func (p *gatewayStatusProvider) Session(ctx context.Context, sessionKey string) (telegram.StatusSessionInfo, bool) {
	if p.sessions == nil {
		return telegram.StatusSessionInfo{}, false
	}
	data := p.sessions.Get(ctx, sessionKey)
	if data == nil {
		return telegram.StatusSessionInfo{}, false
	}
	return telegram.StatusSessionInfo{
		Model:            data.Model,
		Provider:         data.Provider,
		UpdatedAt:        data.Updated,
		InputTokens:      data.InputTokens,
		OutputTokens:     data.OutputTokens,
		LastPromptTokens: data.LastPromptTokens,
		ContextWindow:    data.ContextWindow,
		CompactionCount:  data.CompactionCount,
	}, true
}

func (p *gatewayStatusProvider) SessionCost(ctx context.Context, sessionKey string) (float64, bool) {
	if p.tracing == nil {
		return 0, false
	}
	return p.tracing.SessionTotalCost(ctx, sessionKey)
}

// systemUptime reads the host uptime from /proc/uptime (Linux only).
// Returns 0 on any failure — the status card omits the segment then.
func systemUptime() time.Duration {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	return parseProcUptime(string(data))
}

// parseProcUptime parses "/proc/uptime" content ("seconds idle" per line).
// Pure function so tests can feed synthetic content.
func parseProcUptime(content string) time.Duration {
	field, _, _ := strings.Cut(strings.TrimSpace(content), " ")
	secs, err := strconv.ParseFloat(field, 64)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs * float64(time.Second))
}
