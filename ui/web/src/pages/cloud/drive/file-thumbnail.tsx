import { cn } from "@/lib/utils";
import { FileIcon } from "@/components/shared/file-tree-file-icon";
import { Folder } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { useEffect, useState } from "react";
import type { CloudFileEntry } from "../hooks/use-cloud";
import type { ThumbnailSize } from "../settings-modal";
import { rawPath } from "./paths";

const IMAGE_EXTS = new Set(["png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "ico", "tiff", "tif"]);

/** Thumbnail size cap — mirrors the preview sheet's image cap (25 MB):
 * never auto-download anything bigger just for a thumbnail. */
const THUMB_CAP = 25 << 20;

/** Thumbnail zone height per preview setting — the grid card container and
 * the fallback icons both follow it. */
const SIZE_CONTAINER: Record<ThumbnailSize, string> = {
  small: "h-16",
  medium: "h-24",
  large: "h-40",
};
const SIZE_BOX: Record<ThumbnailSize, string> = {
  small: "h-8 w-8",
  medium: "h-12 w-12",
  large: "h-20 w-20",
};
const SIZE_GLYPH: Record<ThumbnailSize, string> = {
  small: "[&>svg]:h-6 [&>svg]:w-6",
  medium: "[&>svg]:h-10 [&>svg]:w-10",
  large: "[&>svg]:h-16 [&>svg]:w-16",
};

/** Container height class for the grid card thumbnail zone. */
export function thumbContainerClass(size: ThumbnailSize): string {
  return SIZE_CONTAINER[size];
}

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
  /** Full remote path (encoded, childPath form). */
  path: string;
  /** Preview setting from the clouds settings modal (default medium). */
  size?: ThumbnailSize;
  className?: string;
}

/**
 * Renders a thumbnail preview for image files fetched via the cloud download
 * endpoint with auth headers. Falls back to the file type icon with an
 * extension badge pinned to the icon's corner.
 *
 * Uses react-query to cache blob URLs per (accountId, path) so repeated
 * renders don't re-fetch.
 */
export function FileThumbnail({ entry, accountId, path, size = "medium", className }: FileThumbnailProps) {
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
      // The API expects the raw remote path — decode the childPath form and
      // let URLSearchParams do the single percent-encode.
      .fetchBlob("/v1/cloud/accounts/" + accountId + "/files/download", { path: rawPath(path) })
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
        strokeWidth={1.5}
        className={cn("shrink-0 fill-sky-100 text-sky-500 dark:fill-sky-950", SIZE_BOX[size], className)}
      />
    );
  }

  // Image with blob URL loaded: show thumbnail with extension badge overlay
  // (fall back to the file icon if the load fails, e.g. a dead cached URL or
  // a provider error)
  if (blobUrl && !failed) {
    const ext = extOf(entry.name).toUpperCase();
    return (
      <span className={cn("relative flex h-full w-full", className)}>
        <img
          src={blobUrl}
          alt={entry.name}
          className="h-full w-full rounded-md object-cover"
          draggable={false}
          onError={() => setFailed(true)}
        />
        {ext && (
          <span className="absolute -bottom-1.5 -right-2 rounded-sm bg-muted px-1 py-px text-[9px] font-medium leading-none text-muted-foreground">
            {ext}
          </span>
        )}
      </span>
    );
  }

  // Fallback: file icon with extension badge pinned to the icon corner
  // (kept close to the glyph, not the card edge, so it reads as a file-type
  // label even when the thumbnail zone is wide).
  const ext = extOf(entry.name).toUpperCase();
  return (
    <span className={cn("flex shrink-0 items-center justify-center", className)}>
      <span className={cn("relative flex items-center justify-center", SIZE_BOX[size], SIZE_GLYPH[size])}>
        <FileIcon name={entry.name} />
        {ext && (
          <span className="absolute -bottom-1.5 -right-2 rounded-sm bg-muted px-1 py-px text-[9px] font-medium leading-none text-muted-foreground">
            {ext}
          </span>
        )}
      </span>
    </span>
  );
}
