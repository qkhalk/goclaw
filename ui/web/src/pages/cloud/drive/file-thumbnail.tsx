import { cn } from "@/lib/utils";
import { FileIcon } from "@/components/shared/file-tree-file-icon";
import { Folder } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { useEffect, useState } from "react";
import { queryKeys } from "@/lib/query-keys";
import type { CloudFileEntry } from "../hooks/use-cloud";
import type { ThumbnailSize } from "../settings-modal";
import { rawPath } from "./paths";

const IMAGE_EXTS = new Set(["png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "ico", "tiff", "tif"]);
const VIDEO_EXTS = new Set(["mp4", "webm", "mov", "m4v"]);

/** Thumbnail size cap for blob-backed image previews — mirrors the preview
 * modal's image cap (25 MB): never auto-download anything bigger. */
const THUMB_CAP = 25 << 20;
/** Video thumbnails stream via signed URLs (browser fetches only metadata +
 * a keyframe through HTTP byte-range), so the cap is generous. */
const VIDEO_THUMB_CAP = 500 << 20;

/** Thumbnail size per preview setting — the grid card container and
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

function isVideo(name: string): boolean {
  return VIDEO_EXTS.has(extOf(name));
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
 * Renders a real preview instead of an icon where the provider can serve one:
 * images via the authed download endpoint (blob, react-query cached), videos
 * via a short-lived signed URL the browser streams with byte-range requests
 * (first frame as the poster, no full download). Falls back to the file type
 * icon with an extension badge pinned to the icon's corner.
 */
export function FileThumbnail({ entry, accountId, path, size = "medium", className }: FileThumbnailProps) {
  const http = useHttp();
  const [blobUrl, setBlobUrl] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [videoFailed, setVideoFailed] = useState(false);

  const video = !entry.is_dir && isVideo(entry.name) && entry.size <= VIDEO_THUMB_CAP;
  const image = !entry.is_dir && isImage(entry.name) && entry.size <= THUMB_CAP;

  // Signed streaming URL for video thumbnails.
  const signed = useQuery({
    queryKey: queryKeys.cloud.thumb(accountId, path),
    enabled: video,
    staleTime: 60_000,
    queryFn: async () => {
      const res = await http.post<{ url: string }>(
        `/v1/cloud/accounts/${accountId}/files/sign`,
        { path: rawPath(path) },
      );
      return res.url;
    },
  });

  // Image thumbnails cache the BLOB (not the object URL) in react-query; the
  // object URL is minted per mount and revoked on unmount — caching URLs
  // would leak the underlying Blob for the page's lifetime.
  const blob = useQuery({
    queryKey: ["cloud", "thumb-blob", accountId, path],
    enabled: image,
    staleTime: 5 * 60_000,
    gcTime: 10 * 60_000,
    queryFn: () =>
      http.fetchBlob("/v1/cloud/accounts/" + accountId + "/files/download", { path: rawPath(path) }),
  });

  useEffect(() => {
    if (!blob.data) return;
    const url = URL.createObjectURL(blob.data);
    setBlobUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [blob.data]);

  // Directory: folder icon
  if (entry.is_dir) {
    return (
      <Folder
        strokeWidth={1.5}
        className={cn("shrink-0 fill-sky-100 text-sky-500 dark:fill-sky-950", SIZE_BOX[size], className)}
      />
    );
  }

  // Video: first-frame poster streamed via the signed URL (icon fallback on
  // failure — e.g. expired token or an unsupported codec).
  if (video && signed.data && !signed.isError && !videoFailed) {
    return (
      <video
        src={`${signed.data}#t=0.001`}
        preload="metadata"
        muted
        playsInline
        tabIndex={-1}
        onError={() => setVideoFailed(true)}
        className={cn("h-full w-full rounded-md bg-black object-cover", className)}
      />
    );
  }

  // Image with blob URL loaded: show thumbnail (fall back to the file icon
  // if the load fails, e.g. a dead cached URL or a provider error)
  if (image && blobUrl && !failed) {
    return (
      <img
        src={blobUrl}
        alt={entry.name}
        className={cn("h-full w-full rounded-md object-cover", className)}
        draggable={false}
        onError={() => setFailed(true)}
      />
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
