import { FolderOpen, Globe, ListTree, SquareTerminal } from "lucide-react";
import { useTranslation } from "react-i18next";
import { ResizeHandle } from "@/components/shared/resize-handle";
import { cn } from "@/lib/utils";
import { FileExplorerPanel } from "./file-explorer-panel";
import { JobsTasksPanel } from "./jobs-tasks-panel";
import { TerminalPanel } from "./terminal-panel";
import { BrowserPanel } from "./browser-panel";
import type { BrowserPanelState } from "@/pages/chat/hooks/use-browser-panel";

/** Identifiers for the single right-hand tabbed side pane. */
export type ChatPaneId = "files" | "jobs" | "terminal" | "browser";

interface ChatSidePaneProps {
  /** Active tab; null = pane closed (renders nothing). */
  active: ChatPaneId | null;
  workspaceId: string | null;
  /** Pane width in px on desktop; null on mobile → fullscreen overlay. */
  width: number | null;
  onResize: (dx: number) => void;
  onResetWidth: () => void;
  onClose: () => void;
  /** Tab switch request from the strip (chat-page decides toggle vs switch). */
  onSelect: (id: ChatPaneId) => void;
  browser: {
    state: BrowserPanelState;
    onIframeLoad: (iframe: HTMLIFrameElement | null) => void;
    onReload: () => void;
  };
}

/**
 * The single right-hand side pane (ZCode-style "Open tab"): one resizable
 * column with a tab strip — Files / Jobs & Tasks / Terminal / Browser.
 * Opening a different tool switches the tab instead of stacking another
 * column, so the chat thread never gets squeezed by multiple panels.
 * Only the active tab's component is mounted; the terminal replays its ring
 * buffer on re-attach and the browser relay URL stays valid within its TTL,
 * so remounting a tab is cheap.
 */
export function ChatSidePane({ active, workspaceId, width, onResize, onResetWidth, onClose, onSelect, browser }: ChatSidePaneProps) {
  const { t } = useTranslation("chat");
  const { t: tCommon } = useTranslation("common");

  if (!active) return null;

  return (
    <div className={cn(
      "flex h-full min-h-0 shrink-0",
      // Mobile: fullscreen overlay above the chat (backdrop rendered by chat-page).
      width === null && "fixed inset-0 z-50",
    )}>
      <ResizeHandle
        side="left"
        onResize={onResize}
        onReset={onResetWidth}
        ariaLabel={tCommon("pane.resize")}
        className={width === null ? "hidden" : ""}
      />
      <div
        className={cn(
          "flex h-full min-h-0 min-w-0 flex-1 flex-col border-l bg-background",
          width === null && "border-l-0 safe-top",
        )}
        style={width !== null ? { width } : undefined}
      >
        {/* Tab strip */}
        <div className="flex shrink-0 items-center gap-0.5 border-b px-1.5 py-1">
          <TabButton
            active={active === "files"}
            icon={<FolderOpen className="h-3.5 w-3.5" />}
            label={t("fileExplorer.title")}
            onClick={() => onSelect("files")}
          />
          <TabButton
            active={active === "jobs"}
            icon={<ListTree className="h-3.5 w-3.5" />}
            label={t("jobsPanel.title")}
            onClick={() => onSelect("jobs")}
          />
          <TabButton
            active={active === "terminal"}
            icon={<SquareTerminal className="h-3.5 w-3.5" />}
            label={t("terminal.title")}
            disabled={!workspaceId}
            title={!workspaceId ? t("terminal.noWorkspace") : undefined}
            onClick={() => onSelect("terminal")}
          />
          <TabButton
            active={active === "browser"}
            icon={<Globe className="h-3.5 w-3.5" />}
            label={t("browserPanel.title")}
            onClick={() => onSelect("browser")}
          />
        </div>

        {/* Active tab body. The panels keep their own headers (refresh /
            new-tab / reload / close actions) — the pane only adds the strip. */}
        {active === "files" && <FileExplorerPanel open onClose={onClose} workspaceId={workspaceId} />}
        {active === "jobs" && <JobsTasksPanel open onClose={onClose} workspaceId={workspaceId} />}
        {active === "terminal" && <TerminalPanel open onClose={onClose} workspaceId={workspaceId} />}
        {active === "browser" && (
          <BrowserPanel
            open
            onClose={onClose}
            state={browser.state}
            onIframeLoad={browser.onIframeLoad}
            onReload={browser.onReload}
          />
        )}
      </div>
    </div>
  );
}

function TabButton({ active, icon, label, onClick, disabled, title }: {
  active: boolean;
  icon: React.ReactNode;
  label: string;
  onClick: () => void;
  disabled?: boolean;
  title?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title ?? label}
      aria-pressed={active}
      className={cn(
        "flex min-w-0 items-center gap-1.5 rounded-md px-2 py-1.5 text-xs transition-colors duration-150",
        "max-[480px]:px-1.5",
        active
          ? "bg-accent text-accent-foreground"
          : "text-muted-foreground hover:bg-accent/60 hover:text-accent-foreground",
        "disabled:pointer-events-none disabled:opacity-50",
      )}
    >
      {icon}
      <span className="truncate">{label}</span>
    </button>
  );
}
