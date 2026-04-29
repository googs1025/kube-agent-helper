"use client";

import { useState } from "react";
import { useI18n } from "@/i18n/context";
import { useModelConfigs, createModelConfig, useK8sNamespaces } from "@/lib/api";
import type { ModelConfig } from "@/lib/types";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

function CreateDialog({ onClose }: { onClose: () => void }) {
  const { t } = useI18n();
  const { data: namespaces } = useK8sNamespaces();
  const [form, setForm] = useState({
    name: "",
    namespace: "kube-agent-helper",
    provider: "anthropic",
    model: "claude-sonnet-4-6",
    baseURL: "",
    maxTurns: 20,
    retries: 0,
    secretRef: "",
    secretKey: "apiKey",
  });
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      await createModelConfig({
        ...form,
        secretRef: form.secretRef || form.name,
        maxTurns: form.maxTurns || undefined,
        baseURL: form.baseURL || undefined,
        retries: form.retries || undefined,
      });
      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally {
      setSubmitting(false);
    }
  };

  const inputClass =
    "w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-lg rounded-lg bg-card border border-border p-6 shadow-xl"
      >
        <h2 className="mb-4 text-lg font-semibold">{t("modelconfigs.create.title")}</h2>
        {error && <p className="mb-3 text-sm text-red-500">{error}</p>}

        <div className="grid grid-cols-2 gap-3 mb-4">
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("modelconfigs.create.name")}
            </label>
            <input
              className={inputClass}
              required
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("modelconfigs.create.namespace")}
            </label>
            <select
              className={inputClass}
              value={form.namespace}
              onChange={(e) => setForm({ ...form, namespace: e.target.value })}
            >
              {(namespaces || []).map((ns) => (
                <option key={ns.name} value={ns.name}>
                  {ns.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("modelconfigs.create.provider")}
            </label>
            <select
              className={inputClass}
              value={form.provider}
              onChange={(e) => setForm({ ...form, provider: e.target.value })}
            >
              <option value="anthropic">anthropic</option>
            </select>
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("modelconfigs.create.model")}
            </label>
            <input
              className={inputClass}
              value={form.model}
              onChange={(e) => setForm({ ...form, model: e.target.value })}
            />
          </div>
          <div className="col-span-2">
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("modelconfigs.create.baseURL")}
            </label>
            <input
              className={inputClass}
              placeholder="https://api.anthropic.com"
              value={form.baseURL}
              onChange={(e) => setForm({ ...form, baseURL: e.target.value })}
            />
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("modelconfigs.create.maxTurns")}
            </label>
            <input
              type="number"
              className={inputClass}
              value={form.maxTurns}
              onChange={(e) => setForm({ ...form, maxTurns: parseInt(e.target.value) || 20 })}
            />
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("runs.form.modelConfigRetries")}
            </label>
            <input
              type="number"
              min={0}
              max={10}
              className={inputClass}
              value={form.retries}
              onChange={(e) => setForm({ ...form, retries: Math.max(0, Math.min(10, parseInt(e.target.value) || 0)) })}
            />
            <p className="text-xs text-gray-400 mt-0.5">{t("runs.form.modelConfigRetriesHint")}</p>
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("modelconfigs.create.secretRef")}
            </label>
            <input
              className={inputClass}
              placeholder={form.name || "secret-name"}
              value={form.secretRef}
              onChange={(e) => setForm({ ...form, secretRef: e.target.value })}
            />
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("modelconfigs.create.secretKey")}
            </label>
            <input
              className={inputClass}
              value={form.secretKey}
              onChange={(e) => setForm({ ...form, secretKey: e.target.value })}
            />
          </div>
        </div>

        <div className="mb-4 rounded-md border border-blue-200 bg-blue-50 p-3 text-xs text-blue-800 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300">
          <p className="font-semibold mb-1">{t("modelconfigs.create.secretHint.title")}</p>
          <p className="mb-1.5 text-blue-700 dark:text-blue-400">{t("modelconfigs.create.secretHint.desc")}</p>
          <pre className="rounded bg-blue-100 p-2 font-mono text-[11px] leading-relaxed dark:bg-blue-900 select-all overflow-x-auto whitespace-pre">
{`kubectl create secret generic ${form.secretRef || form.name || "<secret-name>"} \\
  --from-literal=${form.secretKey || "apiKey"}=<your-api-key> \\
  -n ${form.namespace || "kube-agent-helper"}`}
          </pre>
        </div>

        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg px-4 py-1.5 text-sm text-muted-foreground hover:bg-muted transition-colors"
          >
            {t("modelconfigs.create.cancel")}
          </button>
          <button
            type="submit"
            disabled={submitting || !form.name}
            className="rounded-lg bg-primary px-4 py-1.5 text-sm font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-50"
          >
            {t("modelconfigs.create.submit")}
          </button>
        </div>
      </form>
    </div>
  );
}

export default function ModelConfigsPage() {
  const { t } = useI18n();
  const { data: configs, isLoading, mutate } = useModelConfigs();
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t("modelconfigs.title")}</h1>
        <button
          onClick={() => setShowCreate(true)}
          className="rounded-lg bg-primary px-4 py-1.5 text-sm font-semibold text-primary-foreground hover:opacity-90"
        >
          + {t("modelconfigs.create.title")}
        </button>
      </div>

      {isLoading && <p className="text-muted-foreground">{t("modelconfigs.loading")}</p>}

      {!isLoading && (!configs || configs.length === 0) && (
        <p className="text-muted-foreground">{t("modelconfigs.empty")}</p>
      )}

      {configs && configs.length > 0 && (
        <div className="rounded-lg border border-border bg-card overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("modelconfigs.col.name")}</TableHead>
                <TableHead>{t("modelconfigs.col.namespace")}</TableHead>
                <TableHead>{t("modelconfigs.col.provider")}</TableHead>
                <TableHead>{t("modelconfigs.col.model")}</TableHead>
                <TableHead>{t("modelconfigs.col.baseURL")}</TableHead>
                <TableHead>{t("modelconfigs.col.maxTurns")}</TableHead>
                <TableHead>{t("runs.form.modelConfigRetries")}</TableHead>
                <TableHead>{t("modelconfigs.col.secret")}</TableHead>
                <TableHead>{t("modelconfigs.col.apiKey")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {configs.map((mc: ModelConfig) => (
                <TableRow key={`${mc.namespace}/${mc.name}`}>
                  <TableCell className="font-medium">{mc.name}</TableCell>
                  <TableCell className="text-muted-foreground">{mc.namespace}</TableCell>
                  <TableCell>
                    <span className="inline-flex items-center rounded-md border border-sky-400/20 bg-sky-500/10 px-2 py-0.5 text-xs font-semibold text-sky-400">
                      {mc.provider}
                    </span>
                  </TableCell>
                  <TableCell className="font-mono text-xs">{mc.model}</TableCell>
                  <TableCell className="text-muted-foreground text-xs font-mono">
                    {mc.baseURL || <span className="italic">default</span>}
                  </TableCell>
                  <TableCell className="text-center">{mc.maxTurns ?? 20}</TableCell>
                  <TableCell className="text-center">{mc.retries ?? 0}</TableCell>
                  <TableCell className="font-mono text-xs">
                    {mc.secretRef}/{mc.secretKey}
                  </TableCell>
                  <TableCell>
                    <code className="rounded-md bg-muted px-2 py-0.5 text-xs font-mono">
                      {mc.apiKey}
                    </code>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {showCreate && (
        <CreateDialog
          onClose={() => {
            setShowCreate(false);
            mutate();
          }}
        />
      )}
    </div>
  );
}
