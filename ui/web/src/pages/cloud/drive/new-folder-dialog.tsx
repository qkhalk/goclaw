import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { toast } from "@/stores/use-toast-store";
import { useCloudFileOps } from "../hooks/use-cloud";
import { childPath } from "./paths";
import { opErrorToast } from "./op-error";

/** "New folder" dialog — creates a folder under the current ?path=. */
export function NewFolderDialog({
  open,
  onOpenChange,
  accountId,
  parent,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  accountId: string;
  parent: string;
}) {
  const { t } = useTranslation("cloud");
  const { t: tc } = useTranslation("common");
  const ops = useCloudFileOps(accountId);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

  async function handleCreate() {
    const trimmed = name.trim();
    if (!trimmed) return;
    setBusy(true);
    try {
      await ops.mkdir(childPath(parent, trimmed));
      toast.success(t("files.done"));
      setName("");
      onOpenChange(false);
    } catch (e) {
      opErrorToast(e, t);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) setName("");
        onOpenChange(o);
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("files.new_folder")}</DialogTitle>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void handleCreate();
          }}
        >
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("files.folder_name_placeholder")}
            autoFocus
            autoComplete="off"
            className="text-base md:text-sm"
          />
          <DialogFooter className="mt-4">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
              {tc("cancel")}
            </Button>
            <Button type="submit" disabled={busy || !name.trim()}>
              {busy ? "…" : t("files.create")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
