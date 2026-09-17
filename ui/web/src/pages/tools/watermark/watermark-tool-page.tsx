import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  CheckCircle2,
  Columns2,
  Download,
  Eraser,
  HelpCircle,
  Loader2,
  Pause,
  Play,
  Plus,
  ShieldCheck,
  Sparkles,
  Trash2,
  XCircle,
  Video,
  ImageIcon,
  GripVertical,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { PageHeader } from "@/components/shared/page-header";
import { DropZone } from "@/components/shared/drop-zone";
import { useAuthStore } from "@/stores/use-auth-store";
import { cn } from "@/lib/utils";
import { RemovalReveal, type WatermarkRectFractions } from "./components/removal-reveal";

// ---------- Types ----------

type TabValue = "images" | "videos";
type ItemStatus = "pending" | "processing" | "done" | "skipped" | "failed";

interface BaseItem {
  id: string;
  file: File;
  srcUrl: string;
  outUrl?: string;
  outName?: string;
  status: ItemStatus;
  error?: string;
  /** Watermark rect in media fractions, for the reveal overlay. */
  rect?: WatermarkRectFractions;
}

interface ImageItem extends BaseItem {
  kind: "image";
  applied?: boolean;
}

interface VideoItem extends BaseItem {
  kind: "video";
  progress: { current: number; total: number } | null;
}

type Item = ImageItem | VideoItem;

type CompareMode = "parallel" | "slider";

// ---------- Constants ----------

const IMAGE_MAX_SIZE = 50 * 1024 * 1024;
const VIDEO_MAX_SIZE = 100 * 1024 * 1024;
const VIDEO_MAX_SECONDS = 120;
const VIDEO_ACCEPT = "video/mp4,video/webm,video/quicktime";

// ---------- Engine helpers ----------

type WatermarkEngine = Awaited<
  ReturnType<typeof import("@pilio/gemini-watermark-remover/browser")["createWatermarkEngine"]>
>;

/** The engine returns OffscreenCanvas in browsers that have it — that class
 * exposes convertToBlob(), not toBlob(). Handle both or every removal fails. */
async function canvasToBlob(canvas: HTMLCanvasElement | OffscreenCanvas): Promise<Blob | null> {
  if ("convertToBlob" in canvas) {
    return canvas.convertToBlob({ type: "image/png" });
  }
  return new Promise((resolve) => (canvas as HTMLCanvasElement).toBlob(resolve, "image/png"));
}

function rectFractions(
  rect: { x: number; y: number; width: number; height: number },
  mediaW: number,
  mediaH: number,
): WatermarkRectFractions | undefined {
  if (!mediaW || !mediaH) return undefined;
  return { x: rect.x / mediaW, y: rect.y / mediaH, w: rect.width / mediaW, h: rect.height / mediaH };
}

// ---------- Status Chip ----------

/** Status "done" is keyed as `processed` in the locale files. */
function statusLabelKey(status: ItemStatus): string {
  return status === "done" ? "watermark.processed" : `watermark.${status}`;
}

function StatusChip({ status, label }: { status: ItemStatus; label: string }) {
  if (status === "pending") return null;
  const color =    status === "processing"
      ? "text-blue-600"
      : status === "done"
        ? "text-green-600"
        : status === "skipped"
          ? "text-muted-foreground"
          : "text-destructive";
  const Icon =
    status === "processing"
      ? Loader2
      : status === "done"
        ? CheckCircle2
        : status === "skipped"
          ? HelpCircle
          : XCircle;
  return (
    <span className={cn("flex items-center gap-1 text-xs", color)}>
      <Icon className={cn("h-3 w-3", status === "processing" && "animate-spin")} />
      {label}
    </span>
  );
}

// ---------- Compare Slider (images) ----------

function ComparisonSlider({
  beforeSrc,
  afterSrc,
  beforeLabel,
  afterLabel,
  dragLabel,
}: {
  beforeSrc: string;
  afterSrc: string;
  beforeLabel: string;
  afterLabel: string;
  dragLabel: string;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [pct, setPct] = useState(50);
  const dragging = useRef(false);

  const updatePct = useCallback((clientX: number) => {
    const el = containerRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const x = Math.max(0, Math.min(clientX - rect.left, rect.width));
    setPct((x / rect.width) * 100);
  }, []);

  const onPointerDown = useCallback(
    (e: React.PointerEvent) => {
      e.preventDefault();
      dragging.current = true;
      (e.target as HTMLElement).setPointerCapture(e.pointerId);
      updatePct(e.clientX);
    },
    [updatePct],
  );

  const onPointerMove = useCallback(
    (e: React.PointerEvent) => {
      if (!dragging.current) return;
      updatePct(e.clientX);
    },
    [updatePct],
  );

  const onPointerUp = useCallback(() => {
    dragging.current = false;
  }, []);

  return (
    <div
      ref={containerRef}
      className="group/slider relative w-full cursor-ew-resize select-none overflow-hidden rounded-lg border bg-muted/30"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      role="slider"
      aria-label={dragLabel}
      aria-valuenow={Math.round(pct)}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      <div className="flex h-[60vh] items-center justify-center p-2">
        <div className="relative inline-block leading-none">
          <img src={afterSrc} alt="" className="max-h-[calc(60vh-1rem)] max-w-full" draggable={false} />
          <img
            src={beforeSrc}
            alt=""
            className="absolute inset-0 h-full w-full"
            style={{ clipPath: `inset(0 ${100 - pct}% 0 0)` }}
            draggable={false}
          />
        </div>
      </div>

      <div
        className="absolute top-0 z-10 h-full w-0.5 bg-white shadow"
        style={{ left: `${pct}%`, transform: "translateX(-50%)" }}
      >
        <div className="absolute left-1/2 top-1/2 flex h-8 w-8 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full bg-white shadow-md ring-2 ring-black/10">
          <GripVertical className="h-4 w-4 text-muted-foreground" />
        </div>
      </div>

      <span className="absolute left-2 top-2 z-20 rounded bg-black/60 px-2 py-0.5 text-xs text-white">
        {beforeLabel}
      </span>
      <span className="absolute right-2 top-2 z-20 rounded bg-black/60 px-2 py-0.5 text-xs text-white">
        {afterLabel}
      </span>
    </div>
  );
}

// ---------- Panel (one side of the parallel compare) ----------

function PanelLabel({ children, tone }: { children: React.ReactNode; tone?: "muted" | "success" }) {
  return (
    <span
      className={cn(
        "absolute left-2 top-2 z-10 rounded px-2 py-0.5 text-[11px] font-medium uppercase tracking-wide",
        tone === "success" ? "bg-green-600/90 text-white" : "bg-black/60 text-white",
      )}
    >
      {children}
    </span>
  );
}

// ---------- Main Page ----------

export function WatermarkToolPage() {
  const { t } = useTranslation("toolbox");
  const userId = useAuthStore((s) => s.userId);
  const [tab, setTab] = useState<TabValue>("images");
  const [items, setItems] = useState<Item[]>([]);
  const [running, setRunning] = useState(false);
  const [mode, setMode] = useState<CompareMode>("parallel");
  const [selected, setSelected] = useState<Partial<Record<TabValue, string>>>({});
  const [revealed, setRevealed] = useState<Set<string>>(new Set());
  const inputRef = useRef<HTMLInputElement>(null);
  const cancelRef = useRef(false);
  const removeFnRef = useRef<
    typeof import("@pilio/gemini-watermark-remover/browser")["removeWatermarkFromImage"] | null
  >(null);
  const engineRef = useRef<WatermarkEngine | null>(null);

  const ensureRemoveFn = useCallback(async () => {
    if (!removeFnRef.current) {
      const mod = await import("@pilio/gemini-watermark-remover/browser");
      removeFnRef.current = mod.removeWatermarkFromImage;
    }
    return removeFnRef.current;
  }, []);

  /** Shared engine: caches the calibrated Gemini alpha maps (GargantuaX /
   * AllenK reverse alpha blending) across items and feeds the reveal overlay
   * with watermark geometry. */
  const ensureEngine = useCallback(async (): Promise<WatermarkEngine> => {
    if (!engineRef.current) {
      const mod = await import("@pilio/gemini-watermark-remover/browser");
      engineRef.current = await mod.createWatermarkEngine();
    }
    return engineRef.current;
  }, []);

  // ---------- File handling ----------

  const addFiles = useCallback(
    (files: File[]) => {
      const isImageTab = tab === "images";
      const accepted = files.filter((f) =>
        isImageTab
          ? f.type.startsWith("image/") && f.size <= IMAGE_MAX_SIZE
          : f.type.startsWith("video/") && f.size <= VIDEO_MAX_SIZE,
      );
      if (accepted.length === 0) return;
      const created: Item[] = accepted.map((file) =>
        isImageTab
          ? ({
              id: `${userId}:${file.name}:${file.lastModified}:${Math.random().toString(36).slice(2)}`,
              kind: "image" as const,
              file,
              srcUrl: URL.createObjectURL(file),
              status: "pending" as const,
            })
          : ({
              id: `${userId}:${file.name}:${file.lastModified}:${Math.random().toString(36).slice(2)}`,
              kind: "video" as const,
              file,
              srcUrl: URL.createObjectURL(file),
              status: "pending" as const,
              progress: null,
            }),
      );
      setItems((prev) => [...prev, ...created]);
      setSelected((prev) => ({ ...prev, [tab]: created[0]?.id }));
    },
    [tab, userId],
  );

  // ---------- Process all ----------

  const patchItem = useCallback((id: string, patch: Partial<ImageItem> & Partial<VideoItem>) => {
    setItems((prev) => prev.map((it) => (it.id === id ? ({ ...it, ...patch } as Item) : it)));
  }, []);

  const processImageItem = useCallback(
    async (item: ImageItem) => {
      const remove = await ensureRemoveFn();
      const engine = await ensureEngine();
      patchItem(item.id, { status: "processing" as const });
      const img = new Image();
      img.src = item.srcUrl;
      await img.decode();
      const { canvas, meta } = await remove(img, { engine });
      const info = engine.getWatermarkInfo(img.naturalWidth, img.naturalHeight);
      const blob = await canvasToBlob(canvas);
      if (!blob) throw new Error("encode failed");
      const applied = meta?.applied ?? false;
      patchItem(item.id, {
        status: (applied ? "done" : "skipped") as ItemStatus,
        applied,
        outUrl: applied ? URL.createObjectURL(blob) : undefined,
        outName: applied ? item.file.name.replace(/\.[^.]+$/, "") + "-no-watermark.png" : undefined,
        error: applied ? undefined : (meta?.skipReason ?? undefined),
        rect: rectFractions(info.position, img.naturalWidth, img.naturalHeight),
      });
    },
    [ensureRemoveFn, ensureEngine, patchItem],
  );

  const processSingleVideo = useCallback(
    async (item: VideoItem, engine: WatermarkEngine) => {
      const videoUrl = item.srcUrl;
      const video = document.createElement("video");
      video.crossOrigin = "anonymous";
      video.muted = false;
      video.playsInline = true;
      video.preload = "auto";
      video.src = videoUrl;

      await new Promise<void>((resolve, reject) => {
        video.onloadedmetadata = () => resolve();
        video.onerror = () => reject(new Error("Failed to load video"));
        setTimeout(() => reject(new Error("Video load timeout")), 10000);
      });

      if (video.duration > VIDEO_MAX_SECONDS) {
        throw new Error(`Video too long (max ${VIDEO_MAX_SECONDS}s)`);
      }

      const vw = video.videoWidth;
      const vh = video.videoHeight;
      const canvas = document.createElement("canvas");
      canvas.width = vw;
      canvas.height = vh;
      const ctx = canvas.getContext("2d", { willReadFrequently: true });
      if (!ctx) throw new Error("Canvas 2D not supported");

      // Anchor the watermark: try pixel detection on a few frames (scene
      // brightness varies; the sparkle may be too faint on the first dark
      // frame), falling back to the Gemini size catalog.
      const seekTo = async (time: number) => {
        video.currentTime = time;
        await new Promise<void>((r) => {
          video.onseeked = () => r();
          setTimeout(() => r(), 800);
        });
      };
      let detected: { x: number; y: number; width: number; height: number } | null = null;
      for (const frac of [0.35, 0.65, 0.15]) {
        await seekTo(video.duration * frac);
        ctx.drawImage(video, 0, 0, vw, vh);
        detected = detectSparkleRect(ctx);
        if (detected) break;
      }
      const info = engine.getWatermarkInfo(vw, vh);
      const rect = detected ?? info.position;
      let alphaMap: Float32Array;
      if (detected) {
        // Measure the watermark's REAL per-pixel opacity from the video
        // itself: the sparkle is static while the scene moves, so the
        // temporal minimum of the rect is watermark + darkest background.
        alphaMap = await temporalAlphaMap(video, ctx, rect, seekTo);
      } else {
        alphaMap = await engine.getAlphaMap(info.size);
      }
      patchItem(item.id, {
        rect: rectFractions(rect, vw, vh),
      });

      // Rewind to the start and lay down the first (already cleaned) frame
      // before the recorder opens, so the export starts at t=0.
      await seekTo(0);
      ctx.drawImage(video, 0, 0, vw, vh);
      unblendWatermarkRegion(ctx, rect, alphaMap);

      // The recorder must NEVER see a raw frame: captureStream samples the
      // canvas asynchronously and can grab it mid-callback (between drawImage
      // and the unblend passes). All processing happens on the work canvas;
      // only finished frames are blitted to the capture canvas.
      const capCanvas = document.createElement("canvas");
      capCanvas.width = vw;
      capCanvas.height = vh;
      const capCtx = capCanvas.getContext("2d");
      if (!capCtx) throw new Error("Canvas 2D not supported");
      capCtx.drawImage(canvas, 0, 0);

      // Capture video + original audio. Tapping the element into an audio
      // graph without a speaker connection keeps playback silent locally
      // while the recorder receives the track.
      const fps = 30;
      const canvasStream = capCanvas.captureStream(fps);
      let audioCtx: AudioContext | null = null;
      try {
        audioCtx = new AudioContext();
        // The processing click counts as user activation — resume() keeps the
        // graph pumping audio into the recorder even if it starts suspended.
        void audioCtx.resume();
        const source = audioCtx.createMediaElementSource(video);
        const dest = audioCtx.createMediaStreamDestination();
        source.connect(dest);
        for (const track of dest.stream.getAudioTracks()) canvasStream.addTrack(track);
      } catch {
        audioCtx?.close().catch(() => {});
        audioCtx = null;
      }

      const mimeCandidates = [
        'video/mp4;codecs="avc1.64001F,mp4a.40.2"',
        "video/mp4",
        'video/webm;codecs="vp9,opus"',
        'video/webm;codecs="vp8,opus"',
        "video/webm",
      ];
      const mimeType = mimeCandidates.find((m) => MediaRecorder.isTypeSupported(m)) ?? "video/webm";

      const recorder = new MediaRecorder(canvasStream, {
        mimeType,
        videoBitsPerSecond: 8_000_000,
        audioBitsPerSecond: 128_000,
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
      patchItem(item.id, { status: "processing" as const, progress: { current: 0, total: totalFrames } });

      recorder.start(250);
      const durationMs = video.duration * 1000;
      const startedAt = performance.now();
      let lastReport = 0;

      try {
        await video.play();
      } catch {
        // Autoplay with sound refused — fall back to a muted pass (no audio track).
        video.muted = true;
        await video.play().catch(() => {});
      }

      await new Promise<void>((resolve) => {
        const step = () => {
          const elapsed = performance.now() - startedAt;
          if (cancelRef.current || video.ended || elapsed >= durationMs) {
            resolve();
            return;
          }
          ctx.drawImage(video, 0, 0, vw, vh);
          unblendWatermarkRegion(ctx, rect, alphaMap);
          capCtx.drawImage(canvas, 0, 0);
          if (elapsed - lastReport > 200) {
            lastReport = elapsed;
            const current = Math.min(totalFrames, Math.round((elapsed / durationMs) * totalFrames));
            patchItem(item.id, { progress: { current, total: totalFrames } });
          }
          requestAnimationFrame(step);
        };
        requestAnimationFrame(step);
      });

      video.pause();
      // Let the encoder flush the last presented frame before closing.
      await new Promise((r) => setTimeout(r, 150));
      if (recorder.state !== "inactive") recorder.stop();
      audioCtx?.close().catch(() => {});

      if (cancelRef.current) return;

      const resultBlob = await recorderDone;
      const outUrl = URL.createObjectURL(resultBlob);
      const baseName = item.file.name.replace(/\.[^.]+$/, "");
      const ext = resultBlob.type.includes("mp4") ? "mp4" : "webm";

      patchItem(item.id, {
        status: "done" as const,
        outUrl,
        outName: `${baseName}-no-watermark.${ext}`,
        progress: { current: totalFrames, total: totalFrames },
      });
    },
    [patchItem],
  );

  const processAll = useCallback(async () => {
    setRunning(true);
    cancelRef.current = false;

    // Image engine failures must not kill the whole batch — load lazily
    // inside each item's try/catch via ensureRemoveFn/ensureEngine.
    const imageItems = items.filter(
      (it): it is ImageItem => it.kind === "image" && it.status === "pending",
    );
    for (const item of imageItems) {
      if (cancelRef.current) break;
      try {
        await processImageItem(item);
      } catch (e) {
        patchItem(item.id, {
          status: "failed" as const,
          error: e instanceof Error ? e.message : String(e),
        });
      }
    }

    const videoItems = items.filter(
      (it): it is VideoItem => it.kind === "video" && it.status === "pending",
    );
    for (const item of videoItems) {
      if (cancelRef.current) break;
      try {
        const engine = await ensureEngine();
        await processSingleVideo(item, engine);
      } catch (e) {
        patchItem(item.id, {
          status: "failed" as const,
          error: e instanceof Error ? e.message : String(e),
        });
      }
    }

    setRunning(false);
  }, [items, processImageItem, processSingleVideo, ensureEngine, patchItem]);

  // ---------- Download ----------

  const downloadUrl = (url: string, name: string) => {
    const a = document.createElement("a");
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    a.remove();
  };

  const downloadAll = () => {
    for (const item of items) {
      if (item.status === "done" && item.outUrl && item.outName) downloadUrl(item.outUrl, item.outName);
    }
  };

  // ---------- Clear / Remove ----------

  const clearAll = () => {
    for (const item of items) {
      URL.revokeObjectURL(item.srcUrl);
      if (item.outUrl) URL.revokeObjectURL(item.outUrl);
    }
    setItems([]);
    setSelected({});
    setRevealed(new Set());
  };

  const removeItem = (id: string) => {
    setItems((prev) => {
      const target = prev.find((it) => it.id === id);
      if (target) {
        URL.revokeObjectURL(target.srcUrl);
        if (target.outUrl) URL.revokeObjectURL(target.outUrl);
      }
      return prev.filter((it) => it.id !== id);
    });
    setSelected((prev) => {
      const next = { ...prev };
      for (const tb of ["images", "videos"] as const) {
        if (next[tb] === id) delete next[tb];
      }
      return next;
    });
  };

  // ---------- Derived ----------

  const doneCount = items.filter((it) => it.status === "done").length;
  const imageItems = items.filter((it): it is ImageItem => it.kind === "image");
  const videoItems = items.filter((it): it is VideoItem => it.kind === "video");
  const activeItems = tab === "images" ? imageItems : videoItems;
  const selectedItem =
    activeItems.find((it) => it.id === selected[tab]) ?? activeItems[0] ?? null;

  const markRevealed = useCallback((id: string) => {
    setRevealed((prev) => {
      if (prev.has(id)) return prev;
      const next = new Set(prev);
      next.add(id);
      return next;
    });
  }, []);

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 py-6">
      <PageHeader
        title={t("watermark.title")}
        description={t("watermark.description")}
        actions={
          <Badge variant="success" className="shrink-0 gap-1 text-[10px]">
            <ShieldCheck className="h-3 w-3" />
            Client-Side Only
          </Badge>
        }
      />

      <Tabs value={tab} onValueChange={(v) => setTab(v as TabValue)}>
        <TabsList>
          <TabsTrigger value="images">
            <ImageIcon className="mr-1.5 h-4 w-4" />
            {t("watermark.tabs.images")}
            {imageItems.length > 0 && (
              <Badge variant="secondary" className="ml-1.5 text-[10px]">
                {imageItems.length}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="videos">
            <Video className="mr-1.5 h-4 w-4" />
            {t("watermark.tabs.videos")}
            {videoItems.length > 0 && (
              <Badge variant="secondary" className="ml-1.5 text-[10px]">
                {videoItems.length}
              </Badge>
            )}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="images" className="flex flex-col gap-4">
          {imageItems.length === 0 ? (
            <UploadCard
              onPick={() => inputRef.current?.click()}
              onFiles={addFiles}
              icon={<ImageIcon className="h-8 w-8 text-muted-foreground" />}
              title={t("watermark.drop_title")}
              hint={t("watermark.drop_hint")}
            />
          ) : (
            <ImageWorkspace
              item={selectedItem as ImageItem | null}
              running={running}
              mode={mode}
              onModeChange={setMode}
              revealed={revealed}
              onRevealDone={markRevealed}
              t={t}
            />
          )}
          {imageItems.length > 0 && (
            <QueueFilmstrip
              items={imageItems}
              selectedId={selectedItem?.id}
              onSelect={(id) => setSelected((prev) => ({ ...prev, images: id }))}
              onAdd={() => inputRef.current?.click()}
              onRemove={removeItem}
              disabled={running}
              t={t}
            />
          )}
        </TabsContent>

        <TabsContent value="videos" className="flex flex-col gap-4">
          {videoItems.length === 0 ? (
            <UploadCard
              onPick={() => inputRef.current?.click()}
              onFiles={addFiles}
              icon={<Video className="h-8 w-8 text-muted-foreground" />}
              title={t("watermark.drop_title_video")}
              hint={`${t("watermark.video.max_duration")} · ${t("watermark.video.max_size")}`}
            />
          ) : (
            <VideoWorkspace
              item={selectedItem as VideoItem | null}
              revealed={revealed}
              onRevealDone={markRevealed}
              t={t}
            />
          )}
          {videoItems.length > 0 && (
            <QueueFilmstrip
              items={videoItems}
              selectedId={selectedItem?.id}
              onSelect={(id) => setSelected((prev) => ({ ...prev, videos: id }))}
              onAdd={() => inputRef.current?.click()}
              onRemove={removeItem}
              disabled={running}
              t={t}
            />
          )}
        </TabsContent>
      </Tabs>

      {/* Hidden file input */}
      <input
        ref={inputRef}
        type="file"
        accept={tab === "images" ? "image/png,image/jpeg,image/webp" : VIDEO_ACCEPT}
        multiple
        className="hidden"
        onChange={(e) => {
          addFiles(Array.from(e.target.files ?? []));
          e.target.value = "";
        }}
      />

      {/* Global action bar */}
      {items.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 border-t pt-4">
          <Button onClick={processAll} disabled={running} className="min-h-11 sm:min-h-9">
            {running ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                {t("watermark.processing")}
              </>
            ) : (
              <>
                <Eraser className="mr-2 h-4 w-4" />
                {t("watermark.process_all")}
                {` (${doneCount}/${items.length})`}
              </>
            )}
          </Button>
          <Button
            variant="outline"
            onClick={downloadAll}
            disabled={doneCount === 0}
            className="min-h-11 sm:min-h-9"
          >
            <Download className="mr-2 h-4 w-4" />
            {t("watermark.download_all")}
          </Button>
          <Button
            variant="ghost"
            onClick={clearAll}
            disabled={running}
            className="min-h-11 sm:min-h-9"
          >
            <Trash2 className="mr-2 h-4 w-4" />
            {t("watermark.clear")}
          </Button>
        </div>
      )}
    </div>
  );
}

// ---------- Upload card ----------

function UploadCard({
  onPick,
  onFiles,
  icon,
  title,
  hint,
}: {
  onPick: () => void;
  onFiles: (files: File[]) => void;
  icon: React.ReactNode;
  title: string;
  hint: string;
}) {
  return (
    <DropZone onDrop={onFiles}>
      <div
        role="button"
        tabIndex={0}
        onClick={onPick}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") onPick();
        }}
        className="flex min-h-44 cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border border-dashed p-6 text-center transition-colors hover:bg-muted/40"
      >
        {icon}
        <p className="text-sm font-medium">{title}</p>
        <p className="text-xs text-muted-foreground">{hint}</p>
      </div>
    </DropZone>
  );
}

// ---------- Inverse alpha blend (video frames) ----------

/** Core math of gemini-watermark-remover: the watermark composites as
 * w = a·255 + (1−a)·v with a known white logo, so each pixel restores via
 * v = (w − 255·a) / (1 − a). Small region (≤200×200) — cheap per frame.
 * Exactly ONE pass: the alpha map already reflects the real opacity, and
 * re-applying the formula over-corrects into a black silhouette. */
function unblendWatermarkRegion(
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
      d[i] = Math.max(0, Math.min(255, ((d[i] ?? 0) - 255 * a) * inv));
      d[i + 1] = Math.max(0, Math.min(255, ((d[i + 1] ?? 0) - 255 * a) * inv));
      d[i + 2] = Math.max(0, Math.min(255, ((d[i + 2] ?? 0) - 255 * a) * inv));
    }
  }
  ctx.putImageData(img, x, y);
}

/** Locate the sparkle watermark directly from pixels: AI-video generators do
 * not all follow Gemini's size catalog, so the catalog position alone misses
 * (e.g. Veo/Flow watermarks sit higher). The sparkle is the brightest small
 * blob in the bottom-right corner — find it and anchor the alpha map there. */
function detectSparkleRect(
  ctx: CanvasRenderingContext2D,
): { x: number; y: number; width: number; height: number } | null {
  const W = ctx.canvas.width;
  const H = ctx.canvas.height;
  const x0 = Math.floor(W * 0.7);
  const y0 = Math.floor(H * 0.65);
  const sw = W - x0;
  const sh = H - y0;
  if (sw <= 8 || sh <= 8) return null;
  const img = ctx.getImageData(x0, y0, sw, sh);
  const d = img.data;
  const lum = (i: number) => 0.299 * (d[i] ?? 0) + 0.587 * (d[i + 1] ?? 0) + 0.114 * (d[i + 2] ?? 0);

  // Brightest pixel in the corner region = sparkle core candidate.
  let best = -1;
  let bestLum = 0;
  for (let i = 0; i < d.length; i += 4) {
    const l = lum(i);
    if (l > bestLum) {
      bestLum = l;
      best = i;
    }
  }
  // A white semi-transparent sparkle must stand out clearly.
  if (best < 0 || bestLum < 110) return null;

  const cx = x0 + ((best / 4) % sw);
  const cy = y0 + Math.floor(best / 4 / sw);
  // Tight clustering: only pixels close in brightness to the core, close in
  // distance — scene highlights (zip ties, lamps) must not inflate the box.
  const thr = Math.max(95, bestLum - 25);
  const win = 60;
  const wx0 = Math.max(x0, cx - win);
  const wy0 = Math.max(y0, cy - win);
  const wx1 = Math.min(W, cx + win);
  const wy1 = Math.min(H, cy + win);
  let minX = wx1, minY = wy1, maxX = wx0, maxY = wy0, count = 0, sumX = 0, sumY = 0;
  for (let py = wy0; py < wy1; py++) {
    for (let px = wx0; px < wx1; px++) {
      const i = ((py - y0) * sw + (px - x0)) * 4;
      if (lum(i) >= thr) {
        count++;
        sumX += px;
        sumY += py;
        if (px < minX) minX = px;
        if (px > maxX) maxX = px;
        if (py < minY) minY = py;
        if (py > maxY) maxY = py;
      }
    }
  }
  if (count < 4) return null;
  const bw = maxX - minX + 1;
  const bh = maxY - minY + 1;
  // The threshold cluster only covers the star's bright core (its glow fades
  // gradually, so an extent-based bbox is unreliable). Validate that the core
  // is compact and star-like (long thin strips are scene highlights), then
  // size the box from the video dimension — AI-video watermarks run ~5.5% of
  // frame width across generators.
  if (bw > 60 || bh > 60 || bw / bh > 2.2 || bh / bw > 2.2) return null;
  const size = Math.max(48, Math.min(200, Math.round(W * 0.07)));
  const centerX = sumX / count;
  const centerY = sumY / count;
  return {
    x: Math.round(centerX - size / 2),
    y: Math.round(centerY - size / 2),
    width: size,
    height: size,
  };
}

/** Measure the watermark's per-pixel opacity from the video itself. The
 * sparkle is static while the scene moves, so the per-pixel temporal minimum
 * is "watermark over the darkest background that pixel ever shows":
 *   min_t(w) = a·255 + (1−a)·B  →  a = (min_t(w) − B) / (255 − B)
 * with B estimated as the rect's darkest percentile (clean background).
 * This is immune to the opacity mismatch of a calibrated foreign map. */
async function temporalAlphaMap(
  video: HTMLVideoElement,
  ctx: CanvasRenderingContext2D,
  rect: { x: number; y: number; width: number; height: number },
  seekTo: (t: number) => Promise<void>,
): Promise<Float32Array> {
  const x = Math.max(0, Math.round(rect.x));
  const y = Math.max(0, Math.round(rect.y));
  const w = Math.min(ctx.canvas.width - x, Math.round(rect.width));
  const h = Math.min(ctx.canvas.height - y, Math.round(rect.height));
  const n = w * h;
  const minLum = new Float32Array(n).fill(255);
  const probes = 8;
  for (let k = 0; k < probes; k++) {
    await seekTo(video.duration * (0.08 + (0.84 * k) / (probes - 1)));
    ctx.drawImage(video, 0, 0, ctx.canvas.width, ctx.canvas.height);
    const d = ctx.getImageData(x, y, w, h).data;
    for (let i = 0; i < n; i++) {
      const l = 0.299 * (d[i * 4] ?? 0) + 0.587 * (d[i * 4 + 1] ?? 0) + 0.114 * (d[i * 4 + 2] ?? 0);
      if (l < (minLum[i] ?? 255)) minLum[i] = l;
    }
  }
  // The background under the watermark must be estimated PER PIXEL: the
  // scene there is often much brighter than the rect's darkest areas, and a
  // single global level over-estimates alpha — the unblend then clamps the
  // star to black. Stage 1 spots clean pixels with a coarse global level;
  // stage 2 gives every pixel a local background from the clean pixels around
  // it (the watermark is sparse, so a small window always holds plenty).
  const sorted = Array.from(minLum).sort((a, b) => a - b);
  const globalBg = sorted[Math.floor(n * 0.1)] ?? 0;
  const coarse = new Uint8Array(n);
  for (let i = 0; i < n; i++) {
    coarse[i] = ((minLum[i] ?? 0) - globalBg) / Math.max(60, 255 - globalBg) < 0.12 ? 1 : 0;
  }
  const map = new Float32Array(n);
  const win = 7; // 15×15 window
  const samples: number[] = [];
  for (let py = 0; py < h; py++) {
    for (let px = 0; px < w; px++) {
      samples.length = 0;
      const wy0 = Math.max(0, py - win);
      const wy1 = Math.min(h - 1, py + win);
      const wx0 = Math.max(0, px - win);
      const wx1 = Math.min(w - 1, px + win);
      for (let qy = wy0; qy <= wy1; qy++) {
        for (let qx = wx0; qx <= wx1; qx++) {
          if (coarse[qy * w + qx]) samples.push(minLum[qy * w + qx] ?? globalBg);
        }
      }
      const bg =
        samples.length >= 8
          ? (samples.sort((a, b) => a - b)[Math.floor(samples.length / 2)] ?? globalBg)
          : globalBg;
      const span = Math.max(80, 255 - bg);
      // Margin keeps clean pixels at exactly 0 despite min-vs-median noise.
      map[py * w + px] = Math.max(0, Math.min(0.9, ((minLum[py * w + px] ?? 0) - bg - 4) / span));
    }
  }
  await seekTo(0);
  return map;
}

// ---------- Image Workspace ----------

function ImageWorkspace({
  item,
  running,
  mode,
  onModeChange,
  revealed,
  onRevealDone,
  t,
}: {
  item: ImageItem | null;
  running: boolean;
  mode: CompareMode;
  onModeChange: (m: CompareMode) => void;
  revealed: Set<string>;
  onRevealDone: (id: string) => void;
  t: (key: string) => string;
}) {
  if (!item) return null;
  const hasResult = item.status === "done" && !!item.outUrl;
  const shouldReveal = hasResult && !revealed.has(item.id);

  return (
    <div className="flex flex-col gap-3">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2">
        <p className="min-w-0 flex-1 truncate text-sm font-medium" title={item.file.name}>
          {item.file.name}
        </p>
        <StatusChip status={item.status} label={t(statusLabelKey(item.status))} />
        {hasResult && (
          <>
            <div className="flex overflow-hidden rounded-md border" role="group" aria-label={t("watermark.comparison.drag_to_compare")}>
              <button
                type="button"
                onClick={() => onModeChange("parallel")}
                aria-pressed={mode === "parallel"}
                className={cn(
                  "flex min-h-9 items-center gap-1.5 px-2.5 text-xs transition-colors min-h-11 sm:min-h-9",
                  mode === "parallel" ? "bg-primary text-primary-foreground" : "hover:bg-muted",
                )}
              >
                <Columns2 className="h-3.5 w-3.5" />
                {t("watermark.mode.parallel")}
              </button>
              <button
                type="button"
                onClick={() => onModeChange("slider")}
                aria-pressed={mode === "slider"}
                className={cn(
                  "flex min-h-9 items-center gap-1.5 px-2.5 text-xs transition-colors min-h-11 sm:min-h-9",
                  mode === "slider" ? "bg-primary text-primary-foreground" : "hover:bg-muted",
                )}
              >
                <GripVertical className="h-3.5 w-3.5" />
                {t("watermark.mode.slider")}
              </button>
            </div>
            {item.outUrl && item.outName && (
              <Button variant="outline" size="sm" asChild className="min-h-11 sm:min-h-9">
                <a href={item.outUrl} download={item.outName}>
                  <Download className="mr-1.5 h-4 w-4" />
                  {t("watermark.download")}
                </a>
              </Button>
            )}
          </>
        )}
      </div>

      {/* Compare area */}
      {mode === "slider" && hasResult && item.outUrl ? (
        <ComparisonSlider
          beforeSrc={item.srcUrl}
          afterSrc={item.outUrl}
          beforeLabel={t("watermark.comparison.before")}
          afterLabel={t("watermark.comparison.after")}
          dragLabel={t("watermark.comparison.drag_to_compare")}
        />
      ) : (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          {/* Before panel */}
          <figure className="relative flex items-center justify-center overflow-hidden rounded-lg border bg-muted/30 p-2 min-h-44">
            <PanelLabel>{t("watermark.original")}</PanelLabel>
            <img
              src={item.srcUrl}
              alt={t("watermark.original")}
              className="max-h-[60vh] w-auto max-w-full rounded object-contain"
              draggable={false}
            />
          </figure>

          {/* After panel */}
          <figure className="relative flex items-center justify-center overflow-hidden rounded-lg border bg-muted/30 p-2 min-h-44">
            <PanelLabel tone={hasResult ? "success" : undefined}>{t("watermark.result")}</PanelLabel>
            {item.status === "processing" ? (
              <div className="flex flex-col items-center gap-2 py-16 text-muted-foreground">
                <Loader2 className="h-6 w-6 animate-spin" />
                <span className="text-xs">{t("watermark.processing")}</span>
              </div>
            ) : hasResult && item.outUrl ? (
              <div className="relative inline-block leading-none">
                <img
                  src={item.outUrl}
                  alt={t("watermark.result")}
                  className="max-h-[60vh] w-auto max-w-full rounded object-contain"
                  draggable={false}
                />
                {shouldReveal && item.rect && (
                  <RemovalReveal
                    rect={item.rect}
                    revealKey={item.id}
                    label={t("watermark.anim.removing")}
                    className="inline-block"
                  />
                )}
              </div>
            ) : (
              <div className="flex flex-col items-center gap-2 py-16 text-muted-foreground">
                <Sparkles className="h-6 w-6" />
                <span className="max-w-[220px] text-center text-xs leading-relaxed">
                  {item.status === "skipped"
                    ? (item.error ?? t("watermark.skipped"))
                    : item.status === "failed"
                      ? (item.error ?? t("watermark.failed"))
                      : t("watermark.empty")}
                </span>
              </div>
            )}
            {shouldReveal && <RevealWatcher onDone={() => onRevealDone(item.id)} />}
          </figure>
        </div>
      )}

      {item.error && item.status === "failed" && (
        <p className="truncate text-xs text-destructive" title={item.error}>
          {item.error}
        </p>
      )}
      {running && item.status === "pending" && (
        <p className="text-xs text-muted-foreground">{t("watermark.processing")}</p>
      )}
    </div>
  );
}

/** Marks a reveal as consumed after the overlay animation window passes. */
function RevealWatcher({ onDone }: { onDone: () => void }) {
  useEffect(() => {
    const timer = setTimeout(onDone, 2400);
    return () => clearTimeout(timer);
  }, [onDone]);
  return null;
}

// ---------- Video Workspace ----------

function VideoWorkspace({
  item,
  revealed,
  onRevealDone,
  t,
}: {
  item: VideoItem | null;
  revealed: Set<string>;
  onRevealDone: (id: string) => void;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const origRef = useRef<HTMLVideoElement>(null);
  const outRef = useRef<HTMLVideoElement>(null);
  const [playing, setPlaying] = useState(false);
  const [time, setTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const shouldReveal = !!item && item.status === "done" && !revealed.has(item.id);

  const hasResult = !!item?.status === true && item.status === "done" && !!item.outUrl;

  // The result video is the clock leader; the original mirrors its position.
  const syncToLeader = useCallback(() => {
    const leader = outRef.current;
    const follower = origRef.current;
    if (leader && follower && Math.abs(follower.currentTime - leader.currentTime) > 0.25) {
      follower.currentTime = leader.currentTime;
    }
  }, []);

  const togglePlay = useCallback(() => {
    const leader = (outRef.current ?? origRef.current) as HTMLVideoElement | null;
    const other = outRef.current ? origRef.current : null;
    if (!leader) return;
    if (leader.paused) {
      leader.muted = false;
      void leader.play().catch(() => {
        leader.muted = true;
        void leader.play().catch(() => {});
      });
      if (other) {
        other.muted = true;
        void other.play().catch(() => {});
      }
    } else {
      leader.pause();
      other?.pause();
    }
  }, []);

  const onSeek = useCallback((value: number) => {
    const leader = outRef.current ?? origRef.current;
    const other = outRef.current ? origRef.current : null;
    if (leader) leader.currentTime = value;
    if (other) other.currentTime = value;
    setTime(value);
  }, []);

  if (!item) return null;

  return (
    <div className="flex flex-col gap-3">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2">
        <p className="min-w-0 flex-1 truncate text-sm font-medium" title={item.file.name}>
          {item.file.name}
        </p>
        <StatusChip status={item.status} label={t(statusLabelKey(item.status))} />
        {hasResult && item.outUrl && item.outName && (
          <Button variant="outline" size="sm" asChild className="min-h-11 sm:min-h-9">
            <a href={item.outUrl} download={item.outName}>
              <Download className="mr-1.5 h-4 w-4" />
              {t("watermark.download")}
            </a>
          </Button>
        )}
      </div>

      {/* Compare area */}
      <div className={cn("grid grid-cols-1 gap-3", hasResult && "md:grid-cols-2")}>
        {/* Before panel */}
        <figure className="relative flex items-center justify-center overflow-hidden rounded-lg border bg-black/90 p-2 min-h-44">
          <PanelLabel>{t("watermark.original")}</PanelLabel>
          <video
            ref={origRef}
            src={item.srcUrl}
            className="max-h-[60vh] w-auto max-w-full rounded"
            muted
            playsInline
            preload="metadata"
            onLoadedMetadata={(e) => {
              if (!outRef.current) setDuration(e.currentTarget.duration);
            }}
          />
        </figure>

        {/* After panel */}
        {hasResult && item.outUrl ? (
          <figure className="relative flex items-center justify-center overflow-hidden rounded-lg border bg-black/90 p-2 min-h-44">
            <PanelLabel tone="success">{t("watermark.result")}</PanelLabel>
            <div className="relative inline-block leading-none">
              <video
                ref={outRef}
                src={item.outUrl}
                className="max-h-[60vh] w-auto max-w-full rounded"
                playsInline
                preload="metadata"
                onTimeUpdate={(e) => {
                  setTime(e.currentTarget.currentTime);
                  syncToLeader();
                }}
                onLoadedMetadata={(e) => setDuration(e.currentTarget.duration)}
                onPlay={() => setPlaying(true)}
                onPause={() => setPlaying(false)}
                onEnded={() => setPlaying(false)}
              />
              {shouldReveal && item.rect && (
                <RemovalReveal
                  rect={item.rect}
                  revealKey={item.id}
                  label={t("watermark.anim.removing")}
                />
              )}
            </div>
            {shouldReveal && <RevealWatcher onDone={() => onRevealDone(item.id)} />}
          </figure>
        ) : (
          <figure className="relative flex items-center justify-center overflow-hidden rounded-lg border bg-muted/30 p-2 min-h-44">
            <PanelLabel>{t("watermark.result")}</PanelLabel>
            {item.status === "processing" ? (
              <div className="flex flex-col items-center gap-3 p-8">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                <Progress
                  value={
                    item.progress && item.progress.total > 0
                      ? (item.progress.current / item.progress.total) * 100
                      : 0
                  }
                  className="h-2 w-48"
                />
                <p className="text-center text-xs text-muted-foreground">
                  {t("watermark.video.processing_frame", {
                    current: item.progress?.current ?? 0,
                    total: item.progress?.total ?? 0,
                  })}
                </p>
              </div>
            ) : (
              <div className="flex flex-col items-center gap-2 p-8 text-muted-foreground">
                <Sparkles className="h-6 w-6" />
                <span className="max-w-[240px] text-center text-xs leading-relaxed">
                  {item.status === "failed"
                    ? (item.error ?? t("watermark.failed"))
                    : item.status === "skipped"
                      ? (item.error ?? t("watermark.skipped"))
                      : t("watermark.video.auto_note")}
                </span>
              </div>
            )}
          </figure>
        )}
      </div>

      {/* Shared transport (result is leader when present) */}
      <div className="flex items-center gap-3 rounded-lg border px-3 py-2">
        <Button
          variant="outline"
          size="icon-sm"
          onClick={togglePlay}
          aria-label={playing ? "Pause" : "Play"}
          className="min-h-11 min-w-11 sm:min-h-9 sm:min-w-9"
        >
          {playing ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4" />}
        </Button>
        <input
          type="range"
          min={0}
          max={duration || 1}
          step={0.05}
          value={time}
          onChange={(e) => onSeek(Number(e.target.value))}
          aria-label={t("watermark.comparison.drag_to_compare")}
          className="h-1.5 flex-1 cursor-pointer appearance-none rounded-full bg-secondary accent-primary [&::-webkit-slider-thumb]:h-3.5 [&::-webkit-slider-thumb]:w-3.5 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-primary"
        />
        <span className="whitespace-nowrap font-mono text-xs tabular-nums text-muted-foreground">
          {formatClock(time)} / {formatClock(duration)}
        </span>
      </div>

      <p className="text-xs text-muted-foreground">{t("watermark.video.audio_note")}</p>
      {item.error && item.status === "failed" && (
        <p className="truncate text-xs text-destructive" title={item.error}>
          {item.error}
        </p>
      )}
    </div>
  );
}

function formatClock(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

// ---------- Queue Filmstrip ----------

function QueueFilmstrip({
  items,
  selectedId,
  onSelect,
  onAdd,
  onRemove,
  disabled,
  t,
}: {
  items: Item[];
  selectedId?: string;
  onSelect: (id: string) => void;
  onAdd: () => void;
  onRemove: (id: string) => void;
  disabled: boolean;
  t: (key: string) => string;
}) {
  return (
    <ul className="flex flex-wrap gap-2" aria-label={t("watermark.download_all")}>
      {items.map((item) => (
        <li key={item.id} className="relative">
          <button
            type="button"
            onClick={() => onSelect(item.id)}
            aria-pressed={selectedId === item.id}
            className={cn(
              "group block h-16 w-24 overflow-hidden rounded-md border-2 transition-colors",
              selectedId === item.id ? "border-primary" : "border-transparent hover:border-muted-foreground/40",
              disabled && "cursor-not-allowed opacity-70",
            )}
            title={item.file.name}
          >
            {item.kind === "image" ? (
              <img src={item.srcUrl} alt="" className="h-full w-full object-cover" />
            ) : (
              <span className="flex h-full w-full items-center justify-center bg-muted/40">
                <Video className="h-5 w-5 text-muted-foreground" />
              </span>
            )}
            <span
              className={cn(
                "absolute right-1 top-1 h-2.5 w-2.5 rounded-full border border-background",
                item.status === "done" && "bg-green-500",
                item.status === "processing" && "animate-pulse bg-blue-500",
                item.status === "pending" && "bg-muted-foreground/40",
                item.status === "skipped" && "bg-yellow-500",
                item.status === "failed" && "bg-destructive",
              )}
            />
          </button>
          <button
            type="button"
            onClick={() => !disabled && onRemove(item.id)}
            disabled={disabled}
            aria-label={t("watermark.remove")}
            className="absolute -right-1.5 -top-1.5 hidden h-5 w-5 items-center justify-center rounded-full bg-destructive text-white group-hover:flex sm:flex sm:opacity-0 sm:transition-opacity sm:hover:opacity-100"
          >
            <XCircle className="h-3.5 w-3.5" />
          </button>
        </li>
      ))}
      <li>
        <button
          type="button"
          onClick={onAdd}
          disabled={disabled}
          aria-label={t("watermark.drop_title")}
          className="flex h-16 w-24 items-center justify-center rounded-md border border-dashed text-muted-foreground transition-colors hover:bg-muted/40 disabled:cursor-not-allowed disabled:opacity-70"
        >
          <Plus className="h-5 w-5" />
        </button>
      </li>
    </ul>
  );
}
