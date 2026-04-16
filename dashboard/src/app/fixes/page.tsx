"use client";

import Link from "next/link";
import { useFixes } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";

const phaseColors: Record<string, string> = {
  PendingApproval: "bg-yellow-100 text-yellow-800",
  Approved: "bg-blue-100 text-blue-800",
  Applying: "bg-blue-100 text-blue-800",
  Succeeded: "bg-green-100 text-green-800",
  Failed: "bg-red-100 text-red-800",
  RolledBack: "bg-orange-100 text-orange-800",
  DryRunComplete: "bg-purple-100 text-purple-800",
};

export default function FixesPage() {
  const { data: fixes, error, isLoading } = useFixes();
  if (isLoading) return <p className="text-gray-500">Loading fixes...</p>;
  if (error) return <p className="text-red-600">Failed to load fixes.</p>;

  const total = fixes?.length ?? 0;
  const pending = fixes?.filter((f) => f.Phase === "PendingApproval").length ?? 0;
  const succeeded = fixes?.filter((f) => f.Phase === "Succeeded").length ?? 0;
  const failed = fixes?.filter((f) => ["Failed", "RolledBack"].includes(f.Phase)).length ?? 0;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Fixes</h1>
      </div>
      <div className="mb-6 grid grid-cols-4 gap-4">
        {[
          { label: "Total", value: total, color: "text-gray-900" },
          { label: "Pending Approval", value: pending, color: "text-yellow-600" },
          { label: "Succeeded", value: succeeded, color: "text-green-600" },
          { label: "Failed / Rolled Back", value: failed, color: "text-red-600" },
        ].map(({ label, value, color }) => (
          <div key={label} className="rounded-lg border bg-white p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
            <p className={`mt-1 text-2xl font-bold ${color}`}>{value}</p>
          </div>
        ))}
      </div>
      {fixes && fixes.length === 0 ? (
        <p className="text-gray-500">No fixes yet.</p>
      ) : (
        <div className="rounded-lg border bg-white">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Phase</TableHead>
                <TableHead>Finding</TableHead>
                <TableHead>Target</TableHead>
                <TableHead>Strategy</TableHead>
                <TableHead>Message</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {fixes?.map((fix) => (
                <TableRow key={fix.ID}>
                  <TableCell>
                    <Link href={`/fixes/${fix.ID}`} className="font-mono text-sm text-blue-600 hover:underline">
                      {fix.ID.slice(0, 8)}...
                    </Link>
                  </TableCell>
                  <TableCell>
                    <Badge className={phaseColors[fix.Phase] || ""}>{fix.Phase}</Badge>
                  </TableCell>
                  <TableCell className="max-w-[200px] truncate text-sm">{fix.FindingTitle}</TableCell>
                  <TableCell className="text-sm text-gray-600">
                    {fix.TargetKind}/{fix.TargetNamespace}/{fix.TargetName}
                  </TableCell>
                  <TableCell><Badge variant="outline">{fix.Strategy}</Badge></TableCell>
                  <TableCell className="max-w-xs truncate text-sm text-gray-600" title={fix.Message || ""}>
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
