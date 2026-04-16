"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useFix, approveFix, rejectFix } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";

const phaseColors: Record<string, string> = {
  PendingApproval: "bg-yellow-100 text-yellow-800",
  Approved: "bg-blue-100 text-blue-800",
  Applying: "bg-blue-100 text-blue-800",
  Succeeded: "bg-green-100 text-green-800",
  Failed: "bg-red-100 text-red-800",
  RolledBack: "bg-orange-100 text-orange-800",
  DryRunComplete: "bg-purple-100 text-purple-800",
};

export default function FixDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { data: fix, error, isLoading, mutate } = useFix(id);
  const [acting, setActing] = useState(false);

  if (isLoading) return <p className="text-gray-500">Loading fix...</p>;
  if (error) return <p className="text-red-600">Failed to load fix.</p>;
  if (!fix) return <p className="text-gray-500">Fix not found.</p>;

  async function handleApprove() {
    setActing(true);
    try {
      await approveFix(id, "dashboard-user");
      mutate();
    } catch { /* ignore */ } finally { setActing(false); }
  }

  async function handleReject() {
    setActing(true);
    try {
      await rejectFix(id);
      mutate();
    } catch { /* ignore */ } finally { setActing(false); }
  }

  return (
    <div>
      <Link href="/fixes" className="text-sm text-blue-600 hover:underline">&larr; Back to Fixes</Link>
      <div className="mt-4 mb-6">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold font-mono">{fix.ID.slice(0, 8)}</h1>
          <Badge className={phaseColors[fix.Phase] || ""}>{fix.Phase}</Badge>
        </div>
        <div className="mt-2 grid grid-cols-2 gap-4 text-sm text-gray-600 sm:grid-cols-4">
          <div><span className="font-medium">Target:</span> {fix.TargetKind}/{fix.TargetNamespace}/{fix.TargetName}</div>
          <div><span className="font-medium">Strategy:</span> {fix.Strategy}</div>
          <div><span className="font-medium">Approval:</span> {fix.ApprovalRequired ? "Required" : "Auto"}</div>
          <div><span className="font-medium">Run:</span>
            <Link href={`/runs/${fix.RunID}`} className="ml-1 text-blue-600 hover:underline">{fix.RunID.slice(0, 8)}</Link>
          </div>
        </div>
        {fix.Message && (
          <div className={`mt-3 rounded-lg border px-3 py-2 text-sm ${
            fix.Phase === "Failed" || fix.Phase === "RolledBack"
              ? "border-red-200 bg-red-50 text-red-700"
              : fix.Phase === "PendingApproval"
                ? "border-yellow-200 bg-yellow-50 text-yellow-800"
                : "border-gray-200 bg-gray-50 text-gray-700"
          }`}>
            {fix.Message}
          </div>
        )}
      </div>

      {fix.Phase === "PendingApproval" && (
        <div className="mb-6 flex gap-3">
          <Button onClick={handleApprove} disabled={acting}>
            {acting ? "Processing..." : "Approve"}
          </Button>
          <Button variant="outline" onClick={handleReject} disabled={acting}>
            Reject
          </Button>
        </div>
      )}

      <Separator className="mb-6" />

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Patch Content</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 mb-2">
            <Badge variant="outline">{fix.PatchType}</Badge>
            <span className="text-xs text-gray-500">Finding: {fix.FindingTitle}</span>
          </div>
          <pre className="overflow-x-auto rounded-lg bg-gray-900 p-4 text-sm text-gray-100">
            {fix.PatchContent}
          </pre>
        </CardContent>
      </Card>
    </div>
  );
}
