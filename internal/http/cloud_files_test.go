package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	cloudmgr "github.com/nextlevelbuilder/goclaw/internal/cloud"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Guard matrix for the Phase 4 cloud file-operation endpoints. The storage
// backend (rcd) is not faked here — every case stops at the auth/guard/
// validation layer, asserting the status each caller class must receive
// BEFORE any rclone work starts. When the guards pass, the request dies in
// the rclone-less storage layer with 502/503 (no rclone binary in the test
// image) — that status is the assertion for "guards passed" (spec step 6).

const (
	cloudTestToken  = "cloud-files-test-token"
	cloudWriteScope = "https://www.googleapis.com/auth/drive"
	cloudReadScope  = "https://www.googleapis.com/auth/drive.readonly"
)

type mockCloudAccountStore struct {
	accounts []store.CloudAccount
}

func (m *mockCloudAccountStore) add(acct store.CloudAccount) {
	m.accounts = append(m.accounts, acct)
}

func (m *mockCloudAccountStore) Upsert(context.Context, *store.CloudAccount) error { return nil }
func (m *mockCloudAccountStore) Get(context.Context, string) (*store.CloudAccount, error) {
	return nil, store.ErrCloudAccountNotFound
}
func (m *mockCloudAccountStore) GetByEmail(context.Context, string, string) (*store.CloudAccount, error) {
	return nil, store.ErrCloudAccountNotFound
}

// List returns the caller's own accounts (ctx tenant + user).
func (m *mockCloudAccountStore) List(ctx context.Context) ([]store.CloudAccount, error) {
	tid := store.TenantIDFromContext(ctx)
	uid := store.UserIDFromContext(ctx)
	var out []store.CloudAccount
	for _, a := range m.accounts {
		if a.TenantID == tid.String() && a.UserID == uid {
			out = append(out, a)
		}
	}
	return out, nil
}

// ListShared returns the tenant-wide shared accounts (any owner).
func (m *mockCloudAccountStore) ListShared(ctx context.Context) ([]store.CloudAccount, error) {
	tid := store.TenantIDFromContext(ctx)
	var out []store.CloudAccount
	for _, a := range m.accounts {
		if a.TenantID == tid.String() && a.Shared {
			out = append(out, a)
		}
	}
	return out, nil
}

// ListTenant returns every account of the ctx tenant (any owner).
func (m *mockCloudAccountStore) ListTenant(ctx context.Context) ([]store.CloudAccount, error) {
	tid := store.TenantIDFromContext(ctx)
	var out []store.CloudAccount
	for _, a := range m.accounts {
		if a.TenantID == tid.String() {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *mockCloudAccountStore) SetShared(context.Context, string, bool) error { return nil }
func (m *mockCloudAccountStore) UpdateTokens(context.Context, string, store.CloudAccountUpdate) error {
	return nil
}
func (m *mockCloudAccountStore) Delete(context.Context, string) error { return nil }

type cloudFilesFixture struct {
	handler http.Handler
	store   *mockCloudAccountStore
	tenant  uuid.UUID
}

// newCloudFilesFixture wires a CloudHandler against in-memory account +
// tenant stores, with the storage service attached.
func newCloudFilesFixture(t *testing.T) *cloudFilesFixture {
	t.Helper()

	tenant := uuid.New()
	accounts := &mockCloudAccountStore{}
	tenants := newMockTenantStore()
	tenants.addTenant(tenant, "acme")
	tenants.setUserRole(tenant, "alice", store.TenantRoleMember)
	tenants.setUserRole(tenant, "bob", store.TenantRoleAdmin)
	tenants.setUserRole(tenant, "carol", store.TenantRoleMember)

	newAcct := func(owner string, shared bool, scopes string) store.CloudAccount {
		return store.CloudAccount{
			ID: uuid.NewString(), TenantID: tenant.String(), UserID: owner,
			Provider: cloudmgr.GoogleProvider, Email: owner + "@example.com",
			Scopes: scopes, Status: "active", Shared: shared,
		}
	}
	accounts.add(newAcct("alice", false, `["`+cloudWriteScope+`"]`))  // own, writable grant
	accounts.add(newAcct("alice", false, `["`+cloudReadScope+`"]`))  // own, read-only grant
	accounts.add(newAcct("carol", true, `["`+cloudWriteScope+`"]`))  // carol's shared account
	accounts.add(newAcct("alice", false, `["`+cloudWriteScope+`"]`)) // transfer source

	mgr := cloudmgr.NewManager(cloudmgr.CloudProviderConfig{}, accounts, "")
	svc := cloudmgr.NewStorageService(mgr, t.TempDir())
	mgr.SetStorageService(svc)

	handler := NewCloudHandler(mgr, accounts, tenants, nil, true, "", 1) // 1 MB upload cap
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	setupTestToken(t, cloudTestToken)
	setupTestTenantStore(t, tenants)

	return &cloudFilesFixture{handler: mux, store: accounts, tenant: tenant}
}

// do issues an authenticated request as userID (empty = unauthenticated).
// contentType is optional ("" for JSON/no-body requests).
func (f *cloudFilesFixture) do(t *testing.T, userID, method, target, contentType string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+cloudTestToken)
		req.Header.Set("X-GoClaw-User-Id", userID)
		req.Header.Set("X-GoClaw-Tenant-Id", f.tenant.String())
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f *cloudFilesFixture) doJSON(t *testing.T, userID, method, target string, v any) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if v != nil {
		buf, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(buf)
	}
	return f.do(t, userID, method, target, "application/json", body)
}

// accountID finds one seeded account by owner/shared/scopes.
func (f *cloudFilesFixture) accountID(t *testing.T, owner string, shared bool, scopes string) string {
	t.Helper()
	for _, a := range f.store.accounts {
		if a.UserID == owner && a.Shared == shared && strings.Contains(a.Scopes, scopes) {
			return a.ID
		}
	}
	t.Fatalf("no account for owner %q (shared=%v scopes~%q)", owner, shared, scopes)
	return ""
}

// storageLayerReached asserts the guards let the request through and the
// failure came from the (rclone-less) storage backend.
func storageLayerReached(t *testing.T, rec *httptest.ResponseRecorder, what string) {
	t.Helper()
	if rec.Code != http.StatusBadGateway && rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("%s: status = %d body = %s, want 502/503 from the storage layer (guards passed)", what, rec.Code, rec.Body.String())
	}
}

// multipartFile builds a multipart/form-data body with one `file` part.
func multipartFile(t *testing.T, name string, size int) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(bytes.Repeat([]byte("a"), size)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func TestCloudFilesUnauthenticated401(t *testing.T) {
	f := newCloudFilesFixture(t)
	for _, tc := range []struct{ method, target string }{
		{"POST", "/v1/cloud/accounts/00000000-0000-0000-0000-000000000001/folders"},
		{"GET", "/v1/cloud/accounts/00000000-0000-0000-0000-000000000001/files/download?path=x"},
		{"DELETE", "/v1/cloud/accounts/00000000-0000-0000-0000-000000000001/files?path=x"},
		{"POST", "/v1/cloud/transfer"},
	} {
		rec := f.do(t, "", tc.method, tc.target, "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: status = %d, want 401", tc.method, tc.target, rec.Code)
		}
	}
}

func TestCloudFilesInvalidAccountID400(t *testing.T) {
	f := newCloudFilesFixture(t)
	rec := f.doJSON(t, "alice", "POST", "/v1/cloud/accounts/not-a-uuid/folders", map[string]string{"path": "d"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s, want 400", rec.Code, rec.Body.String())
	}
}

func TestCloudFilesUnknownAccount404(t *testing.T) {
	f := newCloudFilesFixture(t)
	rec := f.doJSON(t, "alice", "POST", "/v1/cloud/accounts/"+uuid.NewString()+"/folders", map[string]string{"path": "d"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s, want 404", rec.Code, rec.Body.String())
	}
}

// Member on a tenant-shared account is read-only: every write verb 403s
// (the guards run before body parsing, so nil bodies are fine).
func TestCloudFilesMemberSharedWrite403(t *testing.T) {
	f := newCloudFilesFixture(t)
	acct := f.accountID(t, "carol", true, cloudWriteScope)
	cases := []struct{ name, method, target string }{
		{"mkdir", "POST", "/v1/cloud/accounts/" + acct + "/folders"},
		{"move", "PATCH", "/v1/cloud/accounts/" + acct + "/files"},
		{"copy", "POST", "/v1/cloud/accounts/" + acct + "/files/copy"},
		{"copyurl", "POST", "/v1/cloud/accounts/" + acct + "/files/copyurl"},
		{"delete", "DELETE", "/v1/cloud/accounts/" + acct + "/files?path=a"},
		{"publiclink", "POST", "/v1/cloud/accounts/" + acct + "/files/publiclink"},
		{"upload", "POST", "/v1/cloud/accounts/" + acct + "/files"},
	}
	for _, tc := range cases {
		rec := f.doJSON(t, "alice", tc.method, tc.target, map[string]string{"path": "d", "from": "a", "to": "b", "url": "https://x.test/a"})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d body = %s, want 403", tc.name, rec.Code, rec.Body.String())
		}
	}
}

// Tenant admin (non-owner) may write a member's shared account: the guards
// pass and the request reaches the storage layer.
func TestCloudFilesTenantAdminSharedWriteAllowed(t *testing.T) {
	f := newCloudFilesFixture(t)
	acct := f.accountID(t, "carol", true, cloudWriteScope)
	rec := f.doJSON(t, "bob", "POST", "/v1/cloud/accounts/"+acct+"/folders", map[string]string{"path": "from-admin"})
	storageLayerReached(t, rec, "tenant admin write")
}

// Account connected with read-only scopes → 403 with the machine-readable
// code the UI uses to offer the re-grant flow.
func TestCloudFilesWriteScopeRequired403(t *testing.T) {
	f := newCloudFilesFixture(t)
	acct := f.accountID(t, "alice", false, cloudReadScope)
	rec := f.doJSON(t, "alice", "POST", "/v1/cloud/accounts/"+acct+"/folders", map[string]string{"path": "d"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s, want 403", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body: %v", err)
	}
	if out["code"] != "cloud_write_scope_required" {
		t.Fatalf("code = %q, want cloud_write_scope_required (body %s)", out["code"], rec.Body.String())
	}
}

func TestCloudFilesOwnerHappyGuards(t *testing.T) {
	f := newCloudFilesFixture(t)
	acct := f.accountID(t, "alice", false, cloudWriteScope)

	t.Run("json writes", func(t *testing.T) {
		cases := []struct{ name, method, target string; body any }{
			{"mkdir", "POST", "/v1/cloud/accounts/" + acct + "/folders", map[string]string{"path": "docs"}},
			{"move", "PATCH", "/v1/cloud/accounts/" + acct + "/files", map[string]string{"from": "a.txt", "to": "b/b.txt"}},
			{"copy", "POST", "/v1/cloud/accounts/" + acct + "/files/copy", map[string]string{"from": "a.txt", "to": "c.txt"}},
			{"copyurl", "POST", "/v1/cloud/accounts/" + acct + "/files/copyurl", map[string]string{"url": "https://example.com/a.png", "path": "a.png"}},
			{"publiclink", "POST", "/v1/cloud/accounts/" + acct + "/files/publiclink", map[string]string{"path": "a.txt"}},
		}
		for _, tc := range cases {
			rec := f.doJSON(t, "alice", tc.method, tc.target, tc.body)
			storageLayerReached(t, rec, tc.name)
		}
	})

	t.Run("delete file", func(t *testing.T) {
		storageLayerReached(t, f.do(t, "alice", "DELETE", "/v1/cloud/accounts/"+acct+"/files?path=a.txt", "", nil), "delete file")
	})
	t.Run("delete empty dir", func(t *testing.T) {
		storageLayerReached(t, f.do(t, "alice", "DELETE", "/v1/cloud/accounts/"+acct+"/files?path=empty&isDir=true", "", nil), "delete dir")
	})
	t.Run("upload", func(t *testing.T) {
		body, ctype := multipartFile(t, "a.txt", 32)
		storageLayerReached(t, f.do(t, "alice", "POST", "/v1/cloud/accounts/"+acct+"/files", ctype, body), "upload")
	})
}

// Member reads a shared account: download skips the write guard entirely.
func TestCloudFilesMemberDownloadAllowed(t *testing.T) {
	f := newCloudFilesFixture(t)
	acct := f.accountID(t, "carol", true, cloudWriteScope)
	rec := f.do(t, "alice", "GET", "/v1/cloud/accounts/"+acct+"/files/download?path=report.pdf", "", nil)
	storageLayerReached(t, rec, "member download")
}

// Path traversal: 400 before any rclone work, on every endpoint that takes
// a remote path.
func TestCloudFilesPathTraversal400(t *testing.T) {
	f := newCloudFilesFixture(t)
	acct := f.accountID(t, "alice", false, cloudWriteScope)
	cases := []struct{ name, method, target string; body any }{
		{"mkdir", "POST", "/v1/cloud/accounts/" + acct + "/folders", map[string]string{"path": "../etc"}},
		{"move-from", "PATCH", "/v1/cloud/accounts/" + acct + "/files", map[string]string{"from": "a/../b", "to": "c"}},
		{"copy-to", "POST", "/v1/cloud/accounts/" + acct + "/files/copy", map[string]string{"from": "a", "to": "..\\x"}},
		{"copyurl-path", "POST", "/v1/cloud/accounts/" + acct + "/files/copyurl", map[string]string{"url": "https://example.com/a", "path": "../x"}},
		{"publiclink", "POST", "/v1/cloud/accounts/" + acct + "/files/publiclink", map[string]string{"path": "../x"}},
	}
	for _, tc := range cases {
		rec := f.doJSON(t, "alice", tc.method, tc.target, tc.body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d body = %s, want 400", tc.name, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "invalid path") {
			t.Fatalf("%s: body = %s, want invalid path", tc.name, rec.Body.String())
		}
	}
	for _, tc := range []struct{ name, target string }{
		{"delete", "/v1/cloud/accounts/" + acct + "/files?path=../secret"},
		{"download", "/v1/cloud/accounts/" + acct + "/files/download?path=..%2Fsecret"},
	} {
		method := "DELETE"
		if tc.name == "download" {
			method = "GET"
		}
		rec := f.do(t, "alice", method, tc.target, "", nil)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid path") {
			t.Fatalf("%s: status = %d body = %s, want 400 invalid path", tc.name, rec.Code, rec.Body.String())
		}
	}
}

func TestCloudFilesCopyURLRejectsNonHTTPScheme(t *testing.T) {
	f := newCloudFilesFixture(t)
	acct := f.accountID(t, "alice", false, cloudWriteScope)
	rec := f.doJSON(t, "alice", "POST", "/v1/cloud/accounts/"+acct+"/files/copyurl",
		map[string]string{"url": "file:///etc/passwd", "path": "x"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s, want 400", rec.Code, rec.Body.String())
	}
}

// Upload over the configured size cap → 413 before the storage layer.
func TestCloudFilesUploadOverCap413(t *testing.T) {
	f := newCloudFilesFixture(t)
	acct := f.accountID(t, "alice", false, cloudWriteScope)
	body, ctype := multipartFile(t, "big.bin", 2<<20) // 2 MB, cap is 1 MB
	rec := f.do(t, "alice", "POST", "/v1/cloud/accounts/"+acct+"/files", ctype, body)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d body = %s, want 413", rec.Code, rec.Body.String())
	}
}

// Upload without a file part → 400.
func TestCloudFilesUploadMissingFileField400(t *testing.T) {
	f := newCloudFilesFixture(t)
	acct := f.accountID(t, "alice", false, cloudWriteScope)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("path", "docs"); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	rec := f.do(t, "alice", "POST", "/v1/cloud/accounts/"+acct+"/files", mw.FormDataContentType(), &buf)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s, want 400", rec.Code, rec.Body.String())
	}
}

func TestCloudFilesTransferValidation(t *testing.T) {
	f := newCloudFilesFixture(t)
	src := f.accountID(t, "alice", false, cloudWriteScope)

	t.Run("bad mode 400", func(t *testing.T) {
		rec := f.doJSON(t, "alice", "POST", "/v1/cloud/transfer", map[string]string{
			"source_account_id": src, "source_path": "a", "target_account_id": src, "target_path": "b", "mode": "delete",
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body = %s, want 400", rec.Code, rec.Body.String())
		}
	})

	t.Run("inaccessible target 404", func(t *testing.T) {
		rec := f.doJSON(t, "alice", "POST", "/v1/cloud/transfer", map[string]string{
			"source_account_id": src, "source_path": "a", "target_account_id": uuid.NewString(), "target_path": "b",
		})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d body = %s, want 404", rec.Code, rec.Body.String())
		}
	})

	t.Run("traversal source 400", func(t *testing.T) {
		rec := f.doJSON(t, "alice", "POST", "/v1/cloud/transfer", map[string]string{
			"source_account_id": src, "source_path": "../a", "target_account_id": src, "target_path": "b",
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body = %s, want 400", rec.Code, rec.Body.String())
		}
	})
}

func TestCloudFilesTransferStatus(t *testing.T) {
	f := newCloudFilesFixture(t)

	t.Run("invalid id 400", func(t *testing.T) {
		rec := f.do(t, "alice", "GET", "/v1/cloud/transfers/abc", "", nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unknown job 404", func(t *testing.T) {
		rec := f.do(t, "alice", "GET", "/v1/cloud/transfers/999999", "", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}
