import { useTranslation } from "react-i18next";
import { ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";
import { pathCrumbs } from "./paths";

/** Drive breadcrumb built from the ?path= URL param. Root button + one button
 * per decoded segment; navigating calls onNavigatePath (URL is the source of
 * truth — no local path state). */
export function DriveBreadcrumbs({
  path,
  rootLabel,
  onNavigatePath,
  className,
  crumbRender,
}: {
  path: string;
  rootLabel: string;
  onNavigatePath: (path: string) => void;
  className?: string;
  /** P5: wrap crumbs in droppable targets (return the element to render). */
  crumbRender?: (crumb: { name: string; path: string }, defaultNode: React.ReactNode) => React.ReactNode;
}) {
  const { t } = useTranslation("cloud");
  const crumbs = pathCrumbs(path);
  const effectiveRootLabel = rootLabel || t("detail.root");

  return (
    <nav
      aria-label="Breadcrumb"
      className={cn("flex min-w-0 flex-1 flex-wrap items-center gap-0.5 text-sm", className)}
    >
      <button
        type="button"
        className={cn(
          "max-w-[140px] truncate rounded px-1.5 py-1 hover:bg-muted",
          crumbs.length === 0 && "font-medium",
        )}
        onClick={() => onNavigatePath("/")}
      >
        {effectiveRootLabel}
      </button>
      {crumbs.map((c) => (
        <span key={c.path} className="flex min-w-0 items-center gap-0.5">
          <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          {crumbRender ? (
            crumbRender(c, (
              <button
                type="button"
                className="max-w-[160px] truncate rounded px-1.5 py-1 hover:bg-muted"
                onClick={() => onNavigatePath(c.path)}
              >
                {c.name}
              </button>
            ))
          ) : (
            <button
              type="button"
              className="max-w-[160px] truncate rounded px-1.5 py-1 hover:bg-muted"
              onClick={() => onNavigatePath(c.path)}
            >
              {c.name}
            </button>
          )}
        </span>
      ))}
    </nav>
  );
}
