"use client";

import Link from "next/link";
import { useFixes } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import { useCluster } from "@/cluster/context";
import { Badge } from "@/components/ui/badge";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";

const fixPhaseConfig: Record<string, { bg: string; text: string; dot: string }> = {
  PendingApproval: { bg: "bg-yellow-500/10", text: "text-yellow-400", dot: "bg-yellow-400" },
  Approved:        { bg: "bg-sky-500/10",    text: "text-sky-400",    dot: "bg-sky-400" },
  Applying:        { bg: "bg-sky-500/10",    text: "text-sky-400",    dot: "bg-sky-400" },
  Succeeded:       { bg: "bg-green-500/10",  text: "text-green-400",  dot: "bg-green-400" },
  Failed:          { bg: "bg-red-500/10",    text: "text-red-400",    dot: "bg-red-400" },
  RolledBack:      { bg: "bg-orange-500/10", text: "text-orange-400", dot: "bg-orange-400" },
  DryRunComplete:  { bg: "bg-purple-500/10", text: "text-purple-400", dot: "bg-purple-400" },
};

function FixPhaseBadge({ phase }: { phase: string }) {
  const c = fixPhaseConfig[phase] ?? { bg: "bg-slate-500/10", text: "text-slate-400", dot: "bg-slate-400" };
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-md border border-current/20 px-2 py-0.5 text-xs font-semibold ${c.bg} ${c.text}`}>
      <span className={`size-1.5 rounded-full ${c.dot}`} />
      {phase}
    </span>
  );
}

export default function FixesPage() {
  const { t } = useI18n();
  const { cluster } = useCluster();
  const { data: fixes, error, isLoading } = useFixes({ cluster });
  if (isLoading) return <p className="text-muted-foreground">{t("common.loading")}</p>;
  if (error) return <p className="text-destructive">{t("common.loadFailed")}</p>;

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
          <div key={label} className="rounded-lg border border-border bg-card p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{label}</p>
            <p className={`mt-1 text-3xl font-bold ${color}`}>{value}</p>
          </div>
        ))}
      </div>
      {fixes && fixes.length === 0 ? (
        <p className="text-muted-foreground">{t("fixes.empty")}</p>
      ) : (
        <div className="rounded-lg border border-border bg-card overflow-hidden">
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
                    <Link href={`/fixes/${fix.ID}`} className="font-mono text-sm text-primary hover:underline">
                      {fix.Name ? (
                        <span className="font-medium">{fix.Name}</span>
                      ) : (
                        <span className="font-mono text-sm">{fix.ID.slice(0, 8)}...</span>
                      )}
                    </Link>
                  </TableCell>
                  <TableCell>
                    <FixPhaseBadge phase={fix.Phase} />
                  </TableCell>
                  <TableCell className="max-w-[200px] truncate text-sm">{fix.FindingTitle}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {fix.TargetKind}/{fix.TargetNamespace}/{fix.TargetName}
                  </TableCell>
                  <TableCell><Badge variant="outline">{fix.Strategy}</Badge></TableCell>
                  <TableCell className="max-w-xs truncate text-sm text-muted-foreground" title={fix.Message || ""}>
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
