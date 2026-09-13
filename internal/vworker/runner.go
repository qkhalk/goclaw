package vworker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

// Runner manages the job lifecycle: validate → materialize → narrate →
// per-scene render → concat → mix audio → finalize.
type Runner struct {
	cfg  WorkerConfig
	mu   sync.Mutex
	jobs map[string]*jobState
}

// jobState holds per-job mutable state.
type jobState struct {
	mu       sync.Mutex
	Status   contract.JobStatus
	Progress int
	Error    string
	Output   string
	OutputSize int64
	DurationMS int64
	cancel   context.CancelFunc
}

// WorkerConfig holds all configuration for the worker.
type WorkerConfig struct {
	Addr          string
	Token         string
	WorkDir       string
	OutputDir     string
	FFmpegPath    string
	FFProbePath   string
	FontFile      string
	MaxSceneSec   float64
	MaxQueue      int
	NarratorVoice string
}

// NewRunner creates a new job runner.
func NewRunner(cfg WorkerConfig) *Runner {
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.FFProbePath == "" {
		cfg.FFProbePath = "ffprobe"
	}
	if cfg.MaxQueue <= 0 {
		cfg.MaxQueue = 5
	}
	return &Runner{
		cfg:  cfg,
		jobs: make(map[string]*jobState),
	}
}

// Submit adds a job to the queue. Returns immediately.
func (r *Runner) Submit(ctx context.Context, job contract.SubmitJob) contract.SubmitJobResponse {
	r.mu.Lock()
	if len(r.jobs) >= r.cfg.MaxQueue {
		r.mu.Unlock()
		return contract.SubmitJobResponse{
			JobID:  job.JobID,
			Status: contract.JobFailed,
		}
	}

	js := &jobState{Status: contract.JobQueued}
	r.jobs[job.JobID] = js
	r.mu.Unlock()

	// Start async
	go r.runJob(job, js)

	return contract.SubmitJobResponse{
		JobID:  job.JobID,
		Status: contract.JobQueued,
	}
}

// GetStatus returns the current state of a job.
func (r *Runner) GetStatus(jobID string) (*contract.JobState, bool) {
	r.mu.Lock()
	js, ok := r.jobs[jobID]
	r.mu.Unlock()
	if !ok {
		return nil, false
	}
	js.mu.Lock()
	defer js.mu.Unlock()
	return &contract.JobState{
		JobID:           jobID,
		Status:          js.Status,
		Progress:        js.Progress,
		Error:           js.Error,
		OutputPath:      js.Output,
		OutputSizeBytes: js.OutputSize,
		DurationMS:      js.DurationMS,
	}, true
}

// Cancel requests cancellation of a running job.
func (r *Runner) Cancel(jobID string) (contract.JobStatus, bool) {
	r.mu.Lock()
	js, ok := r.jobs[jobID]
	r.mu.Unlock()
	if !ok {
		return "", false
	}
	js.mu.Lock()
	defer js.mu.Unlock()
	if js.cancel != nil {
		js.cancel()
	}
	return js.Status, true
}

// ActiveJobs returns the number of queued + rendering jobs.
func (r *Runner) ActiveJobs() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, js := range r.jobs {
		js.mu.Lock()
		if js.Status == contract.JobQueued || js.Status == contract.JobRendering {
			count++
		}
		js.mu.Unlock()
	}
	return count
}

// runJob executes the full rendering pipeline for a single job.
func (r *Runner) runJob(job contract.SubmitJob, js *jobState) {
	start := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	js.mu.Lock()
	js.cancel = cancel
	js.mu.Unlock()

	// Create temp dir for this job
	tempDir, err := NewTempDir(r.cfg.WorkDir, job.JobID)
	if err != nil {
		r.failJob(js, fmt.Sprintf("create temp dir: %v", err))
		return
	}
	defer CleanupDir(tempDir)

	sb := job.Storyboard
	if sb == nil {
		r.failJob(js, "nil storyboard")
		return
	}

	// Validate
	if err := sb.Validate(); err != nil {
		r.failJob(js, fmt.Sprintf("invalid storyboard: %v", err))
		return
	}

	ffcfg := FFmpegConfig{
		FFmpegPath:  r.cfg.FFmpegPath,
		FFProbePath: r.cfg.FFProbePath,
		FontFile:    r.cfg.FontFile,
		MaxSceneSec: r.cfg.MaxSceneSec,
	}

	// Determine canvas dimensions
	canvasW, canvasH, fps := effectiveCanvas(sb)

	// Set rendering status
	js.mu.Lock()
	js.Status = contract.JobRendering
	js.Progress = 0
	js.mu.Unlock()

	narrator := NarratorFromName("edge", r.cfg.NarratorVoice)

	// Build narration map from submitted pre-synthesized files
	narrMap := make(map[int]string)
	for _, na := range job.Narration {
		narrMap[na.SceneIndex] = na.AudioPath
	}

	// Per-scene: narrate + render
	sceneFiles := make([]string, len(sb.Scenes))
	narrFiles := make([]string, len(sb.Scenes))
	for i, sc := range sb.Scenes {
		select {
		case <-ctx.Done():
			r.cancelJob(js)
			return
		default:
		}

		slog.Info("rendering scene", "job", job.JobID, "scene", i, "type", sc.Type)

		// Narration: use pre-synthesized or synthesize on the fly
		var narrPath string
		if p, ok := narrMap[i]; ok && p != "" {
			// Pre-synthesized narration
			resolved, err := Materialize(ctx, r.cfg.WorkDir, p, "")
			if err != nil {
				slog.Warn("narration materialize failed, skipping", "scene", i, "err", err)
			} else {
				narrPath = resolved
			}
		} else if sc.Narration != nil && sc.Narration.Text != "" {
			var narrErr error
			narrPath, narrErr = SynthesizeScene(ctx, narrator, i,
				sc.Narration.Text, sc.Narration.Voice, tempDir)
			if narrErr != nil {
				slog.Warn("narration synth failed, skipping", "scene", i, "err", narrErr)
			}
		}
		narrFiles[i] = narrPath

		// Materialize source asset
		scenePath, err := r.materializeSource(ctx, &sb.Scenes[i], tempDir)
		if err != nil {
			r.failJob(js, fmt.Sprintf("scene %d materialize: %v", i, err))
			return
		}
		// Update scene source to the materialized local path
		sb.Scenes[i].Source = scenePath

		// Build and run ffmpeg
		outPath := sceneOutputPath(tempDir, i)
		args := r.buildSceneArgs(ffcfg, &sb.Scenes[i], canvasW, canvasH, fps, outPath)

		if err := execFFmpeg(ctx, r.cfg.FFmpegPath, args); err != nil {
			r.failJob(js, fmt.Sprintf("scene %d render: %v", i, err))
			return
		}
		sceneFiles[i] = outPath

		// Update progress: scenes complete / total * 80% (leave 20% for concat+mix)
		progress := int(float64(i+1) / float64(len(sb.Scenes)) * 80)
		js.mu.Lock()
		js.Progress = progress
		js.mu.Unlock()
	}

	// Concat scenes
	concatPath := filepath.Join(tempDir, "concat.txt")
	if err := writeConcatFile(concatPath, sceneFiles); err != nil {
		r.failJob(js, fmt.Sprintf("write concat file: %v", err))
		return
	}
	concatOut := filepath.Join(tempDir, "concat.mp4")
	concatArgs := buildConcatArgs(ffcfg, sceneFiles, concatPath, concatOut)
	if err := execFFmpeg(ctx, r.cfg.FFmpegPath, concatArgs); err != nil {
		r.failJob(js, fmt.Sprintf("concat: %v", err))
		return
	}

	js.mu.Lock()
	js.Progress = 85
	js.mu.Unlock()

	// Mix audio (narration + BGM)
	format, _, _ := sb.EffectiveOutput()
	outputFilename := fmt.Sprintf("%s.%s", job.JobID, format)
	outputPath := filepath.Join(r.cfg.OutputDir, outputFilename)

	// Materialize BGM if present
	bgmPath := ""
	if sb.Audio.BGMPath != "" {
		var err error
		bgmPath, err = MaterializeBGM(ctx, r.cfg.WorkDir, sb.Audio.BGMPath)
		if err != nil {
			slog.Warn("BGM materialize failed, skipping", "err", err)
		}
	}

	// Filter out empty narration files
	validNarrFiles := filterEmpty(narrFiles)

	mixArgs := buildMixArgs(ffcfg, concatOut, outputPath,
		validNarrFiles, bgmPath, sb.Audio, fps)

	if err := execFFmpeg(ctx, r.cfg.FFmpegPath, mixArgs); err != nil {
		r.failJob(js, fmt.Sprintf("mix audio: %v", err))
		return
	}

	js.mu.Lock()
	js.Progress = 95
	js.mu.Unlock()

	// Finalize: get output file info
	info, err := os.Stat(outputPath)
	if err != nil {
		r.failJob(js, fmt.Sprintf("stat output: %v", err))
		return
	}

	elapsed := time.Since(start)
	js.mu.Lock()
	js.Status = contract.JobDone
	js.Progress = 100
	js.Output = outputPath
	js.OutputSize = info.Size()
	js.DurationMS = elapsed.Milliseconds()
	js.mu.Unlock()

	slog.Info("job complete", "job", job.JobID, "output", outputPath,
		"size", info.Size(), "duration", elapsed)
}

// buildSceneArgs dispatches to the appropriate scene builder.
func (r *Runner) buildSceneArgs(cfg FFmpegConfig, sc *contract.Scene, canvasW, canvasH, fps int, outputPath string) []string {
	switch sc.Type {
	case contract.SceneImage:
		return buildImageSceneArgs(cfg, *sc, canvasW, canvasH, fps, outputPath)
	case contract.SceneVideo:
		return buildVideoSceneArgs(cfg, *sc, canvasW, canvasH, fps, outputPath)
	case contract.SceneColor:
		return buildColorSceneArgs(cfg, *sc, canvasW, canvasH, fps, outputPath)
	default:
		return nil
	}
}

// materializeSource resolves the scene source to a local file.
func (r *Runner) materializeSource(ctx context.Context, sc *contract.Scene, tempDir string) (string, error) {
	if sc.Type == contract.SceneColor {
		return "", nil // color scenes have no source file
	}
	return Materialize(ctx, r.cfg.WorkDir, sc.Source, "")
}

func (r *Runner) failJob(js *jobState, msg string) {
	js.mu.Lock()
	defer js.mu.Unlock()
	js.Status = contract.JobFailed
	js.Error = msg
	slog.Error("job failed", "error", msg)
}

func (r *Runner) cancelJob(js *jobState) {
	js.mu.Lock()
	defer js.mu.Unlock()
	js.Status = contract.JobCancelled
}

// writeConcatFile writes the ffmpeg concat demuxer file list.
func writeConcatFile(path string, files []string) error {
	var b strings.Builder
	for _, f := range files {
		// Escape single quotes in file paths for concat demuxer
		escaped := strings.ReplaceAll(f, "'", "'\\''")
		fmt.Fprintf(&b, "file '%s'\n", escaped)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// effectiveCanvas returns the canvas dimensions with defaults applied.
func effectiveCanvas(sb *contract.Storyboard) (w, h, fps int) {
	w, h, fps = sb.Canvas.Width, sb.Canvas.Height, sb.Canvas.FPS
	if w <= 0 {
		w = 1080
	}
	if h <= 0 {
		h = 1920
	}
	if fps <= 0 {
		fps = 30
	}
	return w, h, fps
}

// filterEmpty removes empty strings from a slice.
func filterEmpty(ss []string) []string {
	var result []string
	for _, s := range ss {
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// sortJobsByAge returns job IDs sorted by creation time (oldest first).
// Used by cleanup to process jobs in order.
func sortJobsByAge(jobs map[string]*jobState) []string {
	type jobAge struct {
		id  string
		age time.Time
	}
	var ages []jobAge
	for id, js := range jobs {
		_ = js
		// Use jobID as fallback since we don't track creation time
		ages = append(ages, jobAge{id: id})
	}
	sort.Slice(ages, func(i, j int) bool {
		return ages[i].id < ages[j].id
	})
	ids := make([]string, len(ages))
	for i, a := range ages {
		ids[i] = a.id
	}
	return ids
}
