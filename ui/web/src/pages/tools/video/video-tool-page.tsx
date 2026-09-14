import { useMemo, useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  Clapperboard,
  ChevronDown,
  ChevronUp,
  Download,
  Loader2,
  RefreshCw,
  X,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { useHttp } from "@/hooks/use-ws";
import { toast } from "@/stores/use-toast-store";
import { cn } from "@/lib/utils";
import { formatFileSize } from "@/lib/format";
import {
  submitRenderJob,
  useVideoCancel,
  useVideoJobs,
  type VideoRenderJob,
} from "./hooks/use-video";
import { useTimeline, type Scene } from "./hooks/use-timeline";
import { useVideoExport, exportExtension } from "./hooks/use-video-export";
import { CanvasPlayer } from "./components/canvas-player";
import { Timeline } from "./components/timeline";
import { SceneCard } from "./components/scene-card";
import { RenderPanel } from "./components/render-panel";

// ── Storyboard (non-scene fields; scenes live in the timeline) ──

interface StoryboardMeta {
  version: number;
  canvas: { width: number; height: number; fps: number };
  audio?: { bgm_path?: string; bgm_volume?: number };
  output?: { format?: string; height?: number };
}

export interface Storyboard extends StoryboardMeta {
  scenes: Scene[];
}

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

export function VideoToolPage() {
  const { t } = useTranslation("toolbox");
  const http = useHttp();
  const { jobs, loading, refresh, progressById } = useVideoJobs(true);
  const cancel = useVideoCancel();

  const [meta, setMeta] = useState<StoryboardMeta>(defaultStoryboard);
  const [showJson, setShowJson] = useState(false);
  const [jsonDraft, setJsonDraft] = useState("");
  const [jsonError, setJsonError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [cancelTarget, setCancelTarget] = useState<VideoRenderJob | null>(null);
  const [jobsOpen, setJobsOpen] = useState(true);

  // Feature gate
  const { data: gate, error: gateError } = useQuery({
    queryKey: ["video", "gate"],
    staleTime: 60_000,
    retry: false,
    queryFn: () => http.get("/v1/video/jobs", { limit: "1" }),
  });
  const enabled = gateError === null || gate !== undefined;

  // Timeline hook — the single source of truth for scenes. The full
  // storyboard is derived, so undo/redo and scene edits can never desync
  // the canvas player or the submit payload from the timeline strip.
  const timeline = useTimeline();

  const sb: Storyboard = useMemo(
    () => ({ ...meta, scenes: timeline.state.scenes }),
    [meta, timeline.state.scenes],
  );

  const updateMeta = useCallback((patch: Partial<StoryboardMeta>) => {
    setMeta((prev) => ({ ...prev, ...patch }));
  }, []);

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

  async function handleSubmit() {
    setSubmitting(true);
    try {
      await submitRenderJob(http, sb);
      toast.success(t("video.created"));
      setJobsOpen(true);
      await refresh();
    } catch (e) {
      toast.error(
        e instanceof Error && e.message
          ? `${t("video.create_failed")}: ${e.message}`
          : t("video.create_failed"),
      );
    } finally {
      setSubmitting(false);
    }
  }

  function loadJson() {
    try {
      const parsed = JSON.parse(jsonDraft) as Storyboard;
      if (!parsed || typeof parsed !== "object") throw new Error("not an object");
      const { scenes, ...parsedMeta } = parsed;
      setMeta({ ...defaultStoryboard(), ...parsedMeta, version: 1 });
      timeline.replaceScenes(scenes ?? []);
      setJsonError("");
      setShowJson(false);
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
    } catch {
      toast.error(t("video.render_panel.server_failed"));
    }
  }

  if (!enabled) {
    return (
      <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
        <PageHeader title={t("video.title")} description={t("video.description")} />
        <EmptyState icon={Clapperboard} title={t("video.not_enabled")} />
      </div>
    );
  }

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 py-6">
      <PageHeader
        title={t("video.title")}
        description={t("video.description")}
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={() => refresh()}
            className="min-h-11 sm:min-h-9"
          >
            <RefreshCw className="mr-2 h-4 w-4" />
            {t("video.jobs_refresh")}
          </Button>
        }
      />

      {/* Jobs list (collapsible) */}
      <div className="rounded-lg border">
        <button
          type="button"
          onClick={() => setJobsOpen((v) => !v)}
          className="flex w-full items-center gap-2 px-4 py-3 text-left text-sm font-medium"
        >
          {jobsOpen ? (
            <ChevronUp className="h-4 w-4" />
          ) : (
            <ChevronDown className="h-4 w-4" />
          )}
          {t("video.jobs_title")}
          {jobs.length > 0 && (
            <Badge
              variant="outline"
              className="ml-auto shrink-0 text-muted-foreground"
            >
              {jobs.length}
            </Badge>
          )}
        </button>
        {jobsOpen && (
          <div className="border-t p-3">
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
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge
                        variant="outline"
                        className={cn(
                          "shrink-0 font-medium",
                          statusClass(job.status),
                        )}
                      >
                        {t(`video.status.${job.status}`)}
                      </Badge>
                      <span className="truncate font-mono text-xs text-muted-foreground">
                        {job.id.slice(0, 8)}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {new Date(job.created_at).toLocaleString()}
                      </span>
                      {job.status === "done" && job.output_size_bytes > 0 && (
                        <span className="text-xs text-muted-foreground">
                          {formatFileSize(job.output_size_bytes)}
                        </span>
                      )}
                      <div className="ml-auto flex items-center gap-1">
                        {job.status === "done" && job.output_path && (
                          <Button
                            variant="outline"
                            size="sm"
                            asChild
                            className="min-h-11 sm:min-h-9"
                          >
                            <a
                              href={`/v1/files/videos/${job.id}.mp4`}
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
                      </div>
                    </div>
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
        )}
      </div>

      {/* Main editor with tabs */}
      <Tabs defaultValue="editor">
        <TabsList>
          <TabsTrigger value="editor">{t("video.tabs.editor")}</TabsTrigger>
          <TabsTrigger value="render">{t("video.tabs.render")}</TabsTrigger>
        </TabsList>

        {/* ── Editor Tab ── */}
        <TabsContent value="editor" className="flex flex-col gap-4">
          {/* Canvas Player */}
          <CanvasPlayer storyboard={sb} />

          {/* Timeline */}
          <div className="rounded-lg border p-3">
            <Timeline
              scenes={timeline.state.scenes}
              selectedIndex={timeline.state.selectedIndex}
              onSelect={timeline.selectScene}
              onAdd={() => timeline.addScene()}
              onRemove={timeline.removeScene}
              onMove={timeline.moveScene}
              canUndo={timeline.canUndo}
              canRedo={timeline.canRedo}
              onUndo={timeline.undo}
              onRedo={timeline.redo}
            />
          </div>

          {/* Scene Editor (selected scene) */}
          {timeline.state.scenes[timeline.state.selectedIndex] && (
            <SceneCard
              key={timeline.state.selectedIndex}
              scene={timeline.state.scenes[timeline.state.selectedIndex]!}
              index={timeline.state.selectedIndex}
              total={timeline.state.scenes.length}
              onUpdate={(patch) =>
                timeline.updateScene(timeline.state.selectedIndex, patch)
              }
              onRemove={() => timeline.removeScene(timeline.state.selectedIndex)}
              onMoveUp={() =>
                timeline.moveScene(
                  timeline.state.selectedIndex,
                  timeline.state.selectedIndex - 1,
                )
              }
              onMoveDown={() =>
                timeline.moveScene(
                  timeline.state.selectedIndex,
                  timeline.state.selectedIndex + 1,
                )
              }
            />
          )}

          {/* JSON mode toggle */}
          <div className="flex items-center justify-between rounded-lg border p-3">
            <span className="text-sm text-muted-foreground">
              {t("video.total_duration", { sec: totalSec.toFixed(1) })} ·{" "}
              {t("video.scenes_count", { n: sb.scenes.length })}
            </span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowJson((v) => !v)}
              className="min-h-11 sm:min-h-9"
            >
              {t("video.json_mode")}
            </Button>
          </div>

          {showJson && (
            <div className="flex flex-col gap-2 rounded-lg border p-3">
              <Textarea
                value={jsonDraft || JSON.stringify(sb, null, 2)}
                onChange={(e) => setJsonDraft(e.target.value)}
                rows={16}
                className="font-mono text-xs"
              />
              {jsonError && (
                <p className="text-xs text-destructive">{jsonError}</p>
              )}
              <div className="flex justify-end gap-2">
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
          )}
        </TabsContent>

        {/* ── Render Tab ── */}
        <TabsContent value="render" className="flex flex-col gap-4">
          <RenderPanel
            storyboard={sb}
            onStoryboardChange={(next) => {
              const { scenes: _scenes, ...nextMeta } = next;
              updateMeta(nextMeta);
            }}
            hardware={hardware}
            isExporting={isExporting}
            progress={progress}
            onExportClient={handleExportClient}
            onExportServer={handleExportServer}
            onCancel={cancelExport}
          />

          {/* Submit to server button */}
          <div className="flex items-center justify-end gap-2 rounded-lg border p-3">
            <Button
              onClick={handleSubmit}
              disabled={submitting || totalSec <= 0 || hasValidationErrors(sb)}
              className="min-h-11 sm:min-h-9"
            >
              {submitting ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  {t("video.submitting")}
                </>
              ) : (
                <>
                  <Clapperboard className="mr-2 h-4 w-4" />
                  {t("video.submit")}
                </>
              )}
            </Button>
          </div>
        </TabsContent>
      </Tabs>

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
    </div>
  );
}
