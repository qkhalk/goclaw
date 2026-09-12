import { useState, useRef, useLayoutEffect } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { Gauge, LineChart } from "lucide-react";
import { usePortalDropdownClose } from "@/hooks/use-portal-dropdown-close";
import type { SessionInfo } from "@/types/session";

/**
 * Paseo-style context meter: a live "used/max (n%)" badge with a detail
 * popover (compaction count, last compaction time, link to the session
 * detail page). Data ships in sessions.list as SessionInfoRich fields
 * (estimatedTokens / contextWindow / compactionCount); hidden only when the
 * session has no known context window.
 */
export function ContextMeter({ session }: { session: SessionInfo }) {
  const { t } = useTranslation("chat");
  const navigate = useNavigate();
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
      right: window.innerWidth - rect.right,
      width: 240,
      zIndex: 9998,
    });
  }, [open]);

  usePortalDropdownClose({
    open,
    onClose: () => setOpen(false),
    ignore: [containerRef, dropdownRef],
  });

  if (!session.contextWindow || session.contextWindow <= 0) return null;
  const used = session.estimatedTokens ?? 0;
  const max = session.contextWindow;
  const percent = Math.min(100, Math.round((used / max) * 100));
  const color =
    percent >= 90 ? "text-destructive" : percent >= 75 ? "text-amber-600 dark:text-amber-400" : "text-muted-foreground";

  const lastCompaction = (() => {
    const raw = session.metadata?.last_compaction_at;
    if (!raw) return null;
    const d = new Date(raw);
    return isNaN(d.getTime()) ? null : d;
  })();

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        title={t("contextUsage.tooltip", {
          used: used.toLocaleString(),
          max: max.toLocaleString(),
          percent,
          compactions: session.compactionCount ?? 0,
          lastCompact: lastCompaction ? lastCompaction.toLocaleString() : t("contextUsage.never"),
        })}
        className={`flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-[11px] hover:bg-accent ${color} max-sm:px-1.5`}
      >
        <Gauge className="h-3 w-3 shrink-0 max-sm:hidden" />
        <span className="font-mono max-sm:hidden">
          {used.toLocaleString()}/{max.toLocaleString()}
        </span>
        <span className="font-mono sm:hidden">{percent}%</span>
        <span className="opacity-70 max-sm:hidden">({percent}%)</span>
      </button>

      {open && createPortal(
        <div
          ref={dropdownRef}
          style={dropdownStyle}
          className="pointer-events-auto rounded-lg border bg-popover p-3 shadow-md"
        >
          <div className="flex items-center gap-2 text-sm">
            <LineChart className={`h-4 w-4 shrink-0 ${color}`} />
            <span className={`font-mono ${color}`}>
              {used.toLocaleString()} / {max.toLocaleString()}
            </span>
            <span className={`ml-auto font-mono text-xs ${color}`}>{percent}%</span>
          </div>
          <div className="mt-2 text-xs leading-relaxed text-muted-foreground">
            {t("contextUsage.tooltip", {
              used: used.toLocaleString(),
              max: max.toLocaleString(),
              percent,
              compactions: session.compactionCount ?? 0,
              lastCompact: lastCompaction ? lastCompaction.toLocaleString() : t("contextUsage.never"),
            })}
          </div>
          <button
            type="button"
            onClick={() => navigate(`/sessions/${encodeURIComponent(session.key)}`)}
            className="mt-3 flex min-h-[44px] w-full items-center justify-center rounded-md border text-xs font-medium hover:bg-accent hover:text-accent-foreground sm:min-h-0 sm:py-1.5"
          >
            {t("contextUsage.linkSessions")}
          </button>
        </div>,
        document.body,
      )}
    </div>
  );
}
