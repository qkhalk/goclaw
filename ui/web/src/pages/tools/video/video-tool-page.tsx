import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ArrowDown,
  ArrowUp,
  Clapperboard,
  ChevronDown,
  ChevronUp,
  Download,
  Loader2,
  Plus,
  Trash2,
  X,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { useHttp } from "@/hooks/use-ws";
import { toast } from "@/stores/use-toast-store";
import { cn } from "@/lib/utils";
import {
  submitRenderJob,
  useVideoCancel,
  useVideoJobs,
  type VideoRenderJob,
} from "./hooks/use-video";

// ── Storyboard client-side shape (mirrors internal/video/types.go) ──

interface KenBurns {
  zoom_from: number;
  zoom_to: number;
  pan: "none" | "left" | "right" | "up" | "down";
}
interface Caption {
  text: string;
  position?: "top" | "center" | "bottom";
  font_size?: number;
}
interface Scene {
  type: "image" | "video" | "color";
  source?: string;
  color?: string;
  duration_sec: number;
  fit?: "cover" | "contain";
  mute?: boolean;
  ken_burns?: KenBurns;
  caption?: Caption;
}
interface Storyboard {
  version: number;
  canvas: { width: number; height: number; fps: number };
  scenes: Scene[];
  audio?: { bgm_path?: string; bgm_volume?: number };
  output?: { format?: string; height?: number };
}

const ASPECTS = {
  "9:16": { width: 1080, height: 1920 },
  "16:9": { width: 1920, height: 1080 },
  "1:1": { width: 1080, height: 1080 },
} as const;
type Aspect = keyof typeof ASPECTS;

function emptyScene(): Scene {
  return { type: "image", source: "", duration_sec: 5, fit: "cover" };
}

function defaultStoryboard(): Storyboard {
  return {
    version: 1,
    canvas: { ...ASPECTS["9:16"], fps: 30 },
    scenes: [emptyScene()],
    output: { height: 720 },
  };
}

function aspectOf(canvas: Storyboard["canvas"]): Aspect | "custom" {
  for (const [key, dim] of Object.entries(ASPECTS)) {
    if (canvas.width === dim.width && canvas.height === dim.height) return key as Aspect;
  }
  return "custom";
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
  const [sb, setSb] = useState<Storyboard>(defaultStoryboard);
  const [showJson, setShowJson] = useState(false);
  const [jsonDraft, setJsonDraft] = useState("");
  const [jsonError, setJsonError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [cancelTarget, setCancelTarget] = useState<VideoRenderJob | null>(null);
  const [jobsOpen, setJobsOpen] = useState(true);

  // Feature gate: a 403 means video surface is disabled on this server.
  const { data: gate, error: gateError } = useQuery({
    queryKey: ["video", "gate"],
    staleTime: 60_000,
    retry: false,
    queryFn: () => http.get("/v1/video/jobs", { limit: "1" }),
  });
  const enabled = gateError === null || gate !== undefined;

  const totalSec = useMemo(
    () => sb.scenes.reduce((acc, s) => acc + (Number(s.duration_sec) || 0), 0),
    [sb.scenes],
  );

  function patchScene(i: number, patch: Partial<Scene>) {
    setSb((prev) => ({
      ...prev,
      scenes: prev.scenes.map((s, j) => (j === i ? { ...s, ...patch } : s)),
    }));
  }

  function moveScene(i: number, dir: -1 | 1) {
    setSb((prev) => {
      const next = [...prev.scenes];
      const j = i + dir;
      if (j < 0 || j >= next.length) return prev;
      const tmp = next[i]!;
      next[i] = next[j]!;
      next[j] = tmp;
      return { ...prev, scenes: next };
    });
  }

  function setAspect(a: Aspect) {
    setSb((prev) => ({ ...prev, canvas: { ...prev.canvas, ...ASPECTS[a] } }));
  }

  async function handleSubmit() {
    setSubmitting(true);
    try {
      await submitRenderJob(http, sb);
      toast.success(t("video.jobs_title"));
      setJobsOpen(true);
      await refresh();
    } catch (e) {
      toast.error(
        e instanceof Error && e.message ? `${t("video.create_failed")}: ${e.message}` : t("video.create_failed"),
      );
    } finally {
      setSubmitting(false);
    }
  }

  function loadJson() {
    try {
      const parsed = JSON.parse(jsonDraft) as Storyboard;
      if (!parsed || typeof parsed !== "object") throw new Error("not an object");
      setSb({ ...defaultStoryboard(), ...parsed, version: 1 });
      setJsonError("");
      setShowJson(false);
    } catch (e) {
      setJsonError(t("video.json_invalid", { error: e instanceof Error ? e.message : String(e) }));
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
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
      <PageHeader
        title={t("video.title")}
        description={t("video.description")}
        actions={
          <Button variant="outline" size="sm" onClick={() => refresh()} className="min-h-11 sm:min-h-9">
            {t("video.new_job")}
          </Button>
        }
      />

      {/* ── Jobs list ── */}
      <div className="rounded-lg border">
        <button
          type="button"
          onClick={() => setJobsOpen((v) => !v)}
          className="flex w-full items-center gap-2 px-4 py-3 text-left text-sm font-medium"
        >
          {jobsOpen ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
          {t("video.jobs_title")}
          {jobs.length > 0 && (
            <Badge variant="outline" className="ml-auto shrink-0 text-muted-foreground">
              {jobs.length}
            </Badge>
          )}
        </button>
        {jobsOpen && (
          <div className="border-t p-3">
            {loading && jobs.length === 0 ? (
              <p className="px-1 py-3 text-sm text-muted-foreground">{t("video.submitting")}</p>
            ) : jobs.length === 0 ? (
              <p className="px-1 py-3 text-sm text-muted-foreground">{t("video.empty_jobs")}</p>
            ) : (
              <ul className="flex flex-col gap-2">
                {jobs.map((job) => (
                  <li key={job.id} className="rounded-md border p-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant="outline" className={cn("shrink-0 font-medium", statusClass(job.status))}>
                        {t(`video.status.${job.status}`)}
                      </Badge>
                      <span className="truncate font-mono text-xs text-muted-foreground">
                        {job.id.slice(0, 8)}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {new Date(job.created_at).toLocaleString()}
                      </span>
                      <div className="ml-auto flex items-center gap-1">
                        {job.status === "done" && job.output_path && (
                          <Button variant="outline" size="sm" asChild className="min-h-11 sm:min-h-9">
                            <a href={`/v1/files/videos/${job.id}.mp4`} download>
                              <Download className="mr-2 h-4 w-4" />
                              {t("video.download")}
                            </a>
                          </Button>
                        )}
                        {(job.status === "queued" || job.status === "rendering") && (
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
                    {(job.status === "queued" || job.status === "rendering") && (
                      <div className="mt-2">
                        <Progress value={progressById.current.get(job.id) ?? 0} className="h-1.5" />
                        <p className="mt-1 text-xs text-muted-foreground">{t("video.worker_note")}</p>
                      </div>
                    )}
                    {job.status === "failed" && job.error && (
                      <p className="mt-1 truncate text-xs text-destructive" title={job.error}>
                        {t("video.error")}: {job.error}
                      </p>
                    )}
                    {job.status === "done" && job.output_size_bytes > 0 && (
                      <p className="mt-1 text-xs text-muted-foreground">
                        {(job.output_size_bytes / 1024 / 1024).toFixed(1)} MB
                      </p>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
      </div>

      {/* ── Composer ── */}
      <div className="flex flex-col gap-4 rounded-lg border p-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-sm font-medium">{t("video.composer")}</p>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">{t("video.total_duration", { sec: totalSec })}</span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowJson((v) => !v)}
              className="min-h-11 sm:min-h-9"
            >
              {t("video.json_mode")}
            </Button>
          </div>
        </div>

        {showJson ? (
          <div className="flex flex-col gap-2">
            <Textarea
              value={jsonDraft || JSON.stringify(sb, null, 2)}
              onChange={(e) => setJsonDraft(e.target.value)}
              rows={16}
              className="font-mono text-xs"
            />
            {jsonError && <p className="text-xs text-destructive">{jsonError}</p>}
            <div className="flex justify-end gap-2">
              <Button variant="outline" size="sm" onClick={loadJson} disabled={!jsonDraft.trim()} className="min-h-11 sm:min-h-9">
                {t("video.json_load")}
              </Button>
            </div>
          </div>
        ) : (
          <>
            {/* Canvas + output */}
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div className="flex flex-col gap-1.5">
                <Label className="text-xs">{t("video.aspect")}</Label>
                <Select
                  value={aspectOf(sb.canvas)}
                  onValueChange={(v) => v !== "custom" && setAspect(v as Aspect)}
                >
                  <SelectTrigger className="text-base md:text-sm" aria-label={t("video.aspect")}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(Object.keys(ASPECTS) as Aspect[]).map((a) => (
                      <SelectItem key={a} value={a}>
                        {a} ({ASPECTS[a].width}×{ASPECTS[a].height})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-col gap-1.5">
                <Label className="text-xs">{t("video.fps")}</Label>
                <Input
                  type="number"
                  min={1}
                  max={60}
                  value={sb.canvas.fps}
                  onChange={(e) =>
                    setSb((p) => ({ ...p, canvas: { ...p.canvas, fps: Number(e.target.value) || 30 } }))
                  }
                  className="text-base md:text-sm"
                />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label className="text-xs">{t("video.output_height")}</Label>
                <Select
                  value={String(sb.output?.height ?? 720)}
                  onValueChange={(v) =>
                    setSb((p) => ({ ...p, output: { ...p.output, height: Number(v) } }))
                  }
                >
                  <SelectTrigger className="text-base md:text-sm" aria-label={t("video.output_height")}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {[480, 720, 1080].map((h) => (
                      <SelectItem key={h} value={String(h)}>
                        {h}p
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Scenes */}
            <div className="flex flex-col gap-3">
              <div className="flex items-center justify-between">
                <Label className="text-xs">{t("video.scenes")}</Label>
                <span className="text-xs text-muted-foreground">{t("video.limit_hint")}</span>
              </div>
              {sb.scenes.map((scene, i) => (
                <div key={i} className="flex flex-col gap-3 rounded-md border p-3">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{t("video.scene_n", { n: i + 1 })}</span>
                    <div className="ml-auto flex items-center gap-1">
                      <Button
                        variant="ghost" size="icon-sm" aria-label={t("video.move_up")}
                        disabled={i === 0} onClick={() => moveScene(i, -1)}
                      >
                        <ArrowUp className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost" size="icon-sm" aria-label={t("video.move_down")}
                        disabled={i === sb.scenes.length - 1} onClick={() => moveScene(i, 1)}
                      >
                        <ArrowDown className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost" size="icon-sm" aria-label={t("video.remove_scene")}
                        disabled={sb.scenes.length === 1}
                        onClick={() => setSb((p) => ({ ...p, scenes: p.scenes.filter((_, j) => j !== i) }))}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>

                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
                    <div className="flex flex-col gap-1.5">
                      <Label className="text-xs">{t("video.type")}</Label>
                      <Select
                        value={scene.type}
                        onValueChange={(v) => patchScene(i, { type: v as Scene["type"] })}
                      >
                        <SelectTrigger className="text-base md:text-sm" aria-label={t("video.type")}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="image">{t("video.type_image")}</SelectItem>
                          <SelectItem value="video">{t("video.type_video")}</SelectItem>
                          <SelectItem value="color">{t("video.type_color")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    {scene.type === "color" ? (
                      <div className="flex flex-col gap-1.5">
                        <Label className="text-xs">{t("video.color")}</Label>
                        <Input
                          value={scene.color ?? "#000000"}
                          onChange={(e) => patchScene(i, { color: e.target.value })}
                          placeholder="#1D4ED8"
                          className="text-base md:text-sm"
                        />
                      </div>
                    ) : (
                      <div className="flex flex-col gap-1.5 sm:col-span-2">
                        <Label className="text-xs">{t("video.source")}</Label>
                        <Input
                          value={scene.source ?? ""}
                          onChange={(e) => patchScene(i, { source: e.target.value })}
                          placeholder={t("video.source_hint")}
                          className="text-base md:text-sm"
                        />
                      </div>
                    )}
                    <div className="flex flex-col gap-1.5">
                      <Label className="text-xs">{t("video.duration")}</Label>
                      <Input
                        type="number"
                        min={1}
                        max={30}
                        value={scene.duration_sec}
                        onChange={(e) =>
                          patchScene(i, { duration_sec: Number(e.target.value) || 1 })
                        }
                        className="text-base md:text-sm"
                      />
                    </div>
                  </div>

                  {scene.type !== "color" && (
                    <div className="flex flex-wrap items-center gap-4">
                      <div className="flex items-center gap-2">
                        <Switch
                          id={`kb-${i}`}
                          checked={scene.ken_burns !== undefined}
                          onCheckedChange={(v) =>
                            patchScene(i, {
                              ken_burns: v
                                ? { zoom_from: 1.0, zoom_to: 1.12, pan: "none" }
                                : undefined,
                            })
                          }
                        />
                        <Label htmlFor={`kb-${i}`} className="text-xs">{t("video.kb_enabled")}</Label>
                      </div>
                      <div className="flex items-center gap-2">
                        <Switch
                          id={`mute-${i}`}
                          checked={scene.mute ?? false}
                          onCheckedChange={(v) => patchScene(i, { mute: v })}
                        />
                        <Label htmlFor={`mute-${i}`} className="text-xs">{t("video.mute")}</Label>
                      </div>
                      {scene.ken_burns && (
                        <>
                          <div className="flex items-center gap-1.5">
                            <Label className="text-xs">{t("video.kb_zoom_from")}</Label>
                            <Input
                              type="number" step={0.01} min={1} max={2}
                              value={scene.ken_burns.zoom_from}
                              onChange={(e) =>
                                patchScene(i, {
                                  ken_burns: { ...scene.ken_burns!, zoom_from: Number(e.target.value) || 1 },
                                })
                              }
                              className="h-8 w-20 text-base md:text-sm"
                            />
                            <Label className="text-xs">{t("video.kb_zoom_to")}</Label>
                            <Input
                              type="number" step={0.01} min={1} max={2}
                              value={scene.ken_burns.zoom_to}
                              onChange={(e) =>
                                patchScene(i, {
                                  ken_burns: { ...scene.ken_burns!, zoom_to: Number(e.target.value) || 1 },
                                })
                              }
                              className="h-8 w-20 text-base md:text-sm"
                            />
                          </div>
                          <div className="flex items-center gap-1.5">
                            <Label className="text-xs">{t("video.kb_pan")}</Label>
                            <Select
                              value={scene.ken_burns.pan}
                              onValueChange={(v) =>
                                patchScene(i, {
                                  ken_burns: { ...scene.ken_burns!, pan: v as KenBurns["pan"] },
                                })
                              }
                            >
                              <SelectTrigger className="h-8 text-base md:text-sm" aria-label={t("video.kb_pan")}>
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                {(["none", "left", "right", "up", "down"] as const).map((p) => (
                                  <SelectItem key={p} value={p}>
                                    {t(`video.kb_pan_${p}`)}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </div>
                        </>
                      )}
                    </div>
                  )}

                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
                    <div className="flex flex-col gap-1.5 sm:col-span-3">
                      <Label className="text-xs">{t("video.caption_text")}</Label>
                      <Input
                        value={scene.caption?.text ?? ""}
                        onChange={(e) =>
                          patchScene(i, {
                            caption: e.target.value
                              ? { ...(scene.caption ?? { position: "bottom" as const }), text: e.target.value }
                              : undefined,
                          })
                        }
                        className="text-base md:text-sm"
                      />
                    </div>
                    <div className="flex flex-col gap-1.5">
                      <Label className="text-xs">{t("video.caption_position")}</Label>
                      <Select
                        value={scene.caption?.position ?? "bottom"}
                        onValueChange={(v) =>
                          patchScene(i, {
                            caption: scene.caption
                              ? { ...scene.caption, position: v as Caption["position"] }
                              : undefined,
                          })
                        }
                      >
                        <SelectTrigger className="text-base md:text-sm" aria-label={t("video.caption_position")}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="top">{t("video.pos_top")}</SelectItem>
                          <SelectItem value="center">{t("video.pos_center")}</SelectItem>
                          <SelectItem value="bottom">{t("video.pos_bottom")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                </div>
              ))}
              <Button
                variant="outline"
                size="sm"
                onClick={() => setSb((p) => ({ ...p, scenes: [...p.scenes, emptyScene()] }))}
                disabled={sb.scenes.length >= 60}
                className="min-h-11 self-start sm:min-h-9"
              >
                <Plus className="mr-2 h-4 w-4" />
                {t("video.add_scene")}
              </Button>
            </div>

            {/* Audio */}
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="flex flex-col gap-1.5">
                <Label className="text-xs">{t("video.bgm_path")}</Label>
                <Input
                  value={sb.audio?.bgm_path ?? ""}
                  onChange={(e) =>
                    setSb((p) => ({
                      ...p,
                      audio: e.target.value ? { bgm_path: e.target.value, bgm_volume: p.audio?.bgm_volume } : undefined,
                    }))
                  }
                  placeholder="audio/bgm.mp3"
                  className="text-base md:text-sm"
                />
              </div>
              {sb.audio && (
                <div className="flex flex-col gap-1.5">
                  <Label className="text-xs">{t("video.bgm_volume")}</Label>
                  <Input
                    type="number" step={0.05} min={0} max={1}
                    value={sb.audio.bgm_volume ?? 0.2}
                    onChange={(e) =>
                      setSb((p) => ({ ...p, audio: { ...p.audio!, bgm_volume: Number(e.target.value) } }))
                    }
                    className="text-base md:text-sm"
                  />
                </div>
              )}
            </div>
          </>
        )}

        <div className="flex items-center justify-end gap-2 border-t pt-3">
          <Button onClick={handleSubmit} disabled={submitting || totalSec <= 0} className="min-h-11 sm:min-h-9">
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
      </div>

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
