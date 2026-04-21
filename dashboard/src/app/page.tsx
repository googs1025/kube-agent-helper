"use client";

import Link from "next/link";
import { useRuns } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import { PhaseBadge } from "@/components/phase-badge";
import { CreateRunDialog } from "@/components/create-run-dialog";
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

export default function RunsPage() {
  const { t } = useI18n();
  const { data: runs, error, isLoading, mutate } = useRuns();
  if (isLoading) return <p className="text-gray-500 dark:text-gray-400">{t("common.loading")}</p>;
  if (error) return <p className="text-red-600 dark:text-red-400">{t("common.loadFailed")}</p>;

  const total = runs?.length ?? 0;
  const running = runs?.filter((r) => r.Status === "Running").length ?? 0;
  const succeeded = runs?.filter((r) => r.Status === "Succeeded").length ?? 0;
  const failed = runs?.filter((r) => r.Status === "Failed").length ?? 0;

  const featureCards = [
    { icon: Activity, title: t("overview.card.runs.title"), desc: t("overview.card.runs.desc"), href: "#runs", color: "text-blue-600 dark:text-blue-400" },
    { icon: Cpu, title: t("overview.card.skills.title"), desc: t("overview.card.skills.desc"), href: "/skills", color: "text-green-600 dark:text-green-400" },
    { icon: Wrench, title: t("overview.card.fixes.title"), desc: t("overview.card.fixes.desc"), href: "/fixes", color: "text-orange-600 dark:text-orange-400" },
  ];

  return (
    <div>
      {/* Overview hero */}
      <div className="mb-8 rounded-xl border bg-gradient-to-br from-blue-50 to-indigo-50 p-6 dark:border-gray-800 dark:from-gray-900 dark:to-gray-800">
        <h1 className="text-2xl font-bold">{t("overview.title")}</h1>
        <p className="mt-1 text-sm text-gray-600 dark:text-gray-400">{t("overview.subtitle")}</p>
        <div className="mt-4 grid grid-cols-3 gap-4">
          {featureCards.map((card) => (
            <Link key={card.title} href={card.href} className="group rounded-lg border bg-white p-4 transition-shadow hover:shadow-md dark:border-gray-700 dark:bg-gray-900">
              <card.icon className={`size-5 ${card.color}`} />
              <h3 className="mt-2 text-sm font-semibold">{card.title}</h3>
              <p className="mt-1 text-xs text-gray-500 dark:text-gray-400 line-clamp-2">{card.desc}</p>
            </Link>
          ))}
        </div>
      </div>

      {/* Runs section */}
      <div id="runs" className="mb-6 flex items-center justify-between">
        <h2 className="text-2xl font-bold">{t("runs.title")}</h2>
        <CreateRunDialog onCreated={() => mutate()} />
      </div>
      <div className="mb-6 grid grid-cols-4 gap-4">
        {[
          { label: t("runs.stat.total"), value: total, color: "text-gray-900 dark:text-gray-100" },
          { label: t("runs.stat.running"), value: running, color: "text-blue-600 dark:text-blue-400" },
          { label: t("runs.stat.succeeded"), value: succeeded, color: "text-green-600 dark:text-green-400" },
          { label: t("runs.stat.failed"), value: failed, color: "text-red-600 dark:text-red-400" },
        ].map(({ label, value, color }) => (
          <div key={label} className="rounded-lg border bg-white p-4 dark:border-gray-800 dark:bg-gray-900">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{label}</p>
            <p className={`mt-1 text-2xl font-bold ${color}`}>{value}</p>
          </div>
        ))}
      </div>
      {runs && runs.length === 0 ? (
        <p className="text-gray-500 dark:text-gray-400">{t("runs.empty")}</p>
      ) : (
        <div className="rounded-lg border bg-white dark:border-gray-800 dark:bg-gray-900">
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
              {runs?.map((run) => {
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
                      <Link href={`/runs/${run.ID}`} className="text-blue-600 hover:underline dark:text-blue-400">
                        {run.Name ? (
                          <span className="font-medium">{run.Name}</span>
                        ) : (
                          <span className="font-mono text-sm">{run.ID.slice(0, 8)}...</span>
                        )}
                      </Link>
                    </TableCell>
                    <TableCell><PhaseBadge phase={run.Status} /></TableCell>
                    <TableCell className="text-sm text-gray-600 dark:text-gray-400">{formatTime(run.CreatedAt)}</TableCell>
                    <TableCell className="text-sm text-gray-600 dark:text-gray-400">{duration(run.StartedAt, run.CompletedAt)}</TableCell>
                    <TableCell className="text-sm text-gray-600 dark:text-gray-400">{target}</TableCell>
                    <TableCell className="max-w-xs truncate text-sm text-gray-600 dark:text-gray-400" title={run.Message || ""}>
                      {run.Message || "-"}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
