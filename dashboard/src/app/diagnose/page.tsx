"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useI18n } from "@/i18n/context";
import { useK8sNamespaces, useK8sResources, getK8sResourceDetail, createRun, useRuns, useModelConfigs } from "@/lib/api";
import { SYMPTOM_PRESETS, symptomsToSkills } from "@/lib/symptoms";
import { PhaseBadge } from "@/components/phase-badge";
import Link from "next/link";

const RESOURCE_TYPES = ["Deployment", "Pod", "StatefulSet", "DaemonSet"];

export default function DiagnosePage() {
  const { t, lang } = useI18n();
  const router = useRouter();

  const [namespace, setNamespace] = useState("");
  const [resourceType, setResourceType] = useState("Deployment");
  const [resourceName, setResourceName] = useState("");
  const [symptoms, setSymptoms] = useState<string[]>([]);
  const [outputLang, setOutputLang] = useState<"zh" | "en">("zh");
  const [schedule, setSchedule] = useState("");
  const [customSchedule, setCustomSchedule] = useState(false);
  const [modelConfigRef, setModelConfigRef] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [createdYAML, setCreatedYAML] = useState<string | null>(null);
  const [createdId, setCreatedId] = useState<string | null>(null);

  const { data: namespaces } = useK8sNamespaces();
  const { data: modelConfigs } = useModelConfigs();
  const { data: resources } = useK8sResources(resourceType, namespace);
  const { data: runs } = useRuns();

  const toggleSymptom = (id: string) => {
    if (id === "full-check") {
      setSymptoms(["full-check"]);
      return;
    }
    setSymptoms((prev) => {
      const without = prev.filter((s) => s !== "full-check");
      return without.includes(id) ? without.filter((s) => s !== id) : [...without, id];
    });
  };

  const handleSubmit = async () => {
    if (symptoms.length === 0) return;
    setSubmitting(true);
    setError("");

    try {
      let labelSelector: Record<string, string> | undefined;
      if (resourceName) {
        const detail = await getK8sResourceDetail(resourceType, namespace, resourceName);
        const spec = (detail as Record<string, unknown>).spec as Record<string, unknown> | undefined;
        const selector = spec?.selector as Record<string, unknown> | undefined;
        const matchLabels = selector?.matchLabels as Record<string, string> | undefined;

        if (matchLabels && (resourceType === "Deployment" || resourceType === "StatefulSet")) {
          labelSelector = matchLabels;
        } else {
          const meta = (detail as Record<string, unknown>).metadata as Record<string, unknown>;
          const labels = meta?.labels as Record<string, string> | undefined;
          if (labels) {
            const appLabel = labels["app"] || labels["app.kubernetes.io/name"];
            if (appLabel) {
              labelSelector = { app: appLabel };
            }
          }
        }
      }

      const symptomSuffix = symptoms.slice(0, 2).join("-");
      const scopeLabel = resourceName || namespace || "cluster";
      const runName = `diagnose-${scopeLabel}-${symptomSuffix}-${Math.random().toString(36).slice(2, 6)}`;

      const { id: runId, yaml } = await createRun({
        name: runName,
        namespace: "kube-agent-helper",
        target: {
          scope: namespace ? "namespace" : "cluster",
          namespaces: namespace ? [namespace] : undefined,
          labelSelector,
        },
        skills: symptomsToSkills(symptoms),
        modelConfigRef: modelConfigRef || (modelConfigs?.[0]?.name ?? "anthropic-credentials"),
        outputLanguage: outputLang,
        ...(schedule ? { schedule } : {}),
      });

      setCreatedYAML(yaml);
      setCreatedId(runId);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  const recentDiagnoses = (runs || [])
    .filter((r) => r.ID && (r.TargetJSON || "").includes("namespace"))
    .slice(0, 5);

  if (createdYAML && createdId) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3 dark:border-green-800 dark:bg-green-950">
          <p className="text-sm font-medium text-green-700 dark:text-green-300">
            {t("diagnose.created")} — <code className="font-mono text-xs">{createdId.slice(0, 8)}</code>
          </p>
        </div>
        <div>
          <p className="text-sm font-medium mb-2 text-muted-foreground">{t("diagnose.createdYAML")}</p>
          <pre className="rounded-lg border border-border bg-[#0a0e14] text-slate-200 p-4 text-xs font-mono overflow-x-auto whitespace-pre leading-relaxed">{createdYAML}</pre>
        </div>
        <div className="flex gap-3">
          <Link
            href={`/runs/${encodeURIComponent(createdId)}`}
            className="rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground hover:opacity-90"
          >
            {t("diagnose.goToRun")} →
          </Link>
          <button
            onClick={() => { setCreatedYAML(null); setCreatedId(null); }}
            className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-muted transition-colors"
          >
            {t("diagnose.createAnother")}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-bold">{t("diagnose.title")}</h1>

      <div className="rounded-lg border border-border bg-card p-6 space-y-6">
        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">{t("diagnose.namespace")}</label>
          <select
            value={namespace}
            onChange={(e) => { setNamespace(e.target.value); setResourceName(""); }}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
          >
            <option value="">{t("diagnose.namespacePlaceholder")}</option>
            {(namespaces || []).map((ns) => (
              <option key={ns.name} value={ns.name}>{ns.name}</option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">{t("diagnose.resourceType")}</label>
          <div className="flex gap-2">
            {RESOURCE_TYPES.map((rt) => (
              <button
                key={rt}
                type="button"
                onClick={() => { setResourceType(rt); setResourceName(""); }}
                className={`rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors ${
                  resourceType === rt
                    ? "border-primary bg-primary/10 text-primary"
                    : "border-border text-muted-foreground hover:border-primary/50 hover:text-foreground"
                }`}
              >
                {rt}
              </button>
            ))}
          </div>
        </div>

        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">
            {t("diagnose.resourceName")}
            <span className="ml-1 normal-case font-normal text-muted-foreground/60">({lang === "zh" ? "可选，留空=全部" : "optional, empty=all"})</span>
          </label>
          <select
            value={resourceName}
            onChange={(e) => setResourceName(e.target.value)}
            disabled={!namespace}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20 disabled:opacity-50"
          >
            <option value="">{t("diagnose.resourceNamePlaceholder")}</option>
            {(resources || []).map((r) => (
              <option key={r.name} value={r.name}>{r.name}</option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-1">{t("diagnose.symptoms")}</label>
          <p className="text-xs text-muted-foreground/70 mb-3">{t("diagnose.symptomsHint")}</p>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {SYMPTOM_PRESETS.map((s) => (
              <label
                key={s.id}
                className={`flex items-center gap-2 rounded-lg border px-3 py-2.5 text-sm cursor-pointer transition-colors ${
                  symptoms.includes(s.id)
                    ? "border-primary bg-primary/10 text-primary"
                    : "border-border text-muted-foreground hover:border-primary/40 hover:text-foreground"
                }`}
              >
                <input
                  type="checkbox"
                  checked={symptoms.includes(s.id)}
                  onChange={() => toggleSymptom(s.id)}
                  className="sr-only"
                />
                <span className={`size-1.5 rounded-full shrink-0 ${symptoms.includes(s.id) ? "bg-primary" : "bg-muted-foreground/40"}`} />
                {lang === "zh" ? s.label_zh : s.label_en}
              </label>
            ))}
          </div>
        </div>

        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">{t("diagnose.outputLanguage")}</label>
          <div className="flex gap-2">
            {(["zh", "en"] as const).map((l) => (
              <button
                key={l}
                type="button"
                onClick={() => setOutputLang(l)}
                className={`rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors ${
                  outputLang === l
                    ? "border-primary bg-primary/10 text-primary"
                    : "border-border text-muted-foreground hover:border-primary/50 hover:text-foreground"
                }`}
              >
                {l === "zh" ? "中文" : "English"}
              </button>
            ))}
          </div>
        </div>

        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">{t("runs.form.modelConfigRef")}</label>
          {(() => {
            const filtered = (modelConfigs || []).filter((mc) => mc.namespace === "kube-agent-helper");
            const displayValue = modelConfigRef || filtered[0]?.name || "";
            return (
              <select
                value={displayValue}
                onChange={(e) => setModelConfigRef(e.target.value)}
                className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
              >
                {filtered.map((mc) => (
                  <option key={`${mc.namespace}/${mc.name}`} value={mc.name}>
                    {mc.name} ({mc.model})
                  </option>
                ))}
                {filtered.length === 0 && (
                  <option value="anthropic-credentials">anthropic-credentials</option>
                )}
              </select>
            );
          })()}
        </div>

        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">{t("diagnose.schedule")}</label>
          <div className="flex flex-wrap gap-2 mb-2">
            {[
              { label: t("diagnose.schedulePreset.none"), value: "" },
              { label: t("diagnose.schedulePreset.hourly"), value: "0 * * * *" },
              { label: t("diagnose.schedulePreset.daily"), value: "0 8 * * *" },
              { label: t("diagnose.schedulePreset.weekly"), value: "0 8 * * 1" },
            ].map((p) => (
              <button
                key={p.value}
                type="button"
                onClick={() => { setSchedule(p.value); setCustomSchedule(false); }}
                className={`rounded-lg border px-3 py-1.5 text-sm transition-colors ${
                  !customSchedule && schedule === p.value
                    ? "border-primary bg-primary/10 text-primary"
                    : "border-border text-muted-foreground hover:border-primary/50"
                }`}
              >
                {p.label}{p.value && <span className="ml-1.5 font-mono text-xs opacity-60">{p.value}</span>}
              </button>
            ))}
            <button
              type="button"
              onClick={() => { setCustomSchedule(true); setSchedule(""); }}
              className={`rounded-lg border px-3 py-1.5 text-sm transition-colors ${
                customSchedule
                  ? "border-primary bg-primary/10 text-primary"
                  : "border-border text-muted-foreground hover:border-primary/50"
              }`}
            >
              {t("diagnose.schedulePreset.custom")}
            </button>
          </div>
          {customSchedule && (
            <input
              type="text"
              value={schedule}
              onChange={(e) => setSchedule(e.target.value)}
              placeholder="*/30 * * * *"
              autoFocus
              className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm font-mono mb-2 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
            />
          )}
          <div className="rounded-lg border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
            <p className="font-medium mb-1">{t("diagnose.cronHelp.title")}</p>
            <code className="block font-mono tracking-wider mb-1">┌─ {t("diagnose.cronHelp.minute")}  (0–59)<br />│ ┌─ {t("diagnose.cronHelp.hour")}    (0–23)<br />│ │ ┌─ {t("diagnose.cronHelp.day")}    (1–31)<br />│ │ │ ┌─ {t("diagnose.cronHelp.month")}  (1–12)<br />│ │ │ │ ┌─ {t("diagnose.cronHelp.weekday")} (0–7)<br />* * * * *</code>
            <p className="mt-1">{t("diagnose.cronHelp.hint")}</p>
          </div>
        </div>

        {error && <p className="text-sm text-red-500">{t("diagnose.error")}: {error}</p>}
        <button
          onClick={handleSubmit}
          disabled={submitting || symptoms.length === 0}
          className="rounded-lg bg-primary px-6 py-2 text-sm font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-50 transition-opacity"
        >
          {submitting ? t("diagnose.submitting") : t("diagnose.submit")}
        </button>
      </div>

      <div>
        <h2 className="text-lg font-semibold mb-3">{t("diagnose.recent")}</h2>
        {recentDiagnoses.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("diagnose.recentEmpty")}</p>
        ) : (
          <div className="space-y-2">
            {recentDiagnoses.map((run) => (
              <Link
                key={run.ID}
                href={`/runs/${encodeURIComponent(run.ID)}`}
                className="flex items-center justify-between rounded-lg border border-border px-4 py-3 hover:bg-muted/30 transition-colors"
              >
                <div className="flex items-center gap-3">
                  <PhaseBadge phase={run.Status} />
                  <span className="text-sm font-medium">{run.ID}</span>
                </div>
                <span className="text-xs text-muted-foreground">
                  {new Date(run.CreatedAt).toLocaleString()}
                </span>
              </Link>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
