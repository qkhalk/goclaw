import { useTranslation } from "react-i18next";
import { User, Bot, Archive } from "lucide-react";
import { GoclawAvatar } from "@/components/chat/goclaw-avatar";
import { MessageContent } from "./message-content";
import { ThinkingBlock } from "./thinking-block";
import { ToolCallCard } from "./tool-call-card";
import { BlockReplyBubble } from "./block-reply-bubble";
import { MediaGallery } from "./media-gallery";
import { Button } from "@/components/ui/button";
import { llmMetaText, type RunLlmMeta } from "./activity-indicator";
import { useSubagentActions } from "@/pages/chat/hooks/use-subagents";
import { useUiStore } from "@/stores/use-ui-store";
import { formatChatTimestamp, resolveTimezone } from "@/lib/format";
import type { ChatMessage } from "@/types/chat";

/**
 * ChatMessage plus live-run stamps attached by use-chat-messages when a run
 * finishes (history reloads lose them — the card then renders as a normal
 * message, which is the documented degradation).
 */
export type ChatRunMessage = ChatMessage & {
  /** Final assistant message of an announce run (subagent result delivery). */
  isAnnounce?: boolean;
  /** provider/model/effort of the run that produced this message. */
  llm?: RunLlmMeta;
};

interface MessageBubbleProps {
  message: ChatMessage;
  /** Agent id — announce cards need it for the archive mutation. */
  agentId?: string;
}

export function MessageBubble({ message, agentId }: MessageBubbleProps) {
  const timezone = useUiStore((s) => s.timezone);
  const { t } = useTranslation("chat");
  const isUser = message.role === "user";
  const isTool = message.role === "tool";

  if (isTool) return null;
  if (message.isNotification) return null;
  if (message.isBlockReply) return <BlockReplyBubble message={message} />;

  const isAssistant = message.role === "assistant";
  const hasThinking = isAssistant && !!message.thinking;
  const hasToolDetails = isAssistant && message.toolDetails && message.toolDetails.length > 0;
  const hasToolCalls = isAssistant && message.tool_calls && message.tool_calls.length > 0;
  const hasContent = !!message.content?.trim();

  if (isAssistant && !hasContent && !hasToolCalls && !hasToolDetails) return null;

  const runMsg = message as ChatRunMessage;
  // Announce run (subagent finished) → compact result card instead of a
  // plain assistant bubble (plan 260922-0554 phase 6, timeline card).
  if (isAssistant && runMsg.isAnnounce && hasContent) {
    return <AnnounceCard message={runMsg} agentId={agentId} timezone={timezone} />;
  }

  const metaLine = isAssistant && hasContent ? llmMetaText(runMsg.llm, t) : null;

  // Tool-only message (no text content) — render compact without bubble wrapper
  const isToolOnly = isAssistant && !hasContent && !hasThinking && (hasToolDetails || hasToolCalls);

  return (
    <div className={`flex gap-2.5 ${isUser ? "flex-row-reverse" : ""}`}>
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border bg-background">
        {isUser ? <User className="h-4 w-4" /> : <GoclawAvatar />}
      </div>

      {isToolOnly ? (
        /* Compact tool-only card — no bubble wrapper, full width */
        <div className="flex-1 min-w-0 rounded-xl border bg-muted divide-y divide-border">
          {hasThinking && (
            <div className="px-2 py-1.5">
              <ThinkingBlock text={message.thinking!} />
            </div>
          )}
          {hasToolDetails && message.toolDetails!.map((entry) => (
            <ToolCallCard key={entry.toolCallId} entry={entry} compact />
          ))}
        </div>
      ) : (
        /* Messenger-style bubbles: user = solid primary right with a tail
           corner, assistant = tinted neutral left. Surface-tint depth only —
           no hairline + shadow mixing. */
        <div
          className={`w-fit max-w-[92%] rounded-2xl px-3.5 py-2 ${
            isUser
              ? "rounded-br-md bg-primary text-primary-foreground"
              : "flex-1 rounded-bl-md bg-muted/50 text-card-foreground"
          }`}
        >
          {hasThinking && (
            <div className="mb-2">
              <ThinkingBlock text={message.thinking!} />
            </div>
          )}
          {hasToolDetails && (
            <div className="mb-2 rounded-lg bg-background/60 divide-y divide-border">
              {message.toolDetails!.map((entry) => (
                <ToolCallCard key={entry.toolCallId} entry={entry} compact />
              ))}
            </div>
          )}
          <MessageContent content={message.content} role={message.role} mediaBasenames={message.mediaItems?.map((m) => m.path.split("/").pop() ?? "").filter(Boolean)} />
          {message.mediaItems && message.mediaItems.length > 0 && (
            <div className="mt-2 overflow-hidden rounded-lg">
              <MediaGallery items={message.mediaItems} />
            </div>
          )}
          {metaLine && (
            <div className="mt-1 text-2xs text-muted-foreground/80">{metaLine}</div>
          )}
          {message.timestamp && (
            <div
              className={`mt-1 text-2xs tabular-nums ${
                isUser ? "text-primary-foreground/70" : "text-muted-foreground"
              }`}
            >
              {formatChatTimestamp(
                // Shift the instant into the viewer's tz by formatting a Date
                // built from the tz-adjusted wall time (keeps Intl locale work).
                new Date(new Date(message.timestamp).toLocaleString("en-US", { timeZone: resolveTimezone(timezone) })),
                t,
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

/**
 * Timeline variant for announce runs (RunKind="announce"): the subagent
 * finished and the parent announced the result. Compact card with a header,
 * the summary body and an inline Archive action sharing the sheet mutation.
 */
function AnnounceCard({
  message,
  agentId,
  timezone,
}: {
  message: ChatRunMessage;
  agentId?: string;
  timezone: string;
}) {
  const { t } = useTranslation("chat");
  const { archiveCompleted, pendingArchiveAll } = useSubagentActions(agentId ?? "");
  const metaLine = llmMetaText(message.llm, t);

  return (
    <div className="flex gap-2.5">
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border bg-background">
        <Bot className="h-4 w-4 text-emerald-500" />
      </div>
      <div className="min-w-0 flex-1 rounded-xl rounded-bl-md border border-emerald-500/30 bg-emerald-500/5 px-3.5 py-2">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-xs font-semibold text-emerald-600 dark:text-emerald-400">
            {t("subagents.announceTitle")}
          </span>
          {message.timestamp && (
            <span className="text-2xs tabular-nums text-muted-foreground">
              {formatChatTimestamp(
                new Date(new Date(message.timestamp).toLocaleString("en-US", { timeZone: resolveTimezone(timezone) })),
                t,
              )}
            </span>
          )}
        </div>

        <div className="mt-1.5 text-sm text-card-foreground">
          <MessageContent content={message.content} role={message.role} mediaBasenames={message.mediaItems?.map((m) => m.path.split("/").pop() ?? "").filter(Boolean)} />
        </div>
        {message.mediaItems && message.mediaItems.length > 0 && (
          <div className="mt-2 overflow-hidden rounded-lg">
            <MediaGallery items={message.mediaItems} />
          </div>
        )}
        {metaLine && <div className="mt-1 text-2xs text-muted-foreground/80">{metaLine}</div>}

        <div className="mt-2 flex flex-wrap items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="gap-1.5 max-sm:min-h-[44px]"
            disabled={!agentId || pendingArchiveAll}
            title={t("subagents.archive_all_hint")}
            onClick={() => void archiveCompleted().catch(() => { /* toast shown in mutation */ })}
          >
            <Archive className="h-3.5 w-3.5" />
            {t("subagents.archive")}
          </Button>
        </div>
      </div>
    </div>
  );
}
