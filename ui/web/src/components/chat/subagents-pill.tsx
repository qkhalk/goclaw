import { useState } from "react";
import { useTranslation } from "react-i18next";
import { CheckCircle2, ChevronUp, Loader2, Bot } from "lucide-react";
import { useSubagents } from "@/pages/chat/hooks/use-subagents";
import { SubagentsSheet } from "./subagents-sheet";

/**
 * Paseo-style tracker pill docked above the composer (next to TeamTasksPill):
 * shows how many subagent tasks are running and how many finished and are
 * waiting to be archived. Hidden while the agent has no subagent tasks.
 * Click opens the management sheet.
 */
export function SubagentsPill({ agentId }: { agentId: string }) {
  const { t } = useTranslation("chat");
  const [open, setOpen] = useState(false);
  const { tasks, activeCount, terminalCount } = useSubagents(agentId);

  if (tasks.length === 0) return null;

  const title =
    activeCount > 0
      ? t("subagents.pillActive", { running: activeCount, completed: terminalCount })
      : t("subagents.pillDone", { n: terminalCount });

  return (
    <div className="mx-3 mb-1 flex shrink-0 flex-wrap gap-2">
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex max-sm:min-h-[44px] items-center gap-1.5 rounded-full border bg-muted/60 px-3 py-1 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        aria-haspopup="dialog"
        aria-expanded={open}
        title={t("subagents.title")}
      >
        <Bot className="h-3.5 w-3.5 shrink-0" />
        {activeCount > 0 && (
          <>
            <span className="inline-flex items-center gap-1">
              <Loader2 className="h-3 w-3 animate-spin" />
              {t("subagents.pillCountRunning", { n: activeCount })}
            </span>
            <span className="h-3 w-px bg-border" />
          </>
        )}
        {terminalCount > 0 && (
          <span className="inline-flex items-center gap-1">
            <CheckCircle2 className="h-3 w-3 text-emerald-500" />
            {t("subagents.pillCountDone", { n: terminalCount })}
          </span>
        )}
        <ChevronUp className="h-3.5 w-3.5 shrink-0" />
        <span className="sr-only">{title}</span>
      </button>

      <SubagentsSheet agentId={agentId} open={open} onOpenChange={setOpen} />
    </div>
  );
}
