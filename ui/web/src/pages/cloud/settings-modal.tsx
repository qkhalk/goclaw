import { useState } from "react";
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
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Switch } from "@/components/ui/switch";
import { ProviderClientSetup } from "./provider-client-setup";
import { ScopeBindingsPanel } from "./scope-bindings-panel";
import { SyncSection } from "./sync-section";
import { useAuthStore } from "@/stores/use-auth-store";
import type { CloudProvider } from "./hooks/use-cloud";

const CLOUD_THUMB_SIZE_KEY = "cloud.thumbnail_size";
const CLOUD_SHOW_HIDDEN_KEY = "cloud.show_hidden";

/** Fired on window whenever a preview setting changes so open drive views
 * (which read these values from localStorage) re-render immediately. */
export const CLOUD_SETTINGS_EVENT = "cloud:settings-changed";

export type ThumbnailSize = "small" | "medium" | "large";

export function getThumbnailSize(): ThumbnailSize {
  try {
    const v = localStorage.getItem(CLOUD_THUMB_SIZE_KEY);
    if (v === "small" || v === "medium" || v === "large") return v;
  } catch { /* ignore */ }
  return "medium";
}

export function getShowHiddenFiles(): boolean {
  try {
    return localStorage.getItem(CLOUD_SHOW_HIDDEN_KEY) === "true";
  } catch { /* ignore */ }
  return false;
}

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

  const [thumbSize, setThumbSize] = useState<ThumbnailSize>(getThumbnailSize);
  const [showHidden, setShowHidden] = useState(getShowHiddenFiles);

  function handleThumbSizeChange(v: string) {
    const val = v as ThumbnailSize;
    setThumbSize(val);
    try { localStorage.setItem(CLOUD_THUMB_SIZE_KEY, val); } catch { /* ignore */ }
    window.dispatchEvent(new Event(CLOUD_SETTINGS_EVENT));
  }

  function handleShowHiddenChange(checked: boolean) {
    setShowHidden(checked);
    try { localStorage.setItem(CLOUD_SHOW_HIDDEN_KEY, String(checked)); } catch { /* ignore */ }
    window.dispatchEvent(new Event(CLOUD_SETTINGS_EVENT));
  }

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

          <div className="flex flex-col gap-4 border-t pt-4">
            <Label className="text-sm font-medium">{t("settings.preview")}</Label>
            <div className="flex flex-col gap-2">
              <Label className="text-xs text-muted-foreground">{t("settings.thumbnail_size")}</Label>
              <RadioGroup
                value={thumbSize}
                onValueChange={handleThumbSizeChange}
                className="flex flex-row gap-4"
              >
                {(["small", "medium", "large"] as const).map((size) => (
                  <span key={size} className="flex items-center gap-1.5">
                    <RadioGroupItem value={size} id={`thumb-${size}`} />
                    <Label htmlFor={`thumb-${size}`} className="cursor-pointer text-sm font-normal">
                      {t(`settings.${size}`)}
                    </Label>
                  </span>
                ))}
              </RadioGroup>
            </div>
            <div className="flex items-center justify-between gap-4">
              <Label htmlFor="show-hidden" className="cursor-pointer text-sm font-normal">
                {t("settings.show_hidden")}
              </Label>
              <Switch id="show-hidden" checked={showHidden} onCheckedChange={handleShowHiddenChange} />
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
