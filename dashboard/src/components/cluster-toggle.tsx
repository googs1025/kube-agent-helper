"use client";

import { useCluster } from "@/cluster/context";
import { useClusterConfigs } from "@/lib/api";
import { useI18n } from "@/i18n/context";

export function ClusterToggle() {
  const { t } = useI18n();
  const { cluster, setCluster } = useCluster();
  const { data: clusters } = useClusterConfigs();

  if (!clusters || clusters.length <= 1) return null;

  return (
    <select
      className="rounded border border-gray-300 bg-white px-2 py-1 text-xs dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100"
      value={cluster}
      onChange={(e) => setCluster(e.target.value)}
      aria-label={t("cluster.label")}
    >
      {clusters.map((c) => (
        <option key={c.name} value={c.name}>
          {c.name === "local" ? t("cluster.local") : c.name}
          {c.phase === "Error" ? " !" : ""}
        </option>
      ))}
    </select>
  );
}
