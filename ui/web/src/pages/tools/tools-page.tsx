import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { Clapperboard, Eraser } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/shared/page-header";
import { ROUTES } from "@/lib/routes";

/** Tools hub: cards linking to each tool page. Heavy work is labeled per
 * tool — client-side (browser) vs server-rendered. */
export function ToolsPage() {
  const { t } = useTranslation("toolbox");
  const navigate = useNavigate();

  const tools = [
    {
      to: ROUTES.TOOLS_VIDEO,
      icon: Clapperboard,
      title: t("hub.video_title"),
      description: t("hub.video_desc"),
      badge: t("hub.server_badge"),
      badgeClass: "text-blue-600 border-blue-500/40",
    },
    {
      to: ROUTES.TOOLS_WATERMARK,
      icon: Eraser,
      title: t("hub.watermark_title"),
      description: t("hub.watermark_desc"),
      badge: t("hub.client_badge"),
      badgeClass: "text-green-600 border-green-500/40",
    },
  ];

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
      <PageHeader title={t("title")} description={t("description")} />

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {tools.map((tool) => (
          <button
            key={tool.to}
            type="button"
            onClick={() => navigate(tool.to)}
            className="flex flex-col items-start gap-2 rounded-lg border p-4 text-left transition-colors hover:bg-muted/40"
          >
            <div className="flex w-full items-start justify-between gap-2">
              <tool.icon className="h-5 w-5 shrink-0" />
              <Badge variant="outline" className={`shrink-0 ${tool.badgeClass}`}>
                {tool.badge}
              </Badge>
            </div>
            <p className="font-medium">{tool.title}</p>
            <p className="text-xs text-muted-foreground">{tool.description}</p>
            <span className="mt-auto text-sm font-medium text-primary">{t("hub.open")} →</span>
          </button>
        ))}
      </div>
    </div>
  );
}
