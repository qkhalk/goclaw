import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import { CloudCog, Copy, Pencil, Plus, RefreshCw, Unplug } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { StatusBadge } from "@/components/shared/status-badge";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { useAuthStore } from "@/stores/use-auth-store";
import {
  useCloudAccounts,
  useCloudSettings,
  useCloudStatus,
  type CloudAccount,
} from "./hooks/use-cloud";

/** One-shot ?connected=/&error= result: returns it for a banner and strips it
 * from the URL so refresh never replays. */
function useCloudResult(): { connected: string; error: string } {
  const [params, setParams] = useSearchParams();
  const consumed = useRef(false);
  const [result, setResult] = useState({ connected: "", error: "" });
  useEffect(() => {
    if (consumed.current) return;
    consumed.current = true;
    const connected = params.get("connected") ?? "";
    const error = params.get("error") ?? "";
    if (connected || error) {
      setResult({ connected, error });
      setParams({}, { replace: true });
    }
  }, [params, setParams]);
  return result;
}

function statusBadge(status: CloudAccount["status"], label: string) {
  switch (status) {
    case "active":
      return <StatusBadge status="success" label={label} />;
    case "expired":
    case "revoked":
      return <StatusBadge status="warning" label={label} />;
    default:
      return <StatusBadge status="error" label={label} />;
  }
}

/** First-run setup card (admins only): save the Google OAuth client once —
 * after a successful save it disappears for good (credentials live in the
 * encrypted secrets store). A discreet pencil re-opens it for rotation. */
function GoogleClientSetup() {
  const { t } = useTranslation("cloud");
  const role = useAuthStore((s) => s.role);
  const isAdmin = role === "admin" || role === "owner";
  const { settings, saveSettings } = useCloudSettings(isAdmin);
  const [clientID, setClientID] = useState("");
  const [secret, setSecret] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(false);

  if (!isAdmin) return null;

  // Already configured and not explicitly editing → hidden for good.
  if (settings?.secret_set && !editing) {
    return (
      <div className="flex justify-end">
        <Button variant="ghost" size="sm" onClick={() => { setClientID(settings.client_id); setEditing(true); }}>
          <Pencil className="mr-2 h-3.5 w-3.5" />
          {t("setup.update")}
        </Button>
      </div>
    );
  }

  async function handleSave() {
    setSaving(true);
    setError("");
    try {
      await saveSettings(clientID.trim(), secret.trim());
      setEditing(false);
      setSecret("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  }

  const configuredOnce = settings?.secret_set ?? false;

  return (
    <div className="rounded-lg border p-4 text-sm">
      <p className="font-medium">{t("setup.title")}</p>
      <p className="mt-1 text-muted-foreground">{t("setup.body")}</p>
      <ol className="mt-2 list-decimal space-y-1 pl-5 text-muted-foreground">
        <li>{t("setup.step1")}</li>
        <li>{t("setup.step2")}</li>
        <li>{t("setup.step3")}</li>
        <li>
          {t("setup.step4")}{" "}
          <span className="inline-flex items-center gap-1">
            <code className="rounded bg-muted px-1 py-0.5 text-xs break-all">
              {settings?.redirect_uri ?? ""}
            </code>
            {settings?.redirect_uri && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-1"
                onClick={() => navigator.clipboard?.writeText(settings.redirect_uri)}
              >
                <Copy className="h-3.5 w-3.5" />
              </Button>
            )}
          </span>
        </li>
      </ol>
      <div className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
        <Input
          value={clientID || settings?.client_id || ""}
          onChange={(e) => setClientID(e.target.value)}
          placeholder={t("setup.client_id")}
          className="text-base md:text-sm"
          autoComplete="off"
        />
        <Input
          type="password"
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
          placeholder={configuredOnce ? t("setup.client_secret_keep") : t("setup.client_secret")}
          className="text-base md:text-sm"
          autoComplete="new-password"
        />
      </div>
      {error && <p className="mt-2 text-xs text-destructive">{error}</p>}
      <div className="mt-3 flex items-center justify-end gap-2">
        {configuredOnce && (
          <Button variant="ghost" size="sm" onClick={() => setEditing(false)}>
            {t("setup.cancel")}
          </Button>
        )}
        <Button size="sm" onClick={handleSave} disabled={saving || !(clientID.trim() || configuredOnce)} className="min-h-11 sm:min-h-9">
          {saving ? t("setup.saving") : t("setup.save")}
        </Button>
      </div>
    </div>
  );
}

/** rclone-style backend catalog. Google is live today (OAuth + Gmail + Drive);
 * the rest share the same rclone engine once their per-provider OAuth apps land. */
const STORAGE_PROVIDERS: { name: string; live?: boolean }[] = [
  { name: "Google Drive", live: true },
  { name: "Microsoft OneDrive" },
  { name: "Dropbox" },
  { name: "Amazon S3 / compatible" },
];

export function CloudPage() {
  const { t } = useTranslation("cloud");
  const result = useCloudResult();

  const { data: cloudStatus } = useCloudStatus();
  const { accounts, loading, refresh, disconnect, startConnect } = useCloudAccounts();
  const [connecting, setConnecting] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<CloudAccount | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const googleConfigured = cloudStatus?.providers?.google?.configured ?? false;
  const cloudEnabled = cloudStatus?.enabled ?? false;

  async function handleConnect() {
    setConnecting(true);
    try {
      const res = await startConnect("google");
      window.location.href = res.auth_url;
    } catch {
      setConnecting(false);
    }
  }

  async function handleDisconnect() {
    if (!deleteTarget) return;
    setDeleteLoading(true);
    try {
      await disconnect(deleteTarget.id);
    } finally {
      setDeleteLoading(false);
      setDeleteTarget(null);
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          <Button variant="outline" size="sm" onClick={() => refresh()}>
            <RefreshCw className="mr-2 h-4 w-4" />
            {t("refresh")}
          </Button>
        }
      />

      {(result.connected || result.error) && (
        <div
          className={
            result.connected
              ? "rounded-lg border border-green-500/40 bg-green-500/10 p-3 text-sm"
              : "rounded-lg border border-red-500/40 bg-red-500/10 p-3 text-sm"
          }
        >
          {result.connected
            ? t("result.connected", { email: result.connected })
            : t("result.error", { code: result.error })}
        </div>
      )}

      {!googleConfigured && <GoogleClientSetup />}

      {loading ? (
        <TableSkeleton rows={2} />
      ) : accounts.length === 0 ? (
        <EmptyState
          icon={CloudCog}
          title={t("empty.title")}
          description={t("empty.description")}
          action={
            googleConfigured && cloudEnabled ? (
              <Button onClick={handleConnect} disabled={connecting} className="min-h-11">
                <Plus className="mr-2 h-4 w-4" />
                {t("connect.google")}
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          {accounts.map((account) => (
            <div
              key={account.id}
              className="flex flex-col gap-3 rounded-lg border p-4"
            >
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <p className="truncate font-medium">{account.email}</p>
                  <p className="truncate text-sm text-muted-foreground">
                    {account.display_name || account.provider}
                  </p>
                </div>
                {statusBadge(account.status, t(`status.${account.status}`))}
              </div>
              {account.status_message && (
                <p className="text-xs text-muted-foreground">{account.status_message}</p>
              )}
              <div className="mt-auto flex items-center justify-between gap-2">
                <span className="text-xs text-muted-foreground">{account.provider}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="min-h-11 text-destructive hover:text-destructive"
                  onClick={() => setDeleteTarget(account)}
                >
                  <Unplug className="mr-2 h-4 w-4" />
                  {t("disconnect")}
                </Button>
              </div>
            </div>
          ))}
          {googleConfigured && cloudEnabled && (
            <Button
              variant="outline"
              className="min-h-11 border-dashed"
              onClick={handleConnect}
              disabled={connecting}
            >
              <Plus className="mr-2 h-4 w-4" />
              {t("connect.another")}
            </Button>
          )}
        </div>
      )}

      <div className="rounded-lg border p-4">
        <p className="text-sm font-medium">{t("providers_section.title")}</p>
        <div className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
          {STORAGE_PROVIDERS.map((p) => (
            <div
              key={p.name}
              className="flex items-center justify-between gap-2 rounded-md border bg-muted/30 px-3 py-2"
            >
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{p.name}</p>
                {p.live && (
                  <p className="truncate text-xs text-muted-foreground">
                    {t("providers_section.google_desc")}
                  </p>
                )}
              </div>
              {p.live ? (
                <Badge variant="outline" className="shrink-0 border-green-500/40 text-green-600">
                  {t("providers_section.available")}
                </Badge>
              ) : (
                <Badge variant="outline" className="shrink-0 text-muted-foreground">
                  {t("providers_section.coming_soon")}
                </Badge>
              )}
            </div>
          ))}
        </div>
        <p className="mt-3 text-xs text-muted-foreground">{t("providers_section.more_via_rclone")}</p>
      </div>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t("disconnect_confirm.title")}
        description={t("disconnect_confirm.description", { email: deleteTarget?.email ?? "" })}
        confirmLabel={t("disconnect")}
        loading={deleteLoading}
        onConfirm={handleDisconnect}
      />
    </div>
  );
}
