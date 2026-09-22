// Package contract is a copy-shape of the gateway's internal/video types.
// The worker binary intentionally does NOT import internal/video; this keeps
// the sidecar independent (separate binary, separate compile graph). A
// golden-file test (golden_test.go) guards the two parsers against drift.
package contract

// Storyboard is the render contract (JSON) between the gateway and worker.
type Storyboard struct {
	Version int      `json:"version"`
	Canvas  Canvas   `json:"canvas"`
	Engine  string   `json:"engine,omitempty"` // default "ffmpeg"
	Scenes  []Scene  `json:"scenes"`
	Audio   AudioMix `json:"audio"`
	Output  Output   `json:"output"`
}

// Canvas is the render frame. Defaults: 1080x1920 (9:16) @30fps.
type Canvas struct {
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
	FPS    int `json:"fps,omitempty"`
}

// SceneKind enumerates the supported scene sources.
type SceneKind string

const (
	SceneImage SceneKind = "image"
	SceneVideo SceneKind = "video"
	SceneColor SceneKind = "color"
)

// Scene enter transitions — exact values of the web editor's SceneTransition
// union ("none"|"fade"|"crossfade"|"slide_left"|"slide_up"). A transition
// decorates the START of a scene: for TRANSITION_SEC seconds the incoming
// scene blends in over the tail of the previous one.
const (
	TransitionNone      = "none"
	TransitionFade      = "fade"
	TransitionCrossfade = "crossfade"
	TransitionSlideLeft = "slide_left"
	TransitionSlideUp   = "slide_up"
)

// Scene is one clip in the timeline.
type Scene struct {
	Type        SceneKind  `json:"type"`
	Source      string     `json:"source,omitempty"` // workspace-relative path or URL
	Color       string     `json:"color,omitempty"`  // "#RRGGBB" for color scenes
	DurationSec float64    `json:"duration_sec"`
	Fit         string     `json:"fit,omitempty"` // cover|contain
	Mute        bool       `json:"mute,omitempty"`
	KenBurns    *KenBurns  `json:"ken_burns,omitempty"`
	Caption     *Caption   `json:"caption,omitempty"`
	Narration   *Narration `json:"narration,omitempty"`
	Transform   *Transform `json:"transform,omitempty"`  // per-scene geometry (image/video scenes)
	Filter      *Filter    `json:"filter,omitempty"`     // per-scene color grade (image/video scenes)
	Transition  string     `json:"transition,omitempty"` // enter transition: none|fade|crossfade|slide_left|slide_up
}

// Transform is an OpenCut-style per-scene geometric transform. The web editor
// pivots at the frame center: scale is a multiple around that center, x/y are
// offsets in % of frame size, rotate is degrees, opacity is 0-1 (0/unset =
// fully opaque).
type Transform struct {
	Scale   float64 `json:"scale,omitempty"`   // multiple around frame center; 1 = identity
	X       float64 `json:"x,omitempty"`       // offset, % of frame width
	Y       float64 `json:"y,omitempty"`       // offset, % of frame height
	Rotate  float64 `json:"rotate,omitempty"`  // degrees, clockwise
	Opacity float64 `json:"opacity,omitempty"` // 0-1 flattening over black; unset = opaque
}

// Filter is a CSS-style color grade applied to the scene's media.
type Filter struct {
	Brightness float64 `json:"brightness,omitempty"` // CSS brightness(k); 1 = normal
	Contrast   float64 `json:"contrast,omitempty"`   // CSS contrast(k); 1 = normal
	Saturate   float64 `json:"saturate,omitempty"`   // CSS saturate(k); 1 = normal
	Blur       float64 `json:"blur,omitempty"`       // Gaussian radius in px; 0 = none
}

// KenBurns animates a slow zoom/pan on image scenes.
type KenBurns struct {
	ZoomFrom float64 `json:"zoom_from,omitempty"`
	ZoomTo   float64 `json:"zoom_to,omitempty"`
	Pan      string  `json:"pan,omitempty"` // none|left|right|up|down
}

// Caption draws text over the scene via drawtext.
type Caption struct {
	Text     string `json:"text"`
	Position string `json:"position,omitempty"` // top|center|bottom
	FontSize int    `json:"font_size,omitempty"`
}

// Narration is the spoken text for the scene.
type Narration struct {
	Text     string `json:"text"`
	Provider string `json:"provider,omitempty"` // default "edge"
	Voice    string `json:"voice,omitempty"`    // e.g. "vi-VN-HoaiMyNeural"
}

// AudioMix configures the final audio graph.
type AudioMix struct {
	BGMPath         string  `json:"bgm_path,omitempty"`
	BGMVolume       float64 `json:"bgm_volume,omitempty"`
	NarrationVolume float64 `json:"narration_volume,omitempty"`
}

// Output describes the delivered file.
type Output struct {
	Format       string `json:"format,omitempty"`        // default mp4
	Height       int    `json:"height,omitempty"`        // 480|720|1080
	VideoBitrate string `json:"video_bitrate,omitempty"` // e.g. "1500k"
}

// Worker job API shapes.

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
	JobID      string           `json:"jobId"`
	Storyboard *Storyboard      `json:"storyboard"`
	Narration  []NarrationAudio `json:"narration,omitempty"`
	AssetsDir  string           `json:"assetsDir,omitempty"`
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
