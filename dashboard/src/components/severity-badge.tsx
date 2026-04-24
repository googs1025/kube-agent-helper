"use client";

import { useI18n } from "@/i18n/context";

const config: Record<string, { bg: string; text: string; dot: string }> = {
  critical: { bg: "bg-red-500/10",    text: "text-red-400",    dot: "bg-red-400" },
  high:     { bg: "bg-orange-500/10", text: "text-orange-400", dot: "bg-orange-400" },
  medium:   { bg: "bg-yellow-500/10", text: "text-yellow-400", dot: "bg-yellow-400" },
  low:      { bg: "bg-sky-500/10",    text: "text-sky-400",    dot: "bg-sky-400" },
};

interface Props {
  severity: string;
}

export function SeverityBadge({ severity }: Props) {
  const { t } = useI18n();
  const c = config[severity] ?? { bg: "bg-slate-500/10", text: "text-slate-400", dot: "bg-slate-400" };
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-md border border-current/20 px-2 py-0.5 text-xs font-semibold ${c.bg} ${c.text}`}>
      <span className={`size-1.5 rounded-full ${c.dot}`} />
      {t(`severity.${severity}`)}
    </span>
  );
}
