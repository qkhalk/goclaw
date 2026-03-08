import { useState, useCallback } from "react";
import { useHttp } from "@/hooks/use-ws";

export interface StorageFile {
  path: string;
  name: string;
  isDir: boolean;
  size: number;
  totalSize: number;
  protected: boolean;
}

interface StorageListResponse {
  files: StorageFile[];
  totalSize: number;
  baseDir: string;
}

interface StorageFileContent {
  content: string;
  path: string;
  size: number;
}

export function useStorage() {
  const http = useHttp();
  const [files, setFiles] = useState<StorageFile[]>([]);
  const [totalSize, setTotalSize] = useState(0);
  const [baseDir, setBaseDir] = useState("");
  const [loading, setLoading] = useState(false);

  const listFiles = useCallback(async () => {
    setLoading(true);
    try {
      const res = await http.get<StorageListResponse>("/v1/storage/files");
      setFiles(res.files ?? []);
      setTotalSize(res.totalSize ?? 0);
      setBaseDir(res.baseDir ?? "");
    } finally {
      setLoading(false);
    }
  }, [http]);

  const readFile = useCallback(
    async (path: string) => {
      return http.get<StorageFileContent>(
        `/v1/storage/files/${encodeURIComponent(path)}`,
      );
    },
    [http],
  );

  const deleteFile = useCallback(
    async (path: string) => {
      await http.delete<{ status: string }>(
        `/v1/storage/files/${encodeURIComponent(path)}`,
      );
    },
    [http],
  );

  return { files, totalSize, baseDir, loading, listFiles, readFile, deleteFile };
}
