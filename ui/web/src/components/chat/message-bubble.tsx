import { useState } from "react";
import { Bot, User, Wrench, ChevronRight } from "lucide-react";
import { MessageContent } from "./message-content";
import type { ChatMessage } from "@/types/chat";
import type { ToolCall } from "@/types/session";

interface MessageBubbleProps {
  message: ChatMessage;
}

export function MessageBubble({ message }: MessageBubbleProps) {
  const isUser = message.role === "user";
  const isTool = message.role === "tool";

  if (isTool) {
    return null; // Tool messages are shown inline with assistant messages
  }

  const hasContent = !!message.content?.trim();
  const hasToolCalls = message.toolCalls && message.toolCalls.length > 0;

  // Skip assistant messages with neither text nor tool calls
  if (!isUser && !hasContent && !hasToolCalls) {
    return null;
  }

  // Tool-call-only assistant message: render compact tool call list
  if (!isUser && !hasContent && hasToolCalls) {
    return (
      <div className="space-y-1">
        {message.toolCalls!.map((tc) => (
          <ToolCallItem key={tc.id} toolCall={tc} />
        ))}
      </div>
    );
  }

  return (
    <div className={`flex gap-3 ${isUser ? "flex-row-reverse" : ""}`}>
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border bg-background">
        {isUser ? (
          <User className="h-4 w-4" />
        ) : (
          <Bot className="h-4 w-4" />
        )}
      </div>

      <div
        className={`max-w-[80%] rounded-lg px-4 py-2 ${
          isUser
            ? "bg-primary text-primary-foreground"
            : "bg-card text-card-foreground border border-border shadow-sm"
        }`}
      >
        {hasToolCalls && (
          <div className="mb-2 space-y-1">
            {message.toolCalls!.map((tc) => (
              <ToolCallItem key={tc.id} toolCall={tc} compact />
            ))}
          </div>
        )}
        <MessageContent content={message.content} role={message.role} />
        {message.timestamp && (
          <div className={`mt-1 text-[10px] ${isUser ? "text-primary-foreground/60" : "text-muted-foreground"}`}>
            {new Date(message.timestamp).toLocaleTimeString([], {
              hour: "numeric",
              minute: "2-digit",
            })}
          </div>
        )}
      </div>
    </div>
  );
}

function ToolCallItem({ toolCall, compact }: { toolCall: ToolCall; compact?: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const hasArgs = toolCall.arguments && Object.keys(toolCall.arguments).length > 0;

  return (
    <div className={compact ? "" : "rounded-md border bg-muted/50 overflow-hidden"}>
      <button
        type="button"
        onClick={() => hasArgs && setExpanded(!expanded)}
        className={`flex items-center gap-1.5 w-full text-left ${
          compact
            ? "text-xs text-muted-foreground py-0.5"
            : "px-3 py-1.5 text-sm hover:bg-muted/80 transition-colors"
        } ${hasArgs ? "cursor-pointer" : "cursor-default"}`}
      >
        {compact ? (
          <Wrench className="h-3 w-3 shrink-0" />
        ) : (
          <Wrench className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        )}
        <span className={`font-mono truncate ${compact ? "" : "text-xs"}`}>{toolCall.name}</span>
        {hasArgs && (
          <ChevronRight className={`h-3 w-3 shrink-0 ml-auto text-muted-foreground transition-transform ${expanded ? "rotate-90" : ""}`} />
        )}
      </button>
      {expanded && hasArgs && (
        <pre className={`text-[11px] text-muted-foreground overflow-x-auto ${
          compact ? "pl-4.5 pb-1" : "px-3 pb-2 border-t bg-muted/30"
        }`}>
          {JSON.stringify(toolCall.arguments, null, 2)}
        </pre>
      )}
    </div>
  );
}
