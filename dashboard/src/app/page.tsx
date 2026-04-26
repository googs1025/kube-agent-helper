"use client";

import Link from "next/link";
import { useRunsPaginated } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import { useCluster } from "@/cluster/context";
import { PhaseBadge } from "@/components/phase-badge";
import { CreateRunDialog } from "@/components/create-run-dialog";
import { Pagination } from "@/components/Pagination";
import { FilterBar, type FilterField } from "@/components/FilterBar";
import { useTableState } from "@/hooks/useTableState";
import { Activity, Cpu, Wrench } from "lucide-react";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";

function formatTime(iso: string | null): string {
  if (!iso) return "-";
  return new Date(iso).toLocaleString();
}

function duration(start: string | null, end: string | null): string {
  if (!start) return "-";
  const s = new Date(start).getTime();
  const e = end ? new Date(end).getTime() : Date.now();
  const sec = Math.round((e - s) / 1000);
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

const RUN_FILTER_FIELDS: FilterField[] = [
  {
    key: "phase",
    labelKey: "filter.phase",
    type: "select",
    options: [
      { value: "Pending", labelKey: "phase.Pending" },
      { value: "Running", labelKey: "phase.Running" },
      { value: "Succeeded", labelKey: "phase.Succeeded" },
      { value: "Failed", labelKey: "phase.Failed" },
      { value: "Scheduled", labelKey: "phase.Scheduled" },
    ],
  },
];

export default function RunsPage() {
  const { t } = useI18n();
  const { cluster } = useCluster();
  const table = useTableState({ pageSize: 20 });
  const { data, error, isLoading, mutate } = useRunsPaginated({
    ...table.params,
    cluster,
  });

  const runs = data?.items;
  const total = data?.total ?? 0;

  const featureCards = [
    { icon: Activity, title: t("overview.card.runs.title"), desc: t("overview.card.runs.desc"), href: "#runs", color: "text-primary" },
    { icon: Cpu, title: t("overview.card.skills.title"), desc: t("overview.card.skills.desc"), href: "/skills", color: "text-green-400" },
    { icon: Wrench, title: t("overview.card.fixes.title"), desc: t("overview.card.fixes.desc"), href: "/fixes", color: "text-orange-400" },
  ];

  return (
    <div>
      {/* Overview hero */}
      <div className="mb-8 relative overflow-hidden rounded-xl border border-border bg-gradient-to-br from-sky-50 to-indigo-50 p-6 dark:from-[#0d1b2e] dark:to-[#130d2e]">
        <div className="absolute inset-x-0 top-0 h-[2px] bg-gradient-to-r from-sky-400 via-indigo-400 to-sky-400" />
        <h1 className="text-2xl font-bold">{t("overview.title")}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t("overview.subtitle")}</p>
        <div className="mt-4 grid grid-cols-3 gap-4">
          {featureCards.map((card) => (
            <Link key={card.title} href={card.href} className="group rounded-lg border border-border bg-background/60 p-4 transition-all hover:border-primary/50 hover:bg-primary/5">
              <card.icon className={`size-5 ${card.color}`} />
              <h3 className="mt-2 text-sm font-semibold">{card.title}</h3>
              <p className="mt-1 text-xs text-muted-foreground line-clamp-2">{card.desc}</p>
            </Link>
          ))}
        </div>
      </div>

      {/* Runs section */}
      <div id="runs" className="mb-6 flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-xl font-semibold">
          {t("runs.title")}
          <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">{total}</span>
        </h2>
        <CreateRunDialog onCreated={() => mutate()} />
      </div>

      <FilterBar
        fields={RUN_FILTER_FIELDS}
        values={table.filters}
        onChange={table.setFilter}
        onClear={table.clearFilters}
      />

      {isLoading && <p className="text-muted-foreground">{t("common.loading")}</p>}
      {error && <p className="text-destructive">{t("common.loadFailed")}</p>}

      {!isLoading && !error && runs && runs.length === 0 ? (
        <p className="text-muted-foreground">{t("runs.empty")}</p>
      ) : null}

      {!isLoading && !error && runs && runs.length > 0 && (
        <>
          <div className="rounded-lg border border-border bg-card overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("runs.col.id")}</TableHead>
                  <TableHead>{t("runs.col.phase")}</TableHead>
                  <TableHead>{t("runs.col.created")}</TableHead>
                  <TableHead>{t("runs.col.duration")}</TableHead>
                  <TableHead>{t("runs.col.target")}</TableHead>
                  <TableHead>{t("runs.col.message")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs.map((run) => {
                  let target = "-";
                  try {
                    const tgt = JSON.parse(run.TargetJSON);
                    target = tgt.namespaces?.join(", ") || tgt.scope || "-";
                  } catch {
                    /* ignore */
                  }
                  return (
                    <TableRow key={run.ID}>
                      <TableCell>
                        <Link href={`/runs/${run.ID}`} className="font-mono text-sm text-primary hover:underline">
                          {run.Name || run.ID.slice(0, 8)}
                        </Link>
                      </TableCell>
                      <TableCell><PhaseBadge phase={run.Status} /></TableCell>
                      <TableCell className="text-sm text-muted-foreground">{formatTime(run.CreatedAt)}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{duration(run.StartedAt, run.CompletedAt)}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{target}</TableCell>
                      <TableCell className="max-w-xs truncate text-sm text-muted-foreground" title={run.Message || ""}>
                        {run.Message || "-"}
                      </TableCell>
                    </TableRow>
                  );
                })}
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
    </div>
  );
}
