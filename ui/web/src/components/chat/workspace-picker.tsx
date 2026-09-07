import { useState, useRef, useEffect, useLayoutEffect } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { Check, ChevronDown, FolderOpen, FolderPlus, Loader2, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { usePortalDropdownClose } from "@/hooks/use-portal-dropdown-close";
import { useWorkspaces } from "@/hooks/use-workspaces";

interface WorkspacePickerProps {
  /** Selected workspace id, or null for no workspace. */
  value: string | null;
  onChange: (id: string | null) => void;
  className?: string;
}

export function WorkspacePicker({ value, onChange, className }: WorkspacePickerProps) {
  const { t } = useTranslation("chat");
  const { workspaces, loading, refresh, create } = useWorkspaces();
  const selected = workspaces.find((ws) => ws.id === value);
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const [dropdownStyle, setDropdownStyle] = useState<React.CSSProperties>({});
  // Inline create form state: without this the picker is a dead end when no
  // workspace exists (terminal/files panels need one).
  const [creating, setCreating] = useState(false);
  const [nameValue, setNameValue] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const nameInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (creating) nameInputRef.current?.focus();
  }, [creating]);

  const submitCreate = async () => {
    const name = nameValue.trim();
    if (!name || submitting) return;
    setSubmitting(true);
    setCreateError(null);
    try {
      const ws = await create(name);
      if (ws) onChange(ws.id);
      setOpen(false);
      setCreating(false);
      setNameValue("");
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  };

  useLayoutEffect(() => {
    if (!open || !containerRef.current) return;
    const rect = containerRef.current.getBoundingClientRect();
    setDropdownStyle({
      position: "fixed",
      top: rect.bottom + 4,
      left: rect.left,
      width: Math.max(rect.width, 240),
      zIndex: 9999,
    });
  }, [open]);

  usePortalDropdownClose({
    open,
    onClose: () => setOpen(false),
    ignore: [containerRef, dropdownRef],
  });

  // Load on first open so the picker stays fresh without polling.
  useEffect(() => {
    if (open) refresh();
  }, [open, refresh]);

  return (
    <div ref={containerRef} className={cn("relative", className)}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex min-h-[44px] w-full items-center gap-2 rounded-lg border bg-background px-3 py-2 text-base hover:bg-accent md:text-sm"
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        <FolderOpen className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="flex-1 truncate text-left font-medium">
          {selected?.name ?? t("workspacePicker.none")}
        </span>
        <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
      </button>

      {open && createPortal(
        <div
          ref={dropdownRef}
          style={dropdownStyle}
          role="listbox"
          aria-label={t("workspacePicker.title")}
          className="pointer-events-auto max-h-60 overflow-y-auto rounded-lg border bg-popover p-1 shadow-md sm:max-h-80 overscroll-contain"
        >
          {loading && workspaces.length === 0 && (
            <div className="px-3 py-2 text-sm text-muted-foreground">
              {t("workspacePicker.loading")}
            </div>
          )}
          {!loading && workspaces.length === 0 && (
            <div className="px-3 py-2 text-sm text-muted-foreground">
              {t("workspacePicker.empty")}
            </div>
          )}
          <button
            type="button"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => { onChange(null); setOpen(false); }}
            role="option"
            aria-selected={value === null}
            className={`flex min-h-[44px] w-full items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-accent ${
              value === null ? "bg-accent" : ""
            }`}
          >
            <span className="flex-1 truncate text-left">
              {t("workspacePicker.none")}
            </span>
            {value === null && <Check className="h-4 w-4 shrink-0" />}
          </button>
          {workspaces.map((ws) => {
            return (
              <button
                key={ws.id}
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => { onChange(ws.id); setOpen(false); }}
                role="option"
                aria-selected={ws.id === value}
                className={`flex min-h-[44px] w-full items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-accent ${
                  ws.id === value ? "bg-accent" : ""
                }`}
              >
                <FolderOpen
                  className={cn(
                    "h-4 w-4 shrink-0",
                    ws.status === "archived" ? "text-muted-foreground/50" : "text-muted-foreground",
                  )}
                />
                <span className="flex-1 truncate text-left">{ws.name}</span>
                {ws.status === "archived" && (
                  <span className="text-xs text-muted-foreground">
                    {t("workspacePicker.archived")}
                  </span>
                )}
                {ws.id === value && <Check className="h-4 w-4 shrink-0" />}
              </button>
            );
          })}
          {/* Create workspace inline — prevents the picker dead end when no
              workspace exists (terminal/files panels require one). */}
          <div className="mt-1 border-t pt-1">
            {creating ? (
              <div className="flex items-center gap-1.5 px-1.5 py-1.5">
                <input
                  ref={nameInputRef}
                  value={nameValue}
                  onChange={(e) => setNameValue(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      void submitCreate();
                    } else if (e.key === "Escape") {
                      setCreating(false);
                      setCreateError(null);
                    }
                  }}
                  placeholder={t("workspacePicker.createPlaceholder")}
                  maxLength={80}
                  aria-label={t("workspacePicker.createPlaceholder")}
                  className="h-9 min-w-0 flex-1 rounded-md border bg-background px-2 text-base outline-none focus:ring-1 focus:ring-ring md:text-sm"
                />
                <button
                  type="button"
                  onClick={() => void submitCreate()}
                  disabled={!nameValue.trim() || submitting}
                  aria-label={t("workspacePicker.createConfirm")}
                  className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md hover:bg-accent disabled:pointer-events-none disabled:opacity-50"
                >
                  {submitting ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Check className="h-4 w-4" />
                  )}
                </button>
                <button
                  type="button"
                  onClick={() => { setCreating(false); setCreateError(null); setNameValue(""); }}
                  aria-label={t("workspacePicker.createCancel")}
                  className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md hover:bg-accent"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            ) : (
              <button
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => { setCreating(true); setCreateError(null); }}
                className="flex min-h-[44px] w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              >
                <FolderPlus className="h-4 w-4 shrink-0" />
                <span className="flex-1 truncate text-left">
                  {t("workspacePicker.create")}
                </span>
              </button>
            )}
            {createError && (
              <p className="px-3 pb-1.5 text-xs text-destructive">
                {t("workspacePicker.createFailed", { message: createError })}
              </p>
            )}
          </div>
        </div>,
        document.body,
      )}
    </div>
  );
}
