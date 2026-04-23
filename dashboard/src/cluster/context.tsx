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
