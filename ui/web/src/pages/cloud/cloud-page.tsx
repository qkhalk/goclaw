import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { useTranslation } from "react-i18next";
import {
  ArrowLeft,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Cloud,
  ClipboardPaste,
  Copy,
  HardDrive,
  PackageOpen,
  Pencil,
  Plus,
  RefreshCw,
  Unplug,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { ScopeBindingsPanel } from "./scope-bindings-panel";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { StatusBadge } from "@/components/shared/status-badge";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { useAuthStore } from "@/stores/use-auth-store";
import { useClipboard } from "@/hooks/use-clipboard";
import {
  useCloudAccounts,
  useCloudSettings,
  useCloudStatus,
  type CloudAccount,
  type CloudProvider,
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

/** Connectable providers (backend mirror: cloud.SupportedProviders) plus the
 * rclone-style coming-soon entries. */
const LIVE_PROVIDERS: { id: CloudProvider; name: string; icon: typeof Cloud }[] = [
  { id: "google", name: "Google Drive", icon: Cloud },
  { id: "onedrive", name: "Microsoft OneDrive", icon: HardDrive },
];
const COMING_SOON_PROVIDERS = ["Dropbox", "Amazon S3 / compatible"];

/** Per-provider admin setup card: one-time OAuth client registration with a
 * step-by-step guide. Hidden for good after a successful save; a discreet
 * pencil re-opens it for rotation. */
function ProviderClientSetup({ provider }: { provider: CloudProvider }) {
  const { t } = useTranslation("cloud");
  const role = useAuthStore((s) => s.role);
  const isAdmin = role === "admin" || role === "owner";
  const { settings, saveSettings } = useCloudSettings(provider, isAdmin);
  const [clientID, setClientID] = useState("");
  const [secret, setSecret] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(false);
  const { copied, copy } = useClipboard();

  if (!isAdmin) return null;

  const redirectUri = settings?.redirect_uri ?? "";

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
  const stepKey = (n: number) => t(`setup.${provider}_step${n}`);

  return (
    <div className="rounded-lg border p-4 text-sm">
      <p className="font-medium">{t(provider === "google" ? "setup.title_google" : "setup.title_onedrive")}</p>
      <p className="mt-1 text-muted-foreground">{t("setup.body")}</p>
      <ol className="mt-2 list-decimal space-y-1 pl-5 text-muted-foreground">
        <li>{stepKey(1)}</li>
        <li>{stepKey(2)}</li>
        <li>{stepKey(3)}</li>
        <li>
          {stepKey(4)}{" "}
          <span className="inline-flex items-center gap-1">
            <code className="rounded bg-muted px-1 py-0.5 text-xs break-all">{redirectUri}</code>
            {redirectUri && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-1"
                onClick={() => void copy(redirectUri)}
              >
                {copied ? <CheckCircle2 className="h-3.5 w-3.5 text-green-600" /> : <Copy className="h-3.5 w-3.5" />}
              </Button>
            )}
          </span>
        </li>
      </ol>
      <div className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
        <Input
          value={clientID || settings?.client_id || ""}
          onChange={(e) => setClientID(e.target.value)}
          placeholder={t(`setup.${provider}_client_id`)}
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

export function CloudPage() {
  const { t } = useTranslation("cloud");
  const result = useCloudResult();
  const role = useAuthStore((s) => s.role);
  const isAdmin = role === "admin" || role === "owner";

  const { data: cloudStatus } = useCloudStatus();
  const { accounts, loading, refresh, disconnect, startConnect, completeConnect, setShared } = useCloudAccounts();
  const [selectedProvider, setSelectedProvider] = useState<CloudProvider | null>(null);
  const [connecting, setConnecting] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<CloudAccount | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [pasteProvider, setPasteProvider] = useState<CloudProvider | null>(null);
  const [pasteURL, setPasteURL] = useState("");
  const [completing, setCompleting] = useState(false);
  const [pasteError, setPasteError] = useState("");
  const [showByoSetup, setShowByoSetup] = useState(false);

  const googleConfigured = cloudStatus?.providers?.google?.configured ?? false;
  const onedriveConfigured = cloudStatus?.providers?.onedrive?.configured ?? false;
  const isConfigured = (p: CloudProvider) => (p === "google" ? googleConfigured : onedriveConfigured);
  const providersReady = LIVE_PROVIDERS.filter((p) => isConfigured(p.id)).length;
  const activeAccounts = accounts.filter((a) => a.status === "active").length;

  const providerAccounts = selectedProvider
    ? accounts.filter((a) => a.provider === selectedProvider)
    : [];

  async function handleConnect(provider: CloudProvider) {
    setConnecting(true);
    setPasteError("");
    try {
      const res = await startConnect(provider);
      if (res.mode === "paste") {
        // Embedded shared client: the consent redirects to a loopback URL
        // nothing is listening on — keep the page alive in this tab and ask
        // the user to paste the address-bar URL back (rclone-style).
        setPasteProvider(provider);
        setPasteURL("");
        window.open(res.auth_url, "_blank", "noopener,noreferrer");
      } else {
        window.location.href = res.auth_url;
      }
    } catch {
      setPasteError("");
    } finally {
      setConnecting(false);
    }
  }

  async function handleComplete() {
    if (!pasteProvider || !pasteURL.trim()) return;
    setCompleting(true);
    setPasteError("");
    try {
      await completeConnect(pasteProvider, pasteURL.trim());
      setPasteProvider(null);
      setPasteURL("");
    } catch (e) {
      setPasteError(e instanceof Error ? e.message : String(e));
    } finally {
      setCompleting(false);
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

      {/* Dashboard tổng: aggregate across all providers */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <div className="rounded-lg border p-4">
          <p className="text-sm text-muted-foreground">{t("dashboard.accounts")}</p>
          <p className="mt-1 text-2xl font-semibold tabular-nums">{accounts.length}</p>
        </div>
        <div className="rounded-lg border p-4">
          <p className="text-sm text-muted-foreground">{t("dashboard.active")}</p>
          <p className="mt-1 text-2xl font-semibold tabular-nums">{activeAccounts}</p>
        </div>
        <div className="rounded-lg border p-4">
          <p className="text-sm text-muted-foreground">{t("dashboard.providers_ready")}</p>
          <p className="mt-1 text-2xl font-semibold tabular-nums">
            {providersReady} / {LIVE_PROVIDERS.length}
          </p>
          <div className="mt-2 flex flex-wrap gap-1.5">
            {LIVE_PROVIDERS.map((p) => (
              <Badge
                key={p.id}
                variant="outline"
                className={
                  isConfigured(p.id)
                    ? "border-green-500/40 text-green-600"
                    : "text-muted-foreground"
                }
              >
                {p.name}
              </Badge>
            ))}
          </div>
        </div>
      </div>

      {loading && !cloudStatus ? (
        <TableSkeleton rows={2} />
      ) : !selectedProvider ? (
        <>
          {/* Provider picker: choose the storage provider first */}
          <div>
            <p className="text-sm font-medium">{t("picker.title")}</p>
            <p className="mt-1 text-sm text-muted-foreground">{t("picker.description")}</p>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {LIVE_PROVIDERS.map((p) => {
              const configured = isConfigured(p.id);
              const count = accounts.filter((a) => a.provider === p.id).length;
              return (
                <button
                  key={p.id}
                  type="button"
                  onClick={() => setSelectedProvider(p.id)}
                  className="flex flex-col items-start gap-2 rounded-lg border p-4 text-left transition-colors hover:bg-muted/40"
                >
                  <div className="flex w-full items-start justify-between gap-2">
                    <p.icon className="h-5 w-5 shrink-0" />
                    <Badge
                      variant="outline"
                      className={
                        configured
                          ? "shrink-0 border-green-500/40 text-green-600"
                          : "shrink-0 text-muted-foreground"
                      }
                    >
                      {configured ? t("picker.available") : t("dashboard.needs_setup")}
                    </Badge>
                  </div>
                  <p className="font-medium">{p.name}</p>
                  <p className="text-xs text-muted-foreground">{t(`picker.${p.id}_desc`)}</p>
                  {count > 0 && (
                    <p className="text-xs text-muted-foreground">
                      {t("dashboard.accounts")}: {count}
                    </p>
                  )}
                </button>
              );
            })}
            {COMING_SOON_PROVIDERS.map((name) => (
              <div
                key={name}
                className="flex items-center justify-between gap-2 rounded-lg border bg-muted/30 p-4 opacity-70"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{name}</p>
                </div>
                <Badge variant="outline" className="shrink-0 text-muted-foreground">
                  {t("picker.coming_soon")}
                </Badge>
              </div>
            ))}
          </div>
          <p className="text-xs text-muted-foreground">{t("picker.more_via_rclone")}</p>
        </>
      ) : (
        <>
          {/* Per-provider view: setup guide first, then connect + accounts */}
          <div>
            <Button variant="ghost" size="sm" className="-ml-2" onClick={() => setSelectedProvider(null)}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              {t("provider.back")}
            </Button>
          </div>

          {/* Paste-back panel for the embedded shared client flow */}
          {pasteProvider === selectedProvider && (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-4 text-sm space-y-3">
              <p className="font-medium">{t("paste.title")}</p>
              <p className="text-muted-foreground">{t("paste.description")}</p>
              <div className="flex flex-col gap-2 sm:flex-row">
                <Input
                  value={pasteURL}
                  onChange={(e) => setPasteURL(e.target.value)}
                  placeholder={t("paste.placeholder")}
                  className="text-base md:text-sm flex-1"
                  autoComplete="off"
                />
                <Button size="sm" onClick={handleComplete} disabled={completing || !pasteURL.trim()} className="min-h-11 sm:min-h-9 shrink-0">
                  <ClipboardPaste className="mr-2 h-4 w-4" />
                  {completing ? t("paste.completing") : t("paste.complete")}
                </Button>
              </div>
              {pasteError && <p className="text-xs text-destructive">{pasteError}</p>}
            </div>
          )}

          {/* Per-scope account bindings (admin): tenant default / group / user */}
          {isAdmin && <ScopeBindingsPanel provider={selectedProvider} />}

          {(
            <>
              <div className="flex items-center justify-between gap-2">
                <p className="text-sm font-medium">{t("provider.accounts_title")}</p>
                <Button
                  size="sm"
                  onClick={() => handleConnect(selectedProvider)}
                  disabled={connecting}
                  className="min-h-11 sm:min-h-9"
                >
                  <Plus className="mr-2 h-4 w-4" />
                  {t(`connect.${selectedProvider}`)}
                </Button>
              </div>
              <p className="-mt-3 text-xs text-muted-foreground">{t("setup.embedded_note")}</p>
              {providerAccounts.length === 0 ? (
                <EmptyState
                  icon={PackageOpen}
                  title={t("empty.title")}
                  description={t("empty.description")}
                />
              ) : (
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  {providerAccounts.map((account) => (
                    <div key={account.id} className="flex flex-col gap-3 rounded-lg border p-4">
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
                      {isAdmin && (
                        <label className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                          <span>{t("share.toggle")}</span>
                          <Switch
                            checked={account.shared}
                            onCheckedChange={(v) => void setShared(account.id, v)}
                          />
                        </label>
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
                  <Button
                    variant="outline"
                    className="min-h-11 border-dashed"
                    onClick={() => handleConnect(selectedProvider)}
                    disabled={connecting}
                  >
                    <Plus className="mr-2 h-4 w-4" />
                    {t("connect.another")}
                  </Button>
                </div>
              )}

              {/* Advanced: BYO OAuth client (branding / quota / Gmail) */}
              <div className="rounded-lg border">
                <button
                  type="button"
                  className="flex w-full items-center gap-2 px-4 py-3 text-left text-sm font-medium"
                  onClick={() => setShowByoSetup((v) => !v)}
                >
                  {showByoSetup ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                  {t("setup.advanced")}
                </button>
                {showByoSetup && (
                  <div className="border-t p-4">
                    <ProviderClientSetup provider={selectedProvider} />
                  </div>
                )}
              </div>
            </>
          )}
        </>
      )}

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
