import { useCallback, useRef } from "react";

// ---------- Types ----------

export type VideoItem = {
  id: string;
  kind: "video";
  file: File;
  srcUrl: string;
  outUrl: string | null;
  outName: string | null;
  status: "pending" | "processing" | "done" | "failed";
  progress: { current: number; total: number } | null;
  error?: string;
};

export type WatermarkEngine = Awaited<
  ReturnType<
    typeof import("@pilio/gemini-watermark-remover/browser")["createWatermarkEngine"]
  >
>;

// ---------- Helpers ----------

/** Inverse alpha blend over the watermark rect — the gemini-watermark-remover
 * core math: the watermark composites as w = a·255 + (1−a)·v with a KNOWN
 * white logo, so each pixel restores via v = (w − 255·a) / (1 − a). Cheap
 * enough per video frame (96×96 / 48×48 region). */
export function unblendWatermarkRegion(
  ctx: CanvasRenderingContext2D,
  rect: { x: number; y: number; width: number; height: number },
  alphaMap: Float32Array,
) {
  const x = Math.max(0, Math.round(rect.x));
  const y = Math.max(0, Math.round(rect.y));
  const w = Math.min(ctx.canvas.width - x, Math.round(rect.width));
  const h = Math.min(ctx.canvas.height - y, Math.round(rect.height));
  if (w <= 0 || h <= 0) return;
  const img = ctx.getImageData(x, y, w, h);
  const d = img.data;
  for (let py = 0; py < h; py++) {
    for (let px = 0; px < w; px++) {
      const a = alphaMap[py * w + px] ?? 0;
      if (a <= 0.001 || a >= 0.999) continue;
      const inv = 1 / (1 - a);
      const i = (py * w + px) * 4;
      const r = d[i] ?? 0;
      const g = d[i + 1] ?? 0;
      const b = d[i + 2] ?? 0;
      d[i] = Math.max(0, Math.min(255, (r - 255 * a) * inv));
      d[i + 1] = Math.max(0, Math.min(255, (g - 255 * a) * inv));
      d[i + 2] = Math.max(0, Math.min(255, (b - 255 * a) * inv));
    }
  }
  ctx.putImageData(img, x, y);
}

// ---------- Callbacks type ----------

export interface VideoProcessCallbacks {
  onProgress: (progress: { current: number; total: number }) => void;
  onDone: (outUrl: string, outName: string) => void;
  onError: (error: string) => void;
}

// ---------- Hook ----------

/**
 * Provides video watermark removal capabilities.
 *
 * - `ensureEngine` lazily loads and caches the Gemini watermark engine.
 * - `processVideo` processes a single video, calling the appropriate callback
 *   for progress, completion, or error. Reads `cancelRef.current` to support
 *   mid-process cancellation.
 * - `isSupported` indicates whether `MediaRecorder` is available in this
 *   browser (required for video encoding).
 */
export function useVideoWatermark() {
  const engineRef = useRef<WatermarkEngine | null>(null);

  /** Lazily create the shared watermark engine (loads the calibrated Gemini
   * alpha maps once — GargantuaX/gemini-watermark-remover core). */
  const ensureEngine = useCallback(async (): Promise<WatermarkEngine> => {
    if (!engineRef.current) {
      const mod = await import("@pilio/gemini-watermark-remover/browser");
      engineRef.current = await mod.createWatermarkEngine();
    }
    return engineRef.current;
  }, []);

  /** Process a single video: decode frames, unblend watermark, re-encode.
   *  Progress/completion/error are reported via the callbacks object. */
  const processVideo = useCallback(
    async (
      item: VideoItem,
      engine: WatermarkEngine,
      cancelRef: React.MutableRefObject<boolean>,
      callbacks: VideoProcessCallbacks,
    ) => {
      const { onProgress, onDone, onError } = callbacks;
      const file = item.file;

      try {
        const videoUrl = URL.createObjectURL(file);
        const video = document.createElement("video");
        video.crossOrigin = "anonymous";
        video.muted = true;
        video.playsInline = true;
        video.preload = "auto";
        video.src = videoUrl;

        await new Promise<void>((resolve, reject) => {
          video.onloadedmetadata = () => resolve();
          video.onerror = () => reject(new Error("Failed to load video"));
          setTimeout(() => reject(new Error("Video load timeout")), 10000);
        });

        if (video.duration > 30) {
          URL.revokeObjectURL(videoUrl);
          throw new Error("Video too long (max 30s)");
        }

        const vw = video.videoWidth;
        const vh = video.videoHeight;
        const canvas = document.createElement("canvas");
        canvas.width = vw;
        canvas.height = vh;
        const ctx = canvas.getContext("2d", { willReadFrequently: true });
        if (!ctx) throw new Error("Canvas 2D not supported");

        // Gemini watermark position comes from the size catalog (96px logo with
        // 64px margins on large outputs, 48px/32px on small ones) and the alpha
        // map is the project's calibrated asset — same data the image pipeline
        // uses, applied per frame here.
        const info = engine.getWatermarkInfo(vw, vh);
        const alphaMap = await engine.getAlphaMap(info.size);

        const fps = 30;
        const canvasStream = canvas.captureStream(fps);
        const mimeType = MediaRecorder.isTypeSupported("video/mp4")
          ? "video/mp4"
          : MediaRecorder.isTypeSupported("video/webm;codecs=vp9")
            ? "video/webm;codecs=vp9"
            : MediaRecorder.isTypeSupported("video/webm;codecs=vp8")
              ? "video/webm;codecs=vp8"
              : "video/webm";

        const recorder = new MediaRecorder(canvasStream, {
          mimeType,
          videoBitsPerSecond: 5_000_000,
        });

        const chunks: Blob[] = [];
        recorder.ondataavailable = (e) => {
          if (e.data.size > 0) chunks.push(e.data);
        };

        const recorderDone = new Promise<Blob>((resolve, reject) => {
          recorder.onstop = () => resolve(new Blob(chunks, { type: mimeType }));
          recorder.onerror = () => reject(new Error("MediaRecorder error"));
        });

        const totalFrames = Math.ceil(video.duration * fps);

        onProgress({ current: 0, total: totalFrames });

        video.currentTime = 0;
        await new Promise<void>((r) => {
          video.onseeked = () => r();
        });

        // MediaRecorder timestamps frames in wall-clock time, so the video
        // plays in REAL time while a rAF loop unblends each presented frame —
        // output duration matches the source.
        recorder.start(250);
        const durationMs = video.duration * 1000;
        const startedAt = performance.now();
        let lastReport = 0;

        video.play().catch(() => {
          /* autoplay of a muted element rarely fails */
        });

        await new Promise<void>((resolve) => {
          const step = () => {
            const elapsed = performance.now() - startedAt;
            if (cancelRef.current || video.ended || elapsed >= durationMs) {
              resolve();
              return;
            }
            ctx.drawImage(video, 0, 0, vw, vh);
            unblendWatermarkRegion(ctx, info.position, alphaMap);
            if (elapsed - lastReport > 200) {
              lastReport = elapsed;
              const current = Math.min(
                totalFrames,
                Math.round((elapsed / durationMs) * totalFrames),
              );
              onProgress({ current, total: totalFrames });
            }
            requestAnimationFrame(step);
          };
          requestAnimationFrame(step);
        });

        video.pause();
        // Let the encoder flush the last presented frame before closing.
        await new Promise((r) => setTimeout(r, 150));
        if (recorder.state !== "inactive") recorder.stop();

        if (cancelRef.current) {
          URL.revokeObjectURL(videoUrl);
          return;
        }

        const resultBlob = await recorderDone;
        const outUrl = URL.createObjectURL(resultBlob);
        const baseName = file.name.replace(/\.[^.]+$/, "");
        const ext = resultBlob.type.includes("mp4") ? "mp4" : "webm";

        onDone(outUrl, `${baseName}-no-watermark.${ext}`);
      } catch (e) {
        onError(e instanceof Error ? e.message : String(e));
      }
    },
    [],
  );

  const isSupported = typeof MediaRecorder !== "undefined";

  return {
    processVideo,
    ensureEngine,
    isSupported,
  } as const;
}
