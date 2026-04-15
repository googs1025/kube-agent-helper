"use client";

import Link from "next/link";
import { useRuns } from "@/lib/api";
import { PhaseBadge } from "@/components/phase-badge";
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
  const { data: runs, error, isLoading } = useRuns();
  if (isLoading) return <p className="text-gray-500">Loading runs...</p>;
  if (error) return <p className="text-red-600">Failed to load runs.</p>;
  return (
    <div>
      <h1 className="mb-6 text-2xl font-bold">Diagnostic Runs</h1>
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
