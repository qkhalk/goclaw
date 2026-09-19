import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Loader2, Mic } from "lucide-react";
import { Button } from "@/components/ui/button";
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
import { useHttp } from "@/hooks/use-ws";
import { ttsCapabilitiesKeys } from "@/api/tts-capabilities";
import { toast } from "@/stores/use-toast-store";

interface CloneVoiceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called with the new voice id ("clone:<id>") after a successful upload. */
  onRegistered?: (voiceId: string) => void;
}

/**
 * Register a personal voice on the self-hosted clone worker: upload 30–60s of
 * reference audio plus a label. The gateway proxies to the worker, which
 * extracts the speaker embedding; the new voice shows up in the voice pickers
 * through /v1/tts/capabilities once invalidated.
 */
export function CloneVoiceDialog({ open, onOpenChange, onRegistered }: CloneVoiceDialogProps) {
  const { t } = useTranslation("toolbox");
  const http = useHttp();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  function reset() {
    setName("");
    setFile(null);
    if (fileInputRef.current) fileInputRef.current.value = "";
  }

  async function submit() {
    if (!name.trim() || !file || submitting) return;
    setSubmitting(true);
    try {
      const body = new FormData();
      body.append("name", name.trim());
      body.append("file", file);
      const res = await fetch("/v1/tts/clone/voices", {
        method: "POST",
        headers: http.getAuthHeaders(),
        body,
      });
      if (!res.ok) {
        const msg = await res.text().catch(() => "");
        let detail = msg;
        try {
          detail = JSON.parse(msg)?.error ?? msg;
        } catch {
          // keep raw text
        }
        throw new Error(detail || `HTTP ${res.status}`);
      }
      const data = (await res.json()) as { voice?: { id?: string } };
      queryClient.invalidateQueries({ queryKey: ttsCapabilitiesKeys.all });
      toast.success(t("video.clone_voice.success"));
      const id = data.voice?.id;
      if (id && onRegistered) onRegistered(`clone:${id}`);
      reset();
      onOpenChange(false);
    } catch (err) {
      toast.error(t("video.clone_voice.failed", { msg: err instanceof Error ? err.message : "" }));
    } finally {
      setSubmitting(false);
    }
  }

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
            <Label htmlFor="clone-voice-name" className="text-xs">{t("video.clone_voice.name")}</Label>
            <Input
              id="clone-voice-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("video.clone_voice.name_placeholder")}
              className="text-base md:text-sm"
              maxLength={100}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="clone-voice-file" className="text-xs">{t("video.clone_voice.file")}</Label>
            <Input
              id="clone-voice-file"
              ref={fileInputRef}
              type="file"
              accept=".wav,.mp3,.m4a,.aac,.ogg,.webm,.flac,audio/*"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              className="text-base md:text-sm"
            />
            <p className="text-xs text-muted-foreground">{t("video.clone_voice.hint")}</p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => { reset(); onOpenChange(false); }} disabled={submitting} className="min-h-11 sm:min-h-9">
            {t("video.cancel")}
          </Button>
          <Button onClick={submit} disabled={submitting || !name.trim() || !file} className="min-h-11 sm:min-h-9">
            {submitting && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
            {t("video.clone_voice.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
