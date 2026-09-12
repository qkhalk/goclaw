package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/cloud/mail"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// fakeMailProvider records the accounts resolved and returns canned clients.
type fakeMailProvider struct {
	accounts   []store.CloudAccount
	lastClient *mail.Client
}

func (f *fakeMailProvider) Accounts(ctx context.Context) ([]store.CloudAccount, error) {
	return f.accounts, nil
}

func (f *fakeMailProvider) MailClient(ctx context.Context, account string) (*mail.Client, error) {
	if len(f.accounts) == 0 {
		return nil, store.ErrCloudAccountNotFound
	}
	return f.lastClient, nil
}

func TestMailActionLabelsNoPermanentDelete(t *testing.T) {
	for _, action := range []string{"archive", "trash", "untrash", "mark_read", "add_label", "remove_label"} {
		add, remove, err := mailActionLabels(action, "x")
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		for _, l := range append(add, remove...) {
			if l == "DELETE" {
				t.Fatalf("delete label leaked for action %s", action)
			}
		}
	}
	if _, _, err := mailActionLabels("delete", ""); err == nil {
		t.Fatal("delete action must be rejected")
	}
}

func TestMailArchiveBatchCap(t *testing.T) {
	provider := &fakeMailProvider{accounts: []store.CloudAccount{{Email: "a@gmail.com", Status: "active"}}}
	toolset := NewCloudMailTools(provider, 1024)

	var tool Tool
	for _, tl := range toolset.Tools() {
		if tl.Name() == "mail_archive" {
			tool = tl
		}
	}
	if tool == nil {
		t.Fatal("mail_archive tool missing from toolset")
	}

	ids := make([]any, mailArchiveBatchMax+1)
	for i := range ids {
		ids[i] = "id"
	}
	res := tool.Execute(context.Background(), map[string]any{"message_ids": ids, "action": "archive"})
	if res == nil || !strings.Contains(res.ForLLM, "max 50") {
		t.Fatalf("batch cap not enforced: %+v", res)
	}
}

func TestCloudAccountsToolShape(t *testing.T) {
	provider := &fakeMailProvider{accounts: []store.CloudAccount{
		{Email: "a@gmail.com", Provider: "google", Status: "active"},
	}}
	toolset := NewCloudMailTools(provider, 1024)

	var tool Tool
	for _, tl := range toolset.Tools() {
		if tl.Name() == "cloud_accounts" {
			tool = tl
		}
	}
	res := tool.Execute(context.Background(), map[string]any{})
	if res == nil || !strings.Contains(res.ForLLM, "a@gmail.com") || !strings.Contains(res.ForLLM, `"mail":true`) {
		t.Fatalf("unexpected output: %+v", res)
	}
}
