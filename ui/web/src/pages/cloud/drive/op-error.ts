import { ApiError } from "@/api/errors";
import { toast } from "@/stores/use-toast-store";

/** Map a file-op failure to a toast: the 403 scope error gets the re-grant
 * hint, everything else a generic failure with the server message. */
export function opErrorToast(e: unknown, t: (key: string) => string, title?: string) {
  if (e instanceof ApiError && e.code === "cloud_write_scope_required") {
    toast.error(title ?? t("files.op_failed"), t("files.scope_required"));
    return;
  }
  const message = e instanceof Error ? e.message : String(e);
  toast.error(title ?? t("files.op_failed"), message);
}
