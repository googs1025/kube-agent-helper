"use client";

import Link from "next/link";
import { useRuns } from "@/lib/api";
import { PhaseBadge } from "@/components/phase-badge";
import { CreateRunDialog } from "@/components/create-run-dialog";
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
  const { data: runs, error, isLoading, mutate } = useRuns();
  if (isLoading) return <p className="text-gray-500">Loading runs...</p>;
  if (error) return <p className="text-red-600">Failed to load runs.</p>;

  const total = runs?.length ?? 0;
  const running = runs?.filter((r) => r.Status === "Running").length ?? 0;
  const succeeded = runs?.filter((r) => r.Status === "Succeeded").length ?? 0;
  const failed = runs?.filter((r) => r.Status === "Failed").length ?? 0;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Diagnostic Runs</h1>
        <CreateRunDialog onCreated={() => mutate()} />
      </div>
      <div className="mb-6 grid grid-cols-4 gap-4">
        {[
          { label: "Total", value: total, color: "text-gray-900" },
          { label: "Running", value: running, color: "text-blue-600" },
          { label: "Succeeded", value: succeeded, color: "text-green-600" },
          { label: "Failed", value: failed, color: "text-red-600" },
        ].map(({ label, value, color }) => (
          <div key={label} className="rounded-lg border bg-white p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
            <p className={`mt-1 text-2xl font-bold ${color}`}>{value}</p>
          </div>
        ))}
      </div>
      {runs && runs.length === 0 ? (
        <p className="text-gray-500">No runs yet.</p>
      ) : (
        <div className="rounded-lg border bg-white">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Phase</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Duration</TableHead>
                <TableHead>Target</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {runs?.map((run) => {
                let target = "-";
                try {
                  const t = JSON.parse(run.TargetJSON);
                  target = t.namespaces?.join(", ") || t.scope || "-";
                } catch {
                  /* ignore */
                }
                return (
                  <TableRow key={run.ID}>
                    <TableCell>
                      <Link href={`/runs/${run.ID}`} className="font-mono text-sm text-blue-600 hover:underline">
                        {run.ID.slice(0, 8)}...
                      </Link>
                    </TableCell>
                    <TableCell><PhaseBadge phase={run.Status} /></TableCell>
                    <TableCell className="text-sm text-gray-600">{formatTime(run.CreatedAt)}</TableCell>
                    <TableCell className="text-sm text-gray-600">{duration(run.StartedAt, run.CompletedAt)}</TableCell>
                    <TableCell className="text-sm text-gray-600">{target}</TableCell>
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
