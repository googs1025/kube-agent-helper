"use client";

import Link from "next/link";
import { useFixes } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import { Badge } from "@/components/ui/badge";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";

const phaseColors: Record<string, string> = {
  PendingApproval: "bg-yellow-100 text-yellow-800 dark:bg-yellow-950 dark:text-yellow-300",
  Approved: "bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300",
  Applying: "bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300",
  Succeeded: "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300",
  Failed: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300",
  RolledBack: "bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-300",
  DryRunComplete: "bg-purple-100 text-purple-800 dark:bg-purple-950 dark:text-purple-300",
};

export default function FixesPage() {
  const { t } = useI18n();
  const { data: fixes, error, isLoading } = useFixes();
  if (isLoading) return <p className="text-gray-500 dark:text-gray-400">{t("common.loading")}</p>;
  if (error) return <p className="text-red-600 dark:text-red-400">{t("common.loadFailed")}</p>;

  const total = fixes?.length ?? 0;
  const pending = fixes?.filter((f) => f.Phase === "PendingApproval").length ?? 0;
  const succeeded = fixes?.filter((f) => f.Phase === "Succeeded").length ?? 0;
  const failed = fixes?.filter((f) => ["Failed", "RolledBack"].includes(f.Phase)).length ?? 0;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t("fixes.title")}</h1>
      </div>
      <div className="mb-6 grid grid-cols-4 gap-4">
        {[
          { label: t("fixes.stat.total"), value: total, color: "text-gray-900 dark:text-gray-100" },
          { label: t("fixes.stat.pending"), value: pending, color: "text-yellow-600 dark:text-yellow-400" },
          { label: t("fixes.stat.succeeded"), value: succeeded, color: "text-green-600 dark:text-green-400" },
          { label: t("fixes.stat.failed"), value: failed, color: "text-red-600 dark:text-red-400" },
        ].map(({ label, value, color }) => (
          <div key={label} className="rounded-lg border bg-white p-4 dark:border-gray-800 dark:bg-gray-900">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{label}</p>
            <p className={`mt-1 text-2xl font-bold ${color}`}>{value}</p>
          </div>
        ))}
      </div>
      {fixes && fixes.length === 0 ? (
        <p className="text-gray-500 dark:text-gray-400">{t("fixes.empty")}</p>
      ) : (
        <div className="rounded-lg border bg-white dark:border-gray-800 dark:bg-gray-900">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("fixes.col.id")}</TableHead>
                <TableHead>{t("fixes.col.phase")}</TableHead>
                <TableHead>{t("fixes.col.finding")}</TableHead>
                <TableHead>{t("fixes.col.target")}</TableHead>
                <TableHead>{t("fixes.col.strategy")}</TableHead>
                <TableHead>{t("fixes.col.message")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {fixes?.map((fix) => (
                <TableRow key={fix.ID}>
                  <TableCell>
                    <Link href={`/fixes/${fix.ID}`} className="font-mono text-sm text-blue-600 hover:underline dark:text-blue-400">
                      {fix.ID.slice(0, 8)}...
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Badge className={phaseColors[fix.Phase] || ""}>{t(`phase.${fix.Phase}`)}</Badge>
                  </TableCell>
                  <TableCell className="max-w-[200px] truncate text-sm">{fix.FindingTitle}</TableCell>
                  <TableCell className="text-sm text-gray-600 dark:text-gray-400">
                    {fix.TargetKind}/{fix.TargetNamespace}/{fix.TargetName}
                  </TableCell>
                  <TableCell><Badge variant="outline">{fix.Strategy}</Badge></TableCell>
                  <TableCell className="max-w-xs truncate text-sm text-gray-600 dark:text-gray-400" title={fix.Message || ""}>
                    {fix.Message || "-"}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
