import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  ChevronLeft,
  ChevronRight,
  Loader2,
  Maximize,
  Minimize,
  Pause,
  Play,
  SkipBack,
  SkipForward,
  Volume2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useTtsCapabilities } from "@/api/tts-capabilities";
import type { useCanvasPlayer } from "../../hooks/use-canvas-player";
import type { NarrationAudioController } from "../../hooks/use-narration-audio";
import { FALLBACK_EDGE_VOICES } from "../../hooks/use-narration-audio";

type PlayerController = ReturnType<typeof useCanvasPlayer>;

function formatTime(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

interface TransportBarProps {
  player: PlayerController;
  sceneCount: number;
  narration?: NarrationAudioController;
  /** Storyboard-level default TTS voice (edge-tts id), applied at submit. */
  defaultVoice?: string;
  onDefaultVoiceChange?: (voice: string) => void;
  isFullscreen: boolean;
  onToggleFullscreen: () => void;
}

/**
 * Filmora-style transport strip under the canvas: frame step, play/pause,
 * seek, timecode, fullscreen — plus the scene indicator, TTS status and the
 * storyboard default voice picker carried over from the old player chrome.
 */
export function TransportBar({
  player,
  sceneCount,
  narration,
  defaultVoice,
  onDefaultVoiceChange,
  isFullscreen,
  onToggleFullscreen,
}: TransportBarProps) {
  const { t } = useTranslation("toolbox");

  const { data: capabilities } = useTtsCapabilities();
  const edgeVoices = useMemo(() => {
    const fromApi = capabilities?.find((p) => p.provider === "edge")?.voices ?? [];
    return fromApi.length > 0 ? fromApi : FALLBACK_EDGE_VOICES;
  }, [capabilities]);

  const { isPlaying, currentTime, totalDuration, currentSceneIndex } = player.state;

  return (
    // One flat wrapping row (no boxed panel): transport cluster, seek,
    // timecode, fullscreen, then the playback-info cluster (scene indicator,
    // TTS status, default voice) pushed right; it wraps below on narrow
    // stages instead of occupying a permanent second chrome row.
    <div className="flex flex-wrap items-center gap-2">
      {/* Primary transport cluster */}
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => player.seek(0)}
          aria-label={t("video.canvas.start")}
          title={t("video.canvas.start")}
          className="min-h-11 min-w-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-9 sm:min-w-9"
        >
          <SkipBack className="h-4 w-4" />
        </Button>

        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => player.stepFrame(-1)}
          aria-label={t("video.canvas.prev_frame")}
          title={t("video.canvas.prev_frame")}
          className="min-h-11 min-w-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-9 sm:min-w-9"
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>

        <Button
          size="icon"
          onClick={() => (isPlaying ? player.pause() : player.play())}
          aria-label={isPlaying ? t("video.canvas.pause") : t("video.canvas.play")}
          title={isPlaying ? t("video.canvas.pause") : t("video.canvas.play")}
          className="min-h-11 min-w-11 rounded-full bg-primary text-primary-foreground shadow-md hover:bg-primary/90 sm:min-h-9 sm:min-w-9"
        >
          {isPlaying ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4" />}
        </Button>

        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => player.stepFrame(1)}
          aria-label={t("video.canvas.next_frame")}
          title={t("video.canvas.next_frame")}
          className="min-h-11 min-w-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-9 sm:min-w-9"
        >
          <ChevronRight className="h-4 w-4" />
        </Button>

        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => player.seek(totalDuration)}
          aria-label={t("video.canvas.end")}
          title={t("video.canvas.end")}
          className="min-h-11 min-w-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-9 sm:min-w-9"
        >
          <SkipForward className="h-4 w-4" />
        </Button>
      </div>

      {/* Seek bar — thin visually, padded wrapper for a touch-sized hit area */}
      <div className="flex min-w-[140px] flex-1 items-center py-2.5">
        <input
          type="range"
          min={0}
          max={totalDuration * 100 || 1}
          value={currentTime * 100}
          onChange={(e) => player.seek(Number(e.target.value) / 100)}
          aria-label={t("video.canvas.play")}
          className="h-1.5 w-full cursor-pointer appearance-none rounded-full bg-white/10 accent-primary [&::-webkit-slider-thumb]:h-3.5 [&::-webkit-slider-thumb]:w-3.5 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-primary"
        />
      </div>

      {/* Timecode */}
      <span className="whitespace-nowrap rounded-md bg-black/40 px-2 py-1 font-mono text-[11px] tabular-nums text-zinc-300">
        {formatTime(currentTime)} / {formatTime(totalDuration)}
      </span>

      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onToggleFullscreen}
        aria-label={t("video.studio.transport.fullscreen")}
        title={t("video.studio.transport.fullscreen")}
        className="min-h-11 min-w-11 text-zinc-300 hover:bg-white/5 hover:text-white sm:min-h-9 sm:min-w-9"
      >
        {isFullscreen ? <Minimize className="h-4 w-4" /> : <Maximize className="h-4 w-4" />}
      </Button>

      {/* Playback-info cluster: scene position, narration synth status and
          the storyboard default voice. */}
      <div className="ml-auto flex flex-wrap items-center gap-x-3 gap-y-1 px-1 text-xs text-zinc-500">
        <span className="whitespace-nowrap tabular-nums">
          {t("video.scene_n", { n: currentSceneIndex + 1 })} / {sceneCount}
        </span>
        {narration && narration.pendingCount > 0 && (
          <span className="inline-flex items-center gap-1 text-primary">
            <Loader2 className="h-3 w-3 animate-spin" />
            {t("video.tts_loading")}
          </span>
        )}
        {onDefaultVoiceChange && (
          <div className="flex items-center gap-1.5">
            <Volume2 className="h-3.5 w-3.5 shrink-0" />
            <Select value={defaultVoice ?? ""} onValueChange={onDefaultVoiceChange}>
              <SelectTrigger
                className="h-9 w-auto max-w-[220px] text-base md:text-sm"
                aria-label={t("video.voice_global")}
              >
                <SelectValue placeholder={t("video.voice_global")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="auto">
                  {t("video.voice_auto")}
                </SelectItem>
                {edgeVoices.map((v) => (
                  <SelectItem key={v.voice_id} value={v.voice_id}>
                    {v.name || v.voice_id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
      </div>
    </div>
  );
}
