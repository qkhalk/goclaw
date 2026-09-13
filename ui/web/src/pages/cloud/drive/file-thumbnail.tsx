import { cn } from "@/lib/utils";
import { FileIcon } from "@/components/shared/file-tree-file-icon";
import { Folder } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { useEffect, useState } from "react";
import type { CloudFileEntry } from "../hooks/use-cloud";

const IMAGE_EXTS = new Set(["png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "ico", "tiff", "tif"]);

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

  const shouldFetch = !entry.is_dir && isImage(entry.name);

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

  // Cleanup blob URLs on unmount
  useEffect(() => {
    return () => {
      if (blobUrl) URL.revokeObjectURL(blobUrl);
    };
  }, [blobUrl]);

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

  // Image with blob URL loaded: show thumbnail
  if (blobUrl) {
    return (
      <img
        src={blobUrl}
        alt={entry.name}
        className={cn(
          "h-24 w-full rounded-md object-cover",
          className,
        )}
        draggable={false}
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
