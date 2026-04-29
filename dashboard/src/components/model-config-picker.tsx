"use client";

import { useState } from "react";

import { useI18n } from "@/i18n/context";
import type { ModelConfig } from "@/lib/types";

const inputClass =
  "w-full rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-sm text-gray-900 outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100";
const labelClass =
  "text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400";
const hintClass = "text-xs text-gray-400 dark:text-gray-500";

export interface ModelConfigPickerProps {
  configs: ModelConfig[];
  primary: string;
  fallbacks: string[];
  onChange: (primary: string, fallbacks: string[]) => void;
  /** Optional namespace filter; when set, only configs in this namespace are listed. */
  namespace?: string;
}

/**
 * Picks a primary ModelConfig and an ordered list of fallbacks.
 *
 * Layout:
 *   ┌─ Primary ──────┐
 *   │ <select>       │
 *   └────────────────┘
 *   ┌─ Fallback chain ───────────────────────────────┐
 *   │ [haiku ↑ ✕] [opus ✕]   [+ add ▾]              │
 *   └────────────────────────────────────────────────┘
 *
 * The component is fully controlled — parent owns state.
 */
export function ModelConfigPicker({
  configs,
  primary,
  fallbacks,
  onChange,
  namespace,
}: ModelConfigPickerProps) {
  const { t } = useI18n();
  const [adding, setAdding] = useState(false);

  const visible = namespace
    ? configs.filter((c) => c.namespace === namespace)
    : configs;
  const candidates = visible
    .map((c) => c.name)
    .filter((n) => n !== primary && !fallbacks.includes(n));

  if (visible.length === 0) {
    return (
      <p className={hintClass}>
        {t("runs.form.fallbackChainEmpty")}
      </p>
    );
  }

  const handlePrimary = (next: string) => {
    onChange(next, fallbacks.filter((f) => f !== next));
  };
  const handleRemove = (name: string) => {
    onChange(primary, fallbacks.filter((f) => f !== name));
  };
  const handleMoveUp = (idx: number) => {
    if (idx === 0) return;
    const next = [...fallbacks];
    [next[idx - 1], next[idx]] = [next[idx], next[idx - 1]];
    onChange(primary, next);
  };
  const handleAdd = (name: string) => {
    onChange(primary, [...fallbacks, name]);
    setAdding(false);
  };

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <label className={labelClass}>{t("runs.form.primaryModel")} *</label>
        <select
          required
          value={primary}
          onChange={(e) => handlePrimary(e.target.value)}
          className={inputClass}
        >
          {!primary && <option value="">—</option>}
          {visible.map((mc) => (
            <option key={`${mc.namespace}/${mc.name}`} value={mc.name}>
              {mc.name} ({mc.model})
            </option>
          ))}
        </select>
      </div>

      <div className="space-y-1.5">
        <label className={labelClass}>{t("runs.form.fallbackChain")}</label>
        <p className={hintClass}>{t("runs.form.fallbackChainHint")}</p>
        <div className="flex flex-wrap items-center gap-2">
          {fallbacks.map((name, i) => (
            <span
              key={name}
              className="inline-flex items-center gap-1 rounded-full border border-gray-200 bg-gray-50 px-3 py-1 text-xs text-gray-700 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300"
            >
              <span className="text-gray-400">{i + 1}.</span>
              <span>{name}</span>
              {i > 0 && (
                <button
                  type="button"
                  aria-label={`${t("runs.form.fallbackChainMoveUp")} ${name}`}
                  onClick={() => handleMoveUp(i)}
                  className="text-gray-400 hover:text-blue-600"
                >
                  ↑
                </button>
              )}
              <button
                type="button"
                aria-label={`${t("runs.form.fallbackChainRemove")} ${name}`}
                onClick={() => handleRemove(name)}
                className="text-gray-400 hover:text-red-600"
              >
                ×
              </button>
            </span>
          ))}
          {candidates.length > 0 && !adding && (
            <button
              type="button"
              onClick={() => setAdding(true)}
              className="inline-flex items-center gap-1 rounded-full border border-dashed border-gray-300 px-3 py-1 text-xs text-gray-500 hover:border-blue-400 hover:text-blue-600 dark:border-gray-700 dark:text-gray-400"
            >
              + {t("runs.form.fallbackChainAdd")}
            </button>
          )}
          {adding && (
            <select
              autoFocus
              defaultValue=""
              onChange={(e) => e.target.value && handleAdd(e.target.value)}
              onBlur={() => setAdding(false)}
              className={`${inputClass} max-w-xs`}
            >
              <option value="" disabled>
                —
              </option>
              {candidates.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          )}
        </div>
      </div>
    </div>
  );
}
