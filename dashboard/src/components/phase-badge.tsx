"use client";

import { useI18n } from "@/i18n/context";

const colors: Record<string, string> = {
  Pending: "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300",
  Running: "bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300",
  Succeeded: "bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-300",
  Failed: "bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-300",
  Scheduled: "bg-purple-100 text-purple-700 dark:bg-purple-950 dark:text-purple-300",
};

interface Props {
  phase: string;
}

export function PhaseBadge({ phase }: Props) {
  const { t } = useI18n();
  const cls = colors[phase] || "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300";
  return <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}>{t(`phase.${phase}`)}</span>;
}
