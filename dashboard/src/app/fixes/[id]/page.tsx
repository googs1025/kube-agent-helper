"use client";

import { use, useState, useEffect, useRef } from "react";
import Link from "next/link";
import { useFix, approveFix, rejectFix } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { ResourceDiff } from "@/components/resource-diff";
import { CRDYamlBlock } from "@/components/crd-yaml-block";
import { computeAfter, decodeBefore } from "@/lib/utils";
import { CheckCircle2, XCircle, Loader2 } from "lucide-react";

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
  const [toast, setToast] = useState<{ type: "success" | "error" | "applying"; message: string } | null>(null);
  const prevPhase = useRef<string | undefined>(undefined);

  // Watch for phase transitions after approve — show feedback
  useEffect(() => {
    if (!fix) return;
    const prev = prevPhase.current;
    prevPhase.current = fix.Phase;
    if (!prev) return; // first load
    if (prev !== fix.Phase) {
      if (fix.Phase === "Succeeded") {
        setToast({ type: "success", message: t("fixes.toast.succeeded") });
      } else if (fix.Phase === "Failed") {
        setToast({ type: "error", message: fix.Message || t("fixes.toast.failed") });
      } else if (fix.Phase === "RolledBack") {
        setToast({ type: "error", message: fix.Message || t("fixes.toast.rolledBack") });
      } else if (fix.Phase === "Approved" || fix.Phase === "Applying") {
        setToast({ type: "applying", message: t("fixes.toast.applying") });
      }
    }
  }, [fix, t]);

  // Auto-dismiss success toast after 5s
  useEffect(() => {
    if (toast && toast.type === "success") {
      const timer = setTimeout(() => setToast(null), 5000);
      return () => clearTimeout(timer);
    }
  }, [toast]);

  if (isLoading) return <p className="text-muted-foreground">{t("common.loading")}</p>;
  if (error) return <p className="text-destructive">{t("common.loadFailed")}</p>;
  if (!fix) return <p className="text-muted-foreground">{t("common.notFound")}</p>;

  async function handleApprove() {
    setActing(true);
    setToast({ type: "applying", message: t("fixes.toast.approving") });
    try {
      await approveFix(id, "dashboard-user");
      // Poll faster for 15s to catch the phase transition quickly
      const interval = setInterval(() => mutate(), 2000);
      setTimeout(() => clearInterval(interval), 15000);
    } catch (err) {
      setToast({ type: "error", message: err instanceof Error ? err.message : t("fixes.toast.failed") });
    } finally { setActing(false); }
  }

  async function handleReject() {
    setActing(true);
    try {
      await rejectFix(id);
      mutate();
      setToast({ type: "error", message: t("fixes.toast.rejected") });
    } catch { /* ignore */ } finally { setActing(false); }
  }

  return (
    <div>
      {/* Toast notification */}
      {toast && (
        <div className={`mb-4 flex items-center gap-3 rounded-lg border px-4 py-3 text-sm animate-in fade-in slide-in-from-top-2 ${
          toast.type === "success"
            ? "border-green-500/20 bg-green-500/10 text-green-400"
            : toast.type === "applying"
              ? "border-primary/20 bg-primary/10 text-primary"
              : "border-red-500/20 bg-red-500/10 text-red-400"
        }`}>
          {toast.type === "success" && <CheckCircle2 className="size-5 text-green-400" />}
          {toast.type === "applying" && <Loader2 className="size-5 animate-spin text-primary" />}
          {toast.type === "error" && <XCircle className="size-5 text-red-400" />}
          <span>{toast.message}</span>
          <button onClick={() => setToast(null)} className="ml-auto text-xs opacity-60 hover:opacity-100">&times;</button>
        </div>
      )}

      <Link href="/fixes" className="text-sm text-primary hover:underline">&larr; {t("fixes.detail.backToFixes")}</Link>
      <div className="mt-4 mb-6">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold font-mono">{fix.Name || fix.ID.slice(0, 8)}</h1>
          <Badge className={phaseColors[fix.Phase] || ""}>{t(`phase.${fix.Phase}`)}</Badge>
        </div>
        <div className="mt-2 grid grid-cols-2 gap-4 text-sm text-muted-foreground sm:grid-cols-4">
          <div><span className="font-medium">{t("fixes.detail.target")}:</span> {fix.TargetKind}/{fix.TargetNamespace}/{fix.TargetName}</div>
          <div><span className="font-medium">{t("fixes.detail.strategy")}:</span> {fix.Strategy}</div>
          <div><span className="font-medium">{t("fixes.detail.approval")}:</span> {fix.ApprovalRequired ? t("fixes.detail.approvalRequired") : t("fixes.detail.approvalAuto")}</div>
          <div><span className="font-medium">{t("fixes.detail.run")}:</span>
            <Link href={`/runs/${fix.RunID}`} className="ml-1 text-primary hover:underline">{fix.RunID.slice(0, 8)}…</Link>
          </div>
        </div>
        {fix.Message && (
          <div className={`mt-3 rounded-lg border px-3 py-2 text-sm ${
            fix.Phase === "Failed" || fix.Phase === "RolledBack"
              ? "border-red-500/20 bg-red-500/10 text-red-400"
              : fix.Phase === "PendingApproval"
                ? "border-yellow-500/20 bg-yellow-500/10 text-yellow-400"
                : "border-border bg-muted/30 text-muted-foreground"
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

      {/* Post-apply verification hint */}
      {(fix.Phase === "Succeeded" || fix.Phase === "Failed" || fix.Phase === "RolledBack") && (
        <Card className="mb-6">
          <CardContent className="py-4">
            <p className="text-sm text-muted-foreground mb-2">
              {fix.Phase === "Succeeded" ? t("fixes.verify.successHint") : t("fixes.verify.failHint")}
            </p>
            <code className="block rounded-lg bg-muted/30 px-4 py-2 text-sm text-muted-foreground select-all">
              kubectl get {fix.TargetKind.toLowerCase()} {fix.TargetName} -n {fix.TargetNamespace} -o yaml
            </code>
          </CardContent>
        </Card>
      )}

      <Separator className="mb-6" />

      {fix.BeforeSnapshot && (
        <Card className="mb-4">
          <CardHeader>
            <CardTitle className="text-base">{t("fixes.detail.diffTitle")}</CardTitle>
          </CardHeader>
          <CardContent>
            {(() => {
              if (fix.PatchType === "json-patch") {
                return <p className="text-sm text-muted-foreground">{t("fixes.detail.diffUnavailable")}</p>;
              }
              const before = decodeBefore(fix.BeforeSnapshot);
              const after = computeAfter(fix.BeforeSnapshot, fix.PatchType, fix.PatchContent);
              if (!after) {
                return <p className="text-sm text-muted-foreground">{t("fixes.detail.diffUnavailable")}</p>;
              }
              return <ResourceDiff before={before} after={after} />;
            })()}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("fixes.detail.patchContent")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 mb-2">
            <Badge variant="outline">{fix.PatchType}</Badge>
            <span className="text-xs text-muted-foreground">{t("fixes.detail.finding")}: {fix.FindingTitle}</span>
          </div>
          <pre className="overflow-x-auto rounded-lg bg-muted/30 p-4 text-sm text-muted-foreground">
            {fix.PatchContent}
          </pre>
        </CardContent>
      </Card>

      <CRDYamlBlock
        title={t("fixes.detail.crdYaml")}
        yaml={[
          `apiVersion: k8sai.io/v1alpha1`,
          `kind: DiagnosticFix`,
          `metadata:`,
          `  name: ${fix.ID}`,
          `  namespace: kube-agent-helper`,
          `spec:`,
          `  diagnosticRunRef: ${fix.RunID}`,
          `  findingTitle: ${JSON.stringify(fix.FindingTitle)}`,
          `  targetKind: ${fix.TargetKind}`,
          `  targetNamespace: ${fix.TargetNamespace}`,
          `  targetName: ${fix.TargetName}`,
          `  strategy: ${fix.Strategy}`,
          `  approvalRequired: ${fix.ApprovalRequired}`,
          `  patchType: ${fix.PatchType}`,
          `  patchContent: |`,
          ...fix.PatchContent.split("\n").map(l => `    ${l}`),
          `status:`,
          `  phase: ${fix.Phase}`,
          fix.ApprovedBy ? `  approvedBy: ${fix.ApprovedBy}` : null,
          fix.Message ? `  message: ${JSON.stringify(fix.Message)}` : null,
        ].filter(Boolean).join("\n")}
      />
    </div>
  );
}
