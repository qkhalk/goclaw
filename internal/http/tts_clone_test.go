package http

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
	"github.com/nextlevelbuilder/goclaw/internal/audio/clone"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// fakeCloneWorker is a minimal clone worker double covering the three
// registry endpoints.
type fakeCloneWorker struct {
	listBody    string
	registerRet string
	deletedID   string
	t           *testing.T
}

func (f *fakeCloneWorker) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/voices":
			_, _ = w.Write([]byte(f.listBody))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/voices":
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				f.t.Errorf("worker multipart parse: %v", err)
			}
			_, _ = w.Write([]byte(f.registerRet))
		case r.Method == http.MethodDelete:
			f.deletedID = strings.TrimPrefix(r.URL.Path, "/v1/voices/")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}
}

func newCloneTestMux(t *testing.T, mgr *audio.Manager, sc store.SystemConfigStore, cs store.ConfigSecretsStore) *http.ServeMux {
	t.Helper()
	helper := func(mgr *audio.Manager, sc store.SystemConfigStore, cs store.ConfigSecretsStore) *http.ServeMux {
		setupTestToken(t, "gateway-token")
		setupTestNoAuthFallback(t, false)
		ts := newMockTenantStore()
		tenantID := uuid.New()
		ts.addTenant(tenantID, "acme")
		ts.setUserRole(tenantID, "op-user", store.TenantRoleOperator)
		setupTestTenantStore(t, ts)

		h := NewTTSHandler(mgr)
		h.SetStores(sc, cs)
		mux := http.NewServeMux()
		h.RegisterRoutes(mux)
		return mux
	}
	return helper(mgr, sc, cs)
}

func cloneRequest(mux *http.ServeMux, method, target string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Authorization", "Bearer gateway-token")
	req.Header.Set("X-GoClaw-User-Id", "op-user")
	req.Header.Set("X-GoClaw-Tenant-Id", "acme")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func TestCloneVoicesNotConfiguredReturns503(t *testing.T) {
	mux := newCloneTestMux(t, nil, &validationSystemConfigStore{data: map[string]string{}}, &validationSecretsStore{data: map[string]string{}})

	rr := cloneRequest(mux, "GET", "/v1/tts/clone/voices", nil, "")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "tts.clone.endpoint") {
		t.Errorf("body should name the missing config: %s", rr.Body.String())
	}
}

func TestCloneVoicesRoutesProxyToWorker(t *testing.T) {
	worker := &fakeCloneWorker{
		listBody:    `{"voices":[{"id":"anh","name":"Giọng anh"}]}`,
		registerRet: `{"voice":{"id":"anh","name":"Giọng anh"}}`,
		t:           t,
	}
	wsrv := httptest.NewServer(worker.handler())
	defer wsrv.Close()

	mgr := audio.NewManager(audio.ManagerConfig{})
	mgr.RegisterTTS(clone.NewProvider(clone.Config{Endpoint: wsrv.URL}))
	mux := newCloneTestMux(t, mgr, &validationSystemConfigStore{data: map[string]string{}}, &validationSecretsStore{data: map[string]string{}})

	// List
	rr := cloneRequest(mux, "GET", "/v1/tts/clone/voices", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", rr.Code, rr.Body.String())
	}
	var listed struct {
		Voices []audio.Voice `json:"voices"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Voices) != 1 || listed.Voices[0].ID != "anh" {
		t.Fatalf("voices = %+v", listed.Voices)
	}

	// Register (multipart name + file)
	var buf strings.Builder
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", "Giọng anh")
	fw, _ := mw.CreateFormFile("file", "sample.wav")
	_, _ = fw.Write([]byte("WAVDATA"))
	_ = mw.Close()
	rr = cloneRequest(mux, "POST", "/v1/tts/clone/voices", strings.NewReader(buf.String()), mw.FormDataContentType())
	if rr.Code != http.StatusOK {
		t.Fatalf("register status = %d: %s", rr.Code, rr.Body.String())
	}

	// Delete invalid id → 400
	rr = cloneRequest(mux, "DELETE", "/v1/tts/clone/voices/..%2Fescape", nil, "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("delete traversal status = %d, want 400", rr.Code)
	}

	// Delete ok
	rr = cloneRequest(mux, "DELETE", "/v1/tts/clone/voices/anh", nil, "")
	if rr.Code != http.StatusOK || worker.deletedID != "anh" {
		t.Fatalf("delete status = %d id=%q", rr.Code, worker.deletedID)
	}
}

func TestCloneVoicesRegisterValidation(t *testing.T) {
	worker := &fakeCloneWorker{registerRet: `{"voice":{"id":"x","name":"x"}}`, t: t}
	wsrv := httptest.NewServer(worker.handler())
	defer wsrv.Close()

	mgr := audio.NewManager(audio.ManagerConfig{})
	mgr.RegisterTTS(clone.NewProvider(clone.Config{Endpoint: wsrv.URL}))
	mux := newCloneTestMux(t, mgr, &validationSystemConfigStore{data: map[string]string{}}, &validationSecretsStore{data: map[string]string{}})

	build := func(name, filename string) (string, string) {
		var buf strings.Builder
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("name", name)
		fw, _ := mw.CreateFormFile("file", filename)
		_, _ = fw.Write([]byte("WAVDATA"))
		_ = mw.Close()
		return buf.String(), mw.FormDataContentType()
	}

	// Missing name
	body, ctype := build("", "sample.wav")
	rr := cloneRequest(mux, "POST", "/v1/tts/clone/voices", strings.NewReader(body), ctype)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing name status = %d, want 400", rr.Code)
	}

	// Unsupported extension
	body, ctype = build("Giọng", "notes.txt")
	rr = cloneRequest(mux, "POST", "/v1/tts/clone/voices", strings.NewReader(body), ctype)
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("bad ext status = %d, want 415", rr.Code)
	}

	// Missing file — send multipart with name only
	var buf strings.Builder
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", "Giọng")
	_ = mw.Close()
	rr = cloneRequest(mux, "POST", "/v1/tts/clone/voices", strings.NewReader(buf.String()), mw.FormDataContentType())
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing file status = %d, want 400", rr.Code)
	}
}

func TestResolveTenantProviderClone(t *testing.T) {
	sc := &validationSystemConfigStore{data: map[string]string{
		"tts.clone.endpoint": "http://192.168.1.50:18795",
		"tts.clone.voice":    "anh",
	}}
	cs := &validationSecretsStore{data: map[string]string{"tts.clone.api_key": "tok"}}
	h := NewTTSHandler(nil)
	h.SetStores(sc, cs)

	p, name, _, err := h.resolveTenantProvider(context.Background(), "clone")
	if err != nil {
		t.Fatalf("resolveTenantProvider: %v", err)
	}
	if name != "clone" || p == nil {
		t.Fatalf("provider = %v name = %q", p, name)
	}

	// No endpoint → error and caller falls back to global manager.
	delete(sc.data, "tts.clone.endpoint")
	if _, _, _, err := h.resolveTenantProvider(context.Background(), "clone"); err == nil {
		t.Fatal("expected error without endpoint")
	}
}

func TestCapabilitiesEnrichesCloneVoices(t *testing.T) {
	worker := &fakeCloneWorker{listBody: `{"voices":[{"id":"anh","name":"Giọng anh"}]}`, t: t}
	wsrv := httptest.NewServer(worker.handler())
	defer wsrv.Close()

	mgr := audio.NewManager(audio.ManagerConfig{})
	mgr.RegisterTTS(clone.NewProvider(clone.Config{Endpoint: wsrv.URL}))

	mux := newCloneTestMux(t, mgr, &validationSystemConfigStore{data: map[string]string{}}, &validationSecretsStore{data: map[string]string{}})
	rr := cloneRequest(mux, "GET", "/v1/tts/capabilities", nil, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d", rr.Code)
	}
	var resp struct {
		Providers []audio.ProviderCapabilities `json:"providers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, p := range resp.Providers {
		if p.Provider == "clone" {
			found = true
			if len(p.Voices) != 1 || p.Voices[0].VoiceID != "anh" {
				t.Errorf("clone voices = %+v, want live worker voices merged", p.Voices)
			}
		}
	}
	if !found {
		t.Error("clone provider missing from capabilities")
	}
}
