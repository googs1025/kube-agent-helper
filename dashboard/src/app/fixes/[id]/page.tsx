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

const fixDetailPhaseConfig: Record<string, { bg: string; text: string; dot: string }> = {
  PendingApproval: { bg: "bg-yellow-500/10", text: "text-yellow-400", dot: "bg-yellow-400" },
  Approved:        { bg: "bg-sky-500/10",    text: "text-sky-400",    dot: "bg-sky-400" },
  Applying:        { bg: "bg-sky-500/10",    text: "text-sky-400",    dot: "bg-sky-400" },
  Succeeded:       { bg: "bg-green-500/10",  text: "text-green-400",  dot: "bg-green-400" },
  Failed:          { bg: "bg-red-500/10",    text: "text-red-400",    dot: "bg-red-400" },
  RolledBack:      { bg: "bg-orange-500/10", text: "text-orange-400", dot: "bg-orange-400" },
  DryRunComplete:  { bg: "bg-purple-500/10", text: "text-purple-400", dot: "bg-purple-400" },
};

function FixDetailPhaseBadge({ phase }: { phase: string }) {
  const c = fixDetailPhaseConfig[phase] ?? { bg: "bg-slate-500/10", text: "text-slate-400", dot: "bg-slate-400" };
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-md border border-current/20 px-2 py-0.5 text-xs font-semibold ${c.bg} ${c.text}`}>
      <span className={`size-1.5 rounded-full ${c.dot}`} />
      {phase}
    </span>
  );
}

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
          <FixDetailPhaseBadge phase={fix.Phase} />
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
