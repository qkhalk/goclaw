import { useState } from "react";
import { useTranslation } from "react-i18next";
import { CircleHelp, CircleCheck, Send } from "lucide-react";
import { useAskOptions } from "./ask-options-context";

interface AskOptionsCardProps {
  question: string;
  options: string[];
}

/**
 * Interactive question card for ask_options tool calls in the web chat:
 * one button per option plus a free-text "Other" path. Selecting an option
 * shows confirm/back buttons before submitting, mirroring the Telegram
 * inline keyboard flow. Long questions are truncated with tap-to-expand.
 */
export function AskOptionsCard({ question, options }: AskOptionsCardProps) {
  const { t } = useTranslation("chat");
  const ask = useAskOptions();
  const [picked, setPicked] = useState<string | null>(null);
  const [confirmStep, setConfirmStep] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [other, setOther] = useState("");
  const [otherOpen, setOtherOpen] = useState(false);

  // Answered state survives reloads: any user message matching the inject
  // prefix for this question counts as answered.
  const answered = picked !== null && !confirmStep && (ask?.isAnswered(question) ?? false);
  const interactive = !!ask && !answered;
  const isLong = question.length > 150;

  const selectOption = (option: string) => {
    if (!interactive || confirmStep) return;
    setPicked(option);
    setConfirmStep(true);
  };

  const confirmAnswer = () => {
    if (!interactive || !picked) return;
    ask?.answer(question, picked);
  };

  const goBack = () => {
    if (!interactive) return;
    setPicked(null);
    setConfirmStep(false);
    setOther("");
    setOtherOpen(false);
  };

  const submitOther = () => {
    const text = other.trim();
    if (!interactive || !text) return;
    setPicked(text);
    setConfirmStep(false);
    ask?.answer(question, text);
  };

  return (
    <div className="rounded-lg border border-border bg-background p-2.5">
      {/* Question text — tap to expand if long */}
      <button
        type="button"
        onClick={() => isLong && setExpanded((v) => !v)}
        className={`flex w-full items-start gap-2 text-left ${isLong ? "cursor-pointer" : ""}`}
      >
        <CircleHelp className="mt-0.5 h-4 w-4 shrink-0 text-blue-500" />
        <p
          className={`min-w-0 flex-1 text-sm font-medium break-words ${
            isLong && !expanded ? "line-clamp-3" : ""
          }`}
        >
          {question}
        </p>
        {isLong && (
          <span className="shrink-0 text-xs text-muted-foreground">
            {expanded ? "▲" : "▼"}
          </span>
        )}
      </button>

      {answered ? (
        <div className="mt-2 flex items-center gap-1.5 rounded-md bg-accent/50 px-2 py-1.5 text-xs text-muted-foreground">
          <CircleCheck className="h-3.5 w-3.5 shrink-0 text-green-600" />
          <span className="truncate" title={picked ?? undefined}>
            {picked ?? t("askOptions.answered")}
          </span>
        </div>
      ) : (
        <>
          {/* Options — one per row */}
          <div className="mt-2 flex flex-col gap-1.5">
            {options.map((option) => {
              const isSelected = picked === option;
              return (
                <button
                  key={option}
                  type="button"
                  onClick={() => selectOption(option)}
                  disabled={!interactive || (confirmStep && !isSelected)}
                  title={interactive ? undefined : t("askOptions.readOnly")}
                  className={`min-h-[36px] rounded-md border px-2.5 py-1.5 text-left text-sm transition-colors ${
                    isSelected
                      ? "border-primary bg-primary/10 font-medium"
                      : interactive && !confirmStep
                        ? "hover:border-primary/60 hover:bg-accent"
                        : "pointer-events-none opacity-60"
                  }`}
                >
                  <span className="line-clamp-2 break-words">
                    {isSelected && "✓ "}
                    {option}
                  </span>
                </button>
              );
            })}
          </div>

          {interactive && (
            <>
              {/* Other button — hidden during confirm step */}
              {!confirmStep && (
                <>
                  <button
                    type="button"
                    onClick={() => setOtherOpen((v) => !v)}
                    className="mt-1.5 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                  >
                    ✏️ {t("askOptions.other")}
                  </button>
                  {otherOpen && (
                    <div className="mt-1 flex items-center gap-1.5">
                      <input
                        autoFocus
                        value={other}
                        onChange={(e) => setOther(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") {
                            e.preventDefault();
                            submitOther();
                          }
                        }}
                        maxLength={500}
                        placeholder={t("askOptions.otherPlaceholder")}
                        className="min-w-0 flex-1 rounded-md border bg-background px-2 py-1.5 text-base outline-none focus:ring-1 focus:ring-ring md:text-sm"
                      />
                      <button
                        type="button"
                        onClick={submitOther}
                        disabled={!other.trim()}
                        aria-label={t("askOptions.send")}
                        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                      >
                        <Send className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  )}
                </>
              )}

              {/* Confirm / Back buttons — visible after selection */}
              {confirmStep && (
                <div className="mt-2 flex items-center gap-2">
                  <button
                    type="button"
                    onClick={confirmAnswer}
                    className="flex items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
                  >
                    ✔ {t("askOptions.confirm")}
                  </button>
                  <button
                    type="button"
                    onClick={goBack}
                    className="flex items-center gap-1 rounded-md border px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-foreground"
                  >
                    ◀ {t("askOptions.back")}
                  </button>
                </div>
              )}
            </>
          )}
        </>
      )}
    </div>
  );
}

/**
 * Type guard for ask_options tool arguments carried by tool.call / tool.result
 * events. Tolerates partial payloads from truncated previews.
 */
export function parseAskOptionsArgs(
  args: Record<string, unknown> | undefined | null,
): { question: string; options: string[] } | null {
  if (!args) return null;
  const question = typeof args.question === "string" ? args.question.trim() : "";
  const rawOptions = args.options;
  if (!question || !Array.isArray(rawOptions) || rawOptions.length === 0) return null;
  const options = rawOptions.filter((o): o is string => typeof o === "string" && o.trim().length > 0);
  if (options.length === 0) return null;
  return { question, options };
}
