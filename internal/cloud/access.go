package cloud

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ErrNoAccessibleAccount wraps ErrNoAccounts for the binding-aware resolver.
var ErrNoAccessibleAccount = errors.New("no accessible cloud account matches the request (connect one on the Clouds page or ask an admin to share/bind one)")

// SetBindingStore wires the per-scope binding store (same DB handle as the
// account store). Optional: without it only explicitly named or own accounts
// resolve (pre-bindings behavior).
func (m *Manager) SetBindingStore(s store.CloudBindingStore) { m.bindings = s }

// accessibleAccounts returns the caller's own accounts plus the tenant-wide
// shared ones, deduped by ID. This is the full set the caller may ever use.
func (m *Manager) accessibleAccounts(ctx context.Context) ([]store.CloudAccount, error) {
	own, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}
	accounts := own
	if shared, serr := m.store.ListShared(ctx); serr == nil && len(shared) > 0 {
		seen := make(map[string]bool, len(own))
		for i := range own {
			seen[own[i].ID] = true
		}
		for i := range shared {
			if !seen[shared[i].ID] {
				accounts = append(accounts, shared[i])
			}
		}
	}
	return accounts, nil
}

// ResolveAccount picks the account for this request. Resolution order:
//
//  1. explicit name (email or account ID) — must be own or tenant-shared;
//  2. group binding for the calling chat (ChannelContextScope group);
//  3. user binding for the calling user;
//  4. tenant default binding;
//  5. own accounts, preferring active (legacy behavior).
//
// usable is the capability predicate (provider + scopes + status) and
// providers is the preference order for binding lookup. Revoked binding
// targets fall through to the next scope.
func (m *Manager) ResolveAccount(ctx context.Context, name string, providers []string, usable func(*store.CloudAccount) bool) (*store.CloudAccount, error) {
	accounts, err := m.accessibleAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, ErrNoAccounts
	}

	byID := make(map[string]*store.CloudAccount, len(accounts))
	revokedEmail := ""
	for i := range accounts {
		a := &accounts[i]
		byID[a.ID] = a
	}

	// 1. Explicit pick: email or ID.
	if name != "" {
		for i := range accounts {
			a := &accounts[i]
			if strings.EqualFold(a.Email, name) || a.ID == name {
				if !usable(a) {
					if a.Status == "revoked" {
						return nil, fmt.Errorf("cloud account %s is revoked — reconnect on the Clouds page", a.Email)
					}
					continue
				}
				return a, nil
			}
		}
		return nil, ErrNoAccounts
	}

	// 2-4. Bindings: group → user → tenant default.
	if m.bindings != nil {
		bindings, berr := m.bindings.ListBindings(ctx)
		if berr != nil {
			slog.Warn("cloud: list bindings failed", "error", berr)
		} else {
			scopes := make([][2]string, 0, 3)
			if cs, ok := store.ChannelContextScopeFromContext(ctx); ok && cs.ScopeType == store.ChannelScopeTypeGroup && cs.ScopeKey != "" {
				scopes = append(scopes, [2]string{store.CloudBindingScopeGroup, cs.ScopeKey})
			}
			if uid := store.UserIDFromContext(ctx); uid != "" {
				scopes = append(scopes, [2]string{store.CloudBindingScopeUser, uid})
			}
			scopes = append(scopes, [2]string{store.CloudBindingScopeTenant, ""})

			for _, sc := range scopes {
				for _, provider := range providers {
					for i := range bindings {
						b := &bindings[i]
						if b.ScopeType != sc[0] || b.ScopeKey != sc[1] || b.Provider != provider {
							continue
						}
						acct := byID[b.AccountID]
						if acct == nil || !usable(acct) {
							if acct != nil && acct.Status == "revoked" {
								revokedEmail = acct.Email
							}
							continue // binding target gone/revoked → next scope
						}
						return acct, nil
					}
				}
			}
		}
	}

	// 5. Legacy fallback: own accounts, prefer active.
	for i := range accounts {
		a := &accounts[i]
		if a.Status == "revoked" {
			revokedEmail = a.Email
			continue
		}
		if usable(a) {
			return a, nil
		}
	}
	if revokedEmail != "" {
		return nil, fmt.Errorf("cloud account %s is revoked — reconnect on the Clouds page", revokedEmail)
	}
	return nil, ErrNoAccessibleAccount
}
