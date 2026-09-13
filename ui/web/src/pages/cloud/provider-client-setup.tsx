import { useState } from "react";
import { useTranslation } from "react-i18next";
import { CheckCircle2, Copy, Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuthStore } from "@/stores/use-auth-store";
import { useClipboard } from "@/hooks/use-clipboard";
import { useCloudSettings, type CloudProvider } from "./hooks/use-cloud";

/** Per-provider admin setup card: one-time OAuth client registration with a
 * step-by-step guide. Hidden for good after a successful save; a discreet
 * pencil re-opens it for rotation. Rendered inside the Cloud settings sheet. */
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
