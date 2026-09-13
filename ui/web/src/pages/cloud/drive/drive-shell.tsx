import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Menu } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { DriveRail } from "./drive-rail";

/** Drive-style page shell: left rail (desktop aside / mobile Sheet) + header
 * slot + scrollable content. The rail lists accounts and the quota card; the
 * page supplies header content (breadcrumbs/title + tools, typically a
 * <DriveTopBar>). Layout is h-dvh-compliant: fills the AppLayout main pane
 * with h-full, never h-screen. */
export function DriveShell({
  accountId,
  railTitle,
  header,
  children,
}: {
  accountId?: string;
  railTitle: string;
  /** Header content (breadcrumbs/title + tools). */
  header: React.ReactNode;
  children: React.ReactNode;
}) {
  const { t } = useTranslation("cloud");
  const [railOpen, setRailOpen] = useState(false);

  // Close the mobile rail when the user navigates to another account.
  useEffect(() => {
    setRailOpen(false);
  }, [accountId]);

  const rail = <DriveRail accountId={accountId} onNavigate={() => setRailOpen(false)} />;

  return (
    <div className="flex h-full min-h-0">
      <aside className="hidden w-64 shrink-0 border-r md:block">{rail}</aside>

      {/* Mobile rail: hamburger in the header opens this Sheet. */}
      <Sheet open={railOpen} onOpenChange={setRailOpen}>
        <SheetContent className="w-80 max-w-[85vw]">
          <SheetHeader className="p-3">
            <SheetTitle className="text-sm">{railTitle || t("drive.my_drives")}</SheetTitle>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-y-auto">{rail}</div>
        </SheetContent>
      </Sheet>

      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-start gap-1 border-b px-2 py-2 md:px-3">
          <Button
            variant="ghost"
            size="icon"
            className="shrink-0 md:hidden"
            aria-label={t("drive.accounts")}
            onClick={() => setRailOpen(true)}
          >
            <Menu className="h-4 w-4" />
          </Button>
          <div className="min-w-0 flex-1">{header}</div>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto safe-bottom overscroll-contain">{children}</div>
      </div>
    </div>
  );
}
