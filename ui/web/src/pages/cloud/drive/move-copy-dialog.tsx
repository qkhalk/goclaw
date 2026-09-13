import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@/stores/use-toast-store";
import { useCloudFileOps } from "../hooks/use-cloud";
import { childPath, parentPath, pathDisplayName } from "./paths";
import { opErrorToast } from "./op-error";
import { FolderPickerDialog } from "./folder-picker-dialog";

/** Move/copy one or more entries into a folder chosen with the picker.
 * Sources are full remote paths (encoded form) within the same account.
 * Entries already living in the destination folder are skipped. */
export function MoveCopyDialog({
  open,
  onOpenChange,
  accountId,
  mode,
  sources,
  initialPath,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  accountId: string;
  mode: "move" | "copy";
  /** Full remote paths to move/copy. */
  sources: string[];
  initialPath: string;
}) {
  const { t } = useTranslation("cloud");
  const ops = useCloudFileOps(accountId);
  const [busy, setBusy] = useState(false);

  async function handlePick(dest: string) {
    setBusy(true);
    try {
      let moved = 0;
      for (const src of sources) {
        if (parentPath(src) === dest) continue;
        const target = childPath(dest, pathDisplayName(src));
        if (mode === "move") await ops.move(src, target);
        else await ops.copy(src, target);
        moved++;
      }
      toast.success(t("files.done"), t("files.selected_count", { count: moved }));
      onOpenChange(false);
    } catch (e) {
      opErrorToast(e, t);
    } finally {
      setBusy(false);
    }
  }

  return (
    <FolderPickerDialog
      open={open}
      onOpenChange={onOpenChange}
      accountId={accountId}
      title={mode === "move" ? t("files.move_title") : t("files.copy_title")}
      confirmLabel={busy ? "…" : mode === "move" ? t("files.move") : t("files.copy")}
      initialPath={initialPath}
      onPick={(p) => void handlePick(p)}
    />
  );
}
