import { Clapperboard, Eraser, Presentation, type LucideIcon } from "lucide-react";
import { ROUTES } from "@/lib/routes";

/**
 * Studio modules — the installable product tools of the Tool Store.
 *
 * The source of truth for installed state is the `studio` category in
 * builtin tools (`GET /v1/tools/builtin`): installed = tenant_enabled ?? enabled.
 * This map only binds each def name to its route/nav identity. When a def is
 * absent (server not yet upgraded to the seed), the module is treated as
 * installed so the nav never loses entries.
 */
export interface StudioModuleMeta {
  name: string;
  route: string;
  icon: LucideIcon;
  /** Nav label key in the "sidebar" namespace. */
  labelKey: string;
  order: number;
}

export const STUDIO_CATEGORY = "studio";

export const STUDIO_MODULES: StudioModuleMeta[] = [
  { name: "video_studio", route: ROUTES.TOOLS_VIDEO, icon: Clapperboard, labelKey: "nav.videoEditor", order: 1 },
  { name: "watermark_studio", route: ROUTES.TOOLS_WATERMARK, icon: Eraser, labelKey: "nav.watermarkRemover", order: 2 },
  { name: "pptx_studio", route: ROUTES.TOOLS_PPTX, icon: Presentation, labelKey: "nav.pptxStudio", order: 3 },
];
