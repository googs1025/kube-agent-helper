"use client";

import { use } from "react";
import Link from "next/link";
import { useRun, useFindings } from "@/lib/api";
import { PhaseBadge } from "@/components/phase-badge";
import { SeverityBadge } from "@/components/severity-badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";

function formatTime(iso: string | null): string {
  if (!iso) return "-";
  return new Date(iso).toLocaleString();
}

const dimensionLabels: Record<string, string> = {
  health: "Health", security: "Security", cost: "Cost", reliability: "Reliability",
};

export default function RunDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { data: run, error: runErr, isLoading: runLoading } = useRun(id);
  const { data: findings, error: findErr, isLoading: findLoading } = useFindings(id);

  if (runLoading) return <p className="text-gray-500">Loading run...</p>;
  if (runErr) return <p className="text-red-600">Failed to load run.</p>;
  if (!run) return <p className="text-gray-500">Run not found.</p>;

  const grouped: Record<string, typeof findings> = {};
  findings?.forEach((f) => {
    const dim = f.Dimension || "other";
    if (!grouped[dim]) grouped[dim] = [];
    grouped[dim]!.push(f);
  });

  const dimOrder = ["health", "security", "cost", "reliability"];
  const sortedDims = Object.keys(grouped).sort(
    (a, b) => (dimOrder.indexOf(a) === -1 ? 99 : dimOrder.indexOf(a)) - (dimOrder.indexOf(b) === -1 ? 99 : dimOrder.indexOf(b))
  );

  return (
    <div>
      <Link href="/" className="text-sm text-blue-600 hover:underline">&larr; Back to Runs</Link>
      <div className="mt-4 mb-6">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold font-mono">{run.ID.slice(0, 8)}</h1>
          <PhaseBadge phase={run.Status} />
        </div>
        <div className="mt-2 grid grid-cols-2 gap-4 text-sm text-gray-600 sm:grid-cols-4">
          <div><span className="font-medium">Created:</span> {formatTime(run.CreatedAt)}</div>
          <div><span className="font-medium">Started:</span> {formatTime(run.StartedAt)}</div>
          <div><span className="font-medium">Completed:</span> {formatTime(run.CompletedAt)}</div>
          <div><span className="font-medium">Findings:</span> {findings?.length ?? 0}</div>
        </div>
        {run.Message && <p className="mt-2 text-sm text-gray-700">{run.Message}</p>}
      </div>
      <Separator className="mb-6" />
      <h2 className="mb-4 text-xl font-semibold">Findings</h2>
      {findLoading && <p className="text-gray-500">Loading findings...</p>}
      {findErr && <p className="text-red-600">Failed to load findings.</p>}
      {findings && findings.length === 0 && <p className="text-gray-500">No findings for this run.</p>}
      <div className="space-y-6">
        {sortedDims.map((dim) => (
          <div key={dim}>
            <h3 className="mb-3 text-lg font-medium capitalize">{dimensionLabels[dim] || dim}</h3>
            <div className="space-y-3">
              {grouped[dim]?.map((f) => (
                <Card key={f.ID}>
                  <CardHeader className="pb-2">
                    <div className="flex items-center justify-between">
                      <CardTitle className="text-base">{f.Title}</CardTitle>
                      <SeverityBadge severity={f.Severity} />
                    </div>
                  </CardHeader>
                  <CardContent>
                    <p className="text-sm text-gray-700">{f.Description}</p>
                    {f.ResourceKind && (
                      <p className="mt-2 font-mono text-xs text-gray-500">
                        {f.ResourceKind}/{f.ResourceNamespace}/{f.ResourceName}
                      </p>
                    )}
                    {f.Suggestion && (
                      <div className="mt-2 rounded bg-blue-50 p-2 text-sm text-blue-800">
                        <span className="font-medium">Suggestion: </span>{f.Suggestion}
                      </div>
                    )}
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
