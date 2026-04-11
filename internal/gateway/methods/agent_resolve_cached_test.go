package methods

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// errSentinelMiss is a sentinel error used to verify DB fallback in cache-miss
// paths. The value is stable across tests so equality checks are reliable.
var errSentinelMiss = errors.New("sentinel: agent miss")

// errorAgentStore embeds store.AgentStore and overrides only GetByID and
// GetByKey to return a sentinel error. All other methods retain the nil embed,
// so any unexpected call path panics — loud failure is intentional.
type errorAgentStore struct {
	store.AgentStore
	err error
}

func (s *errorAgentStore) GetByID(context.Context, uuid.UUID) (*store.AgentData, error) {
	return nil, s.err
}

func (s *errorAgentStore) GetByKey(context.Context, string) (*store.AgentData, error) {
	return nil, s.err
}

// TestResolveAgentUUIDCached_NilRouterCallsResolveAgentUUID verifies that when
// router is nil the helper delegates directly to resolveAgentUUID — exercising
// the slow DB path. With a stub store returning a sentinel error, the helper
// must propagate that error unchanged.
func TestResolveAgentUUIDCached_NilRouterCallsResolveAgentUUID(t *testing.T) {
	stub := &errorAgentStore{err: errSentinelMiss}

	_, err := resolveAgentUUIDCached(context.Background(), nil, stub, uuid.New().String())

	if !errors.Is(err, errSentinelMiss) {
		t.Errorf("expected sentinel miss error, got %v", err)
	}
}

// TestResolveAgentUUIDCached_CacheMissFallsBack — when the router is set but
// has no cached entry for the given agent_key, the helper must fall back to
// the DB path. The stub store's sentinel error confirms we took the fallback.
func TestResolveAgentUUIDCached_CacheMissFallsBack(t *testing.T) {
	r := agent.NewRouter()
	stub := &errorAgentStore{err: errSentinelMiss}

	_, err := resolveAgentUUIDCached(context.Background(), r, stub, "missing-agent-key")

	if !errors.Is(err, errSentinelMiss) {
		t.Errorf("expected sentinel miss error, got %v", err)
	}
}

// TestResolveAgentUUIDCached_UUIDInputTakesDBPath — per Phase 2 H1, a caller
// passing the UUID form is never cached under the raw UUID key, so the helper
// must fall through to the DB path. Verifies this by checking the stub's
// sentinel error surfaces.
func TestResolveAgentUUIDCached_UUIDInputTakesDBPath(t *testing.T) {
	r := agent.NewRouter()
	stub := &errorAgentStore{err: errSentinelMiss}

	_, err := resolveAgentUUIDCached(context.Background(), r, stub, uuid.New().String())

	if !errors.Is(err, errSentinelMiss) {
		t.Errorf("expected sentinel miss error, got %v", err)
	}
}
