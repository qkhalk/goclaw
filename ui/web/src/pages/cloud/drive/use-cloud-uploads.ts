import { useCallback, useRef, useState } from "react";
import { useCloudFileOps } from "../hooks/use-cloud";

export type UploadStatus = "queued" | "uploading" | "done" | "failed" | "canceled";

export interface UploadItem {
  id: string;
  name: string;
  size: number;
  loaded: number;
  status: UploadStatus;
}

interface QueueJob {
  id: string;
  file: File;
  dir: string;
}

let uploadSeq = 0;

/** Sequential upload queue with per-file progress for one cloud account.
 * Files are uploaded one at a time (provider rate-limit friendly); the
 * destination folder is captured at enqueue time so navigating mid-upload
 * never redirects a file into the wrong folder. Abort = cancel the active
 * XHR (the job is marked canceled). */
export function useCloudUploads(accountId: string) {
  const ops = useCloudFileOps(accountId);
  const [items, setItems] = useState<UploadItem[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);

  const queueRef = useRef<QueueJob[]>([]);
  const runningRef = useRef(false);
  const abortRef = useRef<AbortController | null>(null);

  const patch = useCallback((id: string, update: Partial<UploadItem>) => {
    setItems((prev) => prev.map((it) => (it.id === id ? { ...it, ...update } : it)));
  }, []);

  const runNext = useCallback(async () => {
    if (runningRef.current) return;
    runningRef.current = true;
    try {
      for (;;) {
        const job = queueRef.current.shift();
        if (!job) break;
        const ac = new AbortController();
        abortRef.current = ac;
        setActiveId(job.id);
        patch(job.id, { status: "uploading" });
        try {
          await ops.upload(job.file, job.dir, {
            signal: ac.signal,
            onProgress: (loaded, total) => patch(job.id, { loaded, size: total }),
          });
          patch(job.id, { status: "done", loaded: job.file.size });
        } catch (e) {
          const aborted = e instanceof DOMException && e.name === "AbortError";
          patch(job.id, {
            status: aborted ? "canceled" : "failed",
            loaded: aborted ? 0 : undefined,
          } as Partial<UploadItem>);
        } finally {
          abortRef.current = null;
          setActiveId(null);
        }
      }
    } finally {
      runningRef.current = false;
    }
  }, [ops, patch]);

  const enqueue = useCallback(
    (files: File[], dir: string) => {
      if (files.length === 0) return;
      const jobs: QueueJob[] = files.map((file) => {
        const id = `up-${++uploadSeq}`;
        return { id, file, dir };
      });
      setItems((prev) => [
        ...prev,
        ...jobs.map((j) => ({ id: j.id, name: j.file.name, size: j.file.size, loaded: 0, status: "queued" as const })),
      ]);
      queueRef.current.push(...jobs);
      void runNext();
    },
    [runNext],
  );

  /** Cancel the in-flight XHR (if any) and drop everything still queued. */
  const cancel = useCallback((id?: string) => {
    if (id === undefined || id === activeId) {
      abortRef.current?.abort();
    }
    queueRef.current = queueRef.current.filter((j) => {
      if (id === undefined || j.id === id) {
        patch(j.id, { status: "canceled" });
        return false;
      }
      return true;
    });
  }, [activeId, patch]);

  const clearFinished = useCallback(() => {
    setItems((prev) => prev.filter((it) => it.status === "queued" || it.status === "uploading"));
  }, []);

  return { items, activeId, enqueue, cancel, clearFinished };
}

export type CloudUploads = ReturnType<typeof useCloudUploads>;
