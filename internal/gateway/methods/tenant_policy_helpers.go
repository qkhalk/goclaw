package methods

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// policyFailure is the translated, client-bound message plus the wire code the
// handler should attach. Handlers call client.SendResponse / sendChatError
// directly; these helpers only centralize the policy decision branches so the
// three creation-site gates and the run-entry gate read the same way.
type policyFailure struct {
	code string
	msg  string
}

// Tenant policy failure wire codes. Suspension and resource caps are
// precondition failures; an unreadable policy row is an internal error.
const (
	tenantPolicySuspended   = "suspended"
	tenantPolicyLimit       = "limit"
	tenantPolicyProviderDen = "provider_denied"
	tenantPolicyModelDen    = "model_denied"
	tenantPolicyInternal     = "internal"
)

// checkTenantActive enforces the tenant policy status at connection/run-entry
// time. A nil policyStore (unwired edition) or a nil/absent tenant ID means
// "no restriction" — matching the store's fast path where a missing policy row
// is treated as active. Returns nil when the tenant may proceed; otherwise a
// policyFailure with the suspension message.
func checkTenantActive(ctx context.Context, policyStore store.AgentPolicies, tenantID uuid.UUID, locale string) *policyFailure {
	if policyStore == nil || tenantID == uuid.Nil {
		return nil
	}
	if err := policyStore.CheckTenantActive(ctx, tenantID); err != nil {
		if errors.Is(err, store.ErrTenantSuspended) {
			return &policyFailure{code: tenantPolicySuspended, msg: i18n.T(locale, i18n.MsgPolicyTenantSuspended)}
		}
		return &policyFailure{code: tenantPolicyInternal, msg: i18n.T(locale, i18n.MsgInternalError, err.Error())}
	}
	return nil
}

// checkTenantLimit enforces one max_* resource cap at creation time. The
// caller passes the store's bound check (e.g. policyStore.CheckCanCreateAgent).
// Returns nil when the tenant may create the resource; otherwise a
// policyFailure with the limit-reached message (including cap name, limit, and
// current count from *TenantLimitError).
func checkTenantLimit(ctx context.Context, policyStore store.AgentPolicies, tenantID uuid.UUID, check func(context.Context) error, locale string) *policyFailure {
	if policyStore == nil || tenantID == uuid.Nil {
		return nil
	}
	if err := check(ctx); err != nil {
		var tle *store.TenantLimitError
		if errors.As(err, &tle) {
			return &policyFailure{code: tenantPolicyLimit, msg: i18n.T(locale, i18n.MsgPolicyLimitReached, capLabel(string(tle.Cap)), tle.Limit, tle.Existing)}
		}
		// DB error reading policy/counts — fail closed unless it's just "no policy".
		if errors.Is(err, store.ErrTenantPolicyNotFound) {
			return nil
		}
		return &policyFailure{code: tenantPolicyInternal, msg: i18n.T(locale, i18n.MsgInternalError, err.Error())}
	}
	return nil
}

// checkProviderModelAccess enforces the provider/model allowlist from the
// tenant policy. Returns nil when the pair is allowed, no allowlist is
// configured, or there is no tenant scope. A missing policy row is treated as
// "no restriction". Empty provider or model strings are skipped (no allowlist
// entry can match empty, so absence means the caller has no explicit value to
// constrain).
func checkProviderModelAccess(ctx context.Context, policyStore store.AgentPolicies, tenantID uuid.UUID, provider, model, locale string) *policyFailure {
	if policyStore == nil || tenantID == uuid.Nil {
		return nil
	}
	policy, err := policyStore.GetTenantPolicy(ctx, tenantID)
	if err != nil {
		// No policy row = no restriction (fast path, mirrors CheckTenantActive).
		if errors.Is(err, store.ErrTenantPolicyNotFound) {
			return nil
		}
		return &policyFailure{code: tenantPolicyInternal, msg: i18n.T(locale, i18n.MsgInternalError, err.Error())}
	}
	if policy == nil {
		return nil
	}
	if provider != "" {
		if allowed, withAllowlist := policy.ProviderAccess(provider); withAllowlist && !allowed {
			return &policyFailure{code: tenantPolicyProviderDen, msg: i18n.T(locale, i18n.MsgPolicyProviderDenied, provider)}
		}
	}
	if model != "" {
		if allowed, withAllowlist := policy.ModelAccess(model); withAllowlist && !allowed {
			return &policyFailure{code: tenantPolicyModelDen, msg: i18n.T(locale, i18n.MsgPolicyModelDenied, model)}
		}
	}
	return nil
}

// capLabel maps a CapName to its friendly noun for the limit message.
func capLabel(capName string) string {
	switch capName {
	case string(store.CapAgents):
		return "agents"
	case string(store.CapSessions):
		return "sessions"
	case string(store.CapTeams):
		return "teams"
	}
	return capName
}