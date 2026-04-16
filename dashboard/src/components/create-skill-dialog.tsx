"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DialogRoot, DialogTrigger, DialogPortal, DialogBackdrop, DialogPopup, DialogTitle, DialogClose } from "@/components/ui/dialog";
import { TagInput } from "@/components/tag-input";
import { createSkill } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import type { CreateSkillRequest } from "@/lib/types";

const AVAILABLE_TOOLS = ["kubectl_get", "kubectl_describe", "events_list", "logs_get"];
const DIMENSIONS = ["health", "security", "cost", "reliability"] as const;

interface Props {
  onCreated: () => void;
}

export function CreateSkillDialog({ onCreated }: Props) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [name, setName] = useState("");
  const [namespace, setNamespace] = useState("kube-agent-helper");
  const [dimension, setDimension] = useState<CreateSkillRequest["dimension"]>("health");
  const [description, setDescription] = useState("");
  const [prompt, setPrompt] = useState("");
  const [tools, setTools] = useState<string[]>([]);
  const [requiresData, setRequiresData] = useState<string[]>([]);
  const [enabled, setEnabled] = useState(true);
  const [priority, setPriority] = useState(100);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (tools.length === 0) { setError(t("skills.form.atLeastOneTool")); return; }

    const body: CreateSkillRequest = {
      name, namespace, dimension, description, prompt, tools,
      requiresData: requiresData.length > 0 ? requiresData : undefined,
      enabled, priority,
    };
    setLoading(true);
    try {
      await createSkill(body);
      setOpen(false);
      onCreated();
      setName(""); setDescription(""); setPrompt(""); setTools([]); setRequiresData([]);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("skills.form.error"));
    } finally {
      setLoading(false);
    }
  }

  const inputClass = "w-full rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-sm text-gray-900 outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100";
  const labelClass = "text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400";
  const hintClass = "font-normal normal-case text-gray-400 dark:text-gray-500";

  return (
    <DialogRoot open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm"><Plus className="size-4" />{t("skills.createButton")}</Button>} />
      <DialogPortal>
        <DialogBackdrop />
        <DialogPopup>
          <form onSubmit={handleSubmit} className="max-h-[85vh] overflow-y-auto p-6 space-y-4">
            <DialogTitle>{t("skills.dialogTitle")}</DialogTitle>

            {error && <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">{error}</div>}

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className={labelClass}>{t("skills.form.name")} * <span className={hintClass}>（{t("skills.form.nameHint")}）</span></label>
                <input required value={name} onChange={(e) => setName(e.target.value)} placeholder="my-security-analyst"
                  pattern="[a-z0-9][a-z0-9\-]*"
                  className={inputClass} />
              </div>
              <div className="space-y-1.5">
                <label className={labelClass}>{t("skills.form.namespace")} *</label>
                <input required value={namespace} onChange={(e) => setNamespace(e.target.value)} placeholder="kube-agent-helper"
                  className={inputClass} />
              </div>
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>{t("skills.form.dimension")} *</label>
              <div className="flex flex-wrap gap-2">
                {DIMENSIONS.map((d) => (
                  <button key={d} type="button" onClick={() => setDimension(d)}
                    className={`rounded-lg px-4 py-1.5 text-sm font-medium transition-colors ${dimension === d ? "bg-blue-600 text-white" : "border border-gray-200 text-gray-600 hover:border-blue-300 dark:border-gray-700 dark:text-gray-400"}`}>
                    {t(`dimension.${d}`)}
                  </button>
                ))}
              </div>
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>{t("skills.form.description")} *</label>
              <input required value={description} onChange={(e) => setDescription(e.target.value)} placeholder={t("skills.form.descriptionPlaceholder")}
                className={inputClass} />
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>{t("skills.form.prompt")} *</label>
              <textarea required rows={4} value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder={t("skills.form.promptPlaceholder")}
                className={inputClass + " resize-y"} />
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>{t("skills.form.tools")} * <span className={hintClass}>（{t("skills.form.toolsHint")}）</span></label>
              <TagInput value={tools} onChange={setTools} suggestions={AVAILABLE_TOOLS} />
            </div>

            <div className="space-y-1.5">
              <label className={labelClass}>{t("skills.form.requiresData")} <span className={hintClass}>（{t("skills.form.requiresDataOptional")}）</span></label>
              <p className="text-xs text-gray-400 dark:text-gray-500">{t("skills.form.requiresDataHint")}</p>
              <TagInput value={requiresData} onChange={setRequiresData} placeholder={t("skills.form.requiresDataPlaceholder")} />
            </div>

            <div className="flex items-center justify-between">
              <label className="flex cursor-pointer items-center gap-2">
                <button type="button" role="switch" aria-checked={enabled} onClick={() => setEnabled(!enabled)}
                  className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${enabled ? "bg-blue-600" : "bg-gray-200 dark:bg-gray-700"}`}>
                  <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${enabled ? "translate-x-4" : "translate-x-1"}`} />
                </button>
                <span className="text-sm text-gray-700 dark:text-gray-300">{t("skills.form.enabled")}</span>
              </label>
              <label className="flex items-center gap-2">
                <span className="text-xs text-gray-500 dark:text-gray-400">{t("skills.form.priority")}</span>
                <input type="number" value={priority} onChange={(e) => setPriority(Number(e.target.value))}
                  className="w-16 rounded-lg border border-gray-200 bg-white px-2 py-1 text-center text-sm text-gray-900 outline-none focus:border-blue-400 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100" />
              </label>
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <DialogClose render={<Button type="button" variant="outline" disabled={loading}>{t("common.cancel")}</Button>} />
              <Button type="submit" disabled={loading}>{loading ? t("skills.form.submitting") : t("skills.form.submit")}</Button>
            </div>
          </form>
        </DialogPopup>
      </DialogPortal>
    </DialogRoot>
  );
}
