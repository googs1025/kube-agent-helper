"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useI18n } from "@/i18n/context";
import { useK8sNamespaces, useK8sResources, getK8sResourceDetail, createRun, useRuns } from "@/lib/api";
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
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const { data: namespaces } = useK8sNamespaces();
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
    if (!namespace || symptoms.length === 0) return;
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
      const runName = resourceName
        ? `diagnose-${resourceName}-${symptomSuffix}-${Math.random().toString(36).slice(2, 6)}`
        : `diagnose-${namespace}-${symptomSuffix}-${Math.random().toString(36).slice(2, 6)}`;

      const runId = await createRun({
        name: runName,
        namespace: "kube-agent-helper",
        target: {
          scope: "namespace",
          namespaces: [namespace],
          labelSelector,
        },
        skills: symptomsToSkills(symptoms),
        modelConfigRef: "anthropic-credentials",
        outputLanguage: outputLang,
        ...(schedule ? { schedule } : {}),
      });

      router.push(`/diagnose/${encodeURIComponent(runId)}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  const recentDiagnoses = (runs || [])
    .filter((r) => r.ID && (r.TargetJSON || "").includes("namespace"))
    .slice(0, 5);

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-bold">{t("diagnose.title")}</h1>

      <div className="rounded-lg border bg-white p-6 shadow-sm dark:bg-gray-900 dark:border-gray-800 space-y-6">
        <div>
          <label className="block text-sm font-medium mb-1">{t("diagnose.namespace")}</label>
          <select
            value={namespace}
            onChange={(e) => { setNamespace(e.target.value); setResourceName(""); }}
            className="w-full rounded border px-3 py-2 text-sm dark:bg-gray-800 dark:border-gray-700"
          >
            <option value="">{t("diagnose.namespacePlaceholder")}</option>
            {(namespaces || []).map((ns) => (
              <option key={ns.name} value={ns.name}>{ns.name}</option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-sm font-medium mb-1">{t("diagnose.resourceType")}</label>
          <div className="flex gap-3">
            {RESOURCE_TYPES.map((rt) => (
              <label key={rt} className="flex items-center gap-1.5 text-sm cursor-pointer">
                <input
                  type="radio" name="resourceType" value={rt}
                  checked={resourceType === rt}
                  onChange={() => { setResourceType(rt); setResourceName(""); }}
                />
                {rt}
              </label>
            ))}
          </div>
        </div>

        <div>
          <label className="block text-sm font-medium mb-1">
            {t("diagnose.resourceName")}
            <span className="text-gray-400 ml-1 font-normal">({lang === "zh" ? "可选，留空=全部" : "optional, empty=all"})</span>
          </label>
          <select
            value={resourceName}
            onChange={(e) => setResourceName(e.target.value)}
            disabled={!namespace}
            className="w-full rounded border px-3 py-2 text-sm dark:bg-gray-800 dark:border-gray-700 disabled:opacity-50"
          >
            <option value="">{t("diagnose.resourceNamePlaceholder")}</option>
            {(resources || []).map((r) => (
              <option key={r.name} value={r.name}>{r.name}</option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-sm font-medium mb-1">{t("diagnose.symptoms")}</label>
          <p className="text-xs text-gray-500 mb-2">{t("diagnose.symptomsHint")}</p>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {SYMPTOM_PRESETS.map((s) => (
              <label
                key={s.id}
                className={`flex items-center gap-2 rounded border px-3 py-2 text-sm cursor-pointer transition-colors ${
                  symptoms.includes(s.id)
                    ? "border-blue-500 bg-blue-50 dark:bg-blue-900/30 dark:border-blue-400"
                    : "border-gray-200 dark:border-gray-700 hover:border-gray-400"
                }`}
              >
                <input
                  type="checkbox"
                  checked={symptoms.includes(s.id)}
                  onChange={() => toggleSymptom(s.id)}
                  className="sr-only"
                />
                {lang === "zh" ? s.label_zh : s.label_en}
              </label>
            ))}
          </div>
        </div>

        <div>
          <label className="block text-sm font-medium mb-1">{t("diagnose.outputLanguage")}</label>
          <div className="flex gap-4">
            <label className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input type="radio" name="outputLang" value="zh" checked={outputLang === "zh"} onChange={() => setOutputLang("zh")} />
              中文
            </label>
            <label className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input type="radio" name="outputLang" value="en" checked={outputLang === "en"} onChange={() => setOutputLang("en")} />
              English
            </label>
          </div>
        </div>

        <div>
          <label className="block text-sm font-medium mb-1">{t("diagnose.schedule")}</label>
          <input
            type="text"
            value={schedule}
            onChange={(e) => setSchedule(e.target.value)}
            placeholder={t("diagnose.schedulePlaceholder")}
            className="w-full rounded border px-3 py-2 text-sm dark:bg-gray-800 dark:border-gray-700 font-mono"
          />
          <p className="text-xs text-gray-500 mt-1">{t("diagnose.scheduleHint")}</p>
        </div>

        {error && <p className="text-sm text-red-600">{t("diagnose.error")}: {error}</p>}
        <button
          onClick={handleSubmit}
          disabled={submitting || !namespace || symptoms.length === 0}
          className="rounded bg-blue-600 px-6 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {submitting ? t("diagnose.submitting") : t("diagnose.submit")}
        </button>
      </div>

      <div>
        <h2 className="text-lg font-semibold mb-3">{t("diagnose.recent")}</h2>
        {recentDiagnoses.length === 0 ? (
          <p className="text-sm text-gray-500">{t("diagnose.recentEmpty")}</p>
        ) : (
          <div className="space-y-2">
            {recentDiagnoses.map((run) => (
              <Link
                key={run.ID}
                href={`/diagnose/${encodeURIComponent(run.ID)}`}
                className="flex items-center justify-between rounded border px-4 py-3 hover:bg-gray-50 dark:border-gray-800 dark:hover:bg-gray-800/50"
              >
                <div className="flex items-center gap-3">
                  <PhaseBadge phase={run.Status} />
                  <span className="text-sm font-medium">{run.ID}</span>
                </div>
                <span className="text-xs text-gray-500">
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
