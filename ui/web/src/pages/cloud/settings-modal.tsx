import { useTranslation } from "react-i18next";
import { Cloud, HardDrive, Settings2 } from "lucide-react";
import { Label } from "@/components/ui/label";
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
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ProviderClientSetup } from "./provider-client-setup";
import { ScopeBindingsPanel } from "./scope-bindings-panel";
import { SyncSection } from "./sync-section";
import { useAuthStore } from "@/stores/use-auth-store";
import type { CloudProvider } from "./hooks/use-cloud";

const PROVIDER_OPTIONS: { id: CloudProvider; label: string; icon: typeof Cloud }[] = [
  { id: "google", label: "Google Drive", icon: Cloud },
  { id: "onedrive", label: "Microsoft OneDrive", icon: HardDrive },
];

/** Cloud settings modal — centered dialog in the same style as the Overview
 * System Settings: provider switch + BYO OAuth client setup + per-scope
 * binding rules + sync pairs — everything that used to clutter the Clouds
 * page body. Full-screen slide-up on mobile (ui/dialog.tsx convention). */
export function SettingsModal({
  open,
  onOpenChange,
  provider,
  onProviderChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider: CloudProvider;
  onProviderChange: (provider: CloudProvider) => void;
}) {
  const { t } = useTranslation("cloud");
  const role = useAuthStore((s) => s.role);
  const isAdmin = role === "admin" || role === "owner";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[90vh] w-[95vw] flex-col sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Settings2 className="h-5 w-5" />
            {t("settings.title")}
          </DialogTitle>
          <DialogDescription>{t("settings.description")}</DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto -mx-4 px-4 sm:-mx-6 sm:px-6">
          <div className="flex flex-col gap-1.5">
            <Label className="text-sm">{t("settings.provider")}</Label>
            <Select value={provider} onValueChange={(v) => onProviderChange(v as CloudProvider)}>
              <SelectTrigger className="w-full" dir="ltr">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PROVIDER_OPTIONS.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    <span className="flex items-center gap-2">
                      <p.icon className="h-4 w-4" />
                      {p.label}
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {isAdmin && <ProviderClientSetup provider={provider} />}
          {isAdmin && <ScopeBindingsPanel provider={provider} />}
          {isAdmin && <SyncSection />}
        </div>
      </DialogContent>
    </Dialog>
  );
}
