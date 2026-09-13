import { Download, FolderOpen, Copy, ArrowRight, ArrowRightLeft, Link, Pencil, Star, Trash2, type LucideIcon } from "lucide-react";
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
  onTransfer?: () => void;
  onDelete?: () => void;
  /** Star/unstar (always available — user-level metadata). */
  onStar?: () => void;
  /** True when the entry is currently starred (labels the toggle). */
  starred?: boolean;
  /** Copy a public share link (write-guarded — owner/admin only). */
  onShare?: () => void;
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
  if (handlers.onStar) {
    actions.push({
      key: "star",
      label: handlers.starred ? t("starred.remove") : t("starred.add"),
      icon: Star,
      onSelect: handlers.onStar,
    });
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
    if (handlers.onTransfer) {
      actions.push({ key: "transfer", label: t("transfer.menu_item"), icon: ArrowRightLeft, onSelect: handlers.onTransfer });
    }
    if (handlers.onShare) {
      actions.push({ key: "share", label: t("share.menu_item"), icon: Link, onSelect: handlers.onShare });
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
