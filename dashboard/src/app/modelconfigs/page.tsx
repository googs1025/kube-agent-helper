"use client";

import { useState } from "react";
import { useI18n } from "@/i18n/context";
import { useModelConfigs, createModelConfig, useK8sNamespaces } from "@/lib/api";
import type { ModelConfig } from "@/lib/types";

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
      });
      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally {
      setSubmitting(false);
    }
  };

  const inputClass =
    "w-full rounded border border-gray-300 bg-white px-3 py-1.5 text-sm dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-lg rounded-lg bg-white p-6 shadow-xl dark:bg-gray-900"
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
          <div />
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

        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded px-4 py-1.5 text-sm text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
          >
            {t("modelconfigs.create.cancel")}
          </button>
          <button
            type="submit"
            disabled={submitting || !form.name}
            className="rounded bg-blue-600 px-4 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
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
          className="rounded bg-blue-600 px-4 py-1.5 text-sm text-white hover:bg-blue-700"
        >
          + {t("modelconfigs.create.title")}
        </button>
      </div>

      {isLoading && <p className="text-gray-500">{t("modelconfigs.loading")}</p>}

      {!isLoading && (!configs || configs.length === 0) && (
        <p className="text-gray-500">{t("modelconfigs.empty")}</p>
      )}

      {configs && configs.length > 0 && (
        <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 text-left text-xs font-medium text-gray-500 dark:bg-gray-800 dark:text-gray-400">
              <tr>
                <th className="px-4 py-3">{t("modelconfigs.col.name")}</th>
                <th className="px-4 py-3">{t("modelconfigs.col.namespace")}</th>
                <th className="px-4 py-3">{t("modelconfigs.col.provider")}</th>
                <th className="px-4 py-3">{t("modelconfigs.col.model")}</th>
                <th className="px-4 py-3">{t("modelconfigs.col.baseURL")}</th>
                <th className="px-4 py-3">{t("modelconfigs.col.maxTurns")}</th>
                <th className="px-4 py-3">{t("modelconfigs.col.secret")}</th>
                <th className="px-4 py-3">{t("modelconfigs.col.apiKey")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {configs.map((mc: ModelConfig) => (
                <tr key={`${mc.namespace}/${mc.name}`} className="hover:bg-gray-50 dark:hover:bg-gray-800/50">
                  <td className="px-4 py-3 font-medium">{mc.name}</td>
                  <td className="px-4 py-3 text-gray-500">{mc.namespace}</td>
                  <td className="px-4 py-3">
                    <span className="rounded bg-blue-100 px-2 py-0.5 text-xs text-blue-800 dark:bg-blue-900 dark:text-blue-200">
                      {mc.provider}
                    </span>
                  </td>
                  <td className="px-4 py-3 font-mono text-xs">{mc.model}</td>
                  <td className="px-4 py-3 text-gray-500 text-xs font-mono">
                    {mc.baseURL || <span className="text-gray-400 italic">default</span>}
                  </td>
                  <td className="px-4 py-3 text-center">{mc.maxTurns ?? 20}</td>
                  <td className="px-4 py-3 text-xs font-mono">
                    {mc.secretRef}/{mc.secretKey}
                  </td>
                  <td className="px-4 py-3">
                    <code className="rounded bg-gray-100 px-2 py-0.5 text-xs dark:bg-gray-800">
                      {mc.apiKey}
                    </code>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
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
