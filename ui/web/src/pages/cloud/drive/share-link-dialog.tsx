import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, Loader2, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useClipboard } from "@/hooks/use-clipboard";
import { useHttp } from "@/hooks/use-ws";
import { toast } from "@/stores/use-toast-store";
import { opErrorToast } from "./op-error";

/** "Sao chép liên kết": POST files/publiclink (Phase 4) for one path, show the
 * URL with a copy button and the public-access warning. Provider gaps (e.g.
 * OneDrive's rc backend) surface the raw error as a toast. */
export function ShareLinkDialog({
  open,
  onOpenChange,
  accountId,
  path,
  name,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  accountId: string;
  /** Full encoded remote path of the item to share. */
  path: string;
  name: string;
}) {
  const { t } = useTranslation("cloud");
  const { t: tc } = useTranslation("common");
  const http = useHttp();
  const { copied, copy } = useClipboard();
  const [url, setUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  // Fetch (or re-fetch) the link each time the dialog opens.
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    setUrl(null);
    http
      .post<{ url: string }>(`/v1/cloud/accounts/${accountId}/files/publiclink`, { path })
      .then((res) => {
        if (!cancelled) setUrl(res.url);
      })
      .catch((e) => {
        if (cancelled) return;
        opErrorToast(e, t);
        onOpenChange(false);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, accountId, path]);

  async function handleCopy() {
    if (!url) return;
    await copy(url);
    toast.success(t("share.copied"));
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex min-w-0 items-center gap-2">
            <Link className="h-4 w-4 shrink-0" />
            <span className="min-w-0 truncate">{t("share.title")}</span>
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-3">
          <p className="truncate text-sm text-muted-foreground" title={name}>
            {name}
          </p>
          {loading ? (
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              {t("share.creating")}
            </p>
          ) : url ? (
            <div className="flex items-center gap-2">
              <Input readOnly value={url} className="min-w-0 flex-1 text-base md:text-sm" dir="ltr" onFocus={(e) => e.target.select()} />
              <Button size="sm" className="min-h-11 shrink-0 sm:min-h-8" onClick={() => void handleCopy()}>
                {copied ? t("share.copied") : tc("copy")}
              </Button>
            </div>
          ) : null}
          <p className="flex items-start gap-1.5 text-xs text-amber-600 dark:text-amber-400">
            <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            {t("share.public_warning")}
          </p>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {tc("close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
