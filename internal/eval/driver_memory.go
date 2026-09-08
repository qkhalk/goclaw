package eval

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// MemoryDriver evaluates memory-fabric isolation: whose memories surface for
// whom. This is the eval answer to "agent nhớ nhầm người" — every case runs
// the real SearchMemories SQL gates, not a mock, so a WHERE-clause regression
// fails here before it reaches users.
type MemoryDriver struct {
	tenantID    uuid.UUID
	otherTenant uuid.UUID
	cleanup     func()
	otherCleanup func()
}

func init() { RegisterDriver(&MemoryDriver{}) }

func (d *MemoryDriver) Name() string { return "memory" }

func (d *MemoryDriver) Setup(env *Env, runID string) (func(), error) {
	d.tenantID, d.cleanup = env.SeedTenant("mem-" + runID)
	d.otherTenant, d.otherCleanup = env.SeedTenant("memx-" + runID)
	return func() {
		d.cleanup()
		d.otherCleanup()
	}, nil
}

// ident maps a logical name (alice) to a per-run unique store identity so
// repeated eval runs never collide with earlier rows.
func (d *MemoryDriver) ident(runID, name string) string {
	if name == "" {
		return ""
	}
	return name + "-" + runID
}

func (d *MemoryDriver) Run(ctx context.Context, env *Env, runID string, c EvalCase) (string, error) {
	if c.Seed == nil || c.Act == nil || c.Expect == nil {
		return "", fmt.Errorf("memory case requires seed, act and expect")
	}
	tctx := env.TenantCtx(d.tenantID)

	for _, m := range c.Seed.Memories {
		if !store.ValidMemoryScope(m.Scope) {
			return "", fmt.Errorf("seed: invalid scope %q", m.Scope)
		}
		if !store.ValidMemoryKind(m.Kind) {
			return "", fmt.Errorf("seed: invalid kind %q", m.Kind)
		}
		mem := &store.Memory{
			Scope:      m.Scope,
			Kind:       m.Kind,
			Content:    m.Content,
			SourceType: m.Source,
			Confidence: m.Confidence,
		}
		if mem.SourceType == "" {
			mem.SourceType = "manual"
		}
		if user := d.ident(runID, m.User); user != "" {
			mem.UserID = &user
		}
		if agent := d.ident(runID, m.Agent); agent != "" {
			mem.AgentID = &agent
		}
		if sess := d.ident(runID, m.Session); sess != "" {
			mem.SessionKey = &sess
		}
		if err := env.MemoryFabric.WriteMemory(tctx, mem); err != nil {
			return "", fmt.Errorf("seed write (scope=%s user=%s): %w", m.Scope, m.User, err)
		}
	}

	detail, err := d.check(tctx, env, runID, c.Act.Search, c.Expect)
	if err != nil {
		return detail, err
	}

	if c.Positive != nil {
		posExpect := &MemoryExpect{Contains: c.Positive.Contains, MinResults: 1}
		if _, err := d.check(tctx, env, runID, c.Positive.Search, posExpect); err != nil {
			return "", fmt.Errorf("positive control: %w", err)
		}
	}
	return detail, nil
}

// check executes one search and applies the expectation, returning a short
// detail line describing what was retrieved. A cross_tenant act runs from the
// second tenant's context to prove the tenant hard gate.
func (d *MemoryDriver) check(ctx context.Context, env *Env, runID string, spec MemoryQuerySpec, exp *MemoryExpect) (string, error) {
	q := store.MemoryQuery{
		UserID:    d.ident(runID, spec.User),
		AgentID:   d.ident(runID, spec.Agent),
		SessionKey: d.ident(runID, spec.Session),
		Scopes:    spec.Scopes,
		Kinds:     spec.Kinds,
		Limit:     spec.Limit,
	}
	searchCtx := ctx
	if spec.CrossTenant {
		searchCtx = env.TenantCtx(d.otherTenant)
	}
	results, err := env.MemoryFabric.SearchMemories(searchCtx, q)
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}

	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("[%s/%s/%s] %s", r.Scope, r.Kind, r.Status, r.Content))
	}
	detail := sb.String()

	var failures []string
	for _, s := range exp.NotContains {
		if strings.Contains(detail, s) {
			failures = append(failures, fmt.Sprintf("results must not contain %q (leak)", s))
		}
	}
	for _, s := range exp.Contains {
		if !strings.Contains(detail, s) {
			failures = append(failures, fmt.Sprintf("results must contain %q", s))
		}
	}
	if exp.MinResults > 0 && len(results) < exp.MinResults {
		failures = append(failures, fmt.Sprintf("need >= %d results, got %d", exp.MinResults, len(results)))
	}
	if exp.MaxResults > 0 && len(results) > exp.MaxResults {
		failures = append(failures, fmt.Sprintf("need <= %d results, got %d", exp.MaxResults, len(results)))
	}
	if len(failures) > 0 {
		return detail, fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return detail, nil
}
