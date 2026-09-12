import { useState, useRef, useEffect, useLayoutEffect } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { Check, ChevronDown, ChevronRight, Folder, FolderOpen, FolderPlus, ListTree, PanelsTopLeft, SquareTerminal } from "lucide-react";
import { Methods } from "@/api/protocol";
import { usePortalDropdownClose } from "@/hooks/use-portal-dropdown-close";
import { useWs } from "@/hooks/use-ws";
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
  const ws = useWs();
  const { workspaces, loading, refresh, create } = useWorkspaces();
  const [open, setOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [newPath, setNewPath] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  // Directory suggestions for the custom-path input (workspace.suggestDirs).
  const [dirSuggestions, setDirSuggestions] = useState<string[]>([]);
  const [activeSug, setActiveSug] = useState(-1);
  const containerRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const [dropdownStyle, setDropdownStyle] = useState<React.CSSProperties>({});

  // Debounced suggestion fetch while the path field is visible. An empty
  // query lists the server-side workspace root, so focusing the field
  // already offers starting points instead of typing blind.
  useEffect(() => {
    if (!showAdvanced) {
      setDirSuggestions([]);
      setActiveSug(-1);
      return;
    }
    const handle = setTimeout(async () => {
      try {
        const res = await ws.call<{ directories: string[] }>(
          Methods.WORKSPACE_SUGGEST_DIRS,
          { query: newPath },
        );
        setDirSuggestions(res.directories ?? []);
        setActiveSug(-1);
      } catch {
        setDirSuggestions([]);
      }
    }, 200);
    return () => clearTimeout(handle);
  }, [newPath, showAdvanced, ws]);

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
    setNewPath("");
    setShowAdvanced(false);
    setCreateError(null);
  };

  const handleCreate = async () => {
    const name = newName.trim();
    const path = newPath.trim();
    if (!name || submitting) return;
    setSubmitting(true);
    setCreateError(null);
    try {
      const ws = await create(name, path || undefined);
      if (ws) pickWorkspace(ws.id);
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
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
                subtext={ws.rootPath}
              />
            ))
          )}
          {creating ? (
            <div className="px-2 py-1.5">
              {/* Name input — primary field */}
              <input
                autoFocus
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && newName.trim()) handleCreate();
                  if (e.key === "Escape") setCreating(false);
                }}
                maxLength={80}
                placeholder={t("workspacePicker.createNamePlaceholder")}
                className="w-full rounded-md border bg-background px-2 py-1.5 text-base outline-none focus:ring-1 focus:ring-ring md:text-sm"
              />

              {/* Location preview */}
              <p className="mt-1.5 truncate text-2xs text-muted-foreground" title={newPath.trim() || undefined}>
                {newPath.trim()
                  ? t("workspacePicker.locationCustom", { path: newPath.trim() })
                  : t("workspacePicker.locationAuto")}
              </p>

              {/* Advanced: path toggle */}
              <button
                type="button"
                onClick={() => setShowAdvanced(!showAdvanced)}
                className="mt-1 flex items-center gap-1 text-2xs text-muted-foreground hover:text-foreground"
              >
                {showAdvanced
                  ? <ChevronDown className="h-3 w-3" />
                  : <ChevronRight className="h-3 w-3" />}
                {t("workspacePicker.advancedPath")}
              </button>
              {showAdvanced && (
                <>
                  <input
                    value={newPath}
                    onChange={(e) => setNewPath(e.target.value)}
                    onKeyDown={(e) => {
                      if (dirSuggestions.length > 0) {
                        if (e.key === "ArrowDown") {
                          e.preventDefault();
                          setActiveSug((i) => Math.min(i + 1, dirSuggestions.length - 1));
                          return;
                        }
                        if (e.key === "ArrowUp") {
                          e.preventDefault();
                          setActiveSug((i) => Math.max(i - 1, -1));
                          return;
                        }
                        if ((e.key === "Enter" || e.key === "Tab") && activeSug >= 0) {
                          e.preventDefault();
                          const picked = dirSuggestions[activeSug];
                          if (picked) setNewPath(picked);
                          return;
                        }
                        if (e.key === "Escape") {
                          // First Escape dismisses the suggestion list only;
                          // stop propagation so the menu itself stays open.
                          e.stopPropagation();
                          e.preventDefault();
                          setDirSuggestions([]);
                          return;
                        }
                      }
                      if (e.key === "Enter" && newName.trim()) handleCreate();
                      if (e.key === "Escape") setCreating(false);
                    }}
                    maxLength={512}
                    spellCheck={false}
                    autoCapitalize="off"
                    autoCorrect="off"
                    placeholder={t("workspacePicker.pathPlaceholder")}
                    title={t("workspacePicker.pathHint")}
                    className="mt-1 w-full rounded-md border bg-background px-2 py-1 text-base outline-none focus:ring-1 focus:ring-ring md:text-sm"
                  />
                  {dirSuggestions.length > 0 && (
                    <div
                      role="listbox"
                      aria-label={t("workspacePicker.advancedPath")}
                      className="mt-1 max-h-48 overflow-y-auto overscroll-contain rounded-md border bg-popover"
                    >
                      {dirSuggestions.map((d, i) => (
                        <button
                          key={d}
                          type="button"
                          role="option"
                          aria-selected={i === activeSug}
                          // mousedown + preventDefault keeps input focus.
                          onMouseDown={(e) => {
                            e.preventDefault();
                            setNewPath(d);
                          }}
                          onMouseEnter={() => setActiveSug(i)}
                          className={`flex w-full items-center gap-1.5 px-2 py-1 text-left text-xs ${
                            i === activeSug ? "bg-accent text-accent-foreground" : ""
                          }`}
                        >
                          <Folder className="h-3 w-3 shrink-0 text-muted-foreground" />
                          <span className="truncate" title={d}>{d}</span>
                        </button>
                      ))}
                    </div>
                  )}
                </>
              )}

              {createError && (
                <p className="mt-1 text-xs text-destructive">
                  {t("workspacePicker.createFailed", { message: createError })}
                </p>
              )}

              {/* Actions */}
              <div className="mt-2 flex items-center justify-end gap-1.5">
                <button
                  type="button"
                  onClick={() => setCreating(false)}
                  className="rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent"
                >
                  {t("workspacePicker.createCancel")}
                </button>
                <button
                  type="button"
                  onClick={handleCreate}
                  disabled={!newName.trim() || submitting}
                  className="rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                >
                  {submitting ? "…" : t("workspacePicker.createConfirm")}
                </button>
              </div>
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
  subtext,
  disabled,
  title,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  label: string;
  /** Secondary line (workspace root path) — also used as the row tooltip. */
  subtext?: string;
  disabled?: boolean;
  title?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title ?? subtext}
      className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm disabled:pointer-events-none disabled:opacity-50 ${
        active ? "bg-accent text-accent-foreground" : "hover:bg-accent/50"
      }`}
    >
      {icon}
      <span className="min-w-0 flex-1">
        <span className="block truncate">{label}</span>
        {subtext && (
          <span className="block truncate text-2xs font-normal text-muted-foreground">{subtext}</span>
        )}
      </span>
      {active && <Check className="h-3.5 w-3.5 shrink-0" />}
    </button>
  );
}
