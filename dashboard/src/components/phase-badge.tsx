"use client";

import { useI18n } from "@/i18n/context";

const config: Record<string, { bg: string; text: string; dot: string; pulse?: boolean }> = {
  Pending:   { bg: "bg-slate-500/10",  text: "text-slate-400",  dot: "bg-slate-400" },
  Running:   { bg: "bg-sky-500/10",    text: "text-sky-400",    dot: "bg-sky-400",   pulse: true },
  Succeeded: { bg: "bg-green-500/10",  text: "text-green-400",  dot: "bg-green-400" },
  Failed:    { bg: "bg-red-500/10",    text: "text-red-400",    dot: "bg-red-400" },
  Unknown:   { bg: "bg-amber-500/10",  text: "text-amber-400",  dot: "bg-amber-400" },
  Scheduled: { bg: "bg-purple-500/10", text: "text-purple-400", dot: "bg-purple-400" },
};

interface Props {
  phase: string;
}

export function PhaseBadge({ phase }: Props) {
  const { t } = useI18n();
  const c = config[phase] ?? { bg: "bg-slate-500/10", text: "text-slate-400", dot: "bg-slate-400" };
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-md border border-current/20 px-2 py-0.5 text-xs font-semibold ${c.bg} ${c.text}`}>
      <span className={`size-1.5 rounded-full ${c.dot} ${c.pulse ? "animate-pulse" : ""}`} />
      {t(`phase.${phase}`)}
    </span>
  );
}
