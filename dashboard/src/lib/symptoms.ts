export interface SymptomPreset {
  id: string;
  label_zh: string;
  label_en: string;
  skills: string[];
}

export const SYMPTOM_PRESETS: SymptomPreset[] = [
  {
    id: "cpu-high",
    label_zh: "CPU 利用率高",
    label_en: "High CPU usage",
    skills: ["pod-health-analyst", "pod-cost-analyst", "alert-responder"],
  },
  {
    id: "memory-high",
    label_zh: "内存使用率高 / OOMKill",
    label_en: "High memory / OOMKill",
    skills: ["pod-health-analyst", "pod-cost-analyst", "alert-responder", "node-health-analyst"],
  },
  {
    id: "request-slow",
    label_zh: "请求延迟高 / 服务不通",
    label_en: "Slow requests / service unreachable",
    skills: ["pod-health-analyst", "config-drift-analyst", "network-troubleshooter"],
  },
  {
    id: "pod-restart",
    label_zh: "Pod 频繁重启",
    label_en: "Pod frequent restarts",
    skills: ["pod-health-analyst", "reliability-analyst", "alert-responder"],
  },
  {
    id: "pod-not-start",
    label_zh: "Pod 启动失败",
    label_en: "Pod failed to start",
    skills: ["pod-health-analyst", "config-drift-analyst", "reliability-analyst", "node-health-analyst", "storage-analyst"],
  },
  {
    id: "scaling-issue",
    label_zh: "扩缩容异常",
    label_en: "Scaling issues (HPA)",
    skills: ["pod-cost-analyst", "reliability-analyst", "node-health-analyst"],
  },
  {
    id: "rollout-stuck",
    label_zh: "滚动更新卡住",
    label_en: "Rollout stuck",
    skills: ["pod-health-analyst", "reliability-analyst", "rollout-analyst"],
  },
  {
    id: "full-check",
    label_zh: "全面体检",
    label_en: "Full health check",
    skills: [],
  },
];

export function symptomsToSkills(symptomIds: string[]): string[] | undefined {
  if (symptomIds.includes("full-check")) return undefined;
  const skills = new Set<string>();
  for (const id of symptomIds) {
    const preset = SYMPTOM_PRESETS.find((p) => p.id === id);
    if (preset) {
      for (const s of preset.skills) skills.add(s);
    }
  }
  return skills.size > 0 ? Array.from(skills) : undefined;
}
