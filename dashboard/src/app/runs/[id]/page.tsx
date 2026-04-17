"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useRun, useFindings, generateFix } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import { PhaseBadge } from "@/components/phase-badge";
import { SeverityBadge } from "@/components/severity-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";

function formatTime(iso: string | null): string {
  if (!iso) return "-";
  return new Date(iso).toLocaleString();
}

export default function RunDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { t } = useI18n();
  const { id } = use(params);
  const { data: run, error: runErr, isLoading: runLoading } = useRun(id);
  const { data: findings, error: findErr, isLoading: findLoading } = useFindings(id);
  const [generating, setGenerating] = useState<Record<string, boolean>>({});

  async function handleGenerate(findingID: string) {
    setGenerating((g) => ({ ...g, [findingID]: true }));
    try {
      await generateFix(findingID);
      // SWR polling (5s interval) will pick up the new FixID; no explicit mutate needed
    } catch (err) {
      console.error("generateFix failed:", err);
    } finally {
      // Keep the "Generating..." label until SWR sees the new FixID (max 60s)
      setTimeout(() => setGenerating((g) => ({ ...g, [findingID]: false })), 60000);
    }
  }

  if (runLoading) return <p className="text-gray-500 dark:text-gray-400">{t("common.loading")}</p>;
  if (runErr) return <p className="text-red-600 dark:text-red-400">{t("common.loadFailed")}</p>;
  if (!run) return <p className="text-gray-500 dark:text-gray-400">{t("common.notFound")}</p>;

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
      <Link href="/" className="text-sm text-blue-600 hover:underline dark:text-blue-400">&larr; {t("runs.detail.backToRuns")}</Link>
      <div className="mt-4 mb-6">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold font-mono">{run.ID.slice(0, 8)}</h1>
          <PhaseBadge phase={run.Status} />
        </div>
        <div className="mt-2 grid grid-cols-2 gap-4 text-sm text-gray-600 sm:grid-cols-4 dark:text-gray-400">
          <div><span className="font-medium">{t("runs.detail.created")}:</span> {formatTime(run.CreatedAt)}</div>
          <div><span className="font-medium">{t("runs.detail.started")}:</span> {formatTime(run.StartedAt)}</div>
          <div><span className="font-medium">{t("runs.detail.completed")}:</span> {formatTime(run.CompletedAt)}</div>
          <div><span className="font-medium">{t("runs.detail.findings")}:</span> {findings?.length ?? 0}</div>
        </div>
        {run.Message && (
          <div className={`mt-3 rounded-lg border px-3 py-2 text-sm ${
            run.Status === "Failed"
              ? "border-red-200 bg-red-50 text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300"
              : run.Status === "Running"
                ? "border-yellow-200 bg-yellow-50 text-yellow-800 dark:border-yellow-900 dark:bg-yellow-950 dark:text-yellow-300"
                : "border-gray-200 bg-gray-50 text-gray-700 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-300"
          }`}>
            {run.Message}
          </div>
        )}
      </div>
      <Separator className="mb-6" />
      <h2 className="mb-4 text-xl font-semibold">{t("runs.findings.title")}</h2>
      {findLoading && <p className="text-gray-500 dark:text-gray-400">{t("runs.findings.loading")}</p>}
      {findErr && <p className="text-red-600 dark:text-red-400">{t("runs.findings.loadFailed")}</p>}
      {findings && findings.length === 0 && <p className="text-gray-500 dark:text-gray-400">{t("runs.findings.empty")}</p>}
      <div className="space-y-6">
        {sortedDims.map((dim) => (
          <div key={dim}>
            <h3 className="mb-3 text-lg font-medium">{t(`dimension.${dim}`)}</h3>
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
                    <p className="text-sm text-gray-700 dark:text-gray-300">{f.Description}</p>
                    {f.ResourceKind && (
                      <p className="mt-2 font-mono text-xs text-gray-500 dark:text-gray-500">
                        {f.ResourceKind}/{f.ResourceNamespace}/{f.ResourceName}
                      </p>
                    )}
                    {f.Suggestion && (
                      <div className="mt-2 rounded bg-blue-50 p-2 text-sm text-blue-800 dark:bg-blue-950 dark:text-blue-200">
                        <span className="font-medium">{t("runs.findings.suggestion")}: </span>{f.Suggestion}
                      </div>
                    )}
                    <div className="mt-3 flex justify-end">
                      {f.FixID ? (
                        <Link href={`/fixes/${f.FixID}`} className="text-sm text-blue-600 hover:underline dark:text-blue-400">
                          {t("runs.findings.viewFix")} →
                        </Link>
                      ) : (
                        <Button size="sm" variant="outline" onClick={() => handleGenerate(f.ID)} disabled={generating[f.ID]}>
                          {generating[f.ID] ? t("runs.findings.generating") : t("runs.findings.generateFix")}
                        </Button>
                      )}
                    </div>
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
