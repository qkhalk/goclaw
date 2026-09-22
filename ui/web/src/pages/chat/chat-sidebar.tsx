import { memo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Archive, ChevronDown, MessageSquare, Plus, RotateCcw, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { AgentSelector } from "@/components/chat/agent-selector";
import { SessionSwitcher, sessionLabel } from "@/components/chat/session-switcher";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { formatRelativeTime } from "@/lib/format";
import type { SessionInfo } from "@/types/session";
import type { ArchivedSessionInfo } from "./hooks/use-chat-sessions";

interface ChatSidebarProps {
  agentId: string;
  onAgentChange: (agentId: string) => void;
  sessions: SessionInfo[];
  sessionsLoading: boolean;
  activeSessionKey: string;
  onSessionSelect: (key: string) => void;
  onDeleteSession?: (key: string) => void;
  /** Soft-archive a finished conversation out of the main list (phase 8). */
  onArchiveSession?: (key: string) => void;
  /** Archived section data + actions (rendered only when N > 0). */
  archivedSessions?: ArchivedSessionInfo[];
  onRestoreSession?: (key: string) => void;
  onNewChat: () => void;
  /** Increment to open the agent dropdown programmatically (empty-state CTA). */
  agentSelectorOpenSignal?: number;
  /** Desktop drag-resizable width (px). Undefined on mobile → w-72 drawer. */
  width?: number;
}

export const ChatSidebar = memo(function ChatSidebar({
  agentId,
  onAgentChange,
  sessions,
  sessionsLoading,
  activeSessionKey,
  onSessionSelect,
  onDeleteSession,
  onArchiveSession,
  archivedSessions,
  onRestoreSession,
  onNewChat,
  agentSelectorOpenSignal,
  width,
}: ChatSidebarProps) {
  const { t } = useTranslation("chat");
  const { t: tc } = useTranslation("common");
  const [archivedOpen, setArchivedOpen] = useState(false);
  const [archivedDeleteTarget, setArchivedDeleteTarget] = useState<ArchivedSessionInfo | null>(null);
  const archived = archivedSessions ?? [];

  return (
    <div
      className="flex h-full w-72 max-w-[85vw] shrink-0 flex-col border-r bg-background"
      style={width !== undefined ? { width, maxWidth: "none" } : undefined}
    >
      {/* Agent selector */}
      <div className="border-b p-3">
        <AgentSelector value={agentId} onChange={onAgentChange} openSignal={agentSelectorOpenSignal} />
      </div>

      {/* New chat button */}
      <div className="p-3">
        <Button
          variant="outline"
          className="w-full justify-start gap-2"
          onClick={onNewChat}
        >
          <Plus className="h-4 w-4" />
          {t("newChat")}
        </Button>
      </div>

      {/* Session list */}
      <div className="flex-1 overflow-y-auto overscroll-contain">
        <SessionSwitcher
          sessions={sessions}
          activeKey={activeSessionKey}
          onSelect={onSessionSelect}
          onDelete={onDeleteSession}
          onArchive={onArchiveSession}
          loading={sessionsLoading}
        />

        {/* Archived section — only when there is something archived (phase 8) */}
        {archived.length > 0 && (
          <div className="border-t">
            <button
              type="button"
              onClick={() => setArchivedOpen((o) => !o)}
              aria-expanded={archivedOpen}
              className="flex w-full items-center gap-2 px-4 py-2.5 text-left text-sm text-muted-foreground hover:bg-muted"
              title={t("sessions.archivedSection", { n: archived.length })}
            >
              <Archive className="h-4 w-4 shrink-0" />
              <span className="min-w-0 flex-1 truncate text-xs font-medium">
                {t("sessions.archivedSection", { n: archived.length })}
              </span>
              <ChevronDown
                className={`h-3.5 w-3.5 shrink-0 transition-transform ${archivedOpen ? "" : "-rotate-90"}`}
              />
            </button>

            {archivedOpen && (
              <div className="space-y-0.5 p-1.5">
                {archived.map((session) => {
                  const label = sessionLabel(session);
                  return (
                    <div
                      key={session.key}
                      className="group flex items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors hover:bg-muted"
                    >
                      <button
                        type="button"
                        onClick={() => onSessionSelect(session.key)}
                        className="flex min-w-0 flex-1 items-center gap-2.5 text-left"
                      >
                        <MessageSquare className="h-4 w-4 shrink-0 text-muted-foreground" />
                        <div className="min-w-0 flex-1">
                          <div className="truncate font-medium text-[13px]">{label}</div>
                          <div className="flex items-center gap-1.5 text-xs-plus text-muted-foreground">
                            <span>{session.messageCount} {tc("messages")}</span>
                            <span>·</span>
                            <span>{formatRelativeTime(session.updated)}</span>
                          </div>
                        </div>
                      </button>
                      {onRestoreSession && (
                        <span
                          role="button"
                          tabIndex={0}
                          onClick={(e) => {
                            e.stopPropagation();
                            onRestoreSession(session.key);
                          }}
                          onKeyDown={(e) => {
                            if (e.key === "Enter" || e.key === " ") {
                              e.stopPropagation();
                              e.preventDefault();
                              onRestoreSession(session.key);
                            }
                          }}
                          className="shrink-0 rounded-md p-1.5 text-muted-foreground opacity-0 transition-opacity hover:bg-accent hover:text-accent-foreground group-hover:opacity-100 max-sm:opacity-100"
                          title={t("sessions.restore")}
                        >
                          <RotateCcw className="h-3.5 w-3.5" />
                        </span>
                      )}
                      {onDeleteSession && (
                        <span
                          role="button"
                          tabIndex={0}
                          onClick={(e) => {
                            e.stopPropagation();
                            setArchivedDeleteTarget(session);
                          }}
                          onKeyDown={(e) => {
                            if (e.key === "Enter" || e.key === " ") {
                              e.stopPropagation();
                              e.preventDefault();
                              setArchivedDeleteTarget(session);
                            }
                          }}
                          className="shrink-0 rounded-md p-1.5 text-muted-foreground opacity-0 transition-opacity hover:bg-destructive/10 hover:text-destructive group-hover:opacity-100 max-sm:opacity-100"
                          title={t("deleteChat")}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </span>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Delete confirm for archived rows (same copy as the active list) */}
      <Dialog
        open={!!archivedDeleteTarget}
        onOpenChange={(open) => !open && setArchivedDeleteTarget(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("deleteChat")}</DialogTitle>
            <DialogDescription>{t("deleteChatConfirm")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setArchivedDeleteTarget(null)}>
              {tc("cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (archivedDeleteTarget) {
                  onDeleteSession?.(archivedDeleteTarget.key);
                  setArchivedDeleteTarget(null);
                }
              }}
            >
              {tc("delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
});
