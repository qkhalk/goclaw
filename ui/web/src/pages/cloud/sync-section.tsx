import { useState } from "react";
import { useTranslation } from "react-i18next";
import { FolderInput, Loader2, Play, Plus, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
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
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { toast } from "@/stores/use-toast-store";
import { cn } from "@/lib/utils";
import {
  useCloudAccounts,
  useCloudSyncPairs,
  type CloudSyncPair,
} from "./hooks/use-cloud";
import { opErrorToast } from "./drive/op-error";
import { FolderPickerDialog } from "./drive/folder-picker-dialog";

/** Sentinel for "no account picked" (Radix Select forbids empty values). */
const NONE = "__none__";

const SCHEDULE_PRESETS = [
  { value: "0", minutes: 0 },
  { value: "60", minutes: 60 },
  { value: "1440", minutes: 1440 },
  { value: "custom", minutes: -1 },
];

type CloudT = (key: string, opts?: Record<string, unknown>) => string;

function scheduleLabel(p: CloudSyncPair, t: CloudT): string {
  if (p.interval_minutes <= 0) return t("sync.schedule.manual");
  if (p.interval_minutes === 60) return t("sync.schedule.hourly");
  if (p.interval_minutes === 1440) return t("sync.schedule.daily");
  return t("sync.schedule.every_n_minutes", { count: p.interval_minutes });
}

function statusBadge(p: CloudSyncPair, t: CloudT) {
  switch (p.last_status) {
    case "ok":
      return <Badge variant="outline" className="border-green-500/40 text-green-600">{t("sync.status.ok")}</Badge>;
    case "error":
      return (
        <span title={p.last_error}>
          <Badge variant="destructive">{t("sync.status.error")}</Badge>
        </span>
      );
    case "running":
      return (
        <Badge variant="outline" className="border-sky-500/40 text-sky-600">
          <Loader2 className="mr-1 h-3 w-3 animate-spin" />
          {t("sync.status.running")}
        </Badge>
      );
    default:
      return <Badge variant="outline" className="text-muted-foreground">{t("sync.status.never")}</Badge>;
  }
}

/** "Đồng bộ dữ liệu" section of the cloud settings sheet: tenant-level one-way
 * folder sync pairs (source account+path → target account+path) with manual
 * runs and optional hourly/daily/custom schedules. Tenant-admin only. */
export function SyncSection() {
  const { t } = useTranslation("cloud");
  const { t: tc } = useTranslation("common");
  const { accounts } = useCloudAccounts();
  const { pairs, loading, createPair, updatePair, deletePair, runPair } = useCloudSyncPairs(true);

  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<CloudSyncPair | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [runningId, setRunningId] = useState<string | null>(null);

  async function handleRun(p: CloudSyncPair) {
    setRunningId(p.id);
    try {
      await runPair(p.id);
      toast.success(t("sync.run_queued"));
    } catch (e) {
      opErrorToast(e, t);
    } finally {
      setRunningId(null);
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await deletePair(deleteTarget.id);
      setDeleteTarget(null);
    } catch (e) {
      opErrorToast(e, t);
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-2">
        <Label className="text-sm">{t("sync.section_title")}</Label>
        <Button variant="outline" size="sm" className="min-h-11 sm:min-h-8" onClick={() => setCreateOpen(true)}>
          <Plus className="mr-1.5 h-4 w-4" />
          {t("sync.add_pair")}
        </Button>
      </div>
      <p className="-mt-1 text-xs text-muted-foreground">{t("sync.section_description")}</p>

      {loading ? (
        <div className="flex items-center gap-2 py-3 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          {tc("loading")}
        </div>
      ) : pairs.length === 0 ? (
        <p className="rounded-md border border-dashed p-3 text-xs text-muted-foreground">{t("sync.empty")}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[600px] text-sm">
            <thead>
              <tr className="border-b text-left text-xs text-muted-foreground">
                <th className="py-1.5 pr-2 font-medium">{t("sync.source")}</th>
                <th className="px-2 py-1.5 font-medium">{t("sync.target")}</th>
                <th className="px-2 py-1.5 font-medium">{t("sync.schedule.label")}</th>
                <th className="px-2 py-1.5 font-medium">{t("sync.last_run")}</th>
                <th className="px-2 py-1.5 font-medium">{t("sync.enabled")}</th>
                <th className="w-20 px-2 py-1.5" />
              </tr>
            </thead>
            <tbody>
              {pairs.map((p) => {
                const src = accounts.find((a) => a.id === p.source_account_id);
                const dst = accounts.find((a) => a.id === p.target_account_id);
                return (
                  <tr key={p.id} className="border-b last:border-0 align-top">
                    <td className="py-2 pr-2">
                      <p className="max-w-[160px] truncate" title={src?.email ?? p.source_account_id}>
                        {src?.email ?? t("sync.account_missing")}
                      </p>
                      <p className="max-w-[160px] truncate text-xs text-muted-foreground" dir="ltr">
                        {p.source_path}
                      </p>
                    </td>
                    <td className="px-2 py-2">
                      <p className="max-w-[160px] truncate" title={dst?.email ?? p.target_account_id}>
                        {dst?.email ?? t("sync.account_missing")}
                      </p>
                      <p className="max-w-[160px] truncate text-xs text-muted-foreground" dir="ltr">
                        {p.target_path}
                      </p>
                    </td>
                    <td className="px-2 py-2 text-xs">{scheduleLabel(p, t)}</td>
                    <td className="px-2 py-2">
                      <div className="flex flex-col gap-1">
                        {statusBadge(p, t)}
                        {p.last_run_at && (
                          <span className="text-xs text-muted-foreground">
                            {new Date(p.last_run_at).toLocaleString()}
                          </span>
                        )}
                        {p.last_status === "error" && p.last_error && (
                          <span className="max-w-[180px] truncate text-xs text-destructive" title={p.last_error}>
                            {p.last_error}
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-2 py-2">
                      <Switch checked={p.enabled} onCheckedChange={(v) => void updatePair(p.id, { enabled: v }).catch((e) => opErrorToast(e, t))} />
                    </td>
                    <td className="px-2 py-2">
                      <div className="flex items-center justify-end gap-0.5">
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={t("sync.run_now")}
                          title={t("sync.run_now")}
                          disabled={runningId === p.id}
                          onClick={() => void handleRun(p)}
                        >
                          {runningId === p.id ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={tc("delete")}
                          title={tc("delete")}
                          className="text-destructive hover:text-destructive"
                          onClick={() => setDeleteTarget(p)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {createOpen && (
        <CreateSyncPairDialog
          accounts={accounts}
          onClose={() => setCreateOpen(false)}
          onCreate={async (input) => {
            await createPair(input);
            setCreateOpen(false);
            toast.success(t("sync.created"));
          }}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t("sync.delete_confirm_title")}
        description={t("sync.delete_confirm")}
        confirmLabel={tc("delete")}
        variant="destructive"
        loading={deleting}
        onConfirm={() => void handleDelete()}
      />
    </div>
  );
}

/** Create dialog: source account+folder → target account+folder + schedule. */
function CreateSyncPairDialog({
  accounts,
  onClose,
  onCreate,
}: {
  accounts: { id: string; email: string }[];
  onClose: () => void;
  onCreate: (input: {
    source_account_id: string;
    source_path: string;
    target_account_id: string;
    target_path: string;
    interval_minutes: number;
    enabled: boolean;
  }) => Promise<void>;
}) {
  const { t } = useTranslation("cloud");
  const { t: tc } = useTranslation("common");
  const [sourceId, setSourceId] = useState(NONE);
  const [targetId, setTargetId] = useState(NONE);
  const [sourcePath, setSourcePath] = useState("/");
  const [targetPath, setTargetPath] = useState("/");
  const [picker, setPicker] = useState<"source" | "target" | null>(null);
  const [schedule, setSchedule] = useState("0");
  const [customMinutes, setCustomMinutes] = useState("120");
  const [enabled, setEnabled] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const pickerAccount = picker === "source" ? sourceId : targetId;
  const minutes =
    schedule === "custom" ? Math.max(1, parseInt(customMinutes, 10) || 0) : parseInt(schedule, 10);
  const sameSpot = sourceId !== NONE && sourceId === targetId && sourcePath === targetPath;
  const valid =
    sourceId !== NONE && targetId !== NONE && Number.isFinite(minutes) && minutes >= 0 && !sameSpot;

  async function submit() {
    setBusy(true);
    setError("");
    try {
      await onCreate({
        source_account_id: sourceId,
        source_path: sourcePath,
        target_account_id: targetId,
        target_path: targetPath,
        interval_minutes: minutes,
        enabled,
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Dialog open onOpenChange={(o) => !o && onClose()}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("sync.add_pair")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label className="text-sm">{t("sync.source")}</Label>
              <AccountPicker
                value={sourceId}
                accounts={accounts}
                onChange={(v) => {
                  setSourceId(v);
                  setSourcePath("/");
                }}
              />
              <FolderButton
                label={t("sync.choose_source_folder")}
                path={sourcePath}
                disabled={sourceId === NONE}
                onClick={() => setPicker("source")}
              />
            </div>

            <div className="space-y-1.5">
              <Label className="text-sm">{t("sync.target")}</Label>
              <AccountPicker
                value={targetId}
                accounts={accounts}
                onChange={(v) => {
                  setTargetId(v);
                  setTargetPath("/");
                }}
              />
              <FolderButton
                label={t("sync.choose_target_folder")}
                path={targetPath}
                disabled={targetId === NONE}
                onClick={() => setPicker("target")}
              />
            </div>

            <div className="space-y-1.5">
              <Label className="text-sm">{t("sync.schedule.label")}</Label>
              <Select value={schedule} onValueChange={setSchedule}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SCHEDULE_PRESETS.map((s) => (
                    <SelectItem key={s.value} value={s.value}>
                      {s.value === "custom"
                        ? t("sync.schedule.custom_minutes")
                        : s.minutes === 0
                          ? t("sync.schedule.manual")
                          : s.minutes === 60
                            ? t("sync.schedule.hourly")
                            : t("sync.schedule.daily")}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {schedule === "custom" && (
                <div className="flex items-center gap-2">
                  <Input
                    type="number"
                    min={1}
                    value={customMinutes}
                    onChange={(e) => setCustomMinutes(e.target.value)}
                    className="w-28 text-base md:text-sm"
                    aria-label={t("sync.interval_minutes")}
                  />
                  <span className="text-xs text-muted-foreground">{t("sync.interval_minutes")}</span>
                </div>
              )}
            </div>

            <label className="flex items-center justify-between gap-2 text-sm">
              <span>{t("sync.enabled")}</span>
              <Switch checked={enabled} onCheckedChange={setEnabled} />
            </label>

            {sameSpot && <p className="text-xs text-destructive">{t("sync.same_pair_error")}</p>}
            {error && <p className="text-xs text-destructive">{error}</p>}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={onClose}>
              {tc("cancel")}
            </Button>
            <Button disabled={!valid || busy} onClick={() => void submit()}>
              {busy ? "…" : tc("save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {pickerAccount && picker && (
        <FolderPickerDialog
          open
          onOpenChange={() => setPicker(null)}
          accountId={pickerAccount}
          title={picker === "source" ? t("sync.choose_source_folder") : t("sync.choose_target_folder")}
          confirmLabel={t("transfer.use_folder")}
          initialPath={picker === "source" ? sourcePath : targetPath}
          onPick={(p) => {
            if (picker === "source") setSourcePath(p);
            else setTargetPath(p);
            setPicker(null);
          }}
        />
      )}
    </>
  );
}

function AccountPicker({
  value,
  accounts,
  onChange,
}: {
  value: string;
  accounts: { id: string; email: string }[];
  onChange: (v: string) => void;
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="w-full" dir="ltr">
        <SelectValue placeholder="—" />
      </SelectTrigger>
      <SelectContent>
        {accounts.map((a) => (
          <SelectItem key={a.id} value={a.id}>
            {a.email}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function FolderButton({
  label,
  path,
  disabled,
  onClick,
}: {
  label: string;
  path: string;
  disabled: boolean;
  onClick: () => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="min-w-0 flex-1 truncate rounded-md border px-2 py-1.5 text-sm text-muted-foreground" dir="ltr">
        {path}
      </span>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={cn("min-h-11 shrink-0 sm:min-h-8")}
        disabled={disabled}
        onClick={onClick}
      >
        <FolderInput className="mr-1.5 h-4 w-4" />
        {label}
      </Button>
    </div>
  );
}
