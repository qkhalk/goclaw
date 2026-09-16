package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	videopkg "github.com/nextlevelbuilder/goclaw/internal/video"
)

// fakeVideoJobStore is an in-memory VideoRenderJobStore for handler tests.
type fakeVideoJobStore struct {
	mu    sync.Mutex
	jobs  map[string]*store.VideoRenderJob
	order []string
}

func newFakeVideoJobStore() *fakeVideoJobStore {
	return &fakeVideoJobStore{jobs: map[string]*store.VideoRenderJob{}}
}

func (f *fakeVideoJobStore) Create(_ context.Context, job *store.VideoRenderJob) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *job
	f.jobs[job.ID] = &cp
	f.order = append(f.order, job.ID)
	return nil
}

func (f *fakeVideoJobStore) Get(_ context.Context, id string) (*store.VideoRenderJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		return nil, store.ErrVideoJobNotFound
	}
	cp := *j
	return &cp, nil
}

func (f *fakeVideoJobStore) UpdateStatus(_ context.Context, id string, upd store.VideoJobUpdate) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[id]
	if !ok {
		return store.ErrVideoJobNotFound
	}
	if upd.Status != "" {
		j.Status = upd.Status
	}
	return nil
}

func (f *fakeVideoJobStore) ListByTenant(_ context.Context, limit int) ([]store.VideoRenderJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.VideoRenderJob, 0, len(f.order))
	for i := len(f.order) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, *f.jobs[f.order[i]])
	}
	return out, nil
}

func (f *fakeVideoJobStore) ClaimNextQueued(_ context.Context) (*store.VideoRenderJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.order {
		if j := f.jobs[id]; j.Status == string(videopkg.JobQueued) {
			j.Status = string(videopkg.JobRendering)
			cp := *j
			return &cp, nil
		}
	}
	return nil, store.ErrNoQueuedJobs
}

func (f *fakeVideoJobStore) DeleteExpired(_ context.Context, before time.Time) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f *fakeVideoJobStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.jobs[id]; !ok {
		return store.ErrVideoJobNotFound
	}
	delete(f.jobs, id)
	for i, v := range f.order {
		if v == id {
			f.order = append(f.order[:i], f.order[i+1:]...)
			break
		}
	}
	return nil
}

// validStoryboard passes internal/video Validate() rules: version 1, one
// color scene with a legal duration.
const validStoryboard = `{"version":1,"canvas":{"width":1080,"height":1920,"fps":30},"scenes":[{"type":"color","color":"#101010","duration_sec":2}]}`

func setupVideoHandler(t *testing.T) (*VideoHandler, *fakeVideoJobStore) {
	t.Helper()
	setupTestToken(t, "test-token")
	fake := newFakeVideoJobStore()
	h := NewVideoHandler(fake, nil, nil, true)
	return h, fake
}

func doVideoReq(t *testing.T, h *VideoHandler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestVideoCreateJob_ObjectStoryboard(t *testing.T) {
	h, fake := setupVideoHandler(t)
	body := `{"storyboard":` + validStoryboard + `}`
	rec := doVideoReq(t, h, "POST", "/v1/video/jobs", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"jobId"`) || !strings.Contains(rec.Body.String(), `"queued"`) {
		t.Errorf("response missing jobId/status: %s", rec.Body.String())
	}
	if len(fake.jobs) != 1 {
		t.Fatalf("store jobs = %d, want 1", len(fake.jobs))
	}
}

func TestVideoCreateJob_RawJSONStoryboard(t *testing.T) {
	h, fake := setupVideoHandler(t)
	body := `{"storyboard_json":` + strconv_Quote(validStoryboard) + `}`
	rec := doVideoReq(t, h, "POST", "/v1/video/jobs", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if len(fake.jobs) != 1 {
		t.Fatalf("store jobs = %d, want 1", len(fake.jobs))
	}
}

func TestVideoCreateJob_InvalidStoryboard_Rejected(t *testing.T) {
	h, fake := setupVideoHandler(t)
	// version 2 is rejected by Validate(); duration 0 also fails rules.
	body := `{"storyboard":{"version":2,"scenes":[]}}`
	rec := doVideoReq(t, h, "POST", "/v1/video/jobs", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if len(fake.jobs) != 0 {
		t.Fatalf("store jobs = %d, want 0", len(fake.jobs))
	}
}

func TestVideoCreateJob_MissingStoryboard_Rejected(t *testing.T) {
	h, _ := setupVideoHandler(t)
	rec := doVideoReq(t, h, "POST", "/v1/video/jobs", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestVideoCreateJob_BadJSON_Rejected(t *testing.T) {
	h, _ := setupVideoHandler(t)
	rec := doVideoReq(t, h, "POST", "/v1/video/jobs", `{"storyboard": not-json}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestVideoCreateJob_Unauthenticated_Rejected(t *testing.T) {
	h, _ := setupVideoHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest("POST", "/v1/video/jobs", strings.NewReader(`{"storyboard":{}}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestVideoCreateJob_Disabled_Rejected(t *testing.T) {
	setupTestToken(t, "test-token")
	h := NewVideoHandler(newFakeVideoJobStore(), nil, nil, false)
	rec := doVideoReq(t, h, "POST", "/v1/video/jobs", `{"storyboard":{}}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
}

func TestVideoListJobs_ContainsCreated(t *testing.T) {
	h, _ := setupVideoHandler(t)
	doVideoReq(t, h, "POST", "/v1/video/jobs", `{"storyboard":`+validStoryboard+`}`)
	rec := doVideoReq(t, h, "GET", "/v1/video/jobs", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"jobs"`) || !strings.Contains(rec.Body.String(), `"status":"queued"`) {
		t.Errorf("list response missing jobs payload: %s", rec.Body.String()[:min(len(rec.Body.String()), 300)])
	}
}

// firstJobID returns the single job created by a POST in these tests.
func firstJobID(fake *fakeVideoJobStore) string {
	for id := range fake.jobs {
		return id
	}
	return ""
}

func TestVideoDeleteJob_TerminalJobRemoved(t *testing.T) {
	h, fake := setupVideoHandler(t)
	doVideoReq(t, h, "POST", "/v1/video/jobs", `{"storyboard":`+validStoryboard+`}`)
	id := firstJobID(fake)
	if err := fake.UpdateStatus(context.Background(), id, store.VideoJobUpdate{Status: "done"}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	rec := doVideoReq(t, h, "DELETE", "/v1/video/jobs/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"deleted"`) {
		t.Errorf("response should report deleted: %s", rec.Body.String())
	}
	if len(fake.jobs) != 0 {
		t.Fatalf("store jobs = %d, want 0 after delete", len(fake.jobs))
	}
}

func TestVideoDeleteJob_ActiveJobCancelledNotRemoved(t *testing.T) {
	h, fake := setupVideoHandler(t)
	doVideoReq(t, h, "POST", "/v1/video/jobs", `{"storyboard":`+validStoryboard+`}`)
	id := firstJobID(fake)

	rec := doVideoReq(t, h, "DELETE", "/v1/video/jobs/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"cancelled"`) {
		t.Errorf("active job should be cancelled: %s", rec.Body.String())
	}
	if len(fake.jobs) != 1 {
		t.Fatalf("store jobs = %d, want 1 (cancel, not delete)", len(fake.jobs))
	}
	if got := fake.jobs[id].Status; got != string(videopkg.JobCancelled) {
		t.Fatalf("status = %q, want cancelled", got)
	}
}

// strconv_Quote avoids importing strconv for one call.
func strconv_Quote(s string) string { return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"` }
