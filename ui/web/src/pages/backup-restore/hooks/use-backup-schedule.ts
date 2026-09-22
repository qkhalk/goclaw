import { useCallback } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useWs } from "@/hooks/use-ws";
import { useAuthStore } from "@/stores/use-auth-store";
import { Methods } from "@/api/protocol";
import { toast } from "@/stores/use-toast-store";

export interface BackupSchedule {
  enabled: boolean;
  interval: string; // duration ("6h") or cron expr ("0 3 * * *")
  destination: string; // "s3"
  retention: number; // keep N most recent in cloud (0 = unlimited)
  last_run?: string;
  next_run?: string;
  last_status?: string; // "ok" | "error" | "running"
  last_error?: string;
}

export interface BackupScheduleInput {
  enabled: boolean;
  interval: string;
  destination: string;
  retention: number;
  [key: string]: unknown;
}

export const backupScheduleQueryKey = ["backup-schedule"] as const;

export function useBackupSchedule() {
  const ws = useWs();
  const connected = useAuthStore((s) => s.connected);

  return useQuery({
    queryKey: backupScheduleQueryKey,
    queryFn: async () => {
      const res = await ws.call<{ schedule: BackupSchedule }>(Methods.BACKUP_SCHEDULE_GET);
      return res.schedule;
    },
    staleTime: 30_000,
    enabled: connected,
  });
}

export function useSaveBackupSchedule() {
  const ws = useWs();
  const queryClient = useQueryClient();
  const { t } = useTranslation("backup");

  return useMutation({
    mutationFn: async (input: BackupScheduleInput) => {
      const res = await ws.call<{ schedule: BackupSchedule }>(Methods.BACKUP_SCHEDULE_SET, input);
      return res.schedule;
    },
    onSuccess: (schedule) => {
      queryClient.setQueryData(backupScheduleQueryKey, schedule);
      toast.success(t("schedule.toast.saved"));
    },
    onError: (err: Error) => {
      toast.error(t("schedule.toast.saveFailed"), err.message);
    },
  });
}

export function useRunBackupSchedule() {
  const ws = useWs();
  const queryClient = useQueryClient();
  const { t } = useTranslation("backup");

  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: backupScheduleQueryKey }),
    [queryClient],
  );

  return useMutation({
    mutationFn: async () => {
      await ws.call(Methods.BACKUP_SCHEDULE_RUN);
    },
    onSuccess: () => {
      toast.success(t("schedule.toast.runStarted"));
      // The run takes minutes — refresh status shortly after and then again.
      setTimeout(invalidate, 5_000);
      setTimeout(invalidate, 30_000);
    },
    onError: (err: Error) => {
      toast.error(t("schedule.toast.runFailed"), err.message);
    },
  });
}
