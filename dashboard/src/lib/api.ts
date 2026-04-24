import useSWR from "swr";
import type { DiagnosticRun, Finding, Skill, CreateRunRequest, CreateSkillRequest, Fix, KubeEvent, ModelConfig } from "./types";

const fetcher = (url: string) => fetch(url).then((res) => res.json());

export interface ClusterItem {
  name: string;
  phase: string;
  prometheusURL?: string;
  description?: string;
}

export function useClusterConfigs() {
  return useSWR<ClusterItem[]>("/api/clusters", fetcher, { refreshInterval: 30000 });
}

export async function createClusterConfig(body: {
  name: string;
  namespace: string;
  secretName: string;
  secretKey: string;
  prometheusURL?: string;
  description?: string;
}) {
  const res = await fetch("/api/clusters", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export function useRuns(opts?: { cluster?: string }) {
  const params = opts?.cluster ? `?cluster=${opts.cluster}` : "";
  return useSWR<DiagnosticRun[]>(`/api/runs${params}`, fetcher, { refreshInterval: 5000 });
}

export function useRun(id: string) {
  return useSWR<DiagnosticRun>(`/api/runs/${id}`, fetcher, { refreshInterval: 5000 });
}

export function useFindings(runId: string) {
  return useSWR<Finding[]>(`/api/runs/${runId}/findings`, fetcher, { refreshInterval: 5000 });
}

export function useSkills() {
  return useSWR<Skill[]>("/api/skills", fetcher, { refreshInterval: 10000 });
}

export async function createRun(body: CreateRunRequest): Promise<{ id: string; yaml: string }> {
  const res = await fetch("/api/runs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
  const obj = await res.json();
  const id = (obj?.metadata?.uid as string) ?? "";
  const yaml = toYAML(obj);
  return { id, yaml };
}

function toYAML(obj: Record<string, unknown>): string {
  // Remove managed fields for readability
  const clean = JSON.parse(JSON.stringify(obj));
  if (clean?.metadata) {
    delete clean.metadata.managedFields;
    delete clean.metadata.resourceVersion;
    delete clean.metadata.generation;
  }
  return jsonToYaml(clean, 0);
}

function jsonToYaml(val: unknown, indent: number): string {
  const pad = "  ".repeat(indent);
  if (val === null || val === undefined) return "null";
  if (typeof val === "boolean") return val ? "true" : "false";
  if (typeof val === "number") return String(val);
  if (typeof val === "string") {
    if (val.includes("\n")) return `|\n${val.split("\n").map(l => pad + "  " + l).join("\n")}`;
    if (/[:{}\[\],#&*?|<>=!%@`]/.test(val) || val === "") return JSON.stringify(val);
    return val;
  }
  if (Array.isArray(val)) {
    if (val.length === 0) return "[]";
    return val.map(v => `\n${pad}- ${jsonToYaml(v, indent + 1).trimStart()}`).join("");
  }
  if (typeof val === "object") {
    const entries = Object.entries(val as Record<string, unknown>).filter(([, v]) => v !== undefined);
    if (entries.length === 0) return "{}";
    return entries.map(([k, v]) => {
      const rendered = jsonToYaml(v, indent + 1);
      if (typeof v === "object" && v !== null && !Array.isArray(v) && Object.keys(v).length > 0)
        return `\n${pad}${k}:${rendered}`;
      if (Array.isArray(v) && v.length > 0)
        return `\n${pad}${k}:${rendered}`;
      return `\n${pad}${k}: ${rendered}`;
    }).join("");
  }
  return String(val);
}

export async function createSkill(body: CreateSkillRequest): Promise<void> {
  const res = await fetch("/api/skills", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
}

export function useFixes(opts?: { cluster?: string }) {
  const params = opts?.cluster ? `?cluster=${opts.cluster}` : "";
  return useSWR<Fix[]>(`/api/fixes${params}`, fetcher, { refreshInterval: 5000 });
}

export function useFix(id: string) {
  return useSWR<Fix>(`/api/fixes/${id}`, fetcher, { refreshInterval: 5000 });
}

export async function approveFix(id: string, approvedBy: string): Promise<void> {
  const res = await fetch(`/api/fixes/${id}/approve`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ approvedBy }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
}

export async function rejectFix(id: string): Promise<void> {
  const res = await fetch(`/api/fixes/${id}/reject`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
}

export async function generateFix(findingID: string): Promise<{ fixID?: string; status?: string }> {
  const res = await fetch(`/api/findings/${findingID}/generate-fix`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
  return res.json();
}

export function useEvents(opts?: { namespace?: string; name?: string; since?: number; limit?: number; cluster?: string }) {
  const params = new URLSearchParams();
  if (opts?.namespace) params.set("namespace", opts.namespace);
  if (opts?.name) params.set("name", opts.name);
  if (opts?.since) params.set("since", String(opts.since));
  if (opts?.limit) params.set("limit", String(opts.limit));
  if (opts?.cluster) params.set("cluster", opts.cluster);
  const query = params.toString();
  return useSWR<KubeEvent[]>(`/api/events${query ? `?${query}` : ""}`, fetcher, { refreshInterval: 15000 });
}

export function useK8sNamespaces() {
  return useSWR<{ name: string }[]>("/api/k8s/resources?kind=Namespace", fetcher);
}

export function useK8sResources(kind: string, namespace: string) {
  const url = namespace
    ? `/api/k8s/resources?kind=${kind}&namespace=${namespace}`
    : null;
  return useSWR<{ name: string; namespace: string }[]>(url, fetcher);
}

export async function getK8sResourceDetail(
  kind: string,
  namespace: string,
  name: string
): Promise<Record<string, unknown>> {
  const res = await fetch(
    `/api/k8s/resources?kind=${kind}&namespace=${namespace}&name=${name}`
  );
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export function useModelConfigs() {
  return useSWR<ModelConfig[]>("/api/modelconfigs", fetcher, { refreshInterval: 10000 });
}

export async function createModelConfig(body: {
  name: string;
  namespace: string;
  provider?: string;
  model?: string;
  baseURL?: string;
  maxTurns?: number;
  secretRef?: string;
  secretKey?: string;
}): Promise<void> {
  const res = await fetch("/api/modelconfigs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
}
