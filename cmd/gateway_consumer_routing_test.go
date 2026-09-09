package cmd

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/sessions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ---- stub RoutingRulesStore honoring the ListRules ordering contract ----

type stubRoutingRulesStore struct {
	rules     []*store.RoutingRule
	listErr   error
	listCalls int
}

func (s *stubRoutingRulesStore) ListRules(_ context.Context, _ string) ([]*store.RoutingRule, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := append([]*store.RoutingRule(nil), s.rules...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *stubRoutingRulesStore) UpsertRule(_ context.Context, _ *store.RoutingRule) error {
	return errors.New("not implemented")
}

func (s *stubRoutingRulesStore) DeleteRule(_ context.Context, _, _ string) error {
	return errors.New("not implemented")
}

// ---- helpers ----

//go:fix inline
func strPtr(s string) *string { return new(s) }

func rule(id string, priority int, enabled bool, match store.RoutingRuleMatch, targetKey string) *store.RoutingRule {
	return &store.RoutingRule{
		ID:             id,
		TenantID:       store.MasterTenantID.String(),
		Priority:       priority,
		Match:          match,
		TargetAgentID:  "00000000-0000-0000-0000-0000000000aa",
		Enabled:        enabled,
		CreatedAt:      time.Now(),
		TargetAgentKey: targetKey,
	}
}

func routingTestCtx() context.Context {
	return store.WithTenantID(context.Background(), store.MasterTenantID)
}

// ---- routingRuleMatches table test ----

func TestRoutingRuleMatches(t *testing.T) {
	tests := []struct {
		name     string
		match    store.RoutingRuleMatch
		channel  string
		chatID   string
		peerKind string
		guildID  string
		want     bool
	}{
		{
			name:  "empty match matches everything (catch-all)",
			match: store.RoutingRuleMatch{},
			want:  true,
		},
		{
			name:    "channel match",
			match:   store.RoutingRuleMatch{Channel: new("telegram-bot")},
			channel: "telegram-bot",
			want:    true,
		},
		{
			name:    "channel mismatch",
			match:   store.RoutingRuleMatch{Channel: new("telegram-bot")},
			channel: "discord-main",
			want:    false,
		},
		{
			name:     "peer kind mismatch",
			match:    store.RoutingRuleMatch{PeerKind: new("direct")},
			peerKind: "group",
			want:     false,
		},
		{
			name:   "peer id mismatch",
			match:  store.RoutingRuleMatch{PeerID: new("chat-1")},
			chatID: "chat-2",
			want:   false,
		},
		{
			name:    "guild id mismatch",
			match:   store.RoutingRuleMatch{GuildID: new("guild-1")},
			guildID: "guild-2",
			want:    false,
		},
		{
			name:  "account id never matches (no inbound account context)",
			match: store.RoutingRuleMatch{AccountID: new("acc-1")},
			want:  false,
		},
		{
			name:     "combined all-equal matches",
			match:    store.RoutingRuleMatch{Channel: new("discord-main"), PeerKind: new("group"), PeerID: new("c1"), GuildID: new("g1")},
			channel:  "discord-main",
			chatID:   "c1",
			peerKind: "group",
			guildID:  "g1",
			want:     true,
		},
		{
			name:     "combined one-mismatch fails",
			match:    store.RoutingRuleMatch{Channel: new("discord-main"), PeerKind: new("group"), PeerID: new("c1")},
			channel:  "discord-main",
			chatID:   "OTHER",
			peerKind: "group",
			want:     false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := routingRuleMatches(tc.match, tc.channel, tc.chatID, tc.peerKind, tc.guildID); got != tc.want {
				t.Fatalf("routingRuleMatches(%+v, %q, %q, %q, %q) = %v, want %v",
					tc.match, tc.channel, tc.chatID, tc.peerKind, tc.guildID, got, tc.want)
			}
		})
	}
}

// ---- matchRoutingRules table test ----

func TestMatchRoutingRules(t *testing.T) {
	t.Run("lowest priority number wins, disabled skipped, first match wins on ties", func(t *testing.T) {
		stub := &stubRoutingRulesStore{rules: []*store.RoutingRule{
			rule("r-10", 10, true, store.RoutingRuleMatch{PeerID: new("other-chat")}, "specific-agent"),
			rule("r-20-disabled", 20, false, store.RoutingRuleMatch{}, "disabled-agent"),
			rule("r-20", 20, true, store.RoutingRuleMatch{}, "catch-all-agent"),
			rule("r-30", 30, true, store.RoutingRuleMatch{}, "fallback-agent"),
		}}
		agent, ok := matchRoutingRules(routingTestCtx(), stub, "telegram-bot", "chat-9", string(sessions.PeerDirect), "")
		if !ok || agent != "catch-all-agent" {
			t.Fatalf("got %q ok=%v, want catch-all-agent (r-20-disabled skipped, tie broken by insertion)", agent, ok)
		}
	})

	t.Run("no match falls through", func(t *testing.T) {
		stub := &stubRoutingRulesStore{rules: []*store.RoutingRule{
			rule("r-1", 10, true, store.RoutingRuleMatch{PeerID: new("chat-1")}, "agent-1"),
		}}
		if _, ok := matchRoutingRules(routingTestCtx(), stub, "telegram-bot", "chat-OTHER", string(sessions.PeerDirect), ""); ok {
			t.Fatal("non-matching rule set must fall through")
		}
	})

	t.Run("lookup error falls through", func(t *testing.T) {
		stub := &stubRoutingRulesStore{listErr: errors.New("db down")}
		if _, ok := matchRoutingRules(routingTestCtx(), stub, "c", "ch", "direct", ""); ok {
			t.Fatal("rules failure must fall through")
		}
	})

	t.Run("nil tenant context falls through", func(t *testing.T) {
		stub := &stubRoutingRulesStore{}
		if _, ok := matchRoutingRules(context.Background(), stub, "c", "ch", "direct", ""); ok {
			t.Fatal("rules require a tenant scope")
		}
		if stub.listCalls != 0 {
			t.Fatal("store must not be queried without tenant scope")
		}
	})

	t.Run("match with unresolvable target is skipped", func(t *testing.T) {
		stub := &stubRoutingRulesStore{rules: []*store.RoutingRule{
			rule("r-broken", 10, true, store.RoutingRuleMatch{}, ""),
			rule("r-good", 20, true, store.RoutingRuleMatch{}, "good-agent"),
		}}
		agent, ok := matchRoutingRules(routingTestCtx(), stub, "c", "ch", "direct", "")
		if !ok || agent != "good-agent" {
			t.Fatalf("got %q ok=%v, want good-agent", agent, ok)
		}
	})
}

// ---- full-chain priority: peer bindings → rules → channel bindings → default ----

func TestResolveAgentRouteForInboundWithRules_ChainPriority(t *testing.T) {
	dbDefault := defaultAgentGetterStub{agent: &store.AgentData{AgentKey: "db-default"}}
	rules := &stubRoutingRulesStore{rules: []*store.RoutingRule{
		rule("r-peer", 10, true, store.RoutingRuleMatch{PeerID: new("vip-chat")}, "rules-peer-agent"),
		rule("r-channel", 20, true, store.RoutingRuleMatch{Channel: new("telegram-bot")}, "rules-channel-agent"),
	}}
	peerBinding := config.AgentBinding{
		AgentID: "binding-peer-agent",
		Match:   config.BindingMatch{Channel: "telegram-bot", Peer: &config.BindingPeer{Kind: "direct", ID: "vip-chat"}},
	}
	channelBinding := config.AgentBinding{
		AgentID: "binding-channel-agent",
		Match:   config.BindingMatch{Channel: "telegram-bot"},
	}

	t.Run("peer binding beats rules", func(t *testing.T) {
		cfg := &config.Config{Bindings: []config.AgentBinding{channelBinding, peerBinding}}
		got := resolveAgentRouteForInboundWithRules(routingTestCtx(), cfg, dbDefault, rules,
			"telegram-bot", "vip-chat", string(sessions.PeerDirect), "")
		if got != "binding-peer-agent" {
			t.Fatalf("got %q, want binding-peer-agent", got)
		}
	})

	t.Run("rules beat channel binding", func(t *testing.T) {
		cfg := &config.Config{Bindings: []config.AgentBinding{channelBinding}}
		got := resolveAgentRouteForInboundWithRules(routingTestCtx(), cfg, dbDefault, rules,
			"telegram-bot", "vip-chat", string(sessions.PeerDirect), "")
		if got != "rules-peer-agent" {
			t.Fatalf("got %q, want rules-peer-agent", got)
		}
	})

	t.Run("channel-level rule beats channel binding for other chats", func(t *testing.T) {
		cfg := &config.Config{Bindings: []config.AgentBinding{channelBinding}}
		got := resolveAgentRouteForInboundWithRules(routingTestCtx(), cfg, dbDefault, rules,
			"telegram-bot", "regular-chat", string(sessions.PeerGroup), "")
		if got != "rules-channel-agent" {
			t.Fatalf("got %q, want rules-channel-agent", got)
		}
	})

	t.Run("channel binding wins when rules do not match channel", func(t *testing.T) {
		discordBinding := config.AgentBinding{
			AgentID: "binding-discord-agent",
			Match:   config.BindingMatch{Channel: "discord-main"},
		}
		cfg := &config.Config{Bindings: []config.AgentBinding{discordBinding}}
		got := resolveAgentRouteForInboundWithRules(routingTestCtx(), cfg, dbDefault, rules,
			"discord-main", "regular-chat", string(sessions.PeerGroup), "")
		if got != "binding-discord-agent" {
			t.Fatalf("got %q, want binding-discord-agent", got)
		}
	})

	t.Run("db default wins when neither bindings nor rules match", func(t *testing.T) {
		cfg := &config.Config{}
		got := resolveAgentRouteForInboundWithRules(routingTestCtx(), cfg, dbDefault, rules,
			"slack-work", "chat-x", string(sessions.PeerDirect), "")
		if got != "db-default" {
			t.Fatalf("got %q, want db-default", got)
		}
	})

	t.Run("rules lookup error falls back to bindings", func(t *testing.T) {
		broken := &stubRoutingRulesStore{listErr: errors.New("db down")}
		cfg := &config.Config{Bindings: []config.AgentBinding{channelBinding}}
		got := resolveAgentRouteForInboundWithRules(routingTestCtx(), cfg, dbDefault, broken,
			"telegram-bot", "regular-chat", string(sessions.PeerGroup), "")
		if got != "binding-channel-agent" {
			t.Fatalf("got %q, want binding-channel-agent (legacy fallback)", got)
		}
	})
}
