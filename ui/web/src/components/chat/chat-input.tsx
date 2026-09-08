import { useState, useRef, useCallback, useEffect, useLayoutEffect, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { Send, Square, Paperclip, X, Mic } from "lucide-react";
import { useVoiceRecorder } from "@/hooks/use-voice-recorder";
import { CommandPalette, slashTokenQuery } from "./command-palette";
import { ComposerToolbar, type ComposerOverrides } from "./composer-toolbar";

export interface AttachedFile {
  file: File;
  /** Server path after upload, set during send */
  serverPath?: string;
}

export type { ComposerOverrides };

interface ChatInputProps {
  onSend: (message: string, files?: AttachedFile[], overrides?: ComposerOverrides) => void;
  onAbort: () => void;
  /** True when main agent or team tasks are active — controls stop button, file attach */
  isBusy: boolean;
  disabled?: boolean;
  files: AttachedFile[];
  onFilesChange: (files: AttachedFile[]) => void;
}

const COMPOSER_OVERRIDE_KEY = "goclaw.composer-override";

function loadComposerOverrides(): ComposerOverrides {
  try {
    const raw = localStorage.getItem(COMPOSER_OVERRIDE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as ComposerOverrides;
    return {
      providerName: typeof parsed.providerName === "string" ? parsed.providerName : undefined,
      model: typeof parsed.model === "string" ? parsed.model : undefined,
      thinkingLevel: typeof parsed.thinkingLevel === "string" ? parsed.thinkingLevel : undefined,
    };
  } catch {
    return {};
  }
}

export function ChatInput({
  onSend,
  onAbort,
  isBusy,
  disabled,
  files,
  onFilesChange,
}: ChatInputProps) {
  const { t } = useTranslation("common");
  const [value, setValue] = useState("");
  const [overrides, setOverrides] = useState<ComposerOverrides>(loadComposerOverrides);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Persist the composer pickers so the selection survives reloads.
  useEffect(() => {
    try {
      localStorage.setItem(COMPOSER_OVERRIDE_KEY, JSON.stringify(overrides));
    } catch {
      // storage unavailable (private mode) — selection just won't persist
    }
  }, [overrides]);

  const handleOverridesChange = useCallback((next: ComposerOverrides) => {
    setOverrides((prev) => ({ ...prev, ...next }));
  }, []);

  // Slash-command palette: open while the draft is a single "/token" being typed.
  const [paletteOpen, setPaletteOpen] = useState(false);
  useEffect(() => {
    setPaletteOpen(slashTokenQuery(value) !== null);
  }, [value]);
  const paletteQuery = slashTokenQuery(value) ?? "";

  const handleSelectToken = useCallback(
    (token: string) => {
      setValue(`/${token} `);
      textareaRef.current?.focus();
    },
    [],
  );
  const voiceRecorder = useVoiceRecorder();

  const formatDuration = (seconds: number) => {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins}:${secs.toString().padStart(2, "0")}`;
  };

  const handleVoiceToggle = useCallback(async () => {
    if (voiceRecorder.isRecording) {
      const blob = await voiceRecorder.stopRecording();
      if (blob) {
        const file = new File([blob], `voice-${Date.now()}.webm`, { type: blob.type });
        onFilesChange([...files, { file }]);
      }
    } else {
      await voiceRecorder.startRecording();
    }
  }, [voiceRecorder, files, onFilesChange]);

  const handleCancelRecording = useCallback(() => {
    voiceRecorder.cancelRecording();
  }, [voiceRecorder]);

  const handleSend = useCallback(() => {
    if ((!value.trim() && files.length === 0) || disabled) return;
    onSend(
      value,
      files.length > 0 ? files : undefined,
      {
        providerName: overrides.providerName || undefined,
        model: overrides.model || undefined,
        thinkingLevel: overrides.thinkingLevel || undefined,
      },
    );
    setValue("");
    onFilesChange([]);
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
    }
  }, [value, files, onSend, onFilesChange, disabled, overrides]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      // IME safety: never intercept keystrokes during composition.
      if (e.nativeEvent.isComposing || e.keyCode === 229) return;
      if (paletteOpen) {
        if (e.key === "Escape") {
          e.preventDefault();
          setPaletteOpen(false);
          return;
        }
        if ((e.key === "Enter" && !e.shiftKey) || e.key === "ArrowUp" || e.key === "ArrowDown") {
          e.preventDefault();
          return;
        }
      }
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [handleSend, paletteOpen],
  );

  const handleInput = useCallback(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, 200) + "px";
  }, []);

  // Sync textarea height on mount and whenever value changes externally (e.g. after send).
  // Prevents browser's default rows=1 height from leaving a gap above the icons.
  useLayoutEffect(() => {
    handleInput();
  }, [value, handleInput]);

  const handleFileSelect = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  const handleFileChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const selected = e.target.files;
    if (!selected) return;
    const newFiles: AttachedFile[] = Array.from(selected).map((f) => ({ file: f }));
    onFilesChange([...files, ...newFiles]);
    e.target.value = "";
  }, [files, onFilesChange]);

  const removeFile = useCallback((index: number) => {
    onFilesChange(files.filter((_, i) => i !== index));
  }, [files, onFilesChange]);

  const hasContent = value.trim().length > 0 || files.length > 0;

  return (
    // `relative` anchors the slash-command palette (absolute bottom-full) to the
    // composer itself instead of some distant positioned ancestor.
    <div
      className="relative mx-3 mb-3 safe-bottom"
      style={{ paddingBottom: `calc(env(safe-area-inset-bottom) + var(--keyboard-height, 0px))` }}
    >
      {/* Attached files preview */}
      {files.length > 0 && (
        <div className="flex flex-wrap gap-1.5 mb-2">
          {files.map((af, i) => (
            <span
              key={i}
              className="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-1 text-xs"
            >
              <span className="max-w-[150px] truncate">{af.file.name}</span>
              <button
                type="button"
                onClick={() => removeFile(i)}
                className="rounded-sm p-0.5 hover:bg-accent"
              >
                <X className="h-3 w-3" />
              </button>
            </span>
          ))}
        </div>
      )}

      <input
        ref={fileInputRef}
        type="file"
        multiple
        onChange={handleFileChange}
        className="hidden"
      />

      {/* Slash-command palette floats above the input container */}
      <CommandPalette
        open={paletteOpen}
        query={paletteQuery}
        onSelect={handleSelectToken}
        onClose={() => setPaletteOpen(false)}
      />

      {/* Paseo-style composer: textarea on top, toolbar row underneath. */}
      <div className="rounded-xl border bg-background/95 backdrop-blur-sm shadow-sm transition-colors focus-within:ring-1 focus-within:ring-ring">
        {/* Textarea or Recording indicator */}
        {voiceRecorder.isRecording ? (
          <div className="flex items-center gap-3 py-3 px-3">
            {/* Waveform animation */}
            <div className="flex items-center gap-0.5">
              {[...Array(5)].map((_, i) => (
                <span
                  key={i}
                  className="w-1 bg-destructive rounded-full animate-pulse"
                  style={{
                    height: `${12 + Math.sin(i * 0.8) * 8}px`,
                    animationDelay: `${i * 0.1}s`,
                  }}
                />
              ))}
            </div>
            <span className="text-sm font-medium tabular-nums">
              {formatDuration(voiceRecorder.duration)}
            </span>
          </div>
        ) : (
          <textarea
            ref={textareaRef}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={handleKeyDown}
            onInput={handleInput}
            placeholder={t("sendMessage")}
            disabled={disabled}
            rows={1}
            className="w-full resize-none bg-transparent py-3 px-3 text-base md:text-sm placeholder:text-muted-foreground focus:outline-none disabled:opacity-50"
          />
        )}

        {/* Toolbar row: attach · model/thinking pickers · mic · send/stop */}
        <div className="flex items-center gap-0.5 px-2 pb-1.5">
          <button
            type="button"
            onClick={handleFileSelect}
            disabled={disabled || isBusy || voiceRecorder.isRecording}
            title={t("attachFile")}
            className="shrink-0 p-2 text-muted-foreground hover:text-foreground transition-colors disabled:opacity-40 cursor-pointer"
          >
            <Paperclip className="h-4 w-4" />
          </button>

          <ComposerToolbar
            value={overrides}
            onChange={handleOverridesChange}
            disabled={disabled || voiceRecorder.isRecording}
          />

          <div className="ml-auto flex shrink-0 items-center gap-1">
            {!voiceRecorder.isRecording && (
              <button
                type="button"
                onClick={handleVoiceToggle}
                disabled={disabled || isBusy}
                title={t("recordVoice")}
                className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground hover:text-foreground transition-colors cursor-pointer disabled:opacity-40"
              >
                <Mic className="h-4 w-4" />
              </button>
            )}

            {voiceRecorder.isRecording ? (
              <>
                <button
                  type="button"
                  onClick={handleCancelRecording}
                  title={t("cancelRecording")}
                  className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                >
                  <X className="h-4 w-4" />
                </button>
                <button
                  type="button"
                  onClick={handleVoiceToggle}
                  title={t("stopRecording")}
                  className="flex h-8 w-8 items-center justify-center rounded-lg bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors"
                >
                  <Square className="h-3.5 w-3.5" />
                </button>
              </>
            ) : isBusy ? (
              <>
                <button
                  type="button"
                  onClick={handleSend}
                  disabled={!value.trim() || disabled}
                  title={t("sendFollowUp")}
                  className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                >
                  <Send className="h-4 w-4" />
                </button>
                <button
                  type="button"
                  onClick={onAbort}
                  title={t("stopGeneration")}
                  className="flex h-8 w-8 items-center justify-center rounded-lg bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors"
                >
                  <Square className="h-3.5 w-3.5" />
                </button>
              </>
            ) : (
              <button
                type="button"
                onClick={handleSend}
                disabled={!hasContent || disabled}
                title={t("sendMessageTitle")}
                className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
              >
                <Send className="h-4 w-4" />
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
