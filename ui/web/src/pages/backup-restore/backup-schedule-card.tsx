import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { CalendarClock, Loader2, Play } from "lucide-react";
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
import {
  useBackupSchedule,
  useSaveBackupSchedule,
  useRunBackupSchedule,
} from "./hooks/use-backup-schedule";

type FrequencyMode = "hours" | "daily" | "weekly";

const HOUR_OPTIONS = [1, 2, 4, 6, 12];
const WEEKDAY_KEYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"] as const;

interface FormState {
  enabled: boolean;
  mode: FrequencyMode;
  everyHours: number;
  hour: number;
  weekday: number;
  retention: number;
}

/** Parse a stored interval (duration or cron expr) into form state. */
function intervalToForm(interval: string): Omit<FormState, "enabled" | "retention"> {
  let m = /^(\d+)h$/.exec(interval.trim());
  if (m) {
    const n = Number(m[1]);
    return { mode: "hours", everyHours: HOUR_OPTIONS.includes(n) ? n : 6, hour: 3, weekday: 1 };
  }
  m = /^0 (\d{1,2}) \* \* \*$/.exec(interval.trim());
  if (m) {
    return { mode: "daily", everyHours: 6, hour: Number(m[1]) % 24, weekday: 1 };
  }
  m = /^0 (\d{1,2}) \* \* ([0-6])$/.exec(interval.trim());
  if (m) {
    return { mode: "weekly", everyHours: 6, hour: Number(m[1]) % 24, weekday: Number(m[3]) };
  }
  // Unknown format (hand-written cron) — default to daily 03:00.
  return { mode: "daily", everyHours: 6, hour: 3, weekday: 1 };
}

/** Build the interval string (duration or cron expr) from form state. */
function formToInterval(form: Omit<FormState, "enabled" | "retention">): string {
  switch (form.mode) {
    case "hours":
      return `${form.everyHours}h`;
    case "weekly":
      return `0 ${form.hour} * * ${form.weekday}`;
    default:
      return `0 ${form.hour} * * *`;
  }
}

function formatDateTime(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString();
}

export function BackupScheduleCard() {
  const { t } = useTranslation("backup");
  const { data: schedule, isLoading } = useBackupSchedule();
  const saveMutation = useSaveBackupSchedule();
  const runMutation = useRunBackupSchedule();

  const [form, setForm] = useState<FormState>({
    enabled: false,
    mode: "daily",
    everyHours: 6,
    hour: 3,
    weekday: 1,
    retention: 0,
  });

  // Hydrate the form once from the persisted schedule (not on later refetches
  // — matching the S3 config panel — so status refreshes never clobber edits).
  const hydrated = useRef(false);
  useEffect(() => {
    if (!schedule || hydrated.current) return;
    hydrated.current = true;
    const parsed = intervalToForm(schedule.interval);
    setForm({
      enabled: schedule.enabled,
      retention: schedule.retention ?? 0,
      ...parsed,
    });
  }, [schedule]);

  const interval = useMemo(() => formToInterval(form), [form]);
  const dirty = schedule ? schedule.interval !== interval || schedule.enabled !== form.enabled || (schedule.retention ?? 0) !== form.retention : true;

  const onSave = () => {
    saveMutation.mutate({
      enabled: form.enabled,
      interval,
      destination: "s3",
      retention: form.retention,
    });
  };

  const onRunNow = () => {
    // Save pending edits first so the manual run uses the visible schedule.
    if (dirty) {
      saveMutation.mutate(
        {
          enabled: form.enabled,
          interval,
          destination: "s3",
          retention: form.retention,
        },
        { onSuccess: () => runMutation.mutate() },
      );
      return;
    }
    runMutation.mutate();
  };

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground py-4 justify-center">
        <Loader2 className="h-4 w-4 animate-spin" />
      </div>
    );
  }

  const statusKey = schedule?.last_status;

  return (
    <div className="space-y-4 border-t pt-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2 min-w-0">
          <CalendarClock className="h-4 w-4 shrink-0 text-muted-foreground" />
          <h3 className="text-sm font-medium truncate">{t("schedule.title")}</h3>
        </div>
        <div className="flex items-center gap-2">
          <Label htmlFor="backup-schedule-enabled" className="text-sm text-muted-foreground">
            {t("schedule.enable")}
          </Label>
          <Switch
            id="backup-schedule-enabled"
            checked={form.enabled}
            onCheckedChange={(checked) => setForm((f) => ({ ...f, enabled: checked }))}
          />
        </div>
      </div>
      <p className="text-sm text-muted-foreground">{t("schedule.description")}</p>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="backup-schedule-frequency">{t("schedule.frequency.label")}</Label>
          <Select
            value={form.mode}
            onValueChange={(v) => setForm((f) => ({ ...f, mode: v as FrequencyMode }))}
            disabled={!form.enabled}
          >
            <SelectTrigger id="backup-schedule-frequency" className="text-base md:text-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="daily">{t("schedule.frequency.daily")}</SelectItem>
              <SelectItem value="weekly">{t("schedule.frequency.weekly")}</SelectItem>
              <SelectItem value="hours">{t("schedule.frequency.hours")}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {form.mode === "hours" && (
          <div className="space-y-2">
            <Label htmlFor="backup-schedule-hours">{t("schedule.everyHours")}</Label>
            <Select
              value={String(form.everyHours)}
              onValueChange={(v) => setForm((f) => ({ ...f, everyHours: Number(v) }))}
              disabled={!form.enabled}
            >
              <SelectTrigger id="backup-schedule-hours" className="text-base md:text-sm">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {HOUR_OPTIONS.map((h) => (
                  <SelectItem key={h} value={String(h)}>
                    {t("schedule.hourValue", { hours: h })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {form.mode !== "hours" && (
          <div className="space-y-2">
            <Label htmlFor="backup-schedule-hour">{t("schedule.hour")}</Label>
            <Select
              value={String(form.hour)}
              onValueChange={(v) => setForm((f) => ({ ...f, hour: Number(v) }))}
              disabled={!form.enabled}
            >
              <SelectTrigger id="backup-schedule-hour" className="text-base md:text-sm">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Array.from({ length: 24 }, (_, h) => (
                  <SelectItem key={h} value={String(h)}>
                    {`${String(h).padStart(2, "0")}:00`}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {form.mode === "weekly" && (
          <div className="space-y-2">
            <Label htmlFor="backup-schedule-weekday">{t("schedule.weekday")}</Label>
            <Select
              value={String(form.weekday)}
              onValueChange={(v) => setForm((f) => ({ ...f, weekday: Number(v) }))}
              disabled={!form.enabled}
            >
              <SelectTrigger id="backup-schedule-weekday" className="text-base md:text-sm">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {WEEKDAY_KEYS.map((key, idx) => (
                  <SelectItem key={key} value={String(idx)}>
                    {t(`schedule.weekdays.${key}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        <div className="space-y-2">
          <Label htmlFor="backup-schedule-retention">{t("schedule.retention")}</Label>
          <Input
            id="backup-schedule-retention"
            type="number"
            min={0}
            inputMode="numeric"
            value={form.retention}
            onChange={(e) =>
              setForm((f) => ({ ...f, retention: Math.max(0, Number(e.target.value) || 0) }))
            }
            disabled={!form.enabled}
            className="text-base md:text-sm"
          />
          <p className="text-xs text-muted-foreground">{t("schedule.retentionHint")}</p>
        </div>
      </div>

      {/* Next / last run status */}
      <div className="rounded-md border bg-muted/50 p-3 text-sm space-y-1">
        <div className="flex justify-between gap-2">
          <span className="text-muted-foreground">{t("schedule.nextRun")}</span>
          <span className="font-mono text-xs text-right">
            {form.enabled && schedule?.next_run ? formatDateTime(schedule.next_run) : t("schedule.never")}
          </span>
        </div>
        <div className="flex justify-between gap-2">
          <span className="text-muted-foreground">{t("schedule.lastRun")}</span>
          <span className="text-right min-w-0">
            {schedule?.last_run ? (
              <>
                <span className="font-mono text-xs">{formatDateTime(schedule.last_run)}</span>
                {statusKey && (
                  <span
                    className={
                      statusKey === "ok"
                        ? "ml-2 text-green-600"
                        : statusKey === "error"
                          ? "ml-2 text-destructive"
                          : "ml-2 text-muted-foreground"
                    }
                  >
                    {t(`schedule.status.${statusKey}`)}
                  </span>
                )}
              </>
            ) : (
              <span className="text-muted-foreground">{t("schedule.never")}</span>
            )}
          </span>
        </div>
        {statusKey === "error" && schedule?.last_error && (
          <p className="text-xs text-destructive break-all">{schedule.last_error}</p>
        )}
      </div>

      <div className="flex items-center justify-end gap-2">
        <Button
          variant="outline"
          onClick={onRunNow}
          disabled={runMutation.isPending || saveMutation.isPending}
        >
          {runMutation.isPending ? (
            <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
          ) : (
            <Play className="mr-1.5 h-4 w-4" />
          )}
          {t("schedule.runNow")}
        </Button>
        <Button onClick={onSave} disabled={!dirty || saveMutation.isPending}>
          {saveMutation.isPending && <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />}
          {t("schedule.save")}
        </Button>
      </div>
    </div>
  );
}
