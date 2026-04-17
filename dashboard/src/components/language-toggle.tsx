"use client";

import { useI18n } from "@/i18n/context";

export function LanguageToggle() {
  const { lang, setLang } = useI18n();
  const next = lang === "zh" ? "en" : "zh";
  const label = lang === "zh" ? "EN" : "中";
  return (
    <button
      type="button"
      onClick={() => setLang(next)}
      aria-label={`Switch to ${next === "zh" ? "Chinese" : "English"}`}
      className="flex h-8 w-8 items-center justify-center rounded-lg text-sm font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
    >
      {label}
    </button>
  );
}
