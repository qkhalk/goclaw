import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Loader2, Bot, Users, PanelRightOpen, PanelRightClose, FolderOpen, ListTree, SquareTerminal } from "lucide-react";
import { useHttp } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import { stripLeadingEmoji } from "@/lib/agent-emoji";
import type { RunActivity, ActiveTeamTask } from "@/types/chat";
import type { AgentData } from "@/types/agent";
import type { SessionInfo } from "@/types/session";
import { WorkspacePicker } from "@/components/chat/workspace-picker";

interface ChatTopBarProps {
  agentId: string;
  isRunning: boolean;
  isBusy: boolean;
  activity: RunActivity | null;
  teamTasks: ActiveTeamTask[];
  onToggleTaskPanel?: () => void;
  taskPanelOpen?: boolean;
  /** Current session — when provided, the bar renders a context-usage badge. */
  session?: SessionInfo | null;
  /** Paseo Phase 3 console panels: workspace-scoped tools on the right. */
  onToggleFiles?: () => void;
  filesPanelOpen?: boolean;
  onToggleJobsTasks?: () => void;
  jobsTasksPanelOpen?: boolean;
  /** Paseo Phase 4 (§25): web terminal panel toggle. */
  onToggleTerminal?: () => void;
  termPanelOpen?: boolean;
  /** Selected workspace id + change callback for the picker. */
  workspaceId?: string | null;
  onWorkspaceChange?: (id: string | null) => void;
}


const phaseLabels: Record<RunActivity["phase"], string> = {
  thinking: "Thinking…",
  tool_exec: "Running tool…",
  streaming: "Responding…",
  compacting: "Compacting…",
  retrying: "Retrying…",
  leader_processing: "Processing team results…",
};

export function ChatTopBar({ agentId, isRunning, isBusy, activity, teamTasks, onToggleTaskPanel, taskPanelOpen, session, onToggleFiles, filesPanelOpen, onToggleJobsTasks, jobsTasksPanelOpen, onToggleTerminal, termPanelOpen, workspaceId, onWorkspaceChange }: ChatTopBarProps) {
  const http = useHttp();
  const { t } = useTranslation("chat");
  const connected = useAuthStore((s) => s.connected);
  const [agent, setAgent] = useState<{ name: string; emoji?: string } | null>(null);

  // Fetch agent display info (lightweight, cached per agentId)
  useEffect(() => {
    if (!connected || !agentId) return;
    setAgent(null);
    http
      .get<{ agents: AgentData[] }>("/v1/agents")
      .then((res) => {
        const found = (res.agents ?? []).find((a) => a.agent_key === agentId);
        if (found) {
          const emoji = found.emoji || undefined;
          setAgent({ name: found.display_name || found.agent_key, emoji });
        } else {
          setAgent({ name: agentId });
        }
      })
      .catch(() => setAgent({ name: agentId }));
  }, [http, connected, agentId]);

  const emoji = agent?.emoji;
  // Avatar emoji renders beside the name; drop a duplicated leading cluster.
  const displayName = agent ? stripLeadingEmoji(emoji, agent.name) : agentId;
  const PanelIcon = taskPanelOpen ? PanelRightClose : PanelRightOpen;

  // Context-usage badge: only renders when the caller passes a session with
  // both estimatedTokens (Phase 4 ContextStage output) and contextWindow.
  // `percent` drives the color ramp so operators spot near-limit sessions.
  const usage = (() => {
    if (!session || !session.contextWindow || session.contextWindow <= 0) {
      return null;
    }
    const used = session.estimatedTokens ?? 0;
    const max = session.contextWindow;
    const percent = Math.min(100, Math.round((used / max) * 100));
    const color =
      percent >= 90 ? "text-destructive" : percent >= 75 ? "text-amber-600 dark:text-amber-400" : "text-muted-foreground";
    return { used, max, percent, color };
  })();

  // Last compaction timestamp ships in sessions.metadata JSONB (Phase 5 follow-up,
  // keyed "last_compaction_at"). Parsed lazily so bad data doesn't crash the bar.
  const lastCompaction = (() => {
    const raw = session?.metadata?.last_compaction_at;
    if (!raw) return null;
    const d = new Date(raw);
    if (isNaN(d.getTime())) return null;
    return d;
  })();

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
        {usage && (
          <div
            className={`hidden sm:flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-[11px] ${usage.color}`}
            title={t("contextUsage.tooltip", {
              used: usage.used.toLocaleString(),
              max: usage.max.toLocaleString(),
              percent: usage.percent,
              compactions: session?.compactionCount ?? 0,
              lastCompact: lastCompaction ? lastCompaction.toLocaleString() : t("contextUsage.never"),
            })}
          >
            <span className="font-mono">
              {usage.used.toLocaleString()}/{usage.max.toLocaleString()}
            </span>
            <span className="opacity-70">({usage.percent}%)</span>
          </div>
        )}

        {/* Workspace picker (Paseo Phase 3) — scopes files/jobs/tasks panels */}
        <WorkspacePicker value={workspaceId ?? null} onChange={(id) => onWorkspaceChange?.(id)} />

        {/* Console panel toggles — file explorer + jobs/tasks */}
        <button
          type="button"
          onClick={onToggleFiles}
          className={`rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground ${filesPanelOpen ? "bg-accent text-accent-foreground" : ""}`}
          title={filesPanelOpen ? "Close files panel" : "Open files panel"}
        >
          <FolderOpen className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={onToggleJobsTasks}
          className={`rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground ${jobsTasksPanelOpen ? "bg-accent text-accent-foreground" : ""}`}
          title={jobsTasksPanelOpen ? "Close jobs/tasks panel" : "Open jobs/tasks panel"}
        >
          <ListTree className="h-4 w-4" />
        </button>
        {/* Web terminal toggle (Paseo Phase 4 / §25) — needs a selected workspace */}
        <button
          type="button"
          onClick={onToggleTerminal}
          disabled={!workspaceId}
          className={`rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50 ${termPanelOpen ? "bg-accent text-accent-foreground" : ""}`}
          title={!workspaceId ? t("terminal.noWorkspace") : termPanelOpen ? "Close terminal panel" : "Open terminal panel"}
        >
          <SquareTerminal className="h-4 w-4" />
        </button>
        {isRunning ? (
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span>{activity ? phaseLabels[activity.phase] : "Running…"}</span>
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          </div>
        ) : isBusy ? (
          <button
            type="button"
            onClick={onToggleTaskPanel}
            className="flex items-center gap-1.5 rounded-md px-1.5 py-0.5 text-xs text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            <Users className="h-3.5 w-3.5" />
            <span>Team: {teamTasks.length} task{teamTasks.length > 1 ? "s" : ""} active</span>
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          </button>
        ) : (
          <span className="text-xs text-muted-foreground/50">Ready</span>
        )}

        {/* Task panel toggle — visible when there are (or recently were) team tasks */}
        {teamTasks.length > 0 && (
          <button
            type="button"
            onClick={onToggleTaskPanel}
            className="relative rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
            title={taskPanelOpen ? "Close task panel" : "Open task panel"}
          >
            <PanelIcon className="h-4 w-4" />
          </button>
        )}
      </div>
    </div>
  );
}
