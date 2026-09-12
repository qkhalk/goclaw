import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Methods } from "@/api/protocol";
import { queryKeys } from "@/lib/query-keys";
import { useAuthStore } from "@/stores/use-auth-store";
import { useWs } from "@/hooks/use-ws";
import type { SkillInfo } from "@/types/skill";

interface CommandPaletteProps {
  open: boolean;
  /** Text after the leading "/" in the composer draft */
  query: string;
  onSelect: (token: string) => void;
  onClose: () => void;
}

/**
 * Returns the query after "/" when the whole draft is a single slash token
 * (optionally empty), else null. Shared with the composer's open condition.
 */
export function slashTokenQuery(value: string): string | null {
  const m = /^\/([a-zA-Z0-9:_-]*)$/.exec(value);
  return m ? (m[1] ?? "") : null;
}

interface PaletteItem {
  /** Slash token without the leading "/", e.g. "gc:plan" */
  token: string;
  label: string;
  description: string;
  /** Skill items have no badge; /gc: items carry their kind */
  badge?: string;
  name?: string;
}

/**
 * Prompt-style workflow commands only. Control-plane commands (status, runs,
 * doctor, approve) were removed — they have dedicated pages (/runs, /approvals)
 * and the palette only inserts text; it cannot execute anything.
 */
const GC_COMMANDS: Array<{ kind: string }> = [
  { kind: "plan" },
  { kind: "fix" },
  { kind: "cook" },
  { kind: "review" },
  { kind: "test" },
  { kind: "debug" },
  { kind: "docs" },
  { kind: "architect" },
  { kind: "uiux" },
  { kind: "mission" },
];

function buildItems(skills: SkillInfo[], describe: (kind: string) => string): PaletteItem[] {
  const commands: PaletteItem[] = GC_COMMANDS.map(({ kind }) => ({
    token: `gc:${kind}`,
    label: `/gc:${kind}`,
    description: describe(kind),
    badge: kind,
  }));
  const skillItems: PaletteItem[] = skills
    .filter((s) => s.enabled !== false && !(s.missing_deps && s.missing_deps.length > 0) && !!s.slug)
    .map((s) => ({
      token: s.slug as string,
      label: `/${s.slug}`,
      description: s.description,
      name: s.name,
    }));
  return [...commands, ...skillItems];
}

function filterItems(items: PaletteItem[], query: string): PaletteItem[] {
  const q = query.trim().toLowerCase();
  if (!q) return items;
  return items.filter(
    (it) =>
      it.token.toLowerCase().startsWith(q) ||
      (it.name ?? "").toLowerCase().includes(q) ||
      it.description.toLowerCase().includes(q),
  );
}

export function CommandPalette({ open, query, onSelect, onClose }: CommandPaletteProps) {
  const { t } = useTranslation("chat");
  const ws = useWs();
  const connected = useAuthStore((s) => s.connected);
  const [activeIndex, setActiveIndex] = useState(0);
  const activeItemRef = useRef<HTMLButtonElement>(null);

  // Shares the queryKey/cache with the Skills page hook (staleTime 60s), so
  // opening the palette usually costs nothing after the first fetch.
  const { data: skills = [] } = useQuery({
    queryKey: queryKeys.skills.all,
    queryFn: async () => {
      const res = await ws.call<{ skills: SkillInfo[] }>(Methods.SKILLS_LIST);
      return res.skills ?? [];
    },
    staleTime: 60_000,
    enabled: connected && open,
  });

  const items = useMemo(
    () => filterItems(buildItems(skills, (kind) => t(`command.${kind}`)), query),
    [skills, query, t],
  );
  const clampedIndex = Math.min(activeIndex, Math.max(items.length - 1, 0));

  useEffect(() => {
    setActiveIndex(0);
  }, [open, query]);

  useEffect(() => {
    activeItemRef.current?.scrollIntoView({ block: "nearest" });
  }, [clampedIndex]);

  const selectItem = useCallback(
    (item: PaletteItem) => {
      onSelect(item.token);
      onClose();
    },
    [onSelect, onClose],
  );

  // Keyboard lives here on a window capture listener so the composer's
  // Enter-to-send branch never sees keystrokes meant for the palette.
  // IME composition is left untouched (isComposing / keyCode 229).
  useEffect(() => {
    if (!open) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.isComposing || e.keyCode === 229) return;
      if (items.length === 0) {
        if (e.key === "Escape") {
          e.preventDefault();
          e.stopPropagation();
          onClose();
        }
        return;
      }
      const current = Math.min(activeIndex, items.length - 1);
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          e.stopPropagation();
          setActiveIndex((current + 1) % items.length);
          break;
        case "ArrowUp":
          e.preventDefault();
          e.stopPropagation();
          setActiveIndex((current - 1 + items.length) % items.length);
          break;
        case "Enter": {
          e.preventDefault();
          e.stopPropagation();
          const item = items[current];
          if (item) selectItem(item);
          break;
        }
        case "Escape":
          e.preventDefault();
          e.stopPropagation();
          onClose();
          break;
      }
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => window.removeEventListener("keydown", onKeyDown, true);
  }, [open, items, activeIndex, selectItem, onClose]);

  if (!open) return null;

  const indexed = items.map((item, index) => ({ item, index }));
  const commandRows = indexed.filter(({ item }) => item.badge !== undefined);
  const skillRows = indexed.filter(({ item }) => item.badge === undefined);

  const renderItem = ({ item, index }: { item: PaletteItem; index: number }) => {
    const active = index === clampedIndex;
    return (
      <button
        key={item.token}
        ref={active ? activeItemRef : undefined}
        type="button"
        role="option"
        aria-selected={active}
        title={item.description}
        // Keep focus in the composer textarea when clicking an item.
        onMouseDown={(e) => e.preventDefault()}
        onMouseEnter={() => setActiveIndex(index)}
        onClick={() => selectItem(item)}
        className={`flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors ${
          active ? "bg-accent text-accent-foreground" : "hover:bg-accent/50"
        }`}
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate font-medium">{item.label}</span>
          <span className="block truncate text-xs text-muted-foreground">{item.description}</span>
        </span>
        {item.badge && (
          <Badge variant="secondary" className="mt-0.5 shrink-0 text-2xs">
            {item.badge}
          </Badge>
        )}
      </button>
    );
  };

  return (
    <div
      role="listbox"
      aria-label={t("palette.commands")}
      className="absolute bottom-full left-0 right-0 z-50 mb-2 max-h-[320px] overflow-y-auto overscroll-contain rounded-lg border bg-popover p-1 text-popover-foreground shadow-md animate-in fade-in-0 zoom-in-95"
    >
      {items.length === 0 ? (
        <div className="px-3 py-6 text-center text-sm text-muted-foreground">
          {t("palette.noResults")}
        </div>
      ) : (
        <>
          {commandRows.length > 0 && (
            <>
              <div className="flex items-baseline justify-between gap-2 px-2 pb-0.5 pt-1">
                <span className="text-2xs font-semibold uppercase tracking-wider text-muted-foreground">
                  {t("palette.commands")}
                </span>
                <span className="truncate text-2xs text-muted-foreground/70">
                  {t("palette.gcHint")}
                </span>
              </div>
              {commandRows.map(renderItem)}
            </>
          )}
          {skillRows.length > 0 && (
            <>
              <div className="px-2 pb-0.5 pt-1 text-2xs font-semibold uppercase tracking-wider text-muted-foreground">
                {t("palette.skills")}
              </div>
              {skillRows.map(renderItem)}
            </>
          )}
        </>
      )}
    </div>
  );
}
