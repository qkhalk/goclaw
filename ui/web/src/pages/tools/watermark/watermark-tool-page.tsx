import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  CheckCircle2,
  Download,
  Eraser,
  HelpCircle,
  Loader2,
  ShieldCheck,
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
import {
  DEFAULT_MASK,
  type MaskRegion,
} from "./hooks/use-video-watermark";

// ---------- Types ----------

type TabValue = "images" | "videos";

type ImageStatus = "pending" | "processing" | "done" | "skipped" | "failed";

interface ImageItem {
  id: string;
  kind: "image";
  file: File;
  srcUrl: string;
  outUrl?: string;
  outName?: string;
  status: ImageStatus;
  error?: string;
  applied?: boolean;
}

interface VideoItem {
  id: string;
  kind: "video";
  file: File;
  srcUrl: string;
  outUrl: string | null;
  outName: string | null;
  status: "pending" | "processing" | "done" | "failed";
  progress: { current: number; total: number } | null;
  error?: string;
  mask: MaskRegion;
}

type Item = ImageItem | VideoItem;

// ---------- Constants ----------

const IMAGE_MAX_SIZE = 50 * 1024 * 1024;
const VIDEO_ACCEPT = "video/mp4,video/webm,video/quicktime";

// ---------- Comparison Slider ----------

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
      className="group/slider relative w-full cursor-ew-resize select-none overflow-hidden rounded-lg border"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      role="slider"
      aria-label={dragLabel}
      aria-valuenow={Math.round(pct)}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      {/* After image (full) */}
      <img src={afterSrc} alt="" className="block w-full" draggable={false} />

      {/* Before image (clipped) */}
      <img
        src={beforeSrc}
        alt=""
        className="absolute inset-0 w-full"
        style={{ clipPath: `inset(0 ${100 - pct}% 0 0)` }}
        draggable={false}
      />

      {/* Divider line */}
      <div
        className="absolute top-0 z-10 h-full w-0.5 bg-white shadow"
        style={{ left: `${pct}%`, transform: "translateX(-50%)" }}
      >
        {/* Handle */}
        <div className="absolute top-1/2 left-1/2 flex h-8 w-8 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full bg-white shadow-md ring-2 ring-black/10">
          <GripVertical className="h-4 w-4 text-muted-foreground" />
        </div>
      </div>

      {/* Labels */}
      <span className="absolute top-2 left-2 z-20 rounded bg-black/60 px-2 py-0.5 text-xs text-white">
        {beforeLabel}
      </span>
      <span className="absolute top-2 right-2 z-20 rounded bg-black/60 px-2 py-0.5 text-xs text-white">
        {afterLabel}
      </span>
    </div>
  );
}

// ---------- Mask Overlay (for video) ----------

function MaskOverlay({
  mask,
  onChange,
}: {
  mask: MaskRegion;
  onChange: (m: MaskRegion) => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <label className="text-xs text-muted-foreground">X:</label>
      <input
        type="range"
        min={0}
        max={100}
        value={mask.x}
        onChange={(e) => onChange({ ...mask, x: +e.target.value })}
        className="w-20 accent-primary"
      />
      <label className="text-xs text-muted-foreground">Y:</label>
      <input
        type="range"
        min={0}
        max={100}
        value={mask.y}
        onChange={(e) => onChange({ ...mask, y: +e.target.value })}
        className="w-20 accent-primary"
      />
      <label className="text-xs text-muted-foreground">W:</label>
      <input
        type="range"
        min={5}
        max={80}
        value={mask.width}
        onChange={(e) => onChange({ ...mask, width: +e.target.value })}
        className="w-20 accent-primary"
      />
      <label className="text-xs text-muted-foreground">H:</label>
      <input
        type="range"
        min={2}
        max={50}
        value={mask.height}
        onChange={(e) => onChange({ ...mask, height: +e.target.value })}
        className="w-20 accent-primary"
      />
      <label className="text-xs text-muted-foreground">Blur:</label>
      <input
        type="range"
        min={2}
        max={40}
        value={mask.blurRadius}
        onChange={(e) => onChange({ ...mask, blurRadius: +e.target.value })}
        className="w-20 accent-primary"
      />
    </div>
  );
}

// ---------- Status Chip ----------

function StatusChip({
  status,
  label,
}: {
  status: ImageStatus | VideoItem["status"];
  label: string;
}) {
  if (status === "pending") return null;
  const color =
    status === "processing"
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

// ---------- Main Page ----------

export function WatermarkToolPage() {
  const { t } = useTranslation("toolbox");
  const userId = useAuthStore((s) => s.userId);
  const [tab, setTab] = useState<TabValue>("images");
  const [items, setItems] = useState<Item[]>([]);
  const [running, setRunning] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const cancelRef = useRef(false);
  const removeFnRef = useRef<
    typeof import("@pilio/gemini-watermark-remover/browser")["removeWatermarkFromImage"] | null
  >(null);

  // Lazily load image remover
  const ensureRemoveFn = useCallback(async () => {
    if (!removeFnRef.current) {
      const mod = await import("@pilio/gemini-watermark-remover/browser");
      removeFnRef.current = mod.removeWatermarkFromImage;
    }
    return removeFnRef.current;
  }, []);

  // ---------- File handling ----------

  const addFiles = useCallback(
    (files: File[]) => {
      if (tab === "images") {
        const accepted = files.filter(
          (f) => f.type.startsWith("image/") && f.size <= IMAGE_MAX_SIZE,
        );
        if (accepted.length === 0) return;
        setItems((prev) => [
          ...prev,
          ...accepted.map((file) => ({
            id: `${userId}:${file.name}:${file.lastModified}:${Math.random().toString(36).slice(2)}`,
            kind: "image" as const,
            file,
            srcUrl: URL.createObjectURL(file),
            status: "pending" as const,
          })),
        ]);
      } else {
        const accepted = files.filter(
          (f) => f.type.startsWith("video/") && f.size <= 100 * 1024 * 1024,
        );
        if (accepted.length === 0) return;
        setItems((prev) => [
          ...prev,
          ...accepted.map((file) => ({
            id: `${userId}:${file.name}:${file.lastModified}:${Math.random().toString(36).slice(2)}`,
            kind: "video" as const,
            file,
            srcUrl: URL.createObjectURL(file),
            outUrl: null,
            outName: null,
            status: "pending" as const,
            progress: null,
            mask: { ...DEFAULT_MASK },
          })),
        ]);
      }
    },
    [tab, userId],
  );

  // ---------- Process all ----------

  const processAll = useCallback(async () => {
    setRunning(true);
    cancelRef.current = false;

    // Process images
    const imageItems = items.filter(
      (it): it is ImageItem => it.kind === "image" && it.status === "pending",
    );
    if (imageItems.length > 0) {
      const remove = await ensureRemoveFn();
      for (const item of imageItems) {
        if (cancelRef.current) break;
        setItems((prev) =>
          prev.map((it) =>
            it.id === item.id ? { ...it, status: "processing" as const } : it,
          ),
        );
        try {
          const img = new Image();
          img.src = item.srcUrl;
          await img.decode();
          const { canvas, meta } = await remove(img);
          const blob = await new Promise<Blob | null>((resolve) =>
            (canvas as HTMLCanvasElement).toBlob(resolve, "image/png"),
          );
          if (!blob) throw new Error("encode failed");
          const applied = meta?.applied ?? false;
          setItems((prev) =>
            prev.map((it) =>
              it.id === item.id && it.kind === "image"
                ? ({
                    ...it,
                    status: (applied ? "done" : "skipped") as ImageStatus,
                    applied,
                    outUrl: applied ? URL.createObjectURL(blob) : it.outUrl,
                    outName: applied
                      ? item.file.name.replace(/\.[^.]+$/, "") + "-no-watermark.png"
                      : it.outName,
                    error: applied ? undefined : (meta?.skipReason ?? undefined),
                  } satisfies ImageItem)
                : it,
            ),
          );
        } catch (e) {
          setItems((prev) =>
            prev.map((it) =>
              it.id === item.id && it.kind === "image"
                ? ({
                    ...it,
                    status: "failed" as const,
                    error: e instanceof Error ? e.message : String(e),
                  } satisfies ImageItem)
                : it,
            ),
          );
        }
      }
    }

    // Process videos (one at a time sequentially)
    const videoItems = items.filter(
      (it): it is VideoItem => it.kind === "video" && it.status === "pending",
    );
    for (const item of videoItems) {
      if (cancelRef.current) break;
      setItems((prev) =>
        prev.map((it) =>
          it.id === item.id
            ? { ...it, status: "processing" as const, progress: null }
            : it,
        ),
      );
      // We need to synchronously update progress, so we use the hook's process
      // and poll its progress state via a wrapper
      try {
        // The hook manages its own state; we need a different approach for multi-video.
        // Instead, use a per-item promise approach with the hook for single video,
        // or inline the processing for batch. We'll use the hook for the first video
        // and note that batch sequential is handled by the loop.
        // For simplicity in this implementation, we process one video at a time using
        // a dedicated processing approach inline.
        await processSingleVideo(item);
      } catch (e) {
        setItems((prev) =>
          prev.map((it) =>
            it.id === item.id && it.kind === "video"
              ? ({
                  ...it,
                  status: "failed" as const,
                  error: e instanceof Error ? e.message : String(e),
                } satisfies VideoItem)
              : it,
          ),
        );
      }
    }

    setRunning(false);
  }, [items, ensureRemoveFn]);

  // ---------- Single video processor (inline, for batch support) ----------

  const processSingleVideo = useCallback(async (item: VideoItem) => {
    const file = item.file;
    const mask = item.mask;

    const videoUrl = URL.createObjectURL(file);
    const video = document.createElement("video");
    video.crossOrigin = "anonymous";
    video.muted = true;
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
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("Canvas 2D not supported");

    const fps = 30;
    const canvasStream = canvas.captureStream(fps);
    const mimeType = MediaRecorder.isTypeSupported("video/webm;codecs=vp9")
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

    // Mask pixel coords
    const mx = Math.round((mask.x / 100) * vw);
    const my = Math.round((mask.y / 100) * vh);
    const mw = Math.round((mask.width / 100) * vw);
    const mh = Math.round((mask.height / 100) * vh);
    const mx0 = Math.max(0, mx - mw / 2);
    const my0 = Math.max(0, my - mh / 2);
    const mx1 = Math.min(vw, mx + mw / 2);
    const my1 = Math.min(vh, my + mh / 2);

    const totalFrames = Math.ceil(video.duration * fps);

    setItems((prev) =>
      prev.map((it) =>
        it.id === item.id
          ? { ...it, progress: { current: 0, total: totalFrames } }
          : it,
      ),
    );

    video.currentTime = 0;
    await new Promise<void>((r) => {
      video.onseeked = () => r();
    });

    recorder.start();

    const waitForFrame = (): Promise<void> =>
      new Promise((resolve) => {
        if (video.readyState >= 2) resolve();
        else video.oncanplay = () => resolve();
      });

    let frame = 0;
    while (!cancelRef.current && frame < totalFrames) {
      await waitForFrame();
      ctx.drawImage(video, 0, 0, vw, vh);

      if (mx1 > mx0 && my1 > my0) {
        const maskData = ctx.getImageData(mx0, my0, mx1 - mx0, my1 - my0);
        const tmpCanvas = document.createElement("canvas");
        tmpCanvas.width = mx1 - mx0;
        tmpCanvas.height = my1 - my0;
        const tmpCtx = tmpCanvas.getContext("2d")!;
        tmpCtx.putImageData(maskData, 0, 0);

        ctx.save();
        ctx.filter = `blur(${mask.blurRadius}px)`;
        ctx.drawImage(tmpCanvas, mx0, my0);
        ctx.restore();

        ctx.save();
        ctx.filter = `blur(${Math.max(1, mask.blurRadius / 3)}px)`;
        ctx.globalAlpha = 0.5;
        ctx.drawImage(tmpCanvas, mx0, my0);
        ctx.restore();
        ctx.globalAlpha = 1;
      }

      frame++;
      setItems((prev) =>
        prev.map((it) =>
          it.id === item.id
            ? { ...it, progress: { current: frame, total: totalFrames } }
            : it,
        ),
      );

      const nextTime = frame / fps;
      if (nextTime < video.duration) {
        video.currentTime = nextTime;
        await new Promise<void>((r) => {
          video.onseeked = () => r();
        });
      }
    }

    if (recorder.state !== "inactive") recorder.stop();

    if (cancelRef.current) {
      URL.revokeObjectURL(videoUrl);
      return;
    }

    const resultBlob = await recorderDone;
    const outUrl = URL.createObjectURL(resultBlob);
    const baseName = file.name.replace(/\.[^.]+$/, "");

    setItems((prev) =>
      prev.map((it) =>
        it.id === item.id && it.kind === "video"
          ? ({
              ...it,
              status: "done" as const,
              outUrl,
              outName: `${baseName}-no-watermark.webm`,
              progress: { current: totalFrames, total: totalFrames },
            } satisfies VideoItem)
          : it,
      ),
    );
  }, []);

  // ---------- Download ----------

  function downloadAll() {
    for (const item of items) {
      if (item.status === "done" && item.kind === "image" && item.outUrl && item.outName) {
        downloadUrl(item.outUrl, item.outName);
      }
      if (item.status === "done" && item.kind === "video" && item.outUrl && item.outName) {
        downloadUrl(item.outUrl, item.outName);
      }
    }
  }

  function downloadUrl(url: string, name: string) {
    const a = document.createElement("a");
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    a.remove();
  }

  // ---------- Clear / Remove ----------

  function clearAll() {
    for (const item of items) {
      URL.revokeObjectURL(item.srcUrl);
      if (item.kind === "image" && item.outUrl) URL.revokeObjectURL(item.outUrl);
      if (item.kind === "video" && item.outUrl) URL.revokeObjectURL(item.outUrl);
    }
    setItems([]);
  }

  function removeItem(id: string) {
    setItems((prev) => {
      const target = prev.find((it) => it.id === id);
      if (target) {
        URL.revokeObjectURL(target.srcUrl);
        if (target.kind === "image" && target.outUrl) URL.revokeObjectURL(target.outUrl);
        if (target.kind === "video" && target.outUrl) URL.revokeObjectURL(target.outUrl);
      }
      return prev.filter((it) => it.id !== id);
    });
  }

  function updateMask(id: string, mask: MaskRegion) {
    setItems((prev) =>
      prev.map((it) => (it.id === id && it.kind === "video" ? { ...it, mask } : it)),
    );
  }

  // ---------- Derived ----------

  const doneCount = items.filter((it) => it.status === "done").length;
  const filteredItems = items.filter((it) =>
    tab === "images" ? it.kind === "image" : it.kind === "video",
  );
  const imageCount = items.filter((it) => it.kind === "image").length;
  const videoCount = items.filter((it) => it.kind === "video").length;

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
      <PageHeader title={t("watermark.title")} description={t("watermark.description")} />

      {/* Privacy badge */}
      <div className="flex items-center gap-2 rounded-lg border border-green-500/30 bg-green-500/5 px-3 py-2">
        <ShieldCheck className="h-4 w-4 shrink-0 text-green-600" />
        <p className="text-sm text-muted-foreground">{t("watermark.privacy")}</p>
        <Badge variant="success" className="ml-auto shrink-0 text-[10px]">
          Client-Side Only
        </Badge>
      </div>

      {/* Tabs */}
      <Tabs value={tab} onValueChange={(v) => setTab(v as TabValue)}>
        <TabsList>
          <TabsTrigger value="images">
            <ImageIcon className="mr-1.5 h-4 w-4" />
            {t("watermark.tabs.images")}
            {imageCount > 0 && (
              <Badge variant="secondary" className="ml-1.5 text-[10px]">
                {imageCount}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="videos">
            <Video className="mr-1.5 h-4 w-4" />
            {t("watermark.tabs.videos")}
            {videoCount > 0 && (
              <Badge variant="secondary" className="ml-1.5 text-[10px]">
                {videoCount}
              </Badge>
            )}
          </TabsTrigger>
        </TabsList>

        {/* Images tab content */}
        <TabsContent value="images" className="flex flex-col gap-4">
          <DropZone onDrop={addFiles} title={t("watermark.drop_title")}>
            <div
              role="button"
              tabIndex={0}
              onClick={() => inputRef.current?.click()}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") inputRef.current?.click();
              }}
              className="flex min-h-40 cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border border-dashed p-6 text-center transition-colors hover:bg-muted/40"
            >
              <Eraser className="h-8 w-8 text-muted-foreground" />
              <p className="text-sm font-medium">{t("watermark.drop_title")}</p>
              <p className="text-xs text-muted-foreground">{t("watermark.drop_hint")}</p>
            </div>
          </DropZone>

          <ImageItemGrid
            items={filteredItems.filter((it): it is ImageItem => it.kind === "image")}
            running={running}
            onRemove={removeItem}
            t={t}
          />
        </TabsContent>

        {/* Videos tab content */}
        <TabsContent value="videos" className="flex flex-col gap-4">
          <DropZone onDrop={addFiles} title={t("watermark.drop_title_video")}>
            <div
              role="button"
              tabIndex={0}
              onClick={() => inputRef.current?.click()}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") inputRef.current?.click();
              }}
              className="flex min-h-40 cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border border-dashed p-6 text-center transition-colors hover:bg-muted/40"
            >
              <Video className="h-8 w-8 text-muted-foreground" />
              <p className="text-sm font-medium">{t("watermark.drop_title_video")}</p>
              <p className="text-xs text-muted-foreground">{t("watermark.video.max_duration")} &middot; {t("watermark.video.max_size")}</p>
            </div>
          </DropZone>

          <VideoItemGrid
            items={filteredItems.filter((it): it is VideoItem => it.kind === "video")}
            running={running}
            onRemove={removeItem}
            onMaskChange={updateMask}
            t={t}
          />
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
                {t("watermark.processed")} ({doneCount}/{items.length})
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

// ---------- Image Grid ----------

function ImageItemGrid({
  items,
  running,
  onRemove,
  t,
}: {
  items: ImageItem[];
  running: boolean;
  onRemove: (id: string) => void;
  t: (key: string) => string;
}) {
  if (items.length === 0) return null;

  return (
    <ul className="flex flex-col gap-4">
      {items.map((item) => (
        <li key={item.id} className="flex flex-col gap-3 rounded-lg border p-4">
          {/* Header row */}
          <div className="flex items-center gap-2">
            <p className="min-w-0 flex-1 truncate text-sm font-medium" title={item.file.name}>
              {item.file.name}
            </p>
            <StatusChip status={item.status} label={t(`watermark.${item.status}`)} />
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("watermark.remove")}
              disabled={running}
              onClick={() => onRemove(item.id)}
            >
              <XCircle className="h-4 w-4" />
            </Button>
          </div>

          {/* Comparison slider (if processed) */}
          {item.outUrl ? (
            <ComparisonSlider
              beforeSrc={item.srcUrl}
              afterSrc={item.outUrl}
              beforeLabel={t("watermark.comparison.before")}
              afterLabel={t("watermark.comparison.after")}
              dragLabel={t("watermark.comparison.drag_to_compare")}
            />
          ) : (
            <div className="flex aspect-video w-full items-center justify-center rounded-md border bg-muted/30">
              {item.status === "processing" ? (
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              ) : (
                <img
                  src={item.srcUrl}
                  alt={t("watermark.comparison.before")}
                  className="h-full w-full rounded-md object-contain"
                />
              )}
            </div>
          )}

          {/* Download button */}
          {item.status === "done" && item.outUrl && item.outName && (
            <Button variant="outline" size="sm" asChild className="min-h-11 w-fit sm:min-h-9">
              <a href={item.outUrl} download={item.outName}>
                <Download className="mr-2 h-4 w-4" />
                {t("watermark.download")}
              </a>
            </Button>
          )}

          {item.error && (
            <p className="truncate text-xs text-destructive" title={item.error}>
              {item.error}
            </p>
          )}
        </li>
      ))}
    </ul>
  );
}

// ---------- Video Grid ----------

function VideoItemGrid({
  items,
  running,
  onRemove,
  onMaskChange,
  t,
}: {
  items: VideoItem[];
  running: boolean;
  onRemove: (id: string) => void;
  onMaskChange: (id: string, mask: MaskRegion) => void;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  if (items.length === 0) return null;

  return (
    <ul className="flex flex-col gap-4">
      {items.map((item) => (
        <li key={item.id} className="flex flex-col gap-3 rounded-lg border p-4">
          {/* Header row */}
          <div className="flex items-center gap-2">
            <p className="min-w-0 flex-1 truncate text-sm font-medium" title={item.file.name}>
              {item.file.name}
            </p>
            <StatusChip status={item.status} label={t(`watermark.${item.status}`)} />
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("watermark.remove")}
              disabled={running}
              onClick={() => onRemove(item.id)}
            >
              <XCircle className="h-4 w-4" />
            </Button>
          </div>

          {/* Video preview with mask overlay */}
          <div className="relative overflow-hidden rounded-md border">
            <video
              src={item.srcUrl}
              className="w-full"
              controls
              preload="metadata"
              muted
            />
            {/* Mask position indicator */}
            <div
              className="pointer-events-none absolute border-2 border-dashed border-yellow-400 bg-yellow-400/20"
              style={{
                left: `${item.mask.x - item.mask.width / 2}%`,
                top: `${item.mask.y - item.mask.height / 2}%`,
                width: `${item.mask.width}%`,
                height: `${item.mask.height}%`,
              }}
            />
          </div>

          {/* Mask controls */}
          <div className="flex flex-col gap-2">
            <p className="text-xs text-muted-foreground">{t("watermark.video.mask_position")}</p>
            <MaskOverlay mask={item.mask} onChange={(m) => onMaskChange(item.id, m)} />
          </div>

          {/* Progress */}
          {item.status === "processing" && item.progress && (
            <div className="flex flex-col gap-1">
              <Progress
                value={
                  item.progress.total > 0
                    ? (item.progress.current / item.progress.total) * 100
                    : 0
                }
                className="h-2"
              />
              <p className="text-center text-xs text-muted-foreground">
                {t("watermark.video.processing_frame", {
                  current: item.progress.current,
                  total: item.progress.total,
                })}
              </p>
            </div>
          )}

          {/* Before/After comparison (if processed) */}
          {item.outUrl && (
            <ComparisonSlider
              beforeSrc={item.srcUrl}
              afterSrc={item.outUrl}
              beforeLabel={t("watermark.comparison.before")}
              afterLabel={t("watermark.comparison.after")}
              dragLabel={t("watermark.comparison.drag_to_compare")}
            />
          )}

          {/* Download button */}
          {item.status === "done" && item.outUrl && item.outName && (
            <Button variant="outline" size="sm" asChild className="min-h-11 w-fit sm:min-h-9">
              <a href={item.outUrl} download={item.outName}>
                <Download className="mr-2 h-4 w-4" />
                {t("watermark.download")}
              </a>
            </Button>
          )}

          {item.error && (
            <p className="truncate text-xs text-destructive" title={item.error}>
              {item.error}
            </p>
          )}
        </li>
      ))}
    </ul>
  );
}
