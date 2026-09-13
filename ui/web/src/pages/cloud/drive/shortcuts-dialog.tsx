import { useTranslation } from "react-i18next";
import {
  ArrowDown,
  ArrowUp,
  CornerUpLeft,
  Delete,
  MousePointerClick,
  Search,
  SquareCheck,
  Trash2,
} from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

/** Keyboard shortcut help (opened with "?"). */
export function ShortcutsDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation("cloud");

  const keys: { icon: typeof ArrowUp; keys: string; action: string }[] = [
    { icon: ArrowDown, keys: "j / ↓", action: t("shortcuts.next") },
    { icon: ArrowUp, keys: "k / ↑", action: t("shortcuts.prev") },
    { icon: MousePointerClick, keys: "Enter", action: t("shortcuts.open") },
    { icon: CornerUpLeft, keys: "Esc", action: t("shortcuts.up") },
    { icon: Delete, keys: "Backspace", action: t("shortcuts.up") },
    { icon: Trash2, keys: "Del", action: t("shortcuts.delete") },
    { icon: SquareCheck, keys: "Ctrl/Cmd + A", action: t("shortcuts.select_all") },
    { icon: Search, keys: "/", action: t("shortcuts.search") },
    { icon: MousePointerClick, keys: "?", action: t("shortcuts.help") },
  ];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("shortcuts.title")}</DialogTitle>
          <DialogDescription>{t("shortcuts.description")}</DialogDescription>
        </DialogHeader>
        <ul className="max-h-80 space-y-1 overflow-y-auto overscroll-contain">
          {keys.map((k) => (
            <li key={k.keys + k.action} className="flex items-center gap-3 rounded-md px-1 py-1 text-sm">
              <k.icon className="h-4 w-4 shrink-0 text-muted-foreground" />
              <kbd className="min-w-[92px] rounded border bg-muted/50 px-1.5 py-0.5 text-center text-xs font-medium">
                {k.keys}
              </kbd>
              <span className="min-w-0 flex-1">{k.action}</span>
            </li>
          ))}
        </ul>
      </DialogContent>
    </Dialog>
  );
}
