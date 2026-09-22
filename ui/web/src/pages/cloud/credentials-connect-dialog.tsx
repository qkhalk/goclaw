import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { KeyRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { toast } from "@/stores/use-toast-store";
import {
  CREDENTIAL_PROVIDER_FIELDS,
  useCloudAccounts,
  type CredentialProviderId,
} from "./hooks/use-cloud";

/** Connect dialog for credential-based providers (S3, B2, pCloud, WebDAV,
 * Azure Blob, GCS, FTP, SFTP, SMB): fields render dynamically from the
 * frontend copy of the registry specs (CREDENTIAL_PROVIDER_FIELDS — keep in
 * sync with internal/cloud/providers.go). secret_textarea fields are
 * multi-line secrets (PEM key, service-account JSON) rendered as a textarea.
 * Submitting calls POST /v1/cloud/connect; the server probes the credentials
 * with rclone BEFORE persisting, so a failure here means the credentials did
 * not work (rclone's own error text is shown). Full-screen slide-up on
 * mobile, centered on desktop (ui/dialog.tsx convention). Inputs are 16px on
 * mobile (text-base md:text-sm) to avoid iOS auto-zoom. */
export function CredentialsConnectDialog({
  open,
  onOpenChange,
  provider,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider: CredentialProviderId;
}) {
  const { t } = useTranslation("cloud");
  const { connectCredentials } = useCloudAccounts();
  const fields = CREDENTIAL_PROVIDER_FIELDS[provider];

  const [displayName, setDisplayName] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  // Reset the form whenever the dialog opens for a (different) provider.
  useEffect(() => {
    if (open) {
      setDisplayName("");
      setValues({});
      setError("");
    }
  }, [open, provider]);

  const providerLabel = t(`credentials.providers.${provider}`);
  const missingRequired = fields.some(
    (f) => f.required && !(values[f.key] ?? "").trim(),
  );

  async function handleSubmit() {
    setSubmitting(true);
    setError("");
    // Trim + drop empty optionals — the backend re-validates everything
    // against its own whitelist anyway.
    const params: Record<string, string> = {};
    for (const f of fields) {
      const v = (values[f.key] ?? "").trim();
      if (v) params[f.key] = v;
    }
    try {
      const res = await connectCredentials(provider, displayName.trim(), params);
      toast.success(t("credentials.connected", { name: res.email }));
      onOpenChange(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[90vh] w-[95vw] flex-col sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <KeyRound className="h-5 w-5" />
            {t("credentials.title", { provider: providerLabel })}
          </DialogTitle>
          <DialogDescription>
            {t("credentials.description", { provider: providerLabel })}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="cred-display-name" className="text-sm">
              {t("credentials.display_name")}
            </Label>
            <Input
              id="cred-display-name"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder={t("credentials.display_name_placeholder", { provider: providerLabel })}
              className="text-base md:text-sm"
              autoComplete="off"
            />
            <p className="text-xs text-muted-foreground">{t("credentials.display_name_hint")}</p>
          </div>

          {fields.map((f) => (
            <div key={f.key} className="flex flex-col gap-1.5">
              <Label htmlFor={`cred-${f.key}`} className="text-sm">
                {t(`credentials.fields.${provider}.${f.key}`)}
                {f.required && <span className="ml-1 text-destructive">*</span>}
              </Label>
              {f.type === "secret_textarea" ? (
                <Textarea
                  id={`cred-${f.key}`}
                  value={values[f.key] ?? ""}
                  onChange={(e) => setValues((prev) => ({ ...prev, [f.key]: e.target.value }))}
                  className="min-h-24 font-mono text-base md:text-sm"
                  autoComplete="new-password"
                  spellCheck={false}
                  dir="ltr"
                />
              ) : (
                <Input
                  id={`cred-${f.key}`}
                  type={f.type === "password" ? "password" : "text"}
                  value={values[f.key] ?? ""}
                  onChange={(e) => setValues((prev) => ({ ...prev, [f.key]: e.target.value }))}
                  className="text-base md:text-sm"
                  autoComplete={f.type === "password" ? "new-password" : "off"}
                  dir="ltr"
                />
              )}
              <p className="text-xs text-muted-foreground">
                {t(`credentials.fields.${provider}.${f.key}_hint`)}
              </p>
            </div>
          ))}

          {error && (
            <p className="text-sm text-destructive" role="alert">
              {t("credentials.probe_failed", { error })}
            </p>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 border-t pt-4">
          <Button
            variant="ghost"
            onClick={() => onOpenChange(false)}
            disabled={submitting}
            className="min-h-11 sm:min-h-9"
          >
            {t("credentials.cancel")}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={submitting || missingRequired}
            className="min-h-11 sm:min-h-9"
          >
            {submitting
              ? t("credentials.verifying")
              : t("credentials.connect")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
