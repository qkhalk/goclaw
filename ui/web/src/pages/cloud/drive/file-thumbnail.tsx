import { cn } from "@/lib/utils";
import { FileIcon } from "@/components/shared/file-tree-file-icon";
import { Folder } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { useEffect, useState } from "react";
import type { CloudFileEntry } from "../hooks/use-cloud";

const IMAGE_EXTS = new Set(["png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "ico", "tiff", "tif"]);

/** Thumbnail size cap — mirrors the preview sheet's image cap (25 MB):
 * never auto-download anything bigger just for a thumbnail. */
const THUMB_CAP = 25 << 20;

function extOf(name: string): string {
  const i = name.lastIndexOf(".");
  return i < 0 ? "" : name.slice(i + 1).toLowerCase();
}

function isImage(name: string): boolean {
  return IMAGE_EXTS.has(extOf(name));
}

interface FileThumbnailProps {
  entry: CloudFileEntry;
  accountId: string;
  /** Full remote path (encoded). */
  path: string;
  className?: string;
}

/**
 * Renders a thumbnail preview for image files fetched via the cloud download
 * endpoint with auth headers. Falls back to the file type icon.
 *
 * Uses react-query to cache blob URLs per (accountId, path) so repeated
 * renders don't re-fetch.
 */
export function FileThumbnail({ entry, accountId, path, className }: FileThumbnailProps) {
  const http = useHttp();
  const queryClient = useQueryClient();
  const [blobUrl, setBlobUrl] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  // Oversized entries skip the fetch entirely — icon fallback immediately.
  const shouldFetch = !entry.is_dir && isImage(entry.name) && entry.size <= THUMB_CAP;

  useEffect(() => {
    if (!shouldFetch) return;

    const queryKey = ["cloud", "thumb", accountId, path];
    const cached = queryClient.getQueryData<string>(queryKey);
    if (cached) {
      setBlobUrl(cached);
      return;
    }

    let cancelled = false;
    http
      .fetchBlob("/v1/cloud/accounts/" + accountId + "/files/download", { path })
      .then((blob) => {
        if (cancelled) return;
        const url = URL.createObjectURL(blob);
        queryClient.setQueryData(queryKey, url);
        setBlobUrl(url);
      })
      .catch(() => {
        // silently fail — fallback to icon
      });
    return () => {
      cancelled = true;
    };
  }, [shouldFetch, accountId, path, http, queryClient]);

  // NOTE: blob URLs live in the react-query cache, which outlives this
  // component — never revoke them on unmount. A revoked URL left in the
  // cache would permanently break the next mount of the same file; the
  // query cache's gcTime releases the blob when the entry expires.

  // Directory: folder icon
  if (entry.is_dir) {
    return (
      <Folder
        className={cn(
          "h-12 w-12 shrink-0 fill-sky-100 text-sky-500 dark:fill-sky-950",
          className,
        )}
      />
    );
  }

  // Image with blob URL loaded: show thumbnail (fall back to the file icon
  // if the load fails, e.g. a dead cached URL or a provider error)
  if (blobUrl && !failed) {
    return (
      <img
        src={blobUrl}
        alt={entry.name}
        className={cn(
          "h-24 w-full rounded-md object-cover",
          className,
        )}
        draggable={false}
        onError={() => setFailed(true)}
      />
    );
  }

  // Fallback: file icon
  return (
    <span
      className={cn(
        "flex h-12 w-12 shrink-0 items-center justify-center [&>svg]:h-10 [&>svg]:w-10",
        className,
      )}
    >
      <FileIcon name={entry.name} />
    </span>
  );
}
