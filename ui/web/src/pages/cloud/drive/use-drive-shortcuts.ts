import { useEffect } from "react";

export interface DriveShortcutHandlers {
  /** Move the keyboard cursor (±1). */
  onCursorMove: (delta: number) => void;
  /** Activate the entry under the cursor (open folder / preview file). */
  onOpen: () => void;
  /** Go up one folder (Esc/Backspace). */
  onUp: () => void;
  /** Delete the selected entries (or the cursor entry). */
  onDelete: () => void;
  /** Select all visible entries. */
  onSelectAll: () => void;
  /** Focus the search input. */
  onSearch: () => void;
  /** Toggle the shortcuts help dialog. */
  onHelp: () => void;
  /** Clear selection (Esc alternative when something is selected). */
  onClear?: () => void;
}

function isTypingTarget(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  const tag = el.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || el.isContentEditable;
}

/** Drive keyboard shortcuts, window-level with an input/textarea guard:
 * j/k or ↑/↓ move the cursor, Enter opens, Esc/Backspace go up, Del deletes,
 * Ctrl/Cmd+A selects all, "/" focuses search, "?" opens the help dialog. */
export function useDriveShortcuts(handlers: DriveShortcutHandlers, enabled: boolean) {
  useEffect(() => {
    if (!enabled) return;
    function onKey(e: KeyboardEvent) {
      if (isTypingTarget(e.target) || e.altKey) return;
      // A Radix dialog/sheet is open (preview, rail sheet, confirm dialogs):
      // let it own the keyboard — Esc must close it, not navigate folders.
      if (document.querySelector('[role="dialog"]')) return;

      // Ctrl/Cmd+A: select all (the file area also guards its own copy — this
      // one runs first and stops propagation-sensitive duplicates via preventDefault).
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "a") {
        e.preventDefault();
        handlers.onSelectAll();
        return;
      }
      if (e.ctrlKey || e.metaKey) return; // let browser shortcuts through

      switch (e.key) {
        case "j":
        case "ArrowDown":
          e.preventDefault();
          handlers.onCursorMove(1);
          break;
        case "k":
        case "ArrowUp":
          e.preventDefault();
          handlers.onCursorMove(-1);
          break;
        case "Enter":
          handlers.onOpen();
          break;
        case "Backspace":
          e.preventDefault();
          handlers.onUp();
          break;
        case "Escape":
          if (handlers.onClear) handlers.onClear();
          else handlers.onUp();
          break;
        case "Delete":
          handlers.onDelete();
          break;
        case "/":
          e.preventDefault();
          handlers.onSearch();
          break;
        case "?":
          e.preventDefault();
          handlers.onHelp();
          break;
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // Handlers are re-created every render on purpose: they close over the
    // current entries/selection. The effect re-subscribes each time — cheap.
  });
}
