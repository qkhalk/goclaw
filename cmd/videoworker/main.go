// Command videoworker is a standalone HTTP server that renders storyboard
// JSON into MP4 video using ffmpeg. It is designed to run alongside the
// GoClaw gateway as an independent sidecar process.
//
// Usage:
//
//	videoworker --addr 127.0.0.1:18791 --token <secret> --work-dir /tmp/vw --output-dir /var/www/videos
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/vworker"
)

func main() {
	var (
		addr        string
		token       string
		workDir     string
		outputDir   string
		ffmpegPath  string
		fontFile    string
		maxSceneSec float64
		maxQueue    int
		narrVoice   string
		ttlMinutes  int
	)

	flag.StringVar(&addr, "addr", "127.0.0.1:18791", "HTTP listen address")
	flag.StringVar(&token, "token", "", "Bearer token for authentication (empty = no auth)")
	flag.StringVar(&workDir, "work-dir", "/tmp/videoworker", "Working directory for temp files")
	flag.StringVar(&outputDir, "output-dir", "/var/www/videos", "Output directory for rendered videos")
	flag.StringVar(&ffmpegPath, "ffmpeg-path", "ffmpeg", "Path to ffmpeg binary")
	flag.StringVar(&fontFile, "font-file", "", "Path to font file for captions (e.g. Noto Sans)")
	flag.Float64Var(&maxSceneSec, "max-scene-sec", 30, "Maximum seconds per scene")
	flag.IntVar(&maxQueue, "max-queue", 5, "Maximum queued jobs")
	flag.StringVar(&narrVoice, "narr-voice", "vi-VN-HoaiMyNeural", "Default narration voice")
	flag.IntVar(&ttlMinutes, "ttl-minutes", 120, "TTL in minutes for orphan temp cleanup")
	flag.Parse()

	// Setup structured logging
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("videoworker starting",
		"addr", addr,
		"work_dir", workDir,
		"output_dir", outputDir,
		"ffmpeg", ffmpegPath,
		"font", fontFile,
	)

	// Ensure directories exist
	for _, dir := range []string{workDir, outputDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Error("create directory", "dir", dir, "err", err)
			os.Exit(1)
		}
	}

	// Cleanup orphaned temp dirs from previous runs
	ttl := time.Duration(ttlMinutes) * time.Minute
	vworker.CleanupSweep(workDir, ttl)

	// Create runner and server
	cfg := vworker.WorkerConfig{
		Addr:          addr,
		Token:         token,
		WorkDir:       workDir,
		OutputDir:     outputDir,
		FFmpegPath:    ffmpegPath,
		FFProbePath:   "ffprobe",
		FontFile:      fontFile,
		MaxSceneSec:   maxSceneSec,
		MaxQueue:      maxQueue,
		NarratorVoice: narrVoice,
	}

	runner := vworker.NewRunner(cfg)
	srv := vworker.NewServer(runner, token)

	// Periodic cleanup ticker
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			vworker.CleanupSweep(workDir, ttl)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		slog.Info("shutdown signal received", "signal", sig)
		os.Exit(0)
	}()

	// Start server (blocks)
	if err := srv.Start(addr); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
