"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useFix, approveFix, rejectFix } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";

const phaseColors: Record<string, string> = {
  PendingApproval: "bg-yellow-100 text-yellow-800 dark:bg-yellow-950 dark:text-yellow-300",
  Approved: "bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300",
  Applying: "bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300",
  Succeeded: "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300",
  Failed: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300",
  RolledBack: "bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-300",
  DryRunComplete: "bg-purple-100 text-purple-800 dark:bg-purple-950 dark:text-purple-300",
};

export default function FixDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { t } = useI18n();
  const { id } = use(params);
  const { data: fix, error, isLoading, mutate } = useFix(id);
  const [acting, setActing] = useState(false);

  if (isLoading) return <p className="text-gray-500 dark:text-gray-400">{t("common.loading")}</p>;
  if (error) return <p className="text-red-600 dark:text-red-400">{t("common.loadFailed")}</p>;
  if (!fix) return <p className="text-gray-500 dark:text-gray-400">{t("common.notFound")}</p>;

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
      <Link href="/fixes" className="text-sm text-blue-600 hover:underline dark:text-blue-400">&larr; {t("fixes.detail.backToFixes")}</Link>
      <div className="mt-4 mb-6">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold font-mono">{fix.ID.slice(0, 8)}</h1>
          <Badge className={phaseColors[fix.Phase] || ""}>{t(`phase.${fix.Phase}`)}</Badge>
        </div>
        <div className="mt-2 grid grid-cols-2 gap-4 text-sm text-gray-600 sm:grid-cols-4 dark:text-gray-400">
          <div><span className="font-medium">{t("fixes.detail.target")}:</span> {fix.TargetKind}/{fix.TargetNamespace}/{fix.TargetName}</div>
          <div><span className="font-medium">{t("fixes.detail.strategy")}:</span> {fix.Strategy}</div>
          <div><span className="font-medium">{t("fixes.detail.approval")}:</span> {fix.ApprovalRequired ? t("fixes.detail.approvalRequired") : t("fixes.detail.approvalAuto")}</div>
          <div><span className="font-medium">{t("fixes.detail.run")}:</span>
            <Link href={`/runs/${fix.RunID}`} className="ml-1 text-blue-600 hover:underline dark:text-blue-400">{fix.RunID.slice(0, 8)}</Link>
          </div>
        </div>
        {fix.Message && (
          <div className={`mt-3 rounded-lg border px-3 py-2 text-sm ${
            fix.Phase === "Failed" || fix.Phase === "RolledBack"
              ? "border-red-200 bg-red-50 text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300"
              : fix.Phase === "PendingApproval"
                ? "border-yellow-200 bg-yellow-50 text-yellow-800 dark:border-yellow-900 dark:bg-yellow-950 dark:text-yellow-300"
                : "border-gray-200 bg-gray-50 text-gray-700 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-300"
          }`}>
            {fix.Message}
          </div>
        )}
      </div>

      {fix.Phase === "PendingApproval" && (
        <div className="mb-6 flex gap-3">
          <Button onClick={handleApprove} disabled={acting}>
            {acting ? t("common.processing") : t("common.approve")}
          </Button>
          <Button variant="outline" onClick={handleReject} disabled={acting}>
            {t("common.reject")}
          </Button>
        </div>
      )}

      <Separator className="mb-6" />

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("fixes.detail.patchContent")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 mb-2">
            <Badge variant="outline">{fix.PatchType}</Badge>
            <span className="text-xs text-gray-500 dark:text-gray-400">{t("fixes.detail.finding")}: {fix.FindingTitle}</span>
          </div>
          <pre className="overflow-x-auto rounded-lg bg-gray-900 p-4 text-sm text-gray-100 dark:bg-gray-950">
            {fix.PatchContent}
          </pre>
        </CardContent>
      </Card>
    </div>
  );
}
