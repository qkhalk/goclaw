import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { CalendarClock, Cloud, HardDrive, Play } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useHttp } from "@/hooks/use-ws";
import { formatFileSize, formatRelativeTime } from "@/lib/format";

interface ScheduleConfig {
  enabled: boolean;
  interval_hours: number;
  destination: "s3" | "cloud";
  cloud_account_id?: string;
  cloud_path?: string;
  keep_count: number;
}

interface ScheduleRun {
  at: string;
  ok: boolean;
  error?: string;
  artifact?: string;
  size_bytes?: number;
  duration_ms?: number;
}

interface ScheduleStatus {
  config: ScheduleConfig;
  last_run: ScheduleRun | null;
  next_due: string | null;
}

const INTERVALS = [1, 6, 12, 24, 48, 168];

/** Periodic backup to the cloud: schedule (interval + destination S3 or a
 * connected drive), retention for S3, last/next run status, and run-now. */
export function SchedulePanel() {
  const { t } = useTranslation("backup");
  const http = useHttp();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<ScheduleConfig | null>(null);
  const [running, setRunning] = useState(false);
  const [message, setMessage] = useState("");

  const { data } = useQuery({
    queryKey: ["backup-schedule"],
    queryFn: () => http.get<ScheduleStatus>("/v1/system/backup/schedule"),
  });

  useEffect(() => {
    if (data?.config && form === null) {
      setForm({
        ...data.config,
        cloud_path: data.config.cloud_path || "GoClaw Backups",
        keep_count: data.config.keep_count || 0,
      });
    }
  }, [data, form]);

  const save = async (next: ScheduleConfig) => {
    setForm(next);
    setMessage("");
    try {
      await http.put("/v1/system/backup/schedule", next);
      await queryClient.invalidateQueries({ queryKey: ["backup-schedule"] });
      setMessage(t("schedule.saved"));
    } catch (e) {
      setMessage(e instanceof Error ? e.message : String(e));
    }
  };

  const runNow = async () => {
    setRunning(true);
    setMessage("");
    try {
      const res = await http.post<{ run: ScheduleRun }>("/v1/system/backup/schedule/run");
      setMessage(res.run.ok ? t("schedule.runOk", { artifact: res.run.artifact ?? "" }) : t("schedule.runFailed", { error: res.run.error ?? "" }));
      await queryClient.invalidateQueries({ queryKey: ["backup-schedule"] });
    } catch (e) {
      setMessage(e instanceof Error ? e.message : String(e));
    } finally {
      setRunning(false);
    }
  };

  if (!form) {
    return <p className="text-sm text-muted-foreground">{t("schedule.loading")}</p>;
  }

  return (
    <div className="space-y-4 rounded-lg border p-4">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-2">
          <CalendarClock className="h-4 w-4 text-muted-foreground" />
          <div>
            <p className="text-sm font-medium">{t("schedule.title")}</p>
            <p className="text-xs text-muted-foreground">{t("schedule.description")}</p>
          </div>
        </div>
        <Switch checked={form.enabled} onCheckedChange={(v) => save({ ...form, enabled: v })} />
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label className="text-sm">{t("schedule.interval")}</Label>
          <Select
            value={String(form.interval_hours || 24)}
            onValueChange={(v) => save({ ...form, interval_hours: Number(v) })}
          >
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              {INTERVALS.map((h) => (
                <SelectItem key={h} value={String(h)}>
                  {t("schedule.hours", { count: h })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-1.5">
          <Label className="text-sm">{t("schedule.destination")}</Label>
          <Select
            value={form.destination}
            onValueChange={(v) => save({ ...form, destination: v as "s3" | "cloud" })}
          >
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="s3">
                <span className="flex items-center gap-2"><HardDrive className="h-4 w-4" /> S3</span>
              </SelectItem>
              <SelectItem value="cloud">
                <span className="flex items-center gap-2"><Cloud className="h-4 w-4" /> {t("schedule.cloudDrive")}</span>
              </SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {form.destination === "s3" ? t("schedule.s3Hint") : t("schedule.cloudHint")}
          </p>
        </div>

        {form.destination === "cloud" && (
          <div className="space-y-1.5 sm:col-span-2">
            <Label className="text-sm">{t("schedule.cloudPath")}</Label>
            <Input
              value={form.cloud_path ?? ""}
              onChange={(e) => setForm({ ...form, cloud_path: e.target.value })}
              onBlur={() => save(form)}
              placeholder="GoClaw Backups"
              className="text-base md:text-sm"
            />
          </div>
        )}

        {form.destination === "s3" && (
          <div className="space-y-1.5">
            <Label className="text-sm">{t("schedule.keepCount")}</Label>
            <Input
              type="number"
              min={0}
              max={100}
              value={form.keep_count}
              onChange={(e) => setForm({ ...form, keep_count: Number(e.target.value) || 0 })}
              onBlur={() => save(form)}
              className="text-base md:text-sm"
            />
            <p className="text-xs text-muted-foreground">{t("schedule.keepCountHint")}</p>
          </div>
        )}
      </div>

      <div className="flex items-center justify-between gap-3 border-t pt-3">
        <div className="min-w-0 text-xs text-muted-foreground">
          {data?.last_run && (
            <p className="truncate">
              {data.last_run.ok ? "✅" : "❌"}{" "}
              {t("schedule.lastRun", {
                when: formatRelativeTime(data.last_run.at),
                size: data.last_run.size_bytes ? formatFileSize(data.last_run.size_bytes) : "",
              })}
              {data.last_run.error ? ` — ${data.last_run.error}` : ""}
            </p>
          )}
          {form.enabled && data?.next_due && (
            <p>{t("schedule.nextDue", { when: formatRelativeTime(data.next_due) })}</p>
          )}
        </div>
        <Button variant="outline" size="sm" onClick={runNow} disabled={running} className="shrink-0">
          <Play className="mr-1.5 h-3.5 w-3.5" />
          {running ? t("schedule.running") : t("schedule.runNow")}
        </Button>
      </div>

      {message && <p className="text-xs text-muted-foreground">{message}</p>}
    </div>
  );
}
