package video

// Worker job API contract (HTTP between gateway and sidecar worker).
// JSON tags are camelCase per the wire convention.

// JobStatus is the worker-side lifecycle state.
type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRendering JobStatus = "rendering"
	JobDone      JobStatus = "done"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// SubmitJob is the POST /v1/jobs request body.
type SubmitJob struct {
	JobID     string     `json:"jobId"`
	Storyboard *Storyboard `json:"storyboard"`
	// Narration is pre-synthesized audio handed to the worker as absolute
	// paths inside assetsDir (gateway synthesizes via Edge TTS in Phase 4 —
	// the worker itself never calls TTS providers).
	Narration []NarrationAudio `json:"narration,omitempty"`
	// AssetsDir is the worker-local directory holding every storyboard
	// asset (gateway mirrors workspace files into it before submit).
	AssetsDir string `json:"assetsDir,omitempty"`
}

// NarrationAudio binds one scene to its synthesized audio file.
type NarrationAudio struct {
	SceneIndex int    `json:"sceneIndex"`
	AudioPath  string `json:"audioPath"`
}

// SubmitJobResponse is the POST /v1/jobs reply.
type SubmitJobResponse struct {
	JobID  string    `json:"jobId"`
	Status JobStatus `json:"status"`
}

// JobState is the GET /v1/jobs/{id} reply.
type JobState struct {
	JobID           string    `json:"jobId"`
	Status          JobStatus `json:"status"`
	Progress        int       `json:"progress"` // 0-100
	Error           string    `json:"error,omitempty"`
	OutputPath      string    `json:"outputPath,omitempty"`
	OutputSizeBytes int64     `json:"outputSizeBytes,omitempty"`
	DurationMS      int64     `json:"durationMs,omitempty"`
}

// CancelResponse is the POST /v1/jobs/{id}/cancel reply.
type CancelResponse struct {
	JobID  string    `json:"jobId"`
	Status JobStatus `json:"status"`
}
