import { useTranslation } from "react-i18next";
import type { CloudFileEntry } from "../hooks/use-cloud";

/** List view of the current folder. Table wrapped in overflow-x-auto with
 * min-w-[600px] per the mobile table rules (AGENTS.md). */
export function DriveTable({
  entries,
  renderRow,
}: {
  entries: CloudFileEntry[];
  renderRow: (entry: CloudFileEntry, index: number) => React.ReactNode;
}) {
  const { t } = useTranslation("cloud");

  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[600px] text-sm">
        <thead>
          <tr className="border-b text-left text-muted-foreground">
            <th className="py-2 pl-2 pr-2 font-medium">{t("detail.col_name")}</th>
            <th className="px-2 text-right font-medium">{t("detail.col_size")}</th>
            <th className="px-2 font-medium">{t("detail.col_modified")}</th>
            <th className="w-10" aria-hidden />
          </tr>
        </thead>
        <tbody>
          {entries.map((e, i) => renderRow(e, i))}
        </tbody>
      </table>
    </div>
  );
}
