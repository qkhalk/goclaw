import { useTranslation } from "react-i18next";
import { Clapperboard, Eraser } from "lucide-react";
import { Link } from "react-router";
import { PageHeader } from "@/components/shared/page-header";
import { ROUTES } from "@/lib/constants";

const TOOLS = [
  {
    key: "videoEditor",
    icon: Clapperboard,
    to: ROUTES.TOOLS_VIDEO,
    color: "text-blue-600 dark:text-blue-400",
    bg: "bg-blue-50 dark:bg-blue-950/40",
  },
  {
    key: "watermarkRemover",
    icon: Eraser,
    to: ROUTES.TOOLS_WATERMARK,
    color: "text-orange-600 dark:text-orange-400",
    bg: "bg-orange-50 dark:bg-orange-950/40",
  },
] as const;

export function ToolsHubPage() {
  const { t } = useTranslation("toolbox");

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
      <PageHeader
        title={t("hub.title")}
        description={t("hub.description")}
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {TOOLS.map((tool) => {
          const Icon = tool.icon;
          return (
            <Link
              key={tool.key}
              to={tool.to}
              className="group flex items-start gap-4 rounded-lg border p-5 transition-colors hover:bg-accent/50"
            >
              <div className={`flex h-12 w-12 shrink-0 items-center justify-center rounded-lg ${tool.bg}`}>
                <Icon className={`h-6 w-6 ${tool.color}`} />
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-sm font-medium group-hover:underline">
                  {t(`hub.tools.${tool.key}`)}
                </span>
                <span className="text-xs text-muted-foreground">
                  {t(`hub.tools.${tool.key}_desc`)}
                </span>
              </div>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
