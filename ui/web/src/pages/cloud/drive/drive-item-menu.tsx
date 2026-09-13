import { Download, FolderOpen, Copy, ArrowRight, Pencil, Trash2, type LucideIcon } from "lucide-react";
import type { CloudFileEntry } from "../hooks/use-cloud";

/** One action in the item ⋮ menu / right-click context menu. Both menus render
 * the same list (built by buildItemActions) so they never drift. */
export interface ItemAction {
  key: string;
  label: string;
  icon: LucideIcon;
  destructive?: boolean;
  onSelect: () => void;
}

export interface ItemActionHandlers {
  onOpen?: () => void;
  onDownload?: () => void;
  onRename?: () => void;
  onMove?: () => void;
  onCopy?: () => void;
  onDelete?: () => void;
}

/** Build the shared action list for one entry. Write actions are omitted when
 * canWrite=false; read actions always render when a handler exists. */
export function buildItemActions(
  entry: Pick<CloudFileEntry, "is_dir">,
  canWrite: boolean,
  handlers: ItemActionHandlers,
  t: (key: string) => string,
): ItemAction[] {
  const actions: ItemAction[] = [];
  if (entry.is_dir && handlers.onOpen) {
    actions.push({ key: "open", label: t("files.open"), icon: FolderOpen, onSelect: handlers.onOpen });
  }
  if (!entry.is_dir && handlers.onDownload) {
    actions.push({ key: "download", label: t("files.download"), icon: Download, onSelect: handlers.onDownload });
  }
  if (canWrite) {
    if (handlers.onRename) {
      actions.push({ key: "rename", label: t("files.rename"), icon: Pencil, onSelect: handlers.onRename });
    }
    if (handlers.onMove) {
      actions.push({ key: "move", label: t("files.move"), icon: ArrowRight, onSelect: handlers.onMove });
    }
    if (handlers.onCopy) {
      actions.push({ key: "copy", label: t("files.copy"), icon: Copy, onSelect: handlers.onCopy });
    }
    if (handlers.onDelete) {
      actions.push({
        key: "delete",
        label: t("files.delete"),
        icon: Trash2,
        destructive: true,
        onSelect: handlers.onDelete,
      });
    }
  }
  return actions;
}
