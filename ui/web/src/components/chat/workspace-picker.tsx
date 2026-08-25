import { useState, useRef, useEffect, useLayoutEffect } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { Check, ChevronDown, FolderOpen } from "lucide-react";
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
  const { workspaces, loading, refresh } = useWorkspaces();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const [dropdownStyle, setDropdownStyle] = useState<React.CSSProperties>({});

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
        </div>,
        document.body,
      )}
    </div>
  );
}
