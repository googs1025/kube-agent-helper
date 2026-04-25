"use client";

import Link from "next/link";
import { useFixesPaginated, batchApproveFixes, batchRejectFixes } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import { useCluster } from "@/cluster/context";
import { Badge } from "@/components/ui/badge";
import { Pagination } from "@/components/Pagination";
import { FilterBar, type FilterField } from "@/components/FilterBar";
import { BatchToolbar } from "@/components/BatchToolbar";
import { useTableState } from "@/hooks/useTableState";
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

const FIX_FILTER_FIELDS: FilterField[] = [
  {
    key: "phase",
    labelKey: "filter.phase",
    type: "select",
    options: [
      { value: "PendingApproval", labelKey: "phase.PendingApproval" },
      { value: "Approved", labelKey: "phase.Approved" },
      { value: "Applying", labelKey: "phase.Applying" },
      { value: "Succeeded", labelKey: "phase.Succeeded" },
      { value: "Failed", labelKey: "phase.Failed" },
      { value: "RolledBack", labelKey: "phase.RolledBack" },
      { value: "DryRunComplete", labelKey: "phase.DryRunComplete" },
    ],
  },
];

export default function FixesPage() {
  const { t } = useI18n();
  const { cluster } = useCluster();
  const table = useTableState({ pageSize: 20 });
  const { data, error, isLoading, mutate } = useFixesPaginated({
    ...table.params,
    cluster,
  });

  const fixes = data?.items;
  const total = data?.total ?? 0;

  const handleBatchApprove = async () => {
    if (table.selected.size === 0) return;
    try {
      await batchApproveFixes(Array.from(table.selected));
      table.clearSelection();
      mutate();
    } catch {
      // ignore
    }
  };

  const handleBatchReject = async () => {
    if (table.selected.size === 0) return;
    try {
      await batchRejectFixes(Array.from(table.selected));
      table.clearSelection();
      mutate();
    } catch {
      // ignore
    }
  };

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t("fixes.title")}</h1>
      </div>

      <FilterBar
        fields={FIX_FILTER_FIELDS}
        values={table.filters}
        onChange={table.setFilter}
        onClear={table.clearFilters}
      />

      {isLoading && <p className="text-muted-foreground">{t("common.loading")}</p>}
      {error && <p className="text-destructive">{t("common.loadFailed")}</p>}

      {!isLoading && !error && fixes && fixes.length === 0 ? (
        <p className="text-muted-foreground">{t("fixes.empty")}</p>
      ) : null}

      {!isLoading && !error && fixes && fixes.length > 0 && (
        <>
          <div className="rounded-lg border border-border bg-card overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">
                    <input
                      type="checkbox"
                      checked={fixes.length > 0 && table.selected.size === fixes.length}
                      onChange={() => table.selectAll(fixes.map((f) => f.ID))}
                      className="rounded border-border"
                    />
                  </TableHead>
                  <TableHead>{t("fixes.col.id")}</TableHead>
                  <TableHead>{t("fixes.col.phase")}</TableHead>
                  <TableHead>{t("fixes.col.finding")}</TableHead>
                  <TableHead>{t("fixes.col.target")}</TableHead>
                  <TableHead>{t("fixes.col.strategy")}</TableHead>
                  <TableHead>{t("fixes.col.message")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {fixes.map((fix) => (
                  <TableRow key={fix.ID} data-selected={table.selected.has(fix.ID) || undefined}>
                    <TableCell>
                      <input
                        type="checkbox"
                        checked={table.selected.has(fix.ID)}
                        onChange={() => table.toggleSelect(fix.ID)}
                        className="rounded border-border"
                      />
                    </TableCell>
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
          <Pagination
            page={table.page}
            pageSize={table.pageSize}
            total={total}
            onPageChange={table.setPage}
            onPageSizeChange={table.setPageSize}
          />
        </>
      )}

      <BatchToolbar
        count={table.selected.size}
        actions={[
          { labelKey: "batch.approve", onClick: handleBatchApprove },
          { labelKey: "batch.reject", variant: "destructive", onClick: handleBatchReject },
        ]}
        onClear={table.clearSelection}
      />
    </div>
  );
}
