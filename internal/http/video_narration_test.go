package http

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	videopkg "github.com/nextlevelbuilder/goclaw/internal/video"
)

// Compile-time guard: the video dispatcher owns the assets dir and must
// satisfy the narration store interface the endpoint consumes.
var _ VideoNarrationStore = (*videopkg.Dispatcher)(nil)

// fakeNarrationStore records the last SaveNarration call.
type fakeNarrationStore struct {
	gotJobID string
	gotScene int
	gotExt   string
	gotData  []byte
	retErr   error
	// jobs maps jobID -> status for NarrationJob. nil = accept any id as
	// queued (so validation-table cases reach their target branch).
	jobs map[string]string
}

func (f *fakeNarrationStore) SaveNarration(jobID string, sceneIndex int, ext string, data []byte) (string, error) {
	if f.retErr != nil {
		return "", f.retErr
	}
	f.gotJobID = jobID
	f.gotScene = sceneIndex
	f.gotExt = ext
	f.gotData = data
	return filepath.Join(".video-assets", jobID, "narration-"+strconv.Itoa(sceneIndex)+"."+ext), nil
}

func (f *fakeNarrationStore) NarrationJob(_ context.Context, jobID string) (string, bool) {
	if f.jobs == nil {
		return "queued", true
	}
	s, ok := f.jobs[jobID]
	return s, ok
}

func newNarrationTestMux(t *testing.T, st VideoNarrationStore) *http.ServeMux {
	t.Helper()
	setupTestToken(t, "gateway-token")
	setupTestNoAuthFallback(t, false)
	ts := newMockTenantStore()
	tenantID := uuid.New()
	ts.addTenant(tenantID, "acme")
	ts.setUserRole(tenantID, "op-user", store.TenantRoleOperator)
	setupTestTenantStore(t, ts)

	h := NewTTSHandler(nil)
	if st != nil {
		h.SetVideoNarrationStore(st)
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func narrationRequest(mux *http.ServeMux, method, target string, body io.Reader, contentType string) *httptest.ResponseRecorder {
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

func narrationMultipart(t *testing.T, jobID, scene, filename string, content []byte) (string, string) {
	t.Helper()
	var buf strings.Builder
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("job_id", jobID)
	_ = mw.WriteField("scene_index", scene)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write(content)
	_ = mw.Close()
	return buf.String(), mw.FormDataContentType()
}

func TestVideoNarrationNotWiredReturns503(t *testing.T) {
	mux := newNarrationTestMux(t, nil)
	body, ct := narrationMultipart(t, uuid.New().String(), "0", "a.wav", []byte("WAVDATA"))
	rr := narrationRequest(mux, "POST", "/v1/video/narration", strings.NewReader(body), ct)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestVideoNarrationUploadHappyPath(t *testing.T) {
	st := &fakeNarrationStore{}
	mux := newNarrationTestMux(t, st)
	jobID := uuid.New().String()
	body, ct := narrationMultipart(t, jobID, "2", "scene-2.wav", []byte("WAVBYTES"))

	rr := narrationRequest(mux, "POST", "/v1/video/narration", strings.NewReader(body), ct)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	if st.gotJobID != jobID || st.gotScene != 2 || st.gotExt != "wav" || string(st.gotData) != "WAVBYTES" {
		t.Fatalf("store got job=%q scene=%d ext=%q data=%q", st.gotJobID, st.gotScene, st.gotExt, st.gotData)
	}
	var resp struct {
		OK         bool   `json:"ok"`
		AudioPath  string `json:"audio_path"`
		SceneIndex int    `json:"scene_index"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK || resp.SceneIndex != 2 || resp.AudioPath == "" {
		t.Fatalf("response = %s", rr.Body.String())
	}
}

func TestVideoNarrationValidation(t *testing.T) {
	st := &fakeNarrationStore{}
	mux := newNarrationTestMux(t, st)

	cases := []struct {
		name     string
		jobID    string
		scene    string
		filename string
		content  []byte
		want     int
	}{
		{"bad job id", "not-a-uuid", "0", "a.wav", []byte("x"), http.StatusBadRequest},
		{"empty job id", "", "0", "a.wav", []byte("x"), http.StatusBadRequest},
		{"bad scene", uuid.New().String(), "-1", "a.wav", []byte("x"), http.StatusBadRequest},
		{"scene too high", uuid.New().String(), "60", "a.wav", []byte("x"), http.StatusBadRequest},
		{"scene not a number", uuid.New().String(), "zero", "a.wav", []byte("x"), http.StatusBadRequest},
		{"bad ext", uuid.New().String(), "0", "a.exe", []byte("x"), http.StatusUnsupportedMediaType},
		{"no ext", uuid.New().String(), "0", "audio", []byte("x"), http.StatusUnsupportedMediaType},
		{"empty file", uuid.New().String(), "0", "a.wav", nil, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, ct := narrationMultipart(t, tc.jobID, tc.scene, tc.filename, tc.content)
			rr := narrationRequest(mux, "POST", "/v1/video/narration", strings.NewReader(body), ct)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

func TestVideoNarrationUnknownJobReturns404(t *testing.T) {
	// Empty (non-nil) jobs map = every id unknown.
	st := &fakeNarrationStore{jobs: map[string]string{}}
	mux := newNarrationTestMux(t, st)
	body, ct := narrationMultipart(t, uuid.New().String(), "0", "a.wav", []byte("x"))
	rr := narrationRequest(mux, "POST", "/v1/video/narration", strings.NewReader(body), ct)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestVideoNarrationNonQueuedJobReturns409(t *testing.T) {
	jobID := uuid.New().String()
	st := &fakeNarrationStore{jobs: map[string]string{jobID: "rendering"}}
	mux := newNarrationTestMux(t, st)
	body, ct := narrationMultipart(t, jobID, "0", "a.wav", []byte("x"))
	rr := narrationRequest(mux, "POST", "/v1/video/narration", strings.NewReader(body), ct)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
}

func TestVideoNarrationOversizeReturns413(t *testing.T) {
	st := &fakeNarrationStore{}
	mux := newNarrationTestMux(t, st)
	big := make([]byte, (20<<20)+1)
	body, ct := narrationMultipart(t, uuid.New().String(), "0", "a.wav", big)
	rr := narrationRequest(mux, "POST", "/v1/video/narration", strings.NewReader(body), ct)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rr.Code)
	}
}

func TestVideoNarrationDispatcherStoreIntegration(t *testing.T) {
	// The real store: a Dispatcher over a temp workspace (store/worker/event
	// deps stay nil — SaveNarration only touches the workspace dir).
	workspace := t.TempDir()
	disp := videopkg.NewDispatcher(&config.VideoConfig{}, nil, nil, nil, workspace)
	jobID := uuid.New().String()

	path, err := disp.SaveNarration(jobID, 3, "wav", []byte("WAVDATA"))
	if err != nil {
		t.Fatalf("SaveNarration: %v", err)
	}
	if !strings.HasPrefix(path, filepath.Join(workspace, ".video-assets", jobID)) {
		t.Fatalf("path %q escapes the job assets dir", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("stored file unreadable: %v", err)
	}
	if string(got) != "WAVDATA" {
		t.Fatalf("stored bytes = %q", got)
	}
	if filepath.Base(path) != "narration-3.wav" {
		t.Fatalf("filename = %q, want narration-3.wav", filepath.Base(path))
	}

	// Jobs without uploads must produce no narration key on the wire —
	// byte-identical worker payloads as before the upload endpoint existed.
	submit := videopkg.SubmitJob{JobID: jobID}
	raw, err := json.Marshal(submit)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "narration") {
		t.Fatalf("empty job JSON must omit narration, got %s", raw)
	}

	// A nil-store dispatcher reports every job as not-found (the endpoint's
	// 404 path), never panics.
	if status, ok := disp.NarrationJob(context.Background(), jobID); ok {
		t.Fatalf("nil-store NarrationJob = (%q, true), want not-found", status)
	}
}
