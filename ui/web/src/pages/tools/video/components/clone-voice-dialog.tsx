import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Loader2, Mic, Square, Trash2, Upload } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useHttp } from "@/hooks/use-ws";
import { ttsCapabilitiesKeys } from "@/api/tts-capabilities";
import { toast } from "@/stores/use-toast-store";
import {
  isModelConfigured,
  listVoices,
  registerVoiceFromBlob,
  removeVoice,
  toCloneVoiceId,
  type ClonedVoice,
} from "@/lib/voice-clone";
import { FALLBACK_EDGE_VOICES } from "../hooks/use-narration-audio";

interface CloneVoiceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called with the new voice id ("clone:<id>") after a successful registration. */
  onRegistered?: (voiceId: string) => void;
}

const MAX_UPLOAD_BYTES = 16 << 20; // mirrors the gateway clone upload cap
/** Hard recorder stop — matches the engine's MAX_REF_SECONDS compute cap. */
const MAX_RECORD_SECONDS = 120;

type RecorderState = "idle" | "requesting" | "recording" | "recorded" | "denied";

/**
 * Register a personal voice for the video studio.
 *
 * Primary path: the reference audio is embedded **in the browser** (OpenVoice
 * speaker encoder via onnxruntime-web) and stored in IndexedDB, so preview
 * narration never depends on a server-side clone worker. A secondary checkbox
 * additionally uploads the same audio to the gateway clone worker for the
 * server-side render pipeline.
 */
export function CloneVoiceDialog({ open, onOpenChange, onRegistered }: CloneVoiceDialogProps) {
  const { t } = useTranslation("toolbox");
  const http = useHttp();
  const queryClient = useQueryClient();

  const [name, setName] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [recorded, setRecorded] = useState<Blob | null>(null);
  const [recorderState, setRecorderState] = useState<RecorderState>("idle");
  const [recordSecs, setRecordSecs] = useState(0);
  const [baseVoice, setBaseVoice] = useState(FALLBACK_EDGE_VOICES[0]?.voice_id ?? "vi-VN-HoaiMyNeural");
  const [alsoServer, setAlsoServer] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [progress, setProgress] = useState(0);
  const [voices, setVoices] = useState<ClonedVoice[]>([]);

  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const streamRef = useRef<MediaStream | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  // Guards the async getUserMedia window: closing/unmounting the dialog while
  // permission is pending must not leave an invisible recorder running.
  const activeRef = useRef(true);
  activeRef.current = open;

  const activeBlob = recorded ?? file;
  // Memoized so re-renders don't leak object URLs per frame.
  const recordedUrl = useMemo(
    () => (recorded ? URL.createObjectURL(recorded) : null),
    [recorded],
  );
  useEffect(() => {
    return () => {
      if (recordedUrl) URL.revokeObjectURL(recordedUrl);
    };
  }, [recordedUrl]);

  const refreshVoices = useCallback(() => {
    void listVoices()
      .then(setVoices)
      .catch(() => setVoices([]));
  }, []);

  useEffect(() => {
    if (open) refreshVoices();
  }, [open, refreshVoices]);

  const stopRecording = useCallback(() => {
    mediaRecorderRef.current?.stop();
  }, []);

  useEffect(() => {
    if (!open) {
      // Leaving the dialog must never leave a mic open.
      stopRecording();
      return;
    }
    return;
  }, [open, stopRecording]);

  useEffect(() => {
    return () => {
      // Unmount while the dialog never opened (or after it closed): the
      // pending getUserMedia would resolve into a dead component.
      activeRef.current = false;
      if (timerRef.current) clearInterval(timerRef.current);
      streamRef.current?.getTracks().forEach((tr) => tr.stop());
    };
  }, []);

  async function startRecording() {
    setRecorderState("requesting");
    setRecorded(null);
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      if (!activeRef.current) {
        // Dialog closed (or unmounted) while permission was pending.
        stream.getTracks().forEach((tr) => tr.stop());
        setRecorderState("idle");
        return;
      }
      streamRef.current = stream;
      const mimeType = pickRecorderMime();
      const rec = mimeType
        ? new MediaRecorder(stream, { mimeType })
        : new MediaRecorder(stream);
      chunksRef.current = [];
      rec.ondataavailable = (e) => {
        if (e.data.size > 0) chunksRef.current.push(e.data);
      };
      rec.onstop = () => {
        const blob = new Blob(chunksRef.current, { type: rec.mimeType || "audio/webm" });
        setRecorded(blob);
        setRecorderState("recorded");
        streamRef.current?.getTracks().forEach((tr) => tr.stop());
        streamRef.current = null;
        if (timerRef.current) clearInterval(timerRef.current);
      };
      mediaRecorderRef.current = rec;
      rec.start();
      setRecordSecs(0);
      setRecorderState("recording");
      timerRef.current = setInterval(() => setRecordSecs((s) => s + 1), 1000);
      // Safety net: hard-stop at MAX_REF_SECONDS so a forgotten dialog can
      // never record unbounded audio.
      setTimeout(() => {
        if (mediaRecorderRef.current === rec && rec.state === "recording") rec.stop();
      }, MAX_RECORD_SECONDS * 1000);
    } catch {
      setRecorderState("denied");
    }
  }

  function reset() {
    setName("");
    setFile(null);
    setRecorded(null);
    setRecorderState("idle");
    setRecordSecs(0);
    setProgress(0);
    if (fileInputRef.current) fileInputRef.current.value = "";
  }

  async function submit() {
    const label = name.trim();
    if (!label || !activeBlob || submitting) return;
    setSubmitting(true);
    setProgress(0);

    // Server registration runs FIRST and INDEPENDENTLY of the browser engine:
    // the checkbox is an explicit user request, and the in-browser pipeline
    // may be unpublished or fail (IndexedDB, decode) without making the
    // server path meaningless. This keeps the pre-engine server-only flow
    // fully working.
    let serverVoiceId: string | null = null;
    if (alsoServer) {
      try {
        const ext = activeBlob === file && file ? serverExt(file.name) : recordingExt(recorded);
        const body = new FormData();
        body.append("name", label);
        body.append(
          "file",
          activeBlob === file && file
            ? file
            : new File([recorded ?? activeBlob], `recording.${ext}`, {
                type: recorded?.type || "audio/webm",
              }),
        );
        const res = await fetch("/v1/tts/clone/voices", {
          method: "POST",
          headers: http.getAuthHeaders(),
          body,
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = (await res.json()) as { voice?: { id?: string } };
        serverVoiceId = typeof data.voice?.id === "string" ? data.voice.id : null;
      } catch (err) {
        console.warn("clone-voice: server registration failed (continuing)", err);
        toast.error(
          t("video.clone_voice.server_failed", {
            defaultValue: "Server registration failed — the voice works for preview only",
          }),
        );
      }
    }

    // Local embedding: powers offline preview, fail-soft — with the server
    // voice registered, the dialog still completes successfully.
    let localId: string | null = null;
    if (isModelConfigured()) {
      try {
        const record = await registerVoiceFromBlob({
          name: label,
          blob: activeBlob,
          baseVoice,
          onProgress: setProgress,
        });
        localId = record.id;
      } catch (err) {
        console.warn("clone-voice: local embedding failed (continuing)", err);
        if (!serverVoiceId) {
          toast.error(
            t("video.clone_voice.failed", {
              msg: err instanceof Error ? err.message : String(err),
            }),
          );
        }
      }
    }

    if (!localId && !serverVoiceId) {
      // Nothing registered — the engine is unpublished (the usual case until
      // the model assets ship). Explain honestly and keep the dialog open.
      toast.error(
        t("video.clone_voice.failed", {
          msg: t("video.clone_voice.unavailable", {
            defaultValue: "browser voice engine is not available yet — plain voices still work",
          }),
        }),
      );
      setSubmitting(false);
      return;
    }

    queryClient.invalidateQueries({ queryKey: ttsCapabilitiesKeys.all });
    refreshVoices();
    toast.success(t("video.clone_voice.success"));
    // The server voice id wins when present: it resolves on the render
    // pipeline, and the narration hook's gateway fallback uses the same id
    // for preview. Local-only voices preview in-browser until uploaded.
    onRegistered?.(toCloneVoiceId(serverVoiceId ?? localId!));
    reset();
    onOpenChange(false);
    setSubmitting(false);
  }

  async function deleteVoice(id: string) {
    await removeVoice(id).catch(() => undefined);
    refreshVoices();
  }

  const canSubmit = Boolean(name.trim() && activeBlob) && !submitting;

  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) reset(); onOpenChange(o); }}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Mic className="h-4 w-4" />
            {t("video.clone_voice.title")}
          </DialogTitle>
          <DialogDescription>{t("video.clone_voice.description")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="clone-voice-name" className="text-xs">
              {t("video.clone_voice.name")}
            </Label>
            <Input
              id="clone-voice-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("video.clone_voice.name_placeholder")}
              className="text-base md:text-sm"
              maxLength={100}
            />
          </div>

          <Tabs defaultValue="record">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="record" className="text-base md:text-sm">
                {t("video.clone_voice.tab_record", { defaultValue: "Record" })}
              </TabsTrigger>
              <TabsTrigger value="upload" className="text-base md:text-sm">
                {t("video.clone_voice.tab_upload", { defaultValue: "Upload file" })}
              </TabsTrigger>
            </TabsList>

            <TabsContent value="record" className="mt-3">
              <div className="flex flex-col items-center gap-2">
                {recorderState === "recording" ? (
                  <Button
                    type="button"
                    variant="destructive"
                    onClick={stopRecording}
                    className="min-h-11 sm:min-h-9 w-full"
                  >
                    <Square className="mr-2 h-3.5 w-3.5" />
                    {t("video.clone_voice.recording", {
                      secs: recordSecs,
                      defaultValue: "Recording… {{secs}}s — tap to stop",
                    })}
                  </Button>
                ) : (
                  <Button
                    type="button"
                    variant="outline"
                    onClick={startRecording}
                    disabled={recorderState === "requesting" || submitting}
                    className="min-h-11 sm:min-h-9 w-full"
                  >
                    {recorderState === "requesting" ? (
                      <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <Mic className="mr-2 h-3.5 w-3.5" />
                    )}
                    {recorded
                      ? t("video.clone_voice.recorded", {
                          secs: Math.max(1, Math.round(recorded.size / 32000)),
                          defaultValue: "Recorded ✓ (re-record clears it)",
                        })
                      : t("video.clone_voice.record_start", { defaultValue: "Start recording" })}
                  </Button>
                )}
                {recorderState === "denied" && (
                  <p className="text-xs text-destructive">
                    {t("video.clone_voice.mic_denied", { defaultValue: "Microphone access was denied" })}
                  </p>
                )}
                {recorded && (
                  <audio controls src={recordedUrl ?? undefined} className="w-full" preload="metadata" />
                )}
              </div>
            </TabsContent>

            <TabsContent value="upload" className="mt-3">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="clone-voice-file" className="text-xs">
                  {t("video.clone_voice.file")}
                </Label>
                <Input
                  id="clone-voice-file"
                  ref={fileInputRef}
                  type="file"
                  accept=".wav,.mp3,.m4a,.aac,.ogg,.webm,.flac,audio/*"
                  onChange={(e) => {
                    const f = e.target.files?.[0] ?? null;
                    if (f && f.size > MAX_UPLOAD_BYTES) {
                      toast.error(t("video.clone_voice.too_large", { defaultValue: "File exceeds 16MB" }));
                      return;
                    }
                    setFile(f);
                  }}
                  className="text-base md:text-sm"
                />
                <p className="text-xs text-muted-foreground">
                  {t("video.clone_voice.hint", {
                    defaultValue:
                      "WAV, MP3, M4A… — at least 5 seconds, 30–60s recommended, max 16MB.",
                  })}
                </p>
              </div>
            </TabsContent>
          </Tabs>

          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">
              {t("video.clone_voice.base_voice", { defaultValue: "Reading voice (base)" })}
            </Label>
            <Select value={baseVoice} onValueChange={setBaseVoice}>
              <SelectTrigger className="text-base md:text-sm" aria-label={t("video.clone_voice.base_voice", { defaultValue: "Reading voice (base)" })}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FALLBACK_EDGE_VOICES.map((v) => (
                  <SelectItem key={v.voice_id} value={v.voice_id} className="text-base md:text-sm">
                    {v.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {t("video.clone_voice.base_voice_hint", {
                defaultValue: "Sentences are read with this voice, then re-timbred to yours.",
              })}
            </p>
          </div>

          {submitting && (
            <div className="flex flex-col gap-1.5">
              <Progress value={Math.round(progress * 100)} />
              <p className="text-xs text-muted-foreground">
                {t("video.clone_voice.progress", {
                  pct: Math.round(progress * 100),
                  defaultValue: "Extracting voice fingerprint… {{pct}}%",
                })}
              </p>
            </div>
          )}

          {voices.length > 0 && (
            <div className="flex flex-col gap-1.5">
              <Label className="text-xs">
                {t("video.clone_voice.group", { defaultValue: "My voices" })}
              </Label>
              <ul className="flex flex-col gap-1">
                {voices.map((v) => (
                  <li key={v.id} className="flex items-center justify-between gap-2 rounded-md border px-2.5 py-1.5">
                    <span className="min-w-0 flex-1 truncate text-sm">
                      {v.name}
                      <span className="ml-2 text-xs text-muted-foreground">
                        {Math.round(v.refSeconds)}s · {new Date(v.createdAt).toLocaleDateString()}
                      </span>
                    </span>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={t("video.clone_voice.delete", { defaultValue: "Delete voice" })}
                      onClick={() => void deleteVoice(v.id)}
                      className="min-h-11 sm:min-h-9 min-w-11 sm:min-w-9 h-9 w-9 text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </li>
                ))}
              </ul>
            </div>
          )}

          <label className="flex items-start gap-2 text-sm">
            <Checkbox
              checked={alsoServer}
              onCheckedChange={(c) => setAlsoServer(c === true)}
              className="mt-0.5"
              aria-label={t("video.clone_voice.also_server", {
                defaultValue: "Also register on the server (render pipeline)",
              })}
            />
            <span className="text-muted-foreground">
              {t("video.clone_voice.also_server", {
                defaultValue: "Also register on the server (render pipeline)",
              })}
            </span>
          </label>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => { reset(); onOpenChange(false); }}
            disabled={submitting}
            className="min-h-11 sm:min-h-9"
          >
            {t("video.cancel")}
          </Button>
          <Button onClick={submit} disabled={!canSubmit} className="min-h-11 sm:min-h-9">
            {submitting ? (
              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
            ) : (
              <Upload className="mr-2 h-3.5 w-3.5" />
            )}
            {t("video.clone_voice.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** MediaRecorder mime the current browser actually supports. */
function pickRecorderMime(): string | null {
  if (typeof MediaRecorder === "undefined") return null;
  for (const candidate of ["audio/webm;codecs=opus", "audio/webm", "audio/mp4"]) {
    if (MediaRecorder.isTypeSupported(candidate)) return candidate;
  }
  return null;
}

/** Extension for the server upload, derived from the original filename. */
function serverExt(filename: string): string {
  const dot = filename.lastIndexOf(".");
  const ext = dot >= 0 ? filename.slice(dot + 1).toLowerCase() : "";
  return ["wav", "mp3", "m4a", "aac", "ogg", "webm", "flac"].includes(ext) ? ext : "wav";
}

/** Extension for a browser recording — the gateway whitelist is extension-
 * based, so a Safari audio/mp4 recording must not be named .webm. */
function recordingExt(blob: Blob | null): string {
  if (blob && blob.type.includes("mp4")) return "m4a";
  return "webm";
}
