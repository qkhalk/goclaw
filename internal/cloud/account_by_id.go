package cloud

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// AccountByID resolves one account by ID from the caller's accessible set
// (own + tenant-shared). Owner-scoped Get would hide shared accounts from
// other members, so token sources must go through here.
func (m *Manager) AccountByID(ctx context.Context, id string) (*store.CloudAccount, error) {
	if id == "" {
		return nil, store.ErrCloudAccountNotFound
	}
	accounts, err := m.accessibleAccounts(ctx)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		if accounts[i].ID == id {
			return &accounts[i], nil
		}
	}
	return nil, store.ErrCloudAccountNotFound
}
