"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DialogRoot, DialogTrigger, DialogPortal, DialogBackdrop, DialogPopup, DialogTitle, DialogClose } from "@/components/ui/dialog";
import { TagInput } from "@/components/tag-input";
import { createRun } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import type { CreateRunRequest } from "@/lib/types";

interface Props {
  onCreated: () => void;
}

export function CreateRunDialog({ onCreated }: Props) {
  const { t, lang } = useI18n();
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [name, setName] = useState("");
  const [namespace, setNamespace] = useState("kube-agent-helper");
  const [scope, setScope] = useState<"namespace" | "cluster">("namespace");
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [labelSelector, setLabelSelector] = useState<string[]>([]);
  const [skills, setSkills] = useState<string[]>([]);
  const [modelConfigRef, setModelConfigRef] = useState("anthropic-credentials");
  const [timeoutSeconds, setTimeoutSeconds] = useState<string>("");
  const [outputLanguage, setOutputLanguage] = useState<"zh" | "en">(lang);
  const [schedule, setSchedule] = useState("");
  const [historyLimit, setHistoryLimit] = useState<string>("");

  function parseLabelSelector(tags: string[]): Record<string, string> {
    const result: Record<string, string> = {};
    for (const tag of tags) {
      const idx = tag.indexOf("=");
      if (idx > 0) result[tag.slice(0, idx)] = tag.slice(idx + 1);
    }
    return result;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    const body: CreateRunRequest = {
      name: name || undefined,
      namespace,
      target: {
        scope,
        namespaces: scope === "namespace" && namespaces.length > 0 ? namespaces : undefined,
        labelSelector: labelSelector.length > 0 ? parseLabelSelector(labelSelector) : undefined,
      },
      skills: skills.length > 0 ? skills : undefined,
      modelConfigRef,
      timeoutSeconds: Number(timeoutSeconds) > 0 ? Number(timeoutSeconds) : undefined,
      outputLanguage,
      schedule: schedule || undefined,
      historyLimit: historyLimit ? Number(historyLimit) : undefined,
    };
    setLoading(true);
    try {
      await createRun(body);
      setOpen(false);
      onCreated();
      setName(""); setNamespaces([]); setLabelSelector([]); setSkills([]); setTimeoutSeconds(""); setSchedule(""); setHistoryLimit("");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("runs.form.error"));
    } finally {
      setLoading(false);
    }
  }

  const inputClass = "w-full rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-sm text-gray-900 outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100";
  const labelClass = "text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400";
  const hintClass = "font-normal normal-case text-gray-400 dark:text-gray-500";

  return (
    <DialogRoot open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm"><Plus className="size-4" />{t("runs.createButton")}</Button>} />
      <DialogPortal>
        <DialogBackdrop />
        <DialogPopup>
          <form onSubmit={handleSubmit} className="max-h-[85vh] overflow-y-auto p-6 space-y-4">
            <DialogTitle>{t("runs.dialogTitle")}</DialogTitle>

            {error && <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">{error}</div>}

            <div className="space-y-1.5">
              <label className={labelClass}>{t("runs.form.name")} <span className={hintClass}>（{t("runs.form.namePlaceholder")}）</span></label>
              <input value={name} onChange={(e) => setName(e.target.value)} placeholder="run-20260416"
                className={inputClass} />
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>{t("runs.form.namespace")} <span className={hintClass}>（{t("runs.form.optional")}）</span></label>
              <input value={namespace} onChange={(e) => setNamespace(e.target.value)} placeholder="kube-agent-helper"
                className={inputClass} />
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>{t("runs.form.scope")} *</label>
              <div className="flex gap-2">
                {(["namespace", "cluster"] as const).map((s) => (
                  <button key={s} type="button" onClick={() => setScope(s)}
                    className={`rounded-lg px-4 py-1.5 text-sm font-medium transition-colors ${scope === s ? "bg-blue-600 text-white" : "border border-gray-200 text-gray-600 hover:border-blue-300 dark:border-gray-700 dark:text-gray-400"}`}>
                    {t(`runs.form.scope.${s}`)}
                  </button>
                ))}
              </div>
              <p className="text-xs text-gray-400 dark:text-gray-500">
                <strong className="text-gray-500 dark:text-gray-400">{t("runs.form.scope.namespace")}</strong> — {t("runs.form.scope.namespaceDesc")} &nbsp;·&nbsp;
                <strong className="text-gray-500 dark:text-gray-400">{t("runs.form.scope.cluster")}</strong> — {t("runs.form.scope.clusterDesc")}
              </p>
            </div>

            {scope === "namespace" && (
              <div className="space-y-1.5">
                <label className={labelClass}>{t("runs.form.namespaces")} <span className={hintClass}>（{t("runs.form.namespacesHint")}）</span></label>
                <TagInput value={namespaces} onChange={setNamespaces} placeholder={t("runs.form.namespacesPlaceholder")} />
              </div>
            )}

            <div className="space-y-1.5">
              <label className={labelClass}>{t("runs.form.labelSelector")} <span className={hintClass}>（{t("runs.form.labelSelectorOptional")}）</span></label>
              <p className="text-xs text-gray-400 dark:text-gray-500">{t("runs.form.labelSelectorHint")}</p>
              <TagInput value={labelSelector} onChange={setLabelSelector} placeholder={t("runs.form.labelSelectorPlaceholder")} />
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>{t("runs.form.skills")} <span className={hintClass}>（{t("runs.form.skillsHint")}）</span></label>
              <TagInput value={skills} onChange={setSkills} placeholder={t("runs.form.skillsPlaceholder")} />
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>{t("runs.form.modelConfigRef")} *</label>
              <input required value={modelConfigRef} onChange={(e) => setModelConfigRef(e.target.value)} placeholder="anthropic-credentials"
                className={inputClass} />
              <p className="text-xs text-gray-400 dark:text-gray-500">{t("runs.form.modelConfigRefHint")}</p>
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>
                {t("runs.form.timeout")} <span className={hintClass}>（{t("runs.form.timeoutHint")}）</span>
              </label>
              <input type="number" min={1} value={timeoutSeconds} onChange={(e) => setTimeoutSeconds(e.target.value)}
                placeholder="600" className={inputClass} />
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>{t("runs.form.outputLanguage")} *</label>
              <div className="flex gap-2">
                {(["zh", "en"] as const).map((l) => (
                  <button key={l} type="button" onClick={() => setOutputLanguage(l)}
                    className={`rounded-lg px-4 py-1.5 text-sm font-medium transition-colors ${outputLanguage === l ? "bg-blue-600 text-white" : "border border-gray-200 text-gray-600 hover:border-blue-300 dark:border-gray-700 dark:text-gray-400"}`}>
                    {t(`runs.form.outputLanguage.${l}`)}
                  </button>
                ))}
              </div>
              <p className="text-xs text-gray-400 dark:text-gray-500">{t("runs.form.outputLanguageHint")}</p>
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>
                {t("runs.form.schedule")} <span className={hintClass}>（{t("runs.form.scheduleHint")}）</span>
              </label>
              <div className="flex flex-wrap gap-1.5 mb-1.5">
                {[
                  { label: t("runs.form.schedulePreset.none"), value: "" },
                  { label: t("runs.form.schedulePreset.hourly"), value: "0 * * * *" },
                  { label: t("runs.form.schedulePreset.daily"), value: "0 8 * * *" },
                  { label: t("runs.form.schedulePreset.weekly"), value: "0 8 * * 1" },
                ].map((p) => (
                  <button key={p.value} type="button" onClick={() => setSchedule(p.value)}
                    className={`rounded px-2.5 py-1 text-xs font-medium transition-colors ${schedule === p.value ? "bg-blue-600 text-white" : "border border-gray-200 text-gray-600 hover:border-blue-300 dark:border-gray-700 dark:text-gray-400"}`}>
                    {p.label}{p.value && <span className="ml-1 font-mono opacity-60">{p.value}</span>}
                  </button>
                ))}
              </div>
              <input value={schedule} onChange={(e) => setSchedule(e.target.value)}
                placeholder="*/30 * * * *  （留空=一次性）"
                className={`${inputClass} font-mono`} />
            </div>

            {schedule && (
              <div className="space-y-1.5">
                <label className={labelClass}>
                  {t("runs.form.historyLimit")} <span className={hintClass}>（{t("runs.form.historyLimitHint")}）</span>
                </label>
                <input type="number" min={1} value={historyLimit} onChange={(e) => setHistoryLimit(e.target.value)}
                  placeholder="10" className={inputClass} />
              </div>
            )}

            <div className="flex justify-end gap-2 pt-2">
              <DialogClose render={<Button type="button" variant="outline" disabled={loading}>{t("common.cancel")}</Button>} />
              <Button type="submit" disabled={loading}>{loading ? t("runs.form.submitting") : t("runs.form.submit")}</Button>
            </div>
          </form>
        </DialogPopup>
      </DialogPortal>
    </DialogRoot>
  );
}
