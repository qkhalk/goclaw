import { useState } from "react";
import { useTranslation } from "react-i18next";
import { AlertCircle, CheckCircle2, ChevronDown, ChevronUp, Copy, Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuthStore } from "@/stores/use-auth-store";
import { useClipboard } from "@/hooks/use-clipboard";
import { useCloudSettings, type CloudProvider } from "./hooks/use-cloud";

/** Per-provider admin setup card: one-time OAuth client registration with a
 * step-by-step guide. Collapsed to a single status row by default — amber
 * "not configured yet" row before the first save (click to expand), slim
 * emerald "saved" row afterwards with a discreet pencil to re-open for
 * rotation. Rendered inside the Cloud settings sheet. */
export function ProviderClientSetup({ provider }: { provider: CloudProvider }) {
  const { t } = useTranslation("cloud");
  const role = useAuthStore((s) => s.role);
  const isAdmin = role === "admin" || role === "owner";
  const { settings, saveSettings } = useCloudSettings(provider, isAdmin);
  const [clientID, setClientID] = useState("");
  const [secret, setSecret] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const { copied, copy } = useClipboard();

  if (!isAdmin) return null;

  const redirectUri = settings?.redirect_uri ?? "";
  const configuredOnce = settings?.secret_set ?? false;
  const title = t(
    provider === "google"
      ? "setup.title_google"
      : provider === "onedrive"
        ? "setup.title_onedrive"
        : "setup.title_dropbox",
  );

  // Configured: slim emerald status row — the banner never comes back.
  if (configuredOnce && !editing) {
    return (
      <div className="flex items-center justify-between gap-2 rounded-lg border px-3 py-1.5 text-sm">
        <span className="flex min-w-0 items-center gap-2 text-muted-foreground">
          <CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-500" />
          <span className="truncate">
            {t("setup.configured")}
            {settings?.client_id && (
              <code className="ml-1.5 rounded bg-muted px-1 py-0.5 text-xs">{settings.client_id}</code>
            )}
          </span>
        </span>
        <Button
          variant="ghost"
          size="sm"
          className="shrink-0"
          onClick={() => { setClientID(settings?.client_id ?? ""); setEditing(true); setExpanded(true); }}
        >
          <Pencil className="mr-2 h-3.5 w-3.5" />
          {t("setup.update")}
        </Button>
      </div>
    );
  }

  // Not configured: collapsed amber summary — one line until expanded.
  if (!configuredOnce && !expanded) {
    return (
      <button
        type="button"
        onClick={() => setExpanded(true)}
        className="flex w-full items-center justify-between gap-2 rounded-lg border border-amber-500/40 bg-amber-500/5 px-3 py-2 text-sm transition-colors hover:bg-amber-500/10"
      >
        <span className="flex min-w-0 items-center gap-2">
          <AlertCircle className="h-4 w-4 shrink-0 text-amber-500" />
          <span className="truncate font-medium">{title}</span>
        </span>
        <span className="flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
          {t("setup.expand")}
          <ChevronDown className="h-3.5 w-3.5" />
        </span>
      </button>
    );
  }

  async function handleSave() {
    setSaving(true);
    setError("");
    try {
      await saveSettings(clientID.trim(), secret.trim());
      setEditing(false);
      setExpanded(false);
      setSecret("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  }

  const stepKey = (n: number) => t(`setup.${provider}_step${n}`);

  return (
    <div className="rounded-lg border p-4 text-sm">
      <button
        type="button"
        onClick={() => { if (!configuredOnce) { setExpanded(false); setEditing(false); } else { setEditing(false); } }}
        className="-mt-1 flex w-full items-center justify-between gap-2 text-left"
      >
        <p className="font-medium">{title}</p>
        <ChevronUp className="h-4 w-4 shrink-0 text-muted-foreground" />
      </button>
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
        <Button variant="ghost" size="sm" onClick={() => { setEditing(false); setExpanded(false); }}>
          {t("setup.cancel")}
        </Button>
        <Button size="sm" onClick={handleSave} disabled={saving || !(clientID.trim() || configuredOnce)} className="min-h-11 sm:min-h-9">
          {saving ? t("setup.saving") : t("setup.save")}
        </Button>
      </div>
    </div>
  );
}
