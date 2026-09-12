import { Loader2, Bot } from "lucide-react";
import { useAgents } from "@/hooks/use-agents";
import { stripLeadingEmoji } from "@/lib/agent-emoji";
import type { SessionInfo } from "@/types/session";
import { ConsoleMenu } from "@/components/chat/console-menu";
import { ContextMeter } from "@/components/chat/context-meter";

interface ChatTopBarProps {
  agentId: string;
  isRunning: boolean;
  /** Current session — when provided, the bar renders the context meter. */
  session?: SessionInfo | null;
  /** Paseo Phase 3 console panels: workspace-scoped tools on the right. */
  onToggleFiles?: () => void;
  filesPanelOpen?: boolean;
  onToggleJobsTasks?: () => void;
  jobsTasksPanelOpen?: boolean;
  /** Paseo Phase 4 (§25): web terminal panel toggle. */
  onToggleTerminal?: () => void;
  termPanelOpen?: boolean;
  /** Selected workspace id + change callback for the console menu. */
  workspaceId?: string | null;
  onWorkspaceChange?: (id: string | null) => void;
}

/**
 * Conversation-only chrome: agent identity, context meter, run spinner and
 * the collapsed console menu. Run-phase text lives in the thread's
 * ActivityIndicator (next to the composer, where the user is looking) — the
 * bar keeps only a quiet spinner. Idle shows nothing (no "Ready" noise).
 */
export function ChatTopBar({
  agentId,
  isRunning,
  session,
  onToggleFiles,
  filesPanelOpen,
  onToggleJobsTasks,
  jobsTasksPanelOpen,
  onToggleTerminal,
  termPanelOpen,
  workspaceId,
  onWorkspaceChange,
}: ChatTopBarProps) {
  const { data: agents = [] } = useAgents();
  const agent = agents.find((a) => a.agent_key === agentId);

  const emoji = agent?.emoji || undefined;
  // Avatar emoji renders beside the name; drop a duplicated leading cluster.
  const displayName = agent ? stripLeadingEmoji(emoji, agent.display_name || agent.agent_key) : agentId;

  return (
    <div className="flex items-center justify-between border-b px-4 py-1.5">
      <div className="flex items-center gap-2">
        {emoji ? (
          <span className="text-base">{emoji}</span>
        ) : (
          <Bot className="h-4 w-4 text-muted-foreground" />
        )}
        <span className="text-sm font-semibold">{displayName}</span>
      </div>

      <div className="flex items-center gap-2">
        {session && <ContextMeter session={session} />}

        <ConsoleMenu
          workspaceId={workspaceId ?? null}
          onWorkspaceChange={(id) => onWorkspaceChange?.(id)}
          filesPanelOpen={!!filesPanelOpen}
          jobsPanelOpen={!!jobsTasksPanelOpen}
          termPanelOpen={!!termPanelOpen}
          onToggleFiles={() => onToggleFiles?.()}
          onToggleJobsTasks={() => onToggleJobsTasks?.()}
          onToggleTerminal={() => onToggleTerminal?.()}
        />

        {isRunning && <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />}
      </div>
    </div>
  );
}
