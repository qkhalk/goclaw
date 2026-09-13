import { useState, useCallback, useEffect, useRef, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useParams, useNavigate } from "react-router";
import { Eye, PanelLeftOpen } from "lucide-react";
import { useAuthStore } from "@/stores/use-auth-store";
import { useIsMobile } from "@/hooks/use-media-query";
import { cn } from "@/lib/utils";
import { ChatSidebar } from "./chat-sidebar";
import { ChatThread } from "./chat-thread";
import { AskOptionsProvider, askAnswerText, type AskOptionsContextValue } from "@/components/chat/ask-options-context";
import { ChatInput, type AttachedFile, type ComposerOverrides } from "@/components/chat/chat-input";
import { ChatTopBar } from "@/components/chat/chat-top-bar";
import { DropZone } from "@/components/chat/drop-zone";
import { TeamTasksPill } from "@/components/chat/team-tasks-pill";
import { useChatSessions } from "./hooks/use-chat-sessions";
import { useChatMessages } from "./hooks/use-chat-messages";
import { useChatSend } from "./hooks/use-chat-send";
import { isOwnSession, parseSessionKey } from "@/lib/session-key";
import { useVirtualKeyboard } from "@/hooks/use-virtual-keyboard";
import { FileExplorerPanel } from "@/components/chat/file-explorer-panel";
import { JobsTasksPanel } from "@/components/chat/jobs-tasks-panel";
import { TerminalPanel } from "@/components/chat/terminal-panel";

export function ChatPage() {
  const { t } = useTranslation("chat");
  const { sessionKey: urlSessionKey } = useParams<{ sessionKey: string }>();
  const navigate = useNavigate();
  const connected = useAuthStore((s) => s.connected);
  const userId = useAuthStore((s) => s.userId);

  const [scrollTrigger, setScrollTrigger] = useState(0);
  const [files, setFiles] = useState<AttachedFile[]>([]);

  // sessionKey derived from URL — single source of truth, no separate state
  const sessionKey = urlSessionKey ?? "";

  // Fallback agent ID used only when URL has no session key
  const [agentIdFallback, setAgentIdFallback] = useState("");

  // Agent is confirmed when URL has a session (agentId parsed) or user explicitly picked one
  const agentConfirmed = !!urlSessionKey || !!agentIdFallback;

  // Derive agentId from URL (source of truth), fallback to state when no session
  const agentId = useMemo(() => {
    if (urlSessionKey) {
      const { agentId: parsed } = parseSessionKey(urlSessionKey);
      if (parsed) return parsed;
    }
    return agentIdFallback;
  }, [urlSessionKey, agentIdFallback]);

  const {
    sessions,
    loading: sessionsLoading,
    refresh: refreshSessions,
    buildNewSessionKey,
    deleteSession,
  } = useChatSessions(agentId);

  const {
    messages,
    streamText,
    thinkingText,
    toolStream,
    isRunning,
    isBusy,
    loading: messagesLoading,
    activity,
    blockReplies,
    teamTasks,
    expectRun,
    addLocalMessage,
  } = useChatMessages(sessionKey, agentId);

  // Refresh sessions when all work completes (main agent + team tasks)
  const prevIsBusyRef = useRef(false);
  useEffect(() => {
    if (prevIsBusyRef.current && !isBusy) {
      refreshSessions();
    }
    prevIsBusyRef.current = isBusy;
  }, [isBusy, refreshSessions]);

  const isOwn = !sessionKey || isOwnSession(sessionKey, userId);

  const handleMessageAdded = useCallback(
    (msg: { role: "user" | "assistant" | "tool"; content: string; timestamp?: number }, key?: string) => {
      addLocalMessage(msg, key);
    },
    [addLocalMessage],
  );

  const { send, abort, error: sendError } = useChatSend({
    agentId,
    onMessageAdded: handleMessageAdded,
    onExpectRun: expectRun,
  });

  const handleNewChat = useCallback(() => {
    navigate(`/chat/${encodeURIComponent(buildNewSessionKey())}`);
  }, [buildNewSessionKey, navigate]);

  const handleSessionSelect = useCallback(
    (key: string) => {
      const { agentId: parsed } = parseSessionKey(key);
      if (parsed) setAgentIdFallback(parsed);
      navigate(`/chat/${encodeURIComponent(key)}`);
    },
    [navigate],
  );

  const handleDeleteSession = useCallback(async (key: string) => {
    await deleteSession(key);
    if (key === sessionKey) {
      const next = sessions.find((s) => s.key !== key);
      if (next) {
        handleSessionSelect(next.key);
      } else {
        handleNewChat();
      }
    }
  }, [deleteSession, sessionKey, sessions, handleSessionSelect, handleNewChat]);

  const handleAgentChange = useCallback(
    (newAgentId: string) => {
      setAgentIdFallback(newAgentId);
      if (sessionKey) {
        navigate("/chat");
      }
    },
    [navigate],
  );

  const handleSend = useCallback(
    (message: string, sendFiles?: AttachedFile[], overrides?: ComposerOverrides) => {
      let key = sessionKey;
      if (!key) {
        key = buildNewSessionKey();
        navigate(`/chat/${encodeURIComponent(key)}`, { replace: true });
      }
      send(message, key, sendFiles, overrides);
      setScrollTrigger((n) => n + 1);
    },
    [sessionKey, send, buildNewSessionKey, navigate],
  );

  const handleDropFiles = useCallback((dropped: File[]) => {
    setFiles((prev) => [...prev, ...dropped.map((f) => ({ file: f }))]);
  }, []);

  const handleAbort = useCallback(() => {
    abort(sessionKey);
  }, [abort, sessionKey]);

  // ask_options question cards: send the picked/typed option as the next user
  // message (same inject format as the Telegram channel) and detect answered
  // questions from history so cards stay resolved across reloads.
  const askOptionsValue = useMemo<AskOptionsContextValue | null>(() => {
    if (!isOwn) return null;
    return {
      answer: (question, answer) => handleSend(askAnswerText(question, answer)),
      isAnswered: (question) => {
        const prefix = `[Answering your question] ${question.trim()} →`;
        return messages.some(
          (m) => m.role === "user" && typeof m.content === "string" && m.content.startsWith(prefix),
        );
      },
    };
  }, [isOwn, handleSend, messages]);

  const isMobile = useIsMobile();
  useVirtualKeyboard();
  const [chatSidebarOpen, setChatSidebarOpen] = useState(false);
  // Incremented by the empty-state CTA to open the sidebar AgentSelector dropdown.
  const [agentSelectorOpenSignal, setAgentSelectorOpenSignal] = useState(0);
  // Paseo Phase 3 console panels: workspace selection + right-side tools.
  const [workspaceId, setWorkspaceId] = useState<string | null>(null);
  const [filesPanelOpen, setFilesPanelOpen] = useState(false);
  const [jobsPanelOpen, setJobsPanelOpen] = useState(false);
  // Paseo Phase 4 (§25): web terminal side panel.
  const [termOpen, setTermOpen] = useState(false);

  const handleSessionSelectMobile = useCallback(
    (key: string) => {
      handleSessionSelect(key);
      setChatSidebarOpen(false);
    },
    [handleSessionSelect],
  );

  const handleNewChatMobile = useCallback(() => {
    handleNewChat();
    setChatSidebarOpen(false);
  }, [handleNewChat]);

  return (
    <div className="relative flex h-full overflow-hidden">
      {/* Chat Sidebar */}
      {isMobile ? (
        <>
          {chatSidebarOpen && (
            <div
              className="fixed inset-0 z-40 bg-black/50"
              onClick={() => setChatSidebarOpen(false)}
            />
          )}
          <div
            className={cn(
              "fixed inset-y-0 left-0 z-50 transition-transform duration-200 ease-in-out",
              chatSidebarOpen ? "translate-x-0" : "-translate-x-full",
            )}
          >
            <ChatSidebar
              agentId={agentId}
              onAgentChange={handleAgentChange}
              sessions={sessions}
              sessionsLoading={sessionsLoading}
              activeSessionKey={sessionKey}
              onSessionSelect={handleSessionSelectMobile}
              onDeleteSession={handleDeleteSession}
              onNewChat={handleNewChatMobile}
              agentSelectorOpenSignal={agentSelectorOpenSignal}
            />
          </div>
        </>
      ) : (
        <ChatSidebar
          agentId={agentId}
          onAgentChange={handleAgentChange}
          sessions={sessions}
          sessionsLoading={sessionsLoading}
          activeSessionKey={sessionKey}
          onSessionSelect={handleSessionSelect}
          onDeleteSession={handleDeleteSession}
          onNewChat={handleNewChat}
          agentSelectorOpenSignal={agentSelectorOpenSignal}
        />
      )}

      {/* Main chat area */}
      <div className="flex min-w-0 flex-1 min-h-0 flex-col">
        {isMobile && (
          <div className="flex shrink-0 items-center border-b px-3 py-2 landscape-compact">
            <button
              onClick={() => setChatSidebarOpen(true)}
              className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              title={t("openSessions")}
            >
              <PanelLeftOpen className="h-4 w-4" />
            </button>
          </div>
        )}

        <div className="shrink-0">
          <ChatTopBar
            agentId={agentId}
            isRunning={isRunning}
            session={sessions.find((s) => s.key === sessionKey) ?? null}
            onToggleFiles={() => setFilesPanelOpen((v) => !v)}
            filesPanelOpen={filesPanelOpen}
            onToggleJobsTasks={() => setJobsPanelOpen((v) => !v)}
            jobsTasksPanelOpen={jobsPanelOpen}
            onToggleTerminal={() => setTermOpen((v) => !v)}
            termPanelOpen={termOpen}
            workspaceId={workspaceId}
            onWorkspaceChange={setWorkspaceId}
          />
        </div>

        {sendError && (
          <div className="shrink-0 border-b bg-destructive/10 px-4 py-2 text-sm text-destructive">
            {sendError}
          </div>
        )}

        <DropZone onDrop={handleDropFiles}>
          <AskOptionsProvider value={askOptionsValue}>
            <ChatThread
              messages={messages}
              streamText={streamText}
              thinkingText={thinkingText}
              toolStream={toolStream}
              blockReplies={blockReplies}
              activity={activity}
              isRunning={isRunning}
              isBusy={isBusy}
              loading={messagesLoading}
              scrollTrigger={scrollTrigger}
            />
          </AskOptionsProvider>

          {!isOwn ? (
            <div className="mx-3 mb-3 flex items-center gap-2 rounded-xl border bg-muted/50 px-4 py-3 text-sm text-muted-foreground shadow-sm">
              <Eye className="h-4 w-4" />
              {t("readOnly")}
            </div>
          ) : !agentConfirmed ? (
            <div className="mx-3 mb-3 safe-bottom">
              <div className="rounded-xl border bg-background/95 backdrop-blur-sm shadow-sm p-4 text-center">
                <p className="text-sm font-medium mb-1">{t("selectAgent.title")}</p>
                <p className="text-xs text-muted-foreground mb-3">{t("selectAgent.description")}</p>
                <button
                  type="button"
                  onClick={() => {
                    if (isMobile) setChatSidebarOpen(true);
                    setAgentSelectorOpenSignal((n) => n + 1);
                  }}
                  className="inline-flex min-h-[44px] items-center justify-center gap-2 rounded-lg border bg-muted/60 px-4 text-sm font-medium hover:bg-accent hover:text-accent-foreground"
                >
                  {t("selectAgent.title")}
                </button>
              </div>
            </div>
          ) : (
            <>
              <TeamTasksPill tasks={teamTasks} />
              <ChatInput
                onSend={handleSend}
                onAbort={handleAbort}
                isBusy={isBusy}
                disabled={!connected}
                files={files}
                onFilesChange={setFiles}
              />
            </>
          )}
        </DropZone>
      </div>

      {/* Mobile overlay backdrops for the console panels — one open at a time */}
      {filesPanelOpen && !jobsPanelOpen && isMobile && (
        <div className="fixed inset-0 z-40 bg-black/50" onClick={() => setFilesPanelOpen(false)} />
      )}
      {jobsPanelOpen && isMobile && (
        <div className="fixed inset-0 z-40 bg-black/50" onClick={() => setJobsPanelOpen(false)} />
      )}
      {termOpen && !filesPanelOpen && !jobsPanelOpen && isMobile && (
        <div className="fixed inset-0 z-40 bg-black/50" onClick={() => setTermOpen(false)} />
      )}

      <FileExplorerPanel
        open={filesPanelOpen}
        onClose={() => setFilesPanelOpen(false)}
        workspaceId={workspaceId}
      />
      <JobsTasksPanel
        open={jobsPanelOpen}
        onClose={() => setJobsPanelOpen(false)}
        workspaceId={workspaceId}
      />
      <TerminalPanel
        open={termOpen}
        onClose={() => setTermOpen(false)}
        workspaceId={workspaceId}
      />
    </div>
  );
}
