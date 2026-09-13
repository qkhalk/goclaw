import { useEffect, useRef, useState } from "react";
import { Navigate, useNavigate, useParams, useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  ArrowLeft,
  Building2,
  ClipboardPaste,
  KeyRound,
  PackageOpen,
  Plus,
  Settings,
  Unplug,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { SettingsSheet } from "./settings-sheet";
import { CLOUD_PROVIDERS } from "./drive/drive-rail";
import { DriveShell } from "./drive/drive-shell";
import { DriveTopBar } from "./drive/drive-topbar";
import { DriveFileArea } from "./drive/drive-file-area";
import {
  SORT_STORAGE_KEY,
  VIEW_MODE_STORAGE_KEY,
  childPath,
  normalizePath,
  type SortSpec,
  type ViewMode,
} from "./drive/paths";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { StatusBadge } from "@/components/shared/status-badge";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { queryKeys } from "@/lib/query-keys";
import { ROUTES } from "@/lib/routes";
import { useAuthStore } from "@/stores/use-auth-store";
import {
  useCloudAccounts,
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

const COMING_SOON_PROVIDERS = ["Dropbox", "Amazon S3 / compatible"];

/** Clouds page — Drive-style shell. Navigation state lives in the URL:
 * /cloud (home) → /cloud/:provider → /cloud/:provider/:accountId?path=…
 * (URL params as source of truth; no duplicate useState for route params). */
export function CloudPage() {
  const { provider, accountId } = useParams();
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { t } = useTranslation("cloud");
  const result = useCloudResult();
  const role = useAuthStore((s) => s.role);
  const userId = useAuthStore((s) => s.userId);
  const isAdmin = role === "admin" || role === "owner";

  const { data: cloudStatus } = useCloudStatus();
  const { accounts, loading, refresh, disconnect, startConnect, completeConnect, setShared } = useCloudAccounts();

  // URL-derived view state (never duplicated into useState).
  const activeProvider: CloudProvider | null =
    provider === "google" || provider === "onedrive" ? provider : null;
  const path = normalizePath(params.get("path"));
  const view: "home" | "provider" | "account" = accountId ? "account" : provider ? "provider" : "home";

  // Ephemeral UI state (intentionally not in the URL).
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState<SortSpec>(() => {
    try {
      const raw = localStorage.getItem(SORT_STORAGE_KEY);
      if (raw) return JSON.parse(raw) as SortSpec;
    } catch {
      /* ignore */
    }
    return { key: "name", dir: "asc" };
  });
  const [viewMode, setViewMode] = useState<ViewMode>(() =>
    localStorage.getItem(VIEW_MODE_STORAGE_KEY) === "list" ? "list" : "grid",
  );
  useEffect(() => {
    localStorage.setItem(VIEW_MODE_STORAGE_KEY, viewMode);
  }, [viewMode]);
  useEffect(() => {
    localStorage.setItem(SORT_STORAGE_KEY, JSON.stringify(sort));
  }, [sort]);

  // Connect flow (home + provider views) — same embedded/paste-back flow.
  const [connecting, setConnecting] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<CloudAccount | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [pasteProvider, setPasteProvider] = useState<CloudProvider | null>(null);
  const [pasteURL, setPasteURL] = useState("");
  const [completing, setCompleting] = useState(false);
  const [pasteError, setPasteError] = useState("");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsProvider, setSettingsProvider] = useState<CloudProvider>("google");

  const account = accountId ? accounts.find((a) => a.id === accountId) : undefined;
  const providerMeta = activeProvider ? CLOUD_PROVIDERS.find((p) => p.id === activeProvider) : null;

  const googleConfigured = cloudStatus?.providers?.google?.configured ?? false;
  const onedriveConfigured = cloudStatus?.providers?.onedrive?.configured ?? false;
  const isConfigured = (p: CloudProvider) => (p === "google" ? googleConfigured : onedriveConfigured);
  const providersReady = CLOUD_PROVIDERS.filter((p) => isConfigured(p.id)).length;
  const activeAccounts = accounts.filter((a) => a.status === "active").length;

  function openSettings() {
    // Default the sheet's provider to whatever view the user is on.
    if (activeProvider) setSettingsProvider(activeProvider);
    setSettingsOpen(true);
  }

  function navigatePath(next: string) {
    setParams({ path: next });
  }

  function openFolder(name: string) {
    navigatePath(childPath(path, name));
  }

  async function handleConnect(p: CloudProvider) {
    setConnecting(true);
    setPasteError("");
    try {
      const res = await startConnect(p);
      if (res.mode === "paste") {
        // Embedded shared client: the consent redirects to a loopback URL
        // nothing is listening on — keep the page alive in this tab and ask
        // the user to paste the address-bar URL back (rclone-style).
        setPasteProvider(p);
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

  function refreshCurrent() {
    if (view === "account" && accountId) {
      void queryClient.invalidateQueries({ queryKey: queryKeys.cloud.allFiles(accountId) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.cloud.about(accountId) });
      return;
    }
    refresh();
  }

  // Invalid provider in the URL → back to the clouds home (no crash).
  if (provider && !activeProvider) {
    return <Navigate to={ROUTES.CLOUD} replace />;
  }

  const gear = (
    <Button variant="ghost" size="icon" onClick={openSettings} aria-label={t("settings.title")} title={t("settings.title")}>
      <Settings className="h-4 w-4" />
    </Button>
  );

  const showPastePanel =
    pasteProvider !== null && (view === "home" || pasteProvider === activeProvider);

  return (
    <DriveShell
      accountId={view === "account" ? accountId : undefined}
      railTitle={t("drive.my_drives")}
      header={
        <DriveTopBar
          path={view === "account" ? path : undefined}
          onNavigatePath={view === "account" ? navigatePath : undefined}
          rootLabel={t("detail.root")}
          title={view === "home" ? t("title") : providerMeta?.name}
          subtitle={view === "home" ? t("description") : undefined}
          showTools={view === "account"}
          search={search}
          onSearchChange={setSearch}
          sort={sort}
          onSortChange={setSort}
          viewMode={viewMode}
          onViewModeChange={setViewMode}
          onRefresh={refreshCurrent}
          right={gear}
        />
      }
    >
      {view === "home" && (
        <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
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
                {providersReady} / {CLOUD_PROVIDERS.length}
              </p>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {CLOUD_PROVIDERS.map((p) => (
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

          {showPastePanel && (
            <PasteBackPanel
              pasteURL={pasteURL}
              setPasteURL={setPasteURL}
              completing={completing}
              pasteError={pasteError}
              onComplete={handleComplete}
            />
          )}

          {/* My drives: every connected account as a clickable drive card */}
          <div>
            <p className="text-sm font-medium">{t("drive.my_drives")}</p>
            {loading ? (
              <div className="mt-3">
                <TableSkeleton rows={2} />
              </div>
            ) : accounts.length === 0 ? (
              <div className="mt-3">
                <EmptyState
                  icon={PackageOpen}
                  title={t("empty.title")}
                  description={t("empty.description")}
                />
              </div>
            ) : (
              <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
                {accounts.map((a) => (
                  <AccountCard
                    key={a.id}
                    account={a}
                    isAdmin={isAdmin}
                    userId={userId}
                    connecting={connecting}
                    onOpen={() => navigate(`/cloud/${a.provider}/${a.id}`)}
                    onRegrant={() => handleConnect(a.provider as CloudProvider)}
                    onSharedChange={(v) => void setShared(a.id, v)}
                    onDisconnect={() => setDeleteTarget(a)}
                  />
                ))}
              </div>
            )}
          </div>

          {/* Provider picker */}
          <div>
            <p className="text-sm font-medium">{t("picker.title")}</p>
            <p className="mt-1 text-sm text-muted-foreground">{t("picker.description")}</p>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {CLOUD_PROVIDERS.map((p) => {
              const configured = isConfigured(p.id);
              const count = accounts.filter((a) => a.provider === p.id).length;
              return (
                <button
                  key={p.id}
                  type="button"
                  onClick={() => navigate(`/cloud/${p.id}`)}
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
        </div>
      )}

      {view === "provider" && activeProvider && (
        <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
          <div>
            <Button variant="ghost" size="sm" className="-ml-2" onClick={() => navigate(ROUTES.CLOUD)}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              {t("provider.back")}
            </Button>
          </div>

          {showPastePanel && (
            <PasteBackPanel
              pasteURL={pasteURL}
              setPasteURL={setPasteURL}
              completing={completing}
              pasteError={pasteError}
              onComplete={handleComplete}
            />
          )}

          <div>
            <div className="flex items-center justify-between gap-2">
              <p className="text-sm font-medium">{t("provider.accounts_title")}</p>
              <Button
                size="sm"
                onClick={() => handleConnect(activeProvider)}
                disabled={connecting}
                className="min-h-11 sm:min-h-9"
              >
                <Plus className="mr-2 h-4 w-4" />
                {t(`connect.${activeProvider}`)}
              </Button>
            </div>
            <p className="-mt-3 text-xs text-muted-foreground">{t("setup.embedded_note")}</p>
            {loading ? (
              <div className="mt-3">
                <TableSkeleton rows={2} />
              </div>
            ) : providerAccounts(accounts, activeProvider).length === 0 ? (
              <div className="mt-3">
                <EmptyState
                  icon={PackageOpen}
                  title={t("empty.title")}
                  description={t("empty.description")}
                />
              </div>
            ) : (
              <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
                {providerAccounts(accounts, activeProvider).map((a) => (
                  <AccountCard
                    key={a.id}
                    account={a}
                    isAdmin={isAdmin}
                    userId={userId}
                    connecting={connecting}
                    onOpen={() => navigate(`/cloud/${a.provider}/${a.id}`)}
                    onRegrant={() => handleConnect(a.provider as CloudProvider)}
                    onSharedChange={(v) => void setShared(a.id, v)}
                    onDisconnect={() => setDeleteTarget(a)}
                  />
                ))}
                <Button
                  variant="outline"
                  className="min-h-11 border-dashed"
                  onClick={() => handleConnect(activeProvider)}
                  disabled={connecting}
                >
                  <Plus className="mr-2 h-4 w-4" />
                  {t("connect.another")}
                </Button>
              </div>
            )}
          </div>
        </div>
      )}

      {view === "account" && accountId && (
        <div className="mx-auto w-full max-w-6xl">
          {loading ? (
            <div className="p-4">
              <TableSkeleton rows={4} />
            </div>
          ) : !account || account.provider !== activeProvider ? (
            <div className="p-6">
              <EmptyState
                icon={PackageOpen}
                title={t("drive.not_found")}
                action={
                  <Button variant="outline" size="sm" onClick={() => navigate(ROUTES.CLOUD)}>
                    <ArrowLeft className="mr-2 h-4 w-4" />
                    {t("drive.back_home")}
                  </Button>
                }
              />
            </div>
          ) : (
            <DriveFileArea
              accountId={accountId}
              provider={account.provider}
              path={path}
              onOpenFolder={(entry) => openFolder(entry.name)}
              search={search}
              sort={sort}
              viewMode={viewMode}
            />
          )}
        </div>
      )}

      <SettingsSheet
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        provider={settingsProvider}
        onProviderChange={setSettingsProvider}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t("disconnect_confirm.title")}
        description={t("disconnect_confirm.description", { email: deleteTarget?.email ?? "" })}
        confirmLabel={t("disconnect")}
        loading={deleteLoading}
        onConfirm={handleDisconnect}
      />
    </DriveShell>
  );
}

function providerAccounts(accounts: CloudAccount[], provider: CloudProvider): CloudAccount[] {
  return accounts.filter((a) => a.provider === provider);
}

/** Paste-back panel for the embedded shared client flow. */
function PasteBackPanel({
  pasteURL,
  setPasteURL,
  completing,
  pasteError,
  onComplete,
}: {
  pasteURL: string;
  setPasteURL: (v: string) => void;
  completing: boolean;
  pasteError: string;
  onComplete: () => void;
}) {
  const { t } = useTranslation("cloud");
  return (
    <div className="space-y-3 rounded-lg border border-amber-500/30 bg-amber-500/5 p-4 text-sm">
      <p className="font-medium">{t("paste.title")}</p>
      <p className="text-muted-foreground">{t("paste.description")}</p>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input
          value={pasteURL}
          onChange={(e) => setPasteURL(e.target.value)}
          placeholder={t("paste.placeholder")}
          className="flex-1 text-base md:text-sm"
          autoComplete="off"
        />
        <Button
          size="sm"
          onClick={onComplete}
          disabled={completing || !pasteURL.trim()}
          className="min-h-11 shrink-0 sm:min-h-9"
        >
          <ClipboardPaste className="mr-2 h-4 w-4" />
          {completing ? t("paste.completing") : t("paste.complete")}
        </Button>
      </div>
      {pasteError && <p className="text-xs text-destructive">{pasteError}</p>}
    </div>
  );
}

/** Clickable drive card for one account (home + provider views). */
function AccountCard({
  account,
  isAdmin,
  userId,
  connecting,
  onOpen,
  onRegrant,
  onSharedChange,
  onDisconnect,
}: {
  account: CloudAccount;
  isAdmin: boolean;
  userId: string;
  connecting: boolean;
  onOpen: () => void;
  onRegrant: () => void;
  onSharedChange: (v: boolean) => void;
  onDisconnect: () => void;
}) {
  const { t } = useTranslation("cloud");
  const canRegrant = !account.shared || account.user_id === userId;

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") onOpen();
      }}
      className="flex cursor-pointer flex-col gap-3 rounded-lg border p-4 text-left transition-colors hover:bg-muted/40"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate font-medium">{account.email}</p>
          <p className="truncate text-sm text-muted-foreground">
            {account.display_name || account.provider}
          </p>
        </div>
        <div className="flex shrink-0 flex-col items-end gap-1">
          {statusBadge(account.status, t(`status.${account.status}`))}
          {account.shared && (
            <Badge variant="outline" className="text-muted-foreground">
              <Building2 className="mr-1 h-3 w-3" />
              {t("drive.shared_tag")}
            </Badge>
          )}
          {account.can_write === false && <Badge variant="warning">{t("readonly_badge")}</Badge>}
        </div>
      </div>
      {account.status_message && (
        <p className="text-xs text-muted-foreground">{account.status_message}</p>
      )}
      {account.can_write === false && canRegrant && (
        <div className="rounded-md border border-amber-500/30 bg-amber-500/5 p-3 text-xs text-muted-foreground">
          <p>{t("regrant_hint")}</p>
          <Button
            variant="outline"
            size="sm"
            className="mt-2 min-h-11 sm:min-h-9"
            disabled={connecting}
            onClick={(e) => {
              e.stopPropagation();
              onRegrant();
            }}
          >
            <KeyRound className="mr-2 h-4 w-4" />
            {t("regrant")}
          </Button>
        </div>
      )}
      {isAdmin && (
        <label
          className="flex items-center justify-between gap-2 text-xs text-muted-foreground"
          onClick={(e) => e.stopPropagation()}
        >
          <span>{t("share.toggle")}</span>
          <Switch checked={account.shared} onCheckedChange={onSharedChange} />
        </label>
      )}
      <div className="mt-auto flex items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">{account.provider}</span>
        <Button
          variant="ghost"
          size="sm"
          className="min-h-11 text-destructive hover:text-destructive"
          onClick={(e) => {
            e.stopPropagation();
            onDisconnect();
          }}
        >
          <Unplug className="mr-2 h-4 w-4" />
          {t("disconnect")}
        </Button>
      </div>
    </div>
  );
}
