"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useI18n } from "@/i18n/context";
import { useRun, useFindings, generateFix } from "@/lib/api";
import { SeverityBadge } from "@/components/severity-badge";
import { PhaseBadge } from "@/components/phase-badge";
import type { Finding } from "@/lib/types";

const SEVERITY_ORDER: Record<string, number> = {
  critical: 0, high: 1, medium: 2, low: 3, info: 4,
};

const SEVERITY_STYLES: Record<string, { border: string; bg: string }> = {
  critical: { border: "border-red-500/30",    bg: "bg-red-500/10" },
  high:     { border: "border-orange-500/30", bg: "bg-orange-500/10" },
  medium:   { border: "border-yellow-500/30", bg: "bg-yellow-500/10" },
  low:      { border: "border-sky-500/30",    bg: "bg-sky-500/10" },
  info:     { border: "border-border",        bg: "bg-muted/30" },
};

function groupBySeverity(findings: Finding[]): [string, Finding[]][] {
  const sorted = [...findings].sort(
    (a, b) => (SEVERITY_ORDER[a.Severity] ?? 99) - (SEVERITY_ORDER[b.Severity] ?? 99)
  );
  const groups = new Map<string, Finding[]>();
  for (const f of sorted) {
    const g = groups.get(f.Severity) || [];
    g.push(f);
    groups.set(f.Severity, g);
  }
  return Array.from(groups.entries());
}

export default function DiagnoseResultPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { t, lang } = useI18n();
  const { data: run, error: runError } = useRun(id);
  const { data: findings } = useFindings(id);
  const [generatingIds, setGeneratingIds] = useState<Set<string>>(new Set());

  if (runError) return <p className="text-destructive">{t("common.loadFailed")}</p>;
  if (!run) return <p className="text-muted-foreground">{t("common.loading")}</p>;

  const grouped = findings ? groupBySeverity(findings) : [];
  const totalFindings = findings?.length ?? 0;

  const displayName = run.ID.startsWith("diagnose-")
    ? run.ID.replace(/^diagnose-/, "").replace(/-[a-z0-9]{4}$/, "").replace(/-/g, " ")
    : run.ID;

  const handleGenerateFix = async (findingId: string) => {
    setGeneratingIds((prev) => new Set(prev).add(findingId));
    try {
      await generateFix(findingId);
    } catch {
      // will show via fix link on next poll
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link href="/diagnose" className="text-sm text-primary hover:underline">
          ← {t("diagnose.title")}
        </Link>
      </div>

      <div className="rounded-lg border border-border bg-card p-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">{displayName}</h1>
            <p className="text-sm text-muted-foreground mt-1">{run.ID}</p>
          </div>
          <PhaseBadge phase={run.Status} />
        </div>

        <div className="mt-4 grid grid-cols-3 gap-4 text-sm">
          <div>
            <span className="text-muted-foreground">{t("runs.detail.created")}</span>
            <p>{new Date(run.CreatedAt).toLocaleString()}</p>
          </div>
          <div>
            <span className="text-muted-foreground">{t("runs.detail.completed")}</span>
            <p>{run.CompletedAt ? new Date(run.CompletedAt).toLocaleString() : "-"}</p>
          </div>
          <div>
            <span className="text-muted-foreground">{t("runs.detail.findings")}</span>
            <p className="font-semibold">{totalFindings}</p>
          </div>
        </div>

        {run.Status === "Running" && (
          <div className="mt-4 flex items-center gap-2 text-sm text-primary">
            <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            {lang === "zh" ? "诊断中..." : "Diagnosing..."}
          </div>
        )}

        {run.Status === "Failed" && run.Message && (
          <div className="mt-4 rounded-lg border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-400">
            {run.Message}
          </div>
        )}
      </div>

      {totalFindings === 0 && run.Status === "Succeeded" && (
        <p className="text-sm text-muted-foreground">{t("runs.findings.empty")}</p>
      )}

      {grouped.map(([severity, items]) => {
        const style = SEVERITY_STYLES[severity] || SEVERITY_STYLES.info;
        return (
          <div key={severity} className="space-y-3">
            <h2 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {t(`severity.${severity}` as Parameters<typeof t>[0]) || severity}
              <span className="rounded-full bg-muted px-2 py-0.5 font-mono">{items.length}</span>
            </h2>
            {items.map((f) => (
              <div key={f.ID} className={`rounded-lg border ${style.border} ${style.bg} p-4 space-y-2`}>
                <div className="flex items-start justify-between gap-2">
                  <h3 className="font-medium">{f.Title}</h3>
                  <SeverityBadge severity={f.Severity} />
                </div>
                {f.ResourceKind && (
                  <p className="text-xs text-muted-foreground">
                    {f.ResourceKind}: {f.ResourceNamespace}/{f.ResourceName}
                  </p>
                )}
                <p className="text-sm">{f.Description}</p>
                {f.Suggestion && (
                  <div className="rounded-lg border border-primary/20 bg-primary/10 p-3 text-sm text-primary">
                    💡 {f.Suggestion}
                  </div>
                )}
                <div className="flex justify-end">
                  {f.FixID ? (
                    <Link href={`/fixes/${f.FixID}`} className="text-sm text-primary hover:underline">
                      {t("runs.findings.viewFix")} →
                    </Link>
                  ) : (
                    <button
                      onClick={() => handleGenerateFix(f.ID)}
                      disabled={generatingIds.has(f.ID)}
                      className="rounded-lg bg-primary px-3 py-1 text-sm font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-50"
                    >
                      {generatingIds.has(f.ID) ? t("runs.findings.generating") : t("runs.findings.generateFix")}
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        );
      })}
    </div>
  );
}
