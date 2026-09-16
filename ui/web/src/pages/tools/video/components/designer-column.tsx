import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Braces, History, Loader2, MessageSquarePlus, Palette, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { ResizeHandle } from "@/components/shared/resize-handle";
import { MessageBubble } from "@/components/chat/message-bubble";
import { ActiveRunZone } from "@/components/chat/active-run-zone";
import { ChatInput, type AttachedFile, type ComposerOverrides } from "@/components/chat/chat-input";
import { useHttp } from "@/hooks/use-ws";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/format";
import { useIsTablet } from "@/hooks/use-media-query";
import { useVirtualKeyboard } from "@/hooks/use-virtual-keyboard";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import {
  useUiStore,
  VIDEO_DESIGNER_WIDTH,
} from "@/stores/use-ui-store";
import type { Storyboard } from "../video-tool-page";
import {
  extractStoryboardBlocks,
  parseStoryboard,
  stripStoryboardBlocks,
  type ParsedStoryboard,
} from "../lib/parse-storyboard-blocks";
import { StoryboardCard } from "./storyboard-card";
import { useDesignerChat, DESIGNER_AGENT_KEY } from "../hooks/use-designer-chat";

/** Widening the designer column must never squeeze the editor below this. */
const MIN_EDITOR_COLUMN_PX = 480;

interface DesignerColumnProps {
  /** Apply a parsed storyboard to the editor (page owns the timeline). */
  onApplyStoryboard: (sb: Storyboard) => void;
  /** Live storyboard from the editor, for the "attach current" button. */
  currentStoryboard: Storyboard;
}

/**
 * The video-designer chat column beside the Video Editor: a design-only
 * agent session with the same chat primitives as /chat, plus storyboard
 * cards under replies that carry a ```storyboard block. Desktop: resizable
 * right rail; mobile: bottom sheet.
 */
export function DesignerColumn({ onApplyStoryboard, currentStoryboard }: DesignerColumnProps) {
  const { t } = useTranslation("toolbox");
  const { t: tCommon } = useTranslation("common");
  // Sheet below lg (plan: <1024px), resizable rail on real desktops.
  const isCompact = useIsTablet();
  useVirtualKeyboard();

  const open = useUiStore((s) => s.videoDesignerOpen);
  const setOpen = useUiStore((s) => s.setVideoDesignerOpen);
  const width = useUiStore((s) => s.videoDesignerWidth);

  const chat = useDesignerChat();
  const http = useHttp();
  const [appliedRaw, setAppliedRaw] = useState<string | null>(null);
  const [attached, setAttached] = useState(false);
  const [dragging, setDragging] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  // Conversation history: past designer sessions (same agent, direct-WS
  // channel) listed on demand from the sessions API.
  const [historyOpen, setHistoryOpen] = useState(false);
  const [history, setHistory] = useState<DesignerSession[] | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);

  const loadHistoryList = useCallback(async () => {
    setHistoryLoading(true);
    try {
      const res = await http.get<{ sessions?: DesignerSession[] }>("/v1/sessions", {
        agentId: DESIGNER_AGENT_KEY,
        limit: "30",
      });
      setHistory(res.sessions ?? []);
    } catch {
      setHistory([]);
    } finally {
      setHistoryLoading(false);
    }
  }, [http]);

  const toggleHistory = useCallback(() => {
    setHistoryOpen((v) => {
      if (!v) void loadHistoryList();
      return !v;
    });
  }, [loadHistoryList]);

  const openFromHistory = useCallback(
    (key: string) => {
      chat.openSession(key);
      setHistoryOpen(false);
    },
    [chat],
  );

  const historyMessagesLabel = useCallback(
    (n: number) => t("video.designer.historyMessages", { n }),
    [t],
  );

  // Feed the composer's model picker with the designer agent's own provider
  // so a model can be picked without switching provider.
  const { agents } = useAgents();
  const designerProvider = useMemo(
    () => agents.find((a) => a.agent_key === DESIGNER_AGENT_KEY)?.provider,
    [agents],
  );

  const resizeDesigner = useCallback((dx: number) => {
    const s = useUiStore.getState();
    const dynamicMax = Math.max(
      VIDEO_DESIGNER_WIDTH.min,
      Math.min(VIDEO_DESIGNER_WIDTH.max, window.innerWidth - MIN_EDITOR_COLUMN_PX),
    );
    s.setVideoDesignerWidth(Math.min(s.videoDesignerWidth - dx, dynamicMax));
  }, []);

  // Keep the latest reply in view: jump on new messages and while streaming.
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  }, [chat.messages.length, chat.streamText, chat.isRunning]);

  // Parse every assistant message once per render pass (tiny JSON, few
  // messages) and remember which message carries which blocks.
  const items = useMemo(() => {
    return chat.messages.map((m) => {
      if (m.role !== "assistant") return { message: m, blocks: [] as ParsedStoryboard[] };
      const blocks = extractStoryboardBlocks(m.content).map((b) => parseStoryboard(b.json));
      if (blocks.length === 0) return { message: m, blocks };
      return { message: { ...m, content: stripStoryboardBlocks(m.content) }, blocks };
    });
  }, [chat.messages]);

  // Provisional card while the agent is still typing: the last block that
  // has already closed inside the stream text.
  const streamBlocks = useMemo(() => {
    if (!chat.isRunning || !chat.streamText) return [] as ParsedStoryboard[];
    return extractStoryboardBlocks(chat.streamText)
      .slice(-1)
      .map((b) => parseStoryboard(b.json));
  }, [chat.isRunning, chat.streamText]);

  const handleApply = useCallback(
    (parsed: ParsedStoryboard) => {
      if (!parsed.ok) return;
      setAppliedRaw(parsed.raw);
      onApplyStoryboard(parsed.sb);
    },
    [onApplyStoryboard],
  );

  const handleSend = useCallback(
    (message: string, files?: AttachedFile[], overrides?: ComposerOverrides) => {
      if (attached) {
        const json = JSON.stringify(currentStoryboard, null, 2);
        chat.send(`${t("video.designer.attachedPrefix")}\n\`\`\`storyboard\n${json}\n\`\`\`\n\n${message}`, files, overrides);
        setAttached(false);
      } else {
        chat.send(message, files, overrides);
      }
    },
    [attached, currentStoryboard, chat, t],
  );

  if (isCompact) {
    return (
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent
          className="max-sm:h-[92dvh] max-sm:overflow-hidden p-0 gap-0"
          aria-describedby={undefined}
        >
          <SheetTitle className="sr-only">{t("video.designer.title")}</SheetTitle>
          {/* grab handle */}
          <div className="mx-auto mt-2 h-1 w-8 shrink-0 rounded-full bg-muted-foreground/40" />
          <div className="flex min-h-0 flex-1 flex-col">
            <ColumnHeader
              title={t("video.designer.title")}
              isRunning={chat.isRunning}
              onHistory={toggleHistory}
              historyLabel={t("video.designer.history")}
              historyActive={historyOpen}
              onAttach={() => setAttached((v) => !v)}
              attachActive={attached}
              attachLabel={t("video.designer.attach")}
              onNewChat={chat.newChat}
              onClose={() => setOpen(false)}
              newChatLabel={t("video.designer.newChat")}
              closeLabel={t("video.designer.close")}
            />
            <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 py-3">
              {historyOpen && (
                <HistoryPanel
                  sessions={history}
                  loading={historyLoading}
                  currentKey={chat.sessionKey}
                  onOpen={openFromHistory}
                  onClose={() => setHistoryOpen(false)}
                  title={t("video.designer.history")}
                  emptyLabel={t("video.designer.historyEmpty")}
                  messagesLabel={historyMessagesLabel}
                />
              )}
              <MessageList
                items={items}
                streamBlocks={streamBlocks}
                streamProps={chat}
                appliedRaw={appliedRaw}
                onApply={handleApply}
                emptyHint={t("video.designer.emptyHint")}
                chips={[t("video.designer.chip1"), t("video.designer.chip2"), t("video.designer.chip3")]}
                onChip={chat.send}
                designerBadge={t("video.designer.badge")}
              />
            </div>
            <ComposerRow
              chat={chat}
              onSend={handleSend}
              attached={attached}
              onDetach={() => setAttached(false)}
              attachedLabel={t("video.designer.attached")}
              designingLabel={t("video.designer.designing")}
              storageKey="goclaw.composer-override:video-designer"
              defaultProviderName={designerProvider}
              sendError={chat.sendError}
            />
          </div>
        </SheetContent>
      </Sheet>
    );
  }

  if (!open) return null;

  return (
    <div className={cn("flex h-full min-h-0 shrink-0", dragging && "select-none")}>
      <ResizeHandle
        side="left"
        onResize={resizeDesigner}
        onReset={() => useUiStore.getState().setVideoDesignerWidth(VIDEO_DESIGNER_WIDTH.default)}
        onDragStart={() => setDragging(true)}
        onDragEnd={() => setDragging(false)}
        ariaLabel={tCommon("pane.resize")}
      />
      <div
        className="flex h-full min-h-0 min-w-0 flex-col border-l bg-background"
        style={{ width }}
      >
        <ColumnHeader
          title={t("video.designer.title")}
          isRunning={chat.isRunning}
          onHistory={toggleHistory}
          historyLabel={t("video.designer.history")}
          historyActive={historyOpen}
          onAttach={() => setAttached((v) => !v)}
          attachActive={attached}
          attachLabel={t("video.designer.attach")}
          onNewChat={chat.newChat}
          onClose={() => setOpen(false)}
          newChatLabel={t("video.designer.newChat")}
          closeLabel={t("video.designer.close")}
        />
        <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 py-3">
          {historyOpen && (
            <HistoryPanel
              sessions={history}
              loading={historyLoading}
              currentKey={chat.sessionKey}
              onOpen={openFromHistory}
              onClose={() => setHistoryOpen(false)}
              title={t("video.designer.history")}
              emptyLabel={t("video.designer.historyEmpty")}
              messagesLabel={historyMessagesLabel}
            />
          )}
          <MessageList
            items={items}
            streamBlocks={streamBlocks}
            streamProps={chat}
            appliedRaw={appliedRaw}
            onApply={handleApply}
            emptyHint={t("video.designer.emptyHint")}
            chips={[t("video.designer.chip1"), t("video.designer.chip2"), t("video.designer.chip3")]}
            onChip={chat.send}
            designerBadge={t("video.designer.badge")}
          />
        </div>
        <ComposerRow
          chat={chat}
          onSend={handleSend}
          attached={attached}
          onDetach={() => setAttached(false)}
          attachedLabel={t("video.designer.attached")}
          designingLabel={t("video.designer.designing")}
          storageKey="goclaw.composer-override:video-designer"
          defaultProviderName={designerProvider}
          sendError={chat.sendError}
        />
      </div>
    </div>
  );
}

type ChatView = ReturnType<typeof useDesignerChat>;

/** Row shape of GET /v1/sessions for the designer agent (subset used). */
interface DesignerSession {
  key: string;
  messageCount: number;
  updated: string;
  label?: string;
}

function ColumnHeader({
  title,
  isRunning,
  onHistory,
  historyLabel,
  historyActive,
  onAttach,
  attachActive,
  attachLabel,
  onNewChat,
  onClose,
  newChatLabel,
  closeLabel,
}: {
  title: string;
  isRunning: boolean;
  onHistory: () => void;
  historyLabel: string;
  historyActive: boolean;
  onAttach: () => void;
  attachActive: boolean;
  attachLabel: string;
  onNewChat: () => void;
  onClose: () => void;
  newChatLabel: string;
  closeLabel: string;
}) {
  return (
    <div className="flex shrink-0 items-center gap-2 border-b px-3 py-2">
      <Palette className="h-4 w-4 shrink-0 text-muted-foreground" />
      <span className="truncate text-sm font-medium">{title}</span>
      <span
        aria-hidden
        className={cn(
          "h-2 w-2 shrink-0 rounded-full",
          isRunning ? "animate-pulse bg-primary" : "bg-muted-foreground/40",
        )}
        title={isRunning ? "..." : ""}
      />
      <div className="ml-auto flex shrink-0 items-center">
        <button
          type="button"
          onClick={onAttach}
          title={attachLabel}
          aria-label={attachLabel}
          aria-pressed={attachActive}
          className={cn(
            "flex h-11 w-11 items-center justify-center rounded-lg transition-colors sm:h-8 sm:w-8",
            attachActive
              ? "bg-accent text-accent-foreground"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          <Braces className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={onHistory}
          title={historyLabel}
          aria-label={historyLabel}
          aria-pressed={historyActive}
          className={cn(
            "flex h-11 w-11 items-center justify-center rounded-lg transition-colors sm:h-8 sm:w-8",
            historyActive
              ? "bg-accent text-accent-foreground"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          <History className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={onNewChat}
          title={newChatLabel}
          aria-label={newChatLabel}
          className="flex h-11 w-11 items-center justify-center rounded-lg text-muted-foreground hover:text-foreground transition-colors sm:h-8 sm:w-8"
        >
          <MessageSquarePlus className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={onClose}
          title={closeLabel}
          aria-label={closeLabel}
          className="flex h-11 w-11 items-center justify-center rounded-lg text-muted-foreground hover:text-foreground transition-colors sm:h-8 sm:w-8"
        >
          <X className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}

/** Sticky overlay listing past designer conversations; click to resume one. */
function HistoryPanel({
  sessions,
  loading,
  currentKey,
  onOpen,
  onClose,
  title,
  emptyLabel,
  messagesLabel,
}: {
  sessions: DesignerSession[] | null;
  loading: boolean;
  currentKey: string;
  onOpen: (key: string) => void;
  onClose: () => void;
  title: string;
  emptyLabel: string;
  messagesLabel: (n: number) => string;
}) {
  return (
    <div className="sticky top-0 z-20 -mx-3 mb-2 border-b bg-background/95 px-3 py-2 backdrop-blur">
      <div className="mb-1 flex items-center gap-2">
        <History className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="text-xs font-medium text-muted-foreground">{title}</span>
        <button
          type="button"
          onClick={onClose}
          className="ml-auto flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground hover:text-foreground"
          aria-label={title}
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>
      {loading ? (
        <div className="flex justify-center py-4">
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        </div>
      ) : !sessions || sessions.length === 0 ? (
        <p className="py-4 text-center text-xs text-muted-foreground">{emptyLabel}</p>
      ) : (
        <ul className="flex max-h-64 flex-col gap-0.5 overflow-y-auto overscroll-contain">
          {sessions.map((s) => (
            <li key={s.key}>
              <button
                type="button"
                onClick={() => onOpen(s.key)}
                className={cn(
                  "flex w-full flex-col items-start gap-0.5 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent",
                  s.key === currentKey && "bg-accent",
                )}
              >
                <span className="line-clamp-1 w-full text-sm">
                  {s.label || `#${s.key.slice(-8)}`}
                </span>
                <span className="text-xs text-muted-foreground">
                  {formatRelativeTime(s.updated)} · {messagesLabel(s.messageCount)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

interface MessageListItem {
  message: Parameters<typeof MessageBubble>[0]["message"];
  blocks: ParsedStoryboard[];
}

function MessageList({
  items,
  streamBlocks,
  streamProps,
  appliedRaw,
  onApply,
  emptyHint,
  chips,
  onChip,
  designerBadge,
}: {
  items: MessageListItem[];
  streamBlocks: ParsedStoryboard[];
  streamProps: ChatView;
  appliedRaw: string | null;
  onApply: (parsed: ParsedStoryboard) => void;
  emptyHint: string;
  chips: string[];
  onChip: (text: string) => void;
  designerBadge: string;
}) {
  if (items.length === 0 && !streamProps.isRunning) {
    return <EmptyDesigner emptyHint={emptyHint} chips={chips} onChip={onChip} />;
  }
  return (
    <div className="flex flex-col gap-3">
      <Badge variant="outline" className="mr-auto shrink-0 text-muted-foreground">
        {designerBadge}
      </Badge>
      {items.map((item, i) => (
        <div key={i}>
          <MessageBubble message={item.message} />
          {item.blocks.map((parsed, j) => (
            <StoryboardCard
              key={j}
              parsed={parsed}
              applied={!!parsed.ok && parsed.raw === appliedRaw}
              onApply={() => onApply(parsed)}
            />
          ))}
        </div>
      ))}
      <ActiveRunZone
        isRunning={streamProps.isRunning}
        activity={streamProps.activity}
        thinkingText={streamProps.thinkingText}
        streamText={streamProps.streamText}
        toolStream={streamProps.toolStream}
        blockReplies={streamProps.blockReplies}
      />
      {streamBlocks.map((parsed, j) => (
        <StoryboardCard
          key={`stream-${j}`}
          parsed={parsed}
          applied={false}
          onApply={() => onApply(parsed)}
        />
      ))}
    </div>
  );
}

function EmptyDesigner({
  emptyHint,
  chips,
  onChip,
}: {
  emptyHint: string;
  chips: string[];
  onChip: (text: string) => void;
}) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 px-2 text-center">
      <Palette className="h-6 w-6 text-muted-foreground" />
      <p className="max-w-[260px] text-sm text-muted-foreground">{emptyHint}</p>
      <div className="flex flex-col items-stretch gap-2 pt-1">
        {chips.map((chip) => (
          <button
            key={chip}
            type="button"
            onClick={() => onChip(chip)}
            className="rounded-full border px-3 py-2 text-sm text-primary hover:bg-accent transition-colors min-h-11 sm:min-h-9"
          >
            {chip}
          </button>
        ))}
      </div>
    </div>
  );
}

function ComposerRow({
  chat,
  onSend,
  attached,
  onDetach,
  attachedLabel,
  designingLabel,
  storageKey,
  defaultProviderName,
  sendError,
}: {
  chat: ChatView;
  onSend: (message: string, files?: AttachedFile[], overrides?: ComposerOverrides) => void;
  attached: boolean;
  onDetach: () => void;
  attachedLabel: string;
  designingLabel: string;
  storageKey: string;
  defaultProviderName?: string;
  sendError: string | null;
}) {
  const [files, setFiles] = useState<AttachedFile[]>([]);
  return (
    <div className="shrink-0">
      {chat.isRunning && (
        <p className="px-4 pb-1 font-mono text-xs text-muted-foreground">{designingLabel}</p>
      )}
      {sendError && (
        <p className="px-4 pb-1 text-xs text-destructive">{sendError}</p>
      )}
      {attached && (
        <button
          type="button"
          onClick={onDetach}
          className="mx-3 mb-1.5 flex w-[calc(100%-1.5rem)] items-center gap-2 rounded-md border border-primary/40 bg-primary/10 px-2.5 py-1 text-left text-xs text-primary transition-colors hover:bg-primary/20"
        >
          <Braces className="h-3.5 w-3.5 shrink-0" />
          <span className="min-w-0 flex-1 truncate">{attachedLabel}</span>
          <X className="h-3 w-3 shrink-0" />
        </button>
      )}
      <ChatInput
        onSend={(message, fs, overrides) => onSend(message, fs, overrides)}
        onAbort={chat.abort}
        isBusy={chat.isBusy}
        files={files}
        onFilesChange={setFiles}
        storageKey={storageKey}
        defaultProviderName={defaultProviderName}
        showPermissionMode={false}
      />
    </div>
  );
}
