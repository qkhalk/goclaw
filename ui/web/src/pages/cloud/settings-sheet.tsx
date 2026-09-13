import { useTranslation } from "react-i18next";
import { Cloud, HardDrive } from "lucide-react";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { ProviderClientSetup } from "./provider-client-setup";
import { ScopeBindingsPanel } from "./scope-bindings-panel";
import { useAuthStore } from "@/stores/use-auth-store";
import type { CloudProvider } from "./hooks/use-cloud";

const PROVIDER_OPTIONS: { id: CloudProvider; label: string; icon: typeof Cloud }[] = [
  { id: "google", label: "Google Drive", icon: Cloud },
  { id: "onedrive", label: "Microsoft OneDrive", icon: HardDrive },
];

/** Cloud settings sheet: provider switch + BYO OAuth client setup + per-scope
 * binding rules — everything that used to clutter the Clouds page body. */
export function SettingsSheet({
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
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t("settings.title")}</SheetTitle>
          <SheetDescription>{t("settings.description")}</SheetDescription>
        </SheetHeader>
        <div className="flex flex-col gap-4 overflow-y-auto px-4 pb-6">
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
        </div>
      </SheetContent>
    </Sheet>
  );
}
