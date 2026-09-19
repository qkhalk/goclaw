import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { Download, Loader2, Plus, Server, Trash2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { isValidSlug } from "@/lib/slug";
import { ROUTES } from "@/lib/routes";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/stores/use-auth-store";
import {
  useMcpCatalog,
  type McpCatalogEntry,
  type McpCustomInstallInput,
  type McpInstallJob,
  type McpInstallResult,
} from "./use-mcp-catalog";

/**
 * "MCP tool servers" section of the Tool Store — GitHub-hosted tool servers that
 * are cloned, dependency-installed and smoke-tested on demand. Install jobs are
 * polled (see use-mcp-catalog) and the cards reflect catalog/package state.
 */
export function McpSection() {
  const { t } = useTranslation("tools");
  const navigate = useNavigate();
  const role = useAuthStore((s) => s.role);
  const isAdmin = role === "admin" || role === "owner";
  const {
    entries,
    loading,
    error,
    job,
    installing,
    activeName,
    install,
    installCustom,
    uninstall,
    actionError,
    clearActionError,
    dismissJob,
  } = useMcpCatalog();

  const [busy, setBusy] = useState<string | null>(null);
  const [confirming, setConfirming] = useState<string | null>(null);
  const [customOpen, setCustomOpen] = useState(false);

  async function handleUninstall(entry: McpCatalogEntry) {
    // Same 2-step inline confirm as the studio modules: first click arms,
    // second click runs the uninstall.
    if (confirming !== entry.name) {
      setConfirming(entry.name);
      return;
    }
    setBusy(entry.name);
    try {
      await uninstall(entry.name);
    } finally {
      setBusy(null);
      setConfirming(null);
    }
  }

  function handleInstall(entry: McpCatalogEntry) {
    clearActionError();
    void install(entry.name);
  }

  // A failed custom install never registers a catalog entry — surface its error
  // at section level because there is no card to attach it to.
  const orphanJobError = job?.status === "error" && !entries.find((e) => e.name === job.name);

  return (
    <section className="mt-8 rounded-lg border p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <Server className="h-4 w-4 text-muted-foreground" />
            <h2 className="text-sm font-semibold">{t("store.mcp_servers_title")}</h2>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{t("store.mcp_servers_subtitle")}</p>
        </div>
        {isAdmin && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setCustomOpen(true)}
            className="min-h-11 shrink-0 sm:min-h-9"
          >
            <Plus className="h-3.5 w-3.5" />
            {t("store.mcp_add_custom")}
          </Button>
        )}
      </div>

      <div className="mt-4">
        {loading ? (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Skeleton className="h-44 rounded-lg" />
            <Skeleton className="h-44 rounded-lg" />
            <Skeleton className="h-44 rounded-lg" />
          </div>
        ) : error ? (
          <p className="text-sm text-destructive">{t("store.mcp_fetch_failed")}</p>
        ) : entries.length === 0 ? (
          <p className="py-2 text-xs text-muted-foreground">{t("store.mcp_custom_desc")}</p>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {entries.map((entry) => (
              <McpEntryCard
                key={entry.name}
                entry={entry}
                isAdmin={isAdmin}
                jobActive={installing && activeName === entry.name}
                job={job && job.name === entry.name ? job : null}
                anyInstalling={installing}
                busy={busy === entry.name}
                confirming={confirming === entry.name && entry.installed}
                actionError={actionError?.for === entry.name ? actionError.message : null}
                onInstall={() => handleInstall(entry)}
                onUninstall={() => void handleUninstall(entry)}
                onGoManage={() => navigate(ROUTES.MCP)}
                onDismissJob={dismissJob}
              />
            ))}
          </div>
        )}
      </div>

      {orphanJobError && job && (
        <p className="mt-3 text-xs text-destructive">
          {t("store.mcp_job_failed")}
          {job.error ? `: ${job.error}` : ""}
        </p>
      )}

      <McpCustomDialog open={customOpen} onOpenChange={setCustomOpen} onSubmit={installCustom} />
    </section>
  );
}

interface McpEntryCardProps {
  entry: McpCatalogEntry;
  isAdmin: boolean;
  /** A job is pending/running for this entry (spinner + installing badge). */
  jobActive: boolean;
  /** Job (any status) when it belongs to this entry, else null. */
  job: McpInstallJob | null;
  /** Any install job is pending/running — the backend serializes installs. */
  anyInstalling: boolean;
  busy: boolean;
  confirming: boolean;
  actionError: string | null;
  onInstall: () => void;
  onUninstall: () => void;
  onGoManage: () => void;
  onDismissJob: () => void;
}

function McpEntryCard({
  entry,
  isAdmin,
  jobActive,
  job,
  anyInstalling,
  busy,
  confirming,
  actionError,
  onInstall,
  onUninstall,
  onGoManage,
  onDismissJob,
}: McpEntryCardProps) {
  const { t } = useTranslation("tools");
  const { t: tCommon } = useTranslation("common");
  const pkg = entry.package;
  const commitShort = pkg?.commit_sha ? pkg.commit_sha.slice(0, 7) : null;
  const stepLabel =
    job?.status === "running" ? (job.step ? t(`store.mcp_step_${job.step}`) : t("store.mcp_installing")) : null;
  const lastLog = job && job.log.length > 0 ? (job.log[job.log.length - 1] ?? null) : null;

  return (
    <div
      className={cn(
        "flex flex-col rounded-lg border p-4 transition-colors",
        entry.installed ? "border-primary/40 bg-primary/5" : "border-border",
      )}
    >
      <h3 className="truncate text-sm font-semibold">{entry.display_name || entry.name}</h3>
      <p className="mt-1 line-clamp-3 text-xs text-muted-foreground">{entry.description}</p>

      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">{entry.runtime}</span>
        {jobActive ? (
          <span className="rounded-full bg-primary/15 px-2 py-0.5 text-[11px] text-primary">
            {t("store.mcp_installing")}
          </span>
        ) : entry.installed ? (
          <span className="rounded-full bg-primary/15 px-2 py-0.5 text-[11px] text-primary">{t("store.mcp_installed")}</span>
        ) : (
          <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
            {t("store.mcp_not_installed")}
          </span>
        )}
        {entry.ram_note && (
          <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
            {t("store.mcp_ram_note", { note: entry.ram_note })}
          </span>
        )}
        {entry.installed && commitShort && (
          <span className="rounded-full bg-muted px-2 py-0.5 font-mono text-[11px] text-muted-foreground">
            {commitShort}
          </span>
        )}
        {entry.installed && pkg && (
          <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
            {t("store.mcp_tool_count", { count: pkg.tool_count })}
          </span>
        )}
      </div>

      {pkg?.status === "failed" && pkg.error && <p className="mt-2 text-xs text-destructive">{pkg.error}</p>}

      <div className="mt-4 flex items-center gap-2">
        {isAdmin &&
          (entry.installed ? (
            <Button
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={onUninstall}
              className={cn(
                "min-h-11 sm:min-h-9",
                confirming && "border-destructive text-destructive hover:bg-destructive/10",
              )}
              aria-label={t("store.mcp_uninstall")}
            >
              {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Trash2 className="h-3.5 w-3.5" />}
              {confirming ? t("store.mcp_confirm_uninstall") : t("store.mcp_uninstall")}
            </Button>
          ) : (
            <Button
              variant="default"
              size="sm"
              disabled={anyInstalling}
              onClick={onInstall}
              className="min-h-11 sm:min-h-9"
              aria-label={t("store.mcp_install")}
            >
              {jobActive ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Download className="h-3.5 w-3.5" />}
              {jobActive ? t("store.mcp_installing") : t("store.mcp_install")}
            </Button>
          ))}
      </div>

      {job && (
        <div className="mt-3 border-t pt-3">
          {job.status === "running" && (
            <>
              <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                <span className="truncate">{stepLabel}</span>
                <span className="shrink-0 tabular-nums">{job.progress}%</span>
              </div>
              <Progress value={job.progress} className="mt-1.5 h-1.5" />
              {lastLog && (
                <p className="mt-1.5 truncate text-[11px] text-muted-foreground" title={lastLog}>
                  {lastLog}
                </p>
              )}
            </>
          )}
          {job.status === "error" && (
            <p className="text-xs text-destructive">
              {t("store.mcp_job_failed")}
              {job.error ? `: ${job.error}` : ""}
            </p>
          )}
          {job.status === "done" && (
            <div className="flex items-start justify-between gap-2">
              <p className="text-xs text-muted-foreground">{t("store.mcp_manage_hint")}</p>
              <div className="flex shrink-0 items-center">
                <Button variant="link" size="sm" className="min-h-11 px-2 sm:min-h-9" onClick={onGoManage}>
                  {t("store.mcp_go_manage")}
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={tCommon("close")}
                  onClick={onDismissJob}
                  className="h-8 w-8 min-h-8 min-w-8 text-muted-foreground hover:text-foreground sm:h-9 sm:w-9 sm:min-h-9 sm:min-w-9"
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </div>
      )}

      {actionError && <p className="mt-3 text-xs text-destructive">{actionError}</p>}
    </div>
  );
}

interface McpCustomDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: McpCustomInstallInput) => Promise<McpInstallResult>;
}

interface CustomFormState {
  name: string;
  displayName: string;
  repo: string;
  ref: string;
  subdir: string;
  runtime: "node" | "python";
  entry: string;
}

const EMPTY_CUSTOM_FORM: CustomFormState = {
  name: "",
  displayName: "",
  repo: "",
  ref: "",
  subdir: "",
  runtime: "node",
  entry: "",
};

/** "Add from GitHub" dialog — installs a tool server from any repo, pinned to a tag/commit. */
function McpCustomDialog({ open, onOpenChange, onSubmit }: McpCustomDialogProps) {
  const { t } = useTranslation("tools");
  const { t: tCommon } = useTranslation("common");
  const [form, setForm] = useState<CustomFormState>(EMPTY_CUSTOM_FORM);
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  function update<K extends keyof CustomFormState>(key: K, value: CustomFormState[K]) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  function requiredError(label: string): string {
    return `${label}: ${tCommon("required")}`;
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const name = form.name.trim();
    const repo = form.repo.trim();
    const entry = form.entry.trim();
    if (!name) {
      setFormError(requiredError(t("store.mcp_custom_name")));
      return;
    }
    if (!isValidSlug(name)) {
      setFormError(requiredError(t("store.mcp_custom_name")));
      return;
    }
    if (!repo) {
      setFormError(requiredError(t("store.mcp_custom_repo")));
      return;
    }
    if (!entry) {
      setFormError(requiredError(t("store.mcp_custom_entry")));
      return;
    }

    setSubmitting(true);
    setFormError(null);
    try {
      const result = await onSubmit({
        name,
        display_name: form.displayName.trim() || undefined,
        repo,
        ref: form.ref.trim() || undefined,
        subdir: form.subdir.trim() || undefined,
        runtime: form.runtime,
        entry,
      });
      if (result.ok) {
        setForm(EMPTY_CUSTOM_FORM);
        onOpenChange(false);
      } else {
        // 409/400 bodies carry an already-localized message — show it as-is.
        setFormError(result.error);
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !submitting && onOpenChange(v)}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("store.mcp_custom_title")}</DialogTitle>
          <DialogDescription>{t("store.mcp_custom_desc")}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-4" onSubmit={(e) => void handleSubmit(e)} noValidate>
          <div className="grid gap-1.5">
            <Label htmlFor="mcp-custom-name">
              {t("store.mcp_custom_name")} <span className="text-destructive">*</span>
            </Label>
            <Input
              id="mcp-custom-name"
              value={form.name}
              onChange={(e) => update("name", e.target.value)}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="mcp-custom-display">{t("store.mcp_custom_display_name")}</Label>
            <Input
              id="mcp-custom-display"
              value={form.displayName}
              onChange={(e) => update("displayName", e.target.value)}
              autoComplete="off"
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="mcp-custom-repo">
              {t("store.mcp_custom_repo")} <span className="text-destructive">*</span>
            </Label>
            <Input
              id="mcp-custom-repo"
              value={form.repo}
              onChange={(e) => update("repo", e.target.value)}
              placeholder={t("store.mcp_custom_repo_ph")}
              inputMode="url"
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-1.5">
              <Label htmlFor="mcp-custom-ref">{t("store.mcp_custom_ref")}</Label>
              <Input
                id="mcp-custom-ref"
                value={form.ref}
                onChange={(e) => update("ref", e.target.value)}
                placeholder={t("store.mcp_custom_ref_ph")}
                autoComplete="off"
                spellCheck={false}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="mcp-custom-subdir">{t("store.mcp_custom_subdir")}</Label>
              <Input
                id="mcp-custom-subdir"
                value={form.subdir}
                onChange={(e) => update("subdir", e.target.value)}
                placeholder={t("store.mcp_custom_subdir_ph")}
                autoComplete="off"
                spellCheck={false}
              />
            </div>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="mcp-custom-runtime">
              {t("store.mcp_custom_runtime")} <span className="text-destructive">*</span>
            </Label>
            <Select
              value={form.runtime}
              onValueChange={(v) => update("runtime", v === "python" ? "python" : "node")}
            >
              <SelectTrigger id="mcp-custom-runtime" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="node">node</SelectItem>
                <SelectItem value="python">python</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="mcp-custom-entry">
              {t("store.mcp_custom_entry")} <span className="text-destructive">*</span>
            </Label>
            <Input
              id="mcp-custom-entry"
              value={form.entry}
              onChange={(e) => update("entry", e.target.value)}
              placeholder={t("store.mcp_custom_entry_ph")}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          {formError && <p className="text-sm text-destructive">{formError}</p>}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={submitting}
              className="min-h-11 sm:min-h-9"
            >
              {tCommon("cancel")}
            </Button>
            <Button type="submit" disabled={submitting} className="min-h-11 sm:min-h-9">
              {submitting && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t("store.mcp_custom_submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
