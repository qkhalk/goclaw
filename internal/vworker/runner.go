package vworker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
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

// narrationTailSec is the breathing room added when a scene is stretched to
// fit its narration — the voice should land, not get clipped on the cut.
const narrationTailSec = 0.35

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
	// Fonts carries the extracted bundled font paths (display/body/mono) used
	// by the caption compositor and text layers. Empty = legacy drawtext only.
	Fonts FontSet
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
	// Capacity counts queued+rendering jobs only. Terminal jobs stay in the
	// map for status queries, so len(r.jobs) would permanently consume queue
	// slots (with max-queue 1, one finished job blocks all future submits).
	r.mu.Lock()
	active := 0
	for _, js := range r.jobs {
		js.mu.Lock()
		if js.Status == contract.JobQueued || js.Status == contract.JobRendering {
			active++
		}
		js.mu.Unlock()
	}
	if active >= r.cfg.MaxQueue {
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
		Fonts:       r.cfg.Fonts,
	}
	// Determine canvas dimensions, scaled to the delivery resolution
	canvasW, canvasH, fps := renderDims(sb)

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

	// Narration pre-pass: synthesize (or materialize) every scene's audio,
	// probe its real duration, and stretch scenes whose narration does not
	// fit. This runs BEFORE the transition-offset math below — extending a
	// scene after it would desync both the xfade offsets and the adelay mix
	// times from the actual scene boundaries. The probed durations also drive
	// the karaoke caption reveal (word k lights at k/N of the voice).
	narrFiles := make([]string, len(sb.Scenes))
	narrDur := make([]float64, len(sb.Scenes))
	for i := range sb.Scenes {
		select {
		case <-ctx.Done():
			r.cancelJob(js)
			return
		default:
		}
		sc := &sb.Scenes[i]
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

		if narrPath == "" {
			continue
		}
		dur, err := ProbeDuration(ctx, r.cfg.FFProbePath, narrPath)
		if err != nil {
			slog.Warn("narration probe failed", "scene", i, "err", err)
			continue
		}
		if dur <= 0 {
			continue
		}
		narrDur[i] = dur
		if need := dur + narrationTailSec; sc.DurationSec < need {
			slog.Info("extending scene to fit narration", "job", job.JobID,
				"scene", i, "from", sc.DurationSec, "to", need)
			sc.DurationSec = need
		}
	}

	// Transition timeline math: xfade overlaps scene tails by transitionSec,
	// so every scene except the last renders transitionSec longer — the
	// output start of scene i then still equals the sum of the original
	// durations, keeping narration alignment exact.
	hasTransitions := false
	for i := 1; i < len(sb.Scenes); i++ {
		if tr := sb.Scenes[i].Transition; tr != "" && tr != "none" {
			hasTransitions = true
			break
		}
	}
	// sceneStarts[i] is scene i's start in the final timeline (sum of the
	// scene durations as they are now — narration-fitted, pre-padding); the
	// adelay mix pins each narration clip to exactly this time.
	sceneStarts := make([]float64, len(sb.Scenes))
	offsets := make([]float64, 0, len(sb.Scenes)-1) // absolute start of scene i (i>=1)
	if hasTransitions {
		// Scene renders grow by transitionSec; let the safety cap follow so
		// the builder never trims the overlap away (that would desync xfade).
		if ffcfg.MaxSceneSec > 0 {
			ffcfg.MaxSceneSec += transitionSec
		}
	}
	cum := 0.0
	for i := 0; i < len(sb.Scenes); i++ {
		sceneStarts[i] = cum
		cum += sb.Scenes[i].DurationSec
		if hasTransitions && i < len(sb.Scenes)-1 {
			offsets = append(offsets, cum)
			sb.Scenes[i].DurationSec += transitionSec
		}
	}
	// Final video length: with xfade the padded tails are consumed by the
	// transitions, so this sum-of-originals is the timeline length in both
	// the concat and xfade paths — the silence base for the audio mix.
	totalDur := cum

	// Per-scene: render (narration was synthesized in the pre-pass above)
	sceneFiles := make([]string, len(sb.Scenes))
	for i, sc := range sb.Scenes {
		select {
		case <-ctx.Done():
			r.cancelJob(js)
			return
		default:
		}

		slog.Info("rendering scene", "job", job.JobID, "scene", i, "type", sc.Type)

		// Materialize source asset
		scenePath, err := r.materializeSource(ctx, &sb.Scenes[i], tempDir)
		if err != nil {
			r.failJob(js, fmt.Sprintf("scene %d materialize: %v", i, err))
			return
		}
		// Update scene source to the materialized local path
		sb.Scenes[i].Source = scenePath

		// Materialize image-layer sources the same way (runner rewrites in
		// place; the ffmpeg builders consume l.Source as a local path).
		for j := range sb.Scenes[i].Layers {
			l := &sb.Scenes[i].Layers[j]
			if l.Kind != contract.LayerImage {
				continue
			}
			lp, err := Materialize(ctx, r.cfg.WorkDir, l.Source, "")
			if err != nil {
				r.failJob(js, fmt.Sprintf("scene %d layer %d materialize: %v", i, j, err))
				return
			}
			l.Source = lp
		}

		// Build and run ffmpeg
		outPath := sceneOutputPath(tempDir, i)
		err = r.renderScene(ctx, ffcfg, &sb.Scenes[i], canvasW, canvasH, fps, outPath, tempDir, i, narrDur[i])
		if err != nil {
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

	// Concat scenes — xfade chain when transitions are present, stream-copy
	// concat demuxer otherwise (cheaper, byte-identical to before).
	// When xfade is needed, scenes are processed in batches of ≤xfadeBatchSize
	// to keep ffmpeg's file handle count and frame buffer memory bounded
	// (the old all-at-once approach OOMed on 5+ scenes at 720p with 350 MB).
	concatOut := filepath.Join(tempDir, "concat.mp4")
	if hasTransitions {
		transitions := make([]string, len(sb.Scenes))
		for i := range sb.Scenes {
			transitions[i] = sb.Scenes[i].Transition
		}
		if err := batchedXfade(ctx, r.cfg.FFmpegPath, sceneFiles, transitions, offsets, fps, tempDir, concatOut); err != nil {
			r.failJob(js, fmt.Sprintf("xfade concat: %v", err))
			return
		}
	} else {
		concatPath := filepath.Join(tempDir, "concat.txt")
		if err := writeConcatFile(concatPath, sceneFiles); err != nil {
			r.failJob(js, fmt.Sprintf("write concat file: %v", err))
			return
		}
		concatArgs := buildConcatArgs(ffcfg, sceneFiles, concatPath, concatOut)
		if err := execFFmpeg(ctx, r.cfg.FFmpegPath, concatArgs); err != nil {
			r.failJob(js, fmt.Sprintf("concat: %v", err))
			return
		}
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

	// Narration tracks pinned to each scene's start in the final timeline —
	// scenes without narration no longer push later audio out of sync.
	var tracks []NarrTrack
	for i, p := range narrFiles {
		if p == "" || i >= len(sceneStarts) {
			continue
		}
		tracks = append(tracks, NarrTrack{Path: p, StartSec: sceneStarts[i]})
	}

	mixArgs := buildMixArgs(ffcfg, concatOut, outputPath,
		tracks, bgmPath, sb.Audio, fps, totalDur)

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

// renderScene dispatches to the appropriate scene builder and runs ffmpeg.
// narrSec is the scene's probed narration duration (0 = none) — it drives the
// karaoke caption timing. Color scenes try the animated gradient first and
// fall back to the flat color source when the local ffmpeg lacks `gradients`
// (pre-4.4 builds).
func (r *Runner) renderScene(ctx context.Context, cfg FFmpegConfig, sc *contract.Scene, canvasW, canvasH, fps int, outputPath, tempDir string, sceneIdx int, narrSec float64) error {
	switch sc.Type {
	case contract.SceneImage:
		args, err := buildImageSceneArgs(cfg, *sc, canvasW, canvasH, fps, outputPath, tempDir, sceneIdx, narrSec)
		if err != nil {
			return err
		}
		return execFFmpeg(ctx, r.cfg.FFmpegPath, args)
	case contract.SceneVideo:
		return execFFmpeg(ctx, r.cfg.FFmpegPath,
			buildVideoSceneArgs(cfg, *sc, canvasW, canvasH, fps, outputPath, tempDir, sceneIdx, narrSec))
	case contract.SceneColor:
		args, err := buildColorSceneArgs(cfg, *sc, canvasW, canvasH, fps, outputPath, tempDir, sceneIdx, true, narrSec)
		if err != nil {
			return err
		}
		if err := execFFmpeg(ctx, r.cfg.FFmpegPath, args); err != nil {
			slog.Warn("gradient color scene failed, retrying flat color", "scene", sceneIdx)
			flatArgs, ferr := buildColorSceneArgs(cfg, *sc, canvasW, canvasH, fps, outputPath, tempDir, sceneIdx, false, narrSec)
			if ferr != nil {
				return ferr
			}
			return execFFmpeg(ctx, r.cfg.FFmpegPath, flatArgs)
		}
		return nil
	default:
		return fmt.Errorf("unknown scene type %q", sc.Type)
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

// renderDims returns the dimensions the filter graph actually renders at:
// the canvas scaled down to output.height when set (720p → 720×1280
// portrait / 1280×720 landscape, never upscaled). Running the pixel-heavy
// chain (zoompan, noise, vignette, overlays) at the delivery size instead
// of the full 1080×1920 canvas cuts per-frame work ~2.25x — the difference
// between a smooth render and a wedged 1-vCPU/512MB box.
func renderDims(sb *contract.Storyboard) (w, h, fps int) {
	w, h, fps = effectiveCanvas(sb)
	_, outShort, _ := sb.EffectiveOutput()
	if outShort <= 0 {
		return w, h, fps
	}
	short := min(w, h)
	if outShort >= short {
		return w, h, fps
	}
	s := float64(outShort) / float64(short)
	w = evenInt(int(math.Round(float64(w) * s)))
	h = evenInt(int(math.Round(float64(h) * s)))
	return w, h, fps
}

// evenInt clamps to a positive even value — yuv420p needs even dimensions.
func evenInt(n int) int {
	if n < 2 {
		return 2
	}
	return n / 2 * 2
}

// filterEmpty removes empty strings from a slice.
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

// batchedXfade processes scene transitions in groups of ≤xfadeBatchSize to
// keep ffmpeg's file handle count and frame buffer memory bounded. Each batch
// produces an intermediate .mp4; batches are then concatenated with stream
// copy (no re-encode) for the final output.
//
// Batch 0: scenes [0..N] → intermediate_001.mp4
// Batch 1: [intermediate_001, scenes N..M] → intermediate_002.mp4
// ...
// Final:   stream-copy concat all intermediates → outputPath
//
// The overlap model: each batch's last scene appears as the first input of
// the next batch. This ensures the xfade transition at the batch boundary
// is handled by the second batch's first xfade (offset=0), which blends the
// overlapping scene from the previous batch's output with the next scene.
//
// Within each batch, offsets are cumulative from the batch start (since the
// intermediate file's internal timeline resets to 0).
func batchedXfade(ctx context.Context, ffmpegPath string, sceneFiles []string, transitions []string, offsets []float64, fps int, tempDir, outputPath string) error {
	n := len(sceneFiles)
	if n == 0 {
		return fmt.Errorf("no scenes to xfade")
	}
	if n == 1 {
		// Single scene — just copy
		return execFFmpeg(ctx, ffmpegPath, []string{
			"-hide_banner", "-loglevel", "warning",
			"-i", sceneFiles[0], "-c", "copy", "-y", outputPath,
		})
	}

	// Collect intermediate files for final concat
	var intermediates []string
	prevXfade := "" // previous batch's output (starts empty = first scene file)

	batchStart := 0
	for batchStart < n {
		// Determine batch end: at most xfadeBatchSize scenes, but we need
		// an overlap scene (last of this batch = first of next) if there
		// are more scenes after this batch.
		batchEnd := batchStart + xfadeBatchSize
		if batchEnd >= n {
			batchEnd = n // include all remaining scenes
		}

		// Build batch inputs and transitions
		batchFiles := sceneFiles[batchStart:batchEnd]
		batchTrans := transitions[batchStart:batchEnd]
		batchOffsets := offsets[batchStart : batchEnd-1] // transitions between batch scenes

		if len(batchFiles) == 1 {
			// Single scene in batch — no xfade needed, just copy
			if prevXfade != "" {
				// This scene was already processed as part of previous
				// batch's overlap — skip.
				batchStart = batchEnd
				continue
			}
			// First batch with single scene (unlikely but safe)
			intermediates = append(intermediates, batchFiles[0])
			batchStart = batchEnd
			continue
		}

		// Compute batch-internal offsets (relative to batch start)
		// The batch offset for scene i within the batch is:
		// sum of durations of scenes batchStart..i-1 (original, before extension)
		// which equals offsets[i-1] - offsets[batchStart-1] (or offsets[i-1] if batchStart=0)
		batchOffsetsRelative := make([]float64, len(batchOffsets))
		for i := range batchOffsets {
			abs := batchOffsets[i]
			if batchStart > 0 {
				abs -= offsets[batchStart-1]
			}
			batchOffsetsRelative[i] = abs
		}

		// If there's a previous batch output, prepend it as the first input
		var xfadeInputs []string
		var xfadeTransitions []string
		var xfadeOffsets []float64
		if prevXfade != "" {
			xfadeInputs = append([]string{prevXfade}, batchFiles...)
			// The first transition (prevXfade → batchFiles[0]) is the batch
			// boundary transition. Its offset is 0 (the overlap scene starts
			// at the end of the previous batch's output, which xfade handles).
			xfadeTransitions = append([]string{batchTrans[0]}, batchTrans...)
			xfadeOffsets = append([]float64{0}, batchOffsetsRelative...)
		} else {
			xfadeInputs = batchFiles
			xfadeTransitions = batchTrans
			xfadeOffsets = batchOffsetsRelative
		}

		// Build batch output path
		batchIdx := len(intermediates)
		batchOut := filepath.Join(tempDir, fmt.Sprintf("xfade_batch_%03d.mp4", batchIdx))

		xfadeArgs := buildXfadeChainArgs(xfadeInputs, xfadeTransitions, xfadeOffsets, fps, batchOut)
		if err := execFFmpeg(ctx, ffmpegPath, xfadeArgs); err != nil {
			return fmt.Errorf("xfade batch %d: %w", batchIdx, err)
		}

		// Don't delete previous intermediate here — intermediates
		// are needed for the final concat. Cleaned up below.
		prevXfade = batchOut
		intermediates = append(intermediates, batchOut)

		batchStart = batchEnd
	}

	// If only one intermediate, just copy to output
	if len(intermediates) == 1 {
		return execFFmpeg(ctx, ffmpegPath, []string{
			"-hide_banner", "-loglevel", "warning",
			"-i", intermediates[0], "-c", "copy", "-y", outputPath,
		})
	}

	// Concat all intermediates with stream copy (no re-encode)
	concatPath := filepath.Join(tempDir, "xfade_concat.txt")
	if err := writeConcatFile(concatPath, intermediates); err != nil {
		return fmt.Errorf("write concat file: %w", err)
	}
	concatArgs := buildConcatArgs(FFmpegConfig{}, intermediates, concatPath, outputPath)
	if err := execFFmpeg(ctx, ffmpegPath, concatArgs); err != nil {
		return fmt.Errorf("xfade final concat: %w", err)
	}

	// Clean up intermediates
	for _, f := range intermediates {
		os.Remove(f)
	}

	return nil
}
