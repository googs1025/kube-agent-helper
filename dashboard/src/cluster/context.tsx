/**
 * 全局集群上下文。
 *
 * 用途：所有 list 类页面（runs/findings/events/fixes）顶部都有 ClusterToggle
 * 切换器，选中后通过 useCluster() hook 拿到当前 cluster name，作为参数传给
 * useRuns({ cluster }) 等 SWR hook，自动加上 ?cluster= 过滤。
 *
 * 持久化：localStorage["kah-cluster"]，刷新页面、切换 tab 都保留。
 *
 * "local" 是特殊值：表示控制器自身所在集群（不带 ?cluster= 即可）。
 */
"use client";

import { createContext, useContext, useState, ReactNode } from "react";

interface ClusterContextValue {
  cluster: string;
  setCluster: (c: string) => void;
}

const ClusterContext = createContext<ClusterContextValue>({
  cluster: "local",
  setCluster: () => {},
});

export function ClusterProvider({ children }: { children: ReactNode }) {
  const [cluster, setCluster] = useState<string>(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("kah-cluster") ?? "local";
    }
    return "local";
  });

  const handleSet = (c: string) => {
    setCluster(c);
    localStorage.setItem("kah-cluster", c);
  };

  return (
    <ClusterContext.Provider value={{ cluster, setCluster: handleSet }}>
      {children}
    </ClusterContext.Provider>
  );
}

export function useCluster() {
  return useContext(ClusterContext);
}
