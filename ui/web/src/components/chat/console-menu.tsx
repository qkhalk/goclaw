import { useState, useRef, useLayoutEffect } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { Check, FolderOpen, FolderPlus, ListTree, PanelsTopLeft, SquareTerminal } from "lucide-react";
import { usePortalDropdownClose } from "@/hooks/use-portal-dropdown-close";
import { useWorkspaces } from "@/hooks/use-workspaces";

interface ConsoleMenuProps {
  workspaceId: string | null;
  onWorkspaceChange: (id: string | null) => void;
  filesPanelOpen: boolean;
  jobsPanelOpen: boolean;
  termPanelOpen: boolean;
  onToggleFiles: () => void;
  onToggleJobsTasks: () => void;
  onToggleTerminal: () => void;
}

/**
 * Single top-bar entry point for the chat console (Paseo Phase 3/4 panels):
 * workspace selection + Files / Jobs & Tasks / Terminal toggles, collapsed
 * behind one button so the bar only carries conversation-relevant chrome.
 * Self-contained workspace list (no nested dropdown inside the menu).
 */
export function ConsoleMenu({
  workspaceId,
  onWorkspaceChange,
  filesPanelOpen,
  jobsPanelOpen,
  termPanelOpen,
  onToggleFiles,
  onToggleJobsTasks,
  onToggleTerminal,
}: ConsoleMenuProps) {
  const { t } = useTranslation("chat");
  const { workspaces, loading, refresh, create } = useWorkspaces();
  const [open, setOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const [dropdownStyle, setDropdownStyle] = useState<React.CSSProperties>({});

  useLayoutEffect(() => {
    if (!open || !containerRef.current) return;
    const rect = containerRef.current.getBoundingClientRect();
    setDropdownStyle({
      position: "fixed",
      top: rect.bottom + 4,
      right: window.innerWidth - rect.right,
      width: 272,
      zIndex: 9998,
    });
  }, [open]);

  usePortalDropdownClose({
    open,
    onClose: () => setOpen(false),
    ignore: [containerRef, dropdownRef],
  });

  const anyPanelOpen = filesPanelOpen || jobsPanelOpen || termPanelOpen;

  const pickWorkspace = (id: string | null) => {
    onWorkspaceChange(id);
    setCreating(false);
    setNewName("");
  };

  const handleCreate = async () => {
    const name = newName.trim();
    if (!name) return;
    try {
      const ws = await create(name);
      if (ws) pickWorkspace(ws.id);
    } catch (err) {
      console.error("[ConsoleMenu] create workspace failed:", err);
    }
  };

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => {
          setOpen(!open);
          if (!open) refresh();
        }}
        aria-label={t("console.button")}
        aria-expanded={open}
        title={t("console.button")}
        className={`rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground ${anyPanelOpen ? "bg-accent text-accent-foreground" : ""}`}
      >
        <PanelsTopLeft className="h-4 w-4" />
      </button>

      {open && createPortal(
        <div
          ref={dropdownRef}
          style={dropdownStyle}
          className="pointer-events-auto max-h-[70vh] overflow-y-auto overscroll-contain rounded-lg border bg-popover p-1 shadow-md"
        >
          {/* Workspace section */}
          <div className="px-2 pb-0.5 pt-1.5 text-2xs font-semibold uppercase tracking-wider text-muted-foreground">
            {t("workspacePicker.title")}
          </div>
          <MenuRow
            active={!workspaceId}
            onClick={() => pickWorkspace(null)}
            icon={<FolderOpen className="h-4 w-4 shrink-0 opacity-0" />}
            label={t("workspacePicker.none")}
          />
          {loading ? (
            <div className="px-3 py-2 text-xs text-muted-foreground">{t("workspacePicker.loading")}</div>
          ) : (
            workspaces.map((ws) => (
              <MenuRow
                key={ws.id}
                active={ws.id === workspaceId}
                onClick={() => pickWorkspace(ws.id)}
                icon={<FolderOpen className="h-4 w-4 shrink-0" />}
                label={ws.name}
              />
            ))
          )}
          {creating ? (
            <div className="flex items-center gap-1 px-2 py-1.5">
              <input
                autoFocus
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleCreate();
                  if (e.key === "Escape") setCreating(false);
                }}
                placeholder={t("workspacePicker.createPlaceholder")}
                className="min-w-0 flex-1 rounded-md border bg-background px-2 py-1 text-sm outline-none focus:ring-1 focus:ring-ring"
              />
              <button
                type="button"
                onClick={handleCreate}
                className="rounded-md px-2 py-1 text-xs font-medium text-primary hover:bg-accent"
              >
                {t("workspacePicker.createConfirm")}
              </button>
            </div>
          ) : (
            <MenuRow
              active={false}
              onClick={() => setCreating(true)}
              icon={<FolderPlus className="h-4 w-4 shrink-0" />}
              label={t("workspacePicker.create")}
            />
          )}

          <div className="my-1 border-t" />

          {/* Panel toggles */}
          <MenuRow
            active={filesPanelOpen}
            onClick={() => { onToggleFiles(); setOpen(false); }}
            icon={<FolderOpen className="h-4 w-4 shrink-0" />}
            label={t("fileExplorer.title")}
          />
          <MenuRow
            active={jobsPanelOpen}
            onClick={() => { onToggleJobsTasks(); setOpen(false); }}
            icon={<ListTree className="h-4 w-4 shrink-0" />}
            label={t("jobsPanel.title")}
          />
          <MenuRow
            active={termPanelOpen}
            disabled={!workspaceId}
            title={!workspaceId ? t("terminal.noWorkspace") : undefined}
            onClick={() => { onToggleTerminal(); setOpen(false); }}
            icon={<SquareTerminal className="h-4 w-4 shrink-0" />}
            label={t("terminal.title")}
          />
        </div>,
        document.body,
      )}
    </div>
  );
}

function MenuRow({
  active,
  onClick,
  icon,
  label,
  disabled,
  title,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  label: string;
  disabled?: boolean;
  title?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm disabled:pointer-events-none disabled:opacity-50 ${
        active ? "bg-accent text-accent-foreground" : "hover:bg-accent/50"
      }`}
    >
      {icon}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {active && <Check className="h-3.5 w-3.5 shrink-0" />}
    </button>
  );
}
