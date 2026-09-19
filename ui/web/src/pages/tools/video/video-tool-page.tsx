import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  Clapperboard,
  Download,
  Eraser,
  RefreshCw,
  Trash2,
  X,
  Pencil,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { applyTheme } from "@/components/providers/theme-provider";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { useHttp } from "@/hooks/use-ws";
import { useUiStore } from "@/stores/use-ui-store";
import { toast } from "@/stores/use-toast-store";
import { cn } from "@/lib/utils";
import { formatFileSize } from "@/lib/format";
import {
  useVideoCancel,
  useVideoDelete,
  useVideoJobs,
  type VideoRenderJob,
} from "./hooks/use-video";
import { useTimeline, type Scene } from "./hooks/use-timeline";
import { useNarrationAudio } from "./hooks/use-narration-audio";
import { normalizeEditorScenes } from "./lib/storyboard-wire";
import { useVideoExport, exportExtension } from "./hooks/use-video-export";
import { CanvasPlayer } from "./components/canvas-player";
import { Timeline } from "./components/timeline";
import { SceneCard } from "./components/scene-card";
import { RenderPanel } from "./components/render-panel";
import { DesignerColumn } from "./components/designer-column";
import { StudioTopbar } from "./components/studio/studio-topbar";
import {
  MediaRail,
  type RailSection,
} from "./components/studio/media-rail";
import {
  InspectorPanel,
  LayersQuickPanel,
  type InspectorTab,
} from "./components/studio/inspector-panel";
import { TimelineToolbar } from "./components/studio/timeline-toolbar";

// ── Storyboard (non-scene fields; scenes live in the timeline) ──

interface StoryboardMeta {
  version: number;
  canvas: { width: number; height: number; fps: number };
  audio?: { bgm_path?: string; bgm_volume?: number };
  output?: { format?: string; height?: number };
  /** Client-only default TTS voice for scene narrations (edge-tts id);
   * applied per scene at submit time, stripped from the wire payload. */
  narration_voice?: string;
}

export interface Storyboard extends StoryboardMeta {
  scenes: Scene[];
}

/** localStorage draft key — the storyboard survives tab/app reloads. */
const DRAFT_KEY = "goclaw:video-draft:v1";
/** Client-only project title (never sent to the server). */
const TITLE_KEY = "goclaw:video-project-title:v1";
/** Client-only rail state: full quick panel vs icon-only strip. */
const RAIL_EXPANDED_KEY = "goclaw:video-rail-expanded:v1";

const ASPECTS = {
  "9:16": { width: 1080, height: 1920 },
  "16:9": { width: 1920, height: 1080 },
  "1:1": { width: 1080, height: 1080 },
} as const;

function defaultStoryboard(): StoryboardMeta {
  return {
    version: 1,
    canvas: { ...ASPECTS["9:16"], fps: 30 },
    output: { height: 720 },
  };
}

function hasValidationErrors(sb: Storyboard): boolean {
  return sb.scenes.some(
    (s) => (s.type === "image" || s.type === "video") && !s.source?.trim(),
  );
}

function statusClass(status: VideoRenderJob["status"]): string {
  switch (status) {
    case "done":
      return "text-green-600";
    case "failed":
      return "text-destructive";
    case "rendering":
    case "queued":
      return "text-blue-600";
    default:
      return "text-muted-foreground";
  }
}

function isTerminal(status: VideoRenderJob["status"]): boolean {
  return status === "done" || status === "failed" || status === "cancelled";
}

export function VideoToolPage() {
  const { t } = useTranslation("toolbox");
  const http = useHttp();
  const designerOpen = useUiStore((s) => s.videoDesignerOpen);
  const setDesignerOpen = useUiStore((s) => s.setVideoDesignerOpen);
  const { jobs, loading, refresh, progressById } = useVideoJobs(true);
  const cancel = useVideoCancel();
  const removeJob = useVideoDelete();

  const [meta, setMeta] = useState<StoryboardMeta>(defaultStoryboard);
  const [jsonOpen, setJsonOpen] = useState(false);
  const [jobsOpen, setJobsOpen] = useState(false);
  const [jsonDraft, setJsonDraft] = useState("");
  const [jsonError, setJsonError] = useState("");
  const [cancelTarget, setCancelTarget] = useState<VideoRenderJob | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<VideoRenderJob | null>(null);
  const [inspectorTab, setInspectorTab] = useState<InspectorTab>("scene");
  const [railSection, setRailSection] = useState<RailSection>("media");
  const [railExpanded, setRailExpanded] = useState(() => {
    try {
      return localStorage.getItem(RAIL_EXPANDED_KEY) !== "0";
    } catch {
      return true;
    }
  });
  const toggleRailExpanded = useCallback(() => {
    setRailExpanded((prev) => {
      const next = !prev;
      try {
        localStorage.setItem(RAIL_EXPANDED_KEY, next ? "1" : "0");
      } catch {
        // Best-effort persistence — the default (expanded) is fine.
      }
      return next;
    });
  }, []);
  const [selectedLayer, setSelectedLayer] = useState(0);
  const [projectTitle, setProjectTitle] = useState(() => {
    try {
      return localStorage.getItem(TITLE_KEY) ?? "";
    } catch {
      return "";
    }
  });

  // Feature gate
  const { data: gate, error: gateError } = useQuery({
    queryKey: ["video", "gate"],
    staleTime: 60_000,
    retry: false,
    queryFn: () => http.get("/v1/video/jobs", { limit: "1" }),
  });
  const enabled = gateError === null || gate !== undefined;

  // The studio is a forced-dark, Filmora-style surface (its editors and
  // Radix portals all key off the root .dark class). Leaving the page
  // re-applies the user's CURRENT theme rather than a mount-time snapshot —
  // the theme may have changed (or follow the OS) while they were here.
  // Applies only once the gate allows the studio; the gate-denied screen
  // renders in the user's own theme.
  useEffect(() => {
    if (!enabled) return;
    document.documentElement.classList.add("dark");
    return () => {
      applyTheme(useUiStore.getState().theme);
    };
  }, [enabled]);

  // Timeline hook — the single source of truth for scenes. The full
  // storyboard is derived, so undo/redo and scene edits can never desync
  // the canvas player or the submit payload from the timeline strip.
  const timeline = useTimeline();

  // Real-TTS narration audio shared by the player, the per-scene listen
  // button, and the duration-fit hint. Falls back to the storyboard-level
  // default voice when a scene has no override.
  const defaultVoice = meta.narration_voice && meta.narration_voice !== "auto" ? meta.narration_voice : undefined;
  const narrationAudio = useNarrationAudio(defaultVoice);

  const sb: Storyboard = useMemo(
    () => ({ ...meta, scenes: timeline.state.scenes }),
    [meta, timeline.state.scenes],
  );

  const updateMeta = useCallback((patch: Partial<StoryboardMeta>) => {
    setMeta((prev) => ({ ...prev, ...patch }));
  }, []);

  const updateProjectTitle = useCallback((title: string) => {
    setProjectTitle(title);
    try {
      // Empty title clears back to the "Untitled project" placeholder.
      if (title === "") localStorage.removeItem(TITLE_KEY);
      else localStorage.setItem(TITLE_KEY, title);
    } catch {
      // Storage unavailable — the title is cosmetic, keep going.
    }
  }, []);

  // Draft persistence: restore once on mount, save debounced on every edit.
  const draftRestoredRef = useRef(false);
  useEffect(() => {
    if (draftRestoredRef.current) return;
    draftRestoredRef.current = true;
    try {
      const raw = localStorage.getItem(DRAFT_KEY);
      if (!raw) return;
      const parsed = JSON.parse(raw) as Storyboard;
      if (!parsed || !Array.isArray(parsed.scenes) || parsed.scenes.length === 0) return;
      const { scenes, ...parsedMeta } = parsed;
      setMeta({ ...defaultStoryboard(), ...parsedMeta, version: 1 });
      timeline.replaceScenes(normalizeEditorScenes(scenes));
      toast.success(t("video.draft_restored"));
    } catch {
      // Corrupted draft — start fresh rather than blocking the page.
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const id = setTimeout(() => {
      try {
        localStorage.setItem(DRAFT_KEY, JSON.stringify(sb));
      } catch {
        // Storage full/unavailable — drafts are best-effort.
      }
    }, 800);
    return () => clearTimeout(id);
  }, [sb]);

  function clearDraft() {
    localStorage.removeItem(DRAFT_KEY);
    setMeta(defaultStoryboard());
    timeline.replaceScenes([{ type: "image", source: "", duration_sec: 5, fit: "cover" }]);
    toast.success(t("video.draft_cleared"));
  }

  /** Apply a designer-produced storyboard: meta fields go to the form,
   * scenes replace the timeline (one history entry, undo works). */
  const applyStoryboard = useCallback((next: Storyboard) => {
    setMeta((prev) => ({
      version: next.version ?? 1,
      canvas: next.canvas ?? defaultStoryboard().canvas,
      audio: next.audio,
      output: next.output,
      narration_voice: next.narration_voice ?? prev.narration_voice,
    }));
    timeline.replaceScenes(
      next.scenes?.length
        ? normalizeEditorScenes(next.scenes)
        : [{ type: "image", source: "", duration_sec: 5, fit: "cover" }],
    );
  }, [timeline]);

  /** Pull a finished job's storyboard back into the editor — tweak and
   * re-render without starting over. */
  const editJob = useCallback(
    (job: VideoRenderJob) => {
      try {
        applyStoryboard(JSON.parse(job.storyboard_json) as Storyboard);
        setInspectorTab("scene");
        setJobsOpen(false);
        toast.success(t("video.job_edit_loaded"));
      } catch {
        toast.error(t("video.job_edit_failed"));
      }
    },
    [applyStoryboard, t],
  );

  // Export
  const {
    hardware,
    exportClient,
    exportServer,
    isExporting,
    progress,
    cancel: cancelExport,
  } = useVideoExport();

  const totalSec = useMemo(
    () => sb.scenes.reduce((acc, s) => acc + (Number(s.duration_sec) || 0), 0),
    [sb.scenes],
  );

  function loadJson() {
    try {
      const parsed = JSON.parse(jsonDraft) as Storyboard;
      if (!parsed || typeof parsed !== "object") throw new Error("not an object");
      const { scenes, ...parsedMeta } = parsed;
      setMeta({ ...defaultStoryboard(), ...parsedMeta, version: 1 });
      timeline.replaceScenes(normalizeEditorScenes(scenes ?? []));
      setJsonError("");
      setJsonOpen(false);
    } catch (e) {
      setJsonError(
        t("video.json_invalid", {
          error: e instanceof Error ? e.message : String(e),
        }),
      );
    }
  }

  function handleExportClient() {
    exportClient(sb, (p) => {
      if (p >= 100) {
        toast.success(t("video.render_panel.export_complete"));
      }
    }).then((blob) => {
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `video-export.${exportExtension(blob)}`;
      a.click();
      URL.revokeObjectURL(url);
    }).catch((e: unknown) => {
      if (e instanceof Error && e.message !== "Export cancelled") {
        toast.error(t("video.render_panel.export_failed", { error: e.message }));
      }
    });
  }

  async function handleExportServer() {
    try {
      await exportServer(sb);
      toast.success(t("video.created"));
      setJobsOpen(true);
      await refresh();
    } catch (e) {
      toast.error(
        e instanceof Error && e.message
          ? `${t("video.create_failed")}: ${e.message}`
          : t("video.create_failed"),
      );
    }
  }

  // ── Studio navigation helpers ──

  const sceneCardWrapRef = useRef<HTMLDivElement>(null);
  const inspectorRef = useRef<HTMLElement>(null);

  /** Rail mic / toolbar mic: reveal the selected scene's narration field
   * inside the inspector's Scene tab and focus it. */
  const scrollToNarration = useCallback(() => {
    setInspectorTab("scene");
    requestAnimationFrame(() => {
      const wrap = sceneCardWrapRef.current;
      if (!wrap) return;
      wrap.scrollIntoView({ behavior: "smooth", block: "start" });
      // The narration textarea is the last <textarea> in the scene editor.
      const boxes = wrap.querySelectorAll("textarea");
      const box = boxes[boxes.length - 1] as HTMLTextAreaElement | undefined;
      box?.scrollIntoView({ behavior: "smooth", block: "center" });
      box?.focus({ preventScroll: true });
    });
  }, []);

  /** Top bar Export: jump to the Render inspector tab (scroll to it on
   * stacked mobile layouts). */
  const openRenderTab = useCallback(() => {
    setInspectorTab("render");
    requestAnimationFrame(() => {
      inspectorRef.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
    });
  }, []);

  /** Rail action: open the Layers tab AND reveal the inspector on stacked
   * mobile layouts (the rail renders below it in the DOM order). */
  const openLayersTab = useCallback(() => {
    setInspectorTab("layers");
    requestAnimationFrame(() => {
      inspectorRef.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
    });
  }, []);

  const selectedIndex = timeline.state.selectedIndex;
  const selectedScene: Scene | undefined = sb.scenes[selectedIndex];
  const selectedLayerCount = selectedScene?.layers?.length ?? 0;
  /** Clamp a stored layer index against the CURRENT scene's layer list. */
  const safeLayerIndex = Math.max(
    0,
    Math.min(selectedLayer, selectedLayerCount - 1),
  );

  if (!enabled) {
    return (
      <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
        <PageHeader title={t("video.title")} description={t("video.description")} />
        <EmptyState icon={Clapperboard} title={t("video.not_enabled")} />
      </div>
    );
  }

  return (
    // Studio shell: forced-dark Filmora-style editor. On desktop it is a
    // fixed-viewport 3-column layout (rail | canvas | inspector) with the
    // toolbar + timeline docked at the bottom; on mobile everything stacks
    // into one scrollable column (player → timeline → inspector → rail).
    <div className="relative flex h-full min-h-0 flex-col bg-[#141519] text-foreground">
      <StudioTopbar
        title={projectTitle}
        onTitleChange={updateProjectTitle}
        sceneCount={sb.scenes.length}
        totalSec={totalSec}
        jobsCount={jobs.length}
        onOpenJobs={() => setJobsOpen(true)}
        designerOpen={designerOpen}
        onToggleDesigner={() => setDesignerOpen(!designerOpen)}
        onExport={openRenderTab}
        exportDisabled={totalSec <= 0 || hasValidationErrors(sb)}
        isExporting={isExporting}
      />

      <div className="grid min-h-0 flex-1 grid-cols-1 overflow-y-auto overscroll-contain lg:grid-cols-[auto_minmax(0,1fr)_360px] lg:grid-rows-[minmax(0,1fr)_auto] lg:overflow-hidden">
        {/* Center: preview stage + transport */}
        <section className="order-1 flex min-h-0 flex-col max-lg:border-b max-lg:border-white/[0.06] lg:col-start-2 lg:row-start-1">
          <CanvasPlayer
            storyboard={sb}
            narration={narrationAudio}
            defaultVoice={defaultVoice}
            onDefaultVoiceChange={(v) => updateMeta({ narration_voice: v === "auto" ? undefined : v })}
          />
        </section>

        {/* Toolbar + timeline (bottom dock on desktop) */}
        <section className="order-2 lg:col-span-3 lg:col-start-1 lg:row-start-2">
          <TimelineToolbar
            canUndo={timeline.canUndo}
            canRedo={timeline.canRedo}
            onUndo={timeline.undo}
            onRedo={timeline.redo}
            onAddScene={() => timeline.addScene()}
            onScrollToNarration={scrollToNarration}
          />
          <Timeline
            scenes={timeline.state.scenes}
            selectedIndex={selectedIndex}
            onSelect={timeline.selectScene}
            onAdd={() => timeline.addScene()}
          />
        </section>

        {/* Right: inspector (Scene | Layers | Render). With no scene to edit
            (only reachable via an empty JSON load) it collapses to a one-line
            empty state instead of three empty tab groups. */}
        <section
          ref={inspectorRef}
          className={cn(
            "order-3 flex min-h-0 flex-col border-white/[0.06] max-lg:border-t lg:col-start-3 lg:row-start-1 lg:max-h-full lg:overflow-hidden lg:border-l",
            selectedScene ? "max-lg:min-h-[320px]" : "max-lg:min-h-[120px]",
          )}
        >
          {selectedScene ? (
          <InspectorPanel
            tab={inspectorTab}
            onTabChange={setInspectorTab}
            sceneTab={
              <div ref={sceneCardWrapRef} className="p-3">
                {selectedScene && (
                  <SceneCard
                    key={selectedIndex}
                    scene={selectedScene}
                    index={selectedIndex}
                    total={sb.scenes.length}
                    narration={narrationAudio}
                    onUpdate={(patch) => timeline.updateScene(selectedIndex, patch)}
                    onRemove={() => timeline.removeScene(selectedIndex)}
                    onMoveUp={() => timeline.moveScene(selectedIndex, selectedIndex - 1)}
                    onMoveDown={() => timeline.moveScene(selectedIndex, selectedIndex + 1)}
                  />
                )}
              </div>
            }
            layersTab={
              <LayersQuickPanel
                scene={selectedScene}
                selectedLayer={safeLayerIndex}
                onSelectLayer={setSelectedLayer}
                onAddLayer={(layer) => timeline.addLayer(selectedIndex, layer)}
                onUpdateLayerTiming={(li, timing) =>
                  timeline.updateLayer(selectedIndex, li, timing)
                }
                onRemoveLayer={(li) => timeline.removeLayer(selectedIndex, li)}
                defaultText={t("video.layer.new_text")}
              />
            }
            renderTab={
              <div className="p-3">
                <RenderPanel
                  storyboard={sb}
                  onStoryboardChange={(next) => {
                    const { scenes: _scenes, ...nextMeta } = next;
                    updateMeta(nextMeta);
                  }}
                  hardware={hardware}
                  isExporting={isExporting}
                  progress={progress}
                  hasErrors={hasValidationErrors(sb)}
                  onExportClient={handleExportClient}
                  onExportServer={handleExportServer}
                  onCancel={cancelExport}
                />
              </div>
            }
          />
          ) : (
            <div className="flex flex-1 items-center justify-center p-4">
              <p className="text-sm text-zinc-500">
                {t("video.studio.inspector.empty", "Select a scene or layer")}
              </p>
            </div>
          )}
        </section>

        {/* Left: quick-insert rail (vertical on desktop, strip on mobile) */}
        <section className="order-4 lg:col-start-1 lg:row-start-1">
          <MediaRail
            scenes={timeline.state.scenes}
            selectedIndex={selectedIndex}
            active={railSection}
            onActiveChange={setRailSection}
            expanded={railExpanded}
            onToggleExpanded={toggleRailExpanded}
            onSelectScene={timeline.selectScene}
            onAddScene={() => timeline.addScene()}
            onUpdateScene={(patch) => timeline.updateScene(selectedIndex, patch)}
            onAddLayer={(layer) => timeline.addLayer(selectedIndex, layer)}
            layerCount={selectedLayerCount}
            onOpenLayersTab={openLayersTab}
            onOpenJson={() => setJsonOpen(true)}
          />
        </section>
      </div>

      {/* AI designer chat: overlay drawer on desktop, portal bottom sheet on
          compact screens (handled inside the component). */}
      <div className="absolute inset-y-0 right-0 z-40">
        <DesignerColumn onApplyStoryboard={applyStoryboard} currentStoryboard={sb} />
      </div>

      {/* ── Render jobs dialog ── */}
      <Dialog open={jobsOpen} onOpenChange={setJobsOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {t("video.jobs_title")}
              {jobs.length > 0 && (
                <Badge variant="outline" className="text-muted-foreground">
                  {jobs.length}
                </Badge>
              )}
              <Button
                variant="ghost"
                size="sm"
                onClick={() => refresh()}
                className="ml-auto min-h-11 sm:min-h-9"
              >
                <RefreshCw className="mr-2 h-4 w-4" />
                {t("video.jobs_refresh")}
              </Button>
            </DialogTitle>
          </DialogHeader>
          <div className="max-h-[60dvh] overflow-y-auto overscroll-contain">
            {loading && jobs.length === 0 ? (
              <p className="px-1 py-3 text-sm text-muted-foreground">
                {t("video.submitting")}
              </p>
            ) : jobs.length === 0 ? (
              <p className="px-1 py-3 text-sm text-muted-foreground">
                {t("video.empty_jobs")}
              </p>
            ) : (
              <ul className="flex flex-col gap-2">
                {jobs.map((job) => (
                  <li key={job.id} className="rounded-md border p-3">
                    <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
                      <Badge
                        variant="outline"
                        className={cn(
                          "shrink-0 font-medium",
                          statusClass(job.status),
                        )}
                      >
                        {t(`video.status.${job.status}`)}
                      </Badge>
                      <span className="truncate font-mono text-xs tabular-nums text-muted-foreground">
                        {job.id.slice(0, 8)}
                      </span>
                      <span className="text-xs tabular-nums text-muted-foreground">
                        {new Date(job.created_at).toLocaleString()}
                      </span>
                      {job.status === "done" && job.output_size_bytes > 0 && (
                        <span className="text-xs tabular-nums text-muted-foreground">
                          {formatFileSize(job.output_size_bytes)}
                        </span>
                      )}
                      <div className="ml-auto flex items-center gap-1">
                        {job.status === "done" && job.storyboard_json && (
                          <Button
                            variant="outline"
                            size="sm"
                            className="min-h-11 sm:min-h-9"
                            title={t("video.job_edit_hint")}
                            onClick={() => editJob(job)}
                          >
                            <Pencil className="mr-2 h-4 w-4" />
                            {t("video.job_edit")}
                          </Button>
                        )}
                        {job.status === "done" && job.output_path && (
                          <Button
                            variant="outline"
                            size="sm"
                            asChild
                            className="min-h-11 sm:min-h-9"
                          >
                            <a
                              href={job.download_url ?? `/v1/files/videos/${job.id}.mp4`}
                              download
                            >
                              <Download className="mr-2 h-4 w-4" />
                              {t("video.download")}
                            </a>
                          </Button>
                        )}
                        {(job.status === "queued" ||
                          job.status === "rendering") && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="min-h-11 text-destructive hover:text-destructive sm:min-h-9"
                            onClick={() => setCancelTarget(job)}
                          >
                            <X className="mr-2 h-4 w-4" />
                            {t("video.cancel")}
                          </Button>
                        )}
                        {isTerminal(job.status) && (
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label={t("video.delete")}
                            title={t("video.delete")}
                            className="min-h-11 min-w-11 text-destructive hover:text-destructive sm:min-h-8 sm:min-w-8"
                            onClick={() => setDeleteTarget(job)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        )}
                      </div>
                    </div>
                    {job.status === "done" && (
                      /* Watch right in the job card — no download round-trip
                         to see what the agent produced. */
                      <video
                        controls
                        preload="metadata"
                        src={job.download_url ?? `/v1/files/videos/${job.id}.mp4`}
                        className="mt-2 max-h-72 w-full max-w-[180px] rounded-md border bg-black"
                      />
                    )}
                    {(job.status === "queued" ||
                      job.status === "rendering") && (
                      <div className="mt-2">
                        <progress
                          className="h-1.5 w-full"
                          value={progressById.current.get(job.id) ?? 0}
                          max={100}
                        />
                        <p className="mt-1 text-xs text-muted-foreground">
                          {t("video.worker_note")}
                        </p>
                      </div>
                    )}
                    {job.status === "failed" && job.error && (
                      <p
                        className="mt-1 truncate text-xs text-destructive"
                        title={job.error}
                      >
                        {t("video.error")}: {job.error}
                      </p>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* ── JSON mode dialog ── */}
      <Dialog open={jsonOpen} onOpenChange={setJsonOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("video.json_mode")}</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Textarea
              value={jsonDraft || JSON.stringify(sb, null, 2)}
              onChange={(e) => setJsonDraft(e.target.value)}
              rows={16}
              /* No text-* override: the base Textarea enforces
                 text-base md:text-sm (16px on mobile — no iOS zoom). */
              className="font-mono"
            />
            {jsonError && (
              <p className="text-xs text-destructive">{jsonError}</p>
            )}
            <div className="flex justify-between gap-2">
              <Button
                variant="ghost"
                size="sm"
                onClick={clearDraft}
                title={t("video.draft_clear_hint")}
                className="min-h-11 sm:min-h-9"
              >
                <Eraser className="mr-2 h-4 w-4" />
                {t("video.draft_clear")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={loadJson}
                disabled={!jsonDraft.trim()}
                className="min-h-11 sm:min-h-9"
              >
                {t("video.json_load")}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={cancelTarget !== null}
        onOpenChange={(open) => !open && setCancelTarget(null)}
        title={t("video.cancel_confirm_title")}
        description={t("video.cancel_confirm_desc")}
        confirmLabel={t("video.cancel")}
        onConfirm={async () => {
          if (!cancelTarget) return;
          try {
            await cancel(cancelTarget.id);
          } finally {
            setCancelTarget(null);
          }
        }}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t("video.delete_confirm_title")}
        description={t("video.delete_confirm_desc")}
        confirmLabel={t("video.delete")}
        variant="destructive"
        onConfirm={async () => {
          if (!deleteTarget) return;
          try {
            await removeJob(deleteTarget.id);
            toast.success(t("video.deleted"));
            await refresh();
          } catch {
            toast.error(t("video.delete_failed"));
          } finally {
            setDeleteTarget(null);
          }
        }}
      />
    </div>
  );
}
