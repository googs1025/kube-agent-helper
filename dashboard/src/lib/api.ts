/**
 * 后端 API 客户端集合（基于 SWR + fetch）。
 *
 * 设计约定：
 *   - 所有读取用 SWR hook（useXxx），自带 revalidate / 缓存
 *   - 所有写入用 async function，返回 Promise<T>
 *   - URL 都走相对路径 /api/...，由 [...proxy] 转发到后端
 *   - useXxx 接受可选的 { cluster } 参数，附加 ?cluster= 用于多集群过滤
 *
 * 刷新频率：
 *   - useRuns / useRun     5 秒（运行中状态变化频繁）
 *   - useClusterConfigs    30 秒（集群配置不常变）
 *   - 列表分页类（useRunsPaginated）跟随表格状态变更触发，不固定轮询
 */
import useSWR from "swr";
import type { DiagnosticRun, Finding, Skill, CreateRunRequest, CreateSkillRequest, Fix, KubeEvent, ModelConfig, PaginatedResult, ListParams } from "./types";

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

export function useRunsPaginated(params: ListParams) {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined) query.set(k, String(v));
  });
  return useSWR<PaginatedResult<DiagnosticRun>>(`/api/runs?${query.toString()}`, fetcher, { refreshInterval: 5000 });
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

export function useFixesPaginated(params: ListParams) {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined) query.set(k, String(v));
  });
  return useSWR<PaginatedResult<Fix>>(`/api/fixes?${query.toString()}`, fetcher, { refreshInterval: 5000 });
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

export function useEventsPaginated(opts: { namespace?: string; name?: string; since?: number; cluster?: string; page?: number; pageSize?: number }) {
  const params = new URLSearchParams();
  if (opts.namespace) params.set("namespace", opts.namespace);
  if (opts.name) params.set("name", opts.name);
  if (opts.since) params.set("since", String(opts.since));
  if (opts.cluster) params.set("cluster", opts.cluster);
  if (opts.page) params.set("page", String(opts.page));
  if (opts.pageSize) params.set("pageSize", String(opts.pageSize));
  const query = params.toString();
  return useSWR<PaginatedResult<KubeEvent>>(`/api/events${query ? `?${query}` : ""}`, fetcher, { refreshInterval: 15000 });
}

export async function deleteRunsBatch(ids: string[]): Promise<void> {
  const res = await fetch("/api/runs/batch", {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids }),
  });
  if (!res.ok) throw new Error(await res.text());
}

export async function batchApproveFixes(ids: string[], approvedBy?: string): Promise<void> {
  const res = await fetch("/api/fixes/batch-approve", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids, approvedBy: approvedBy || "dashboard-user" }),
  });
  if (!res.ok) throw new Error(await res.text());
}

export async function batchRejectFixes(ids: string[]): Promise<void> {
  const res = await fetch("/api/fixes/batch-reject", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids }),
  });
  if (!res.ok) throw new Error(await res.text());
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

// ── Notification configs ──────────────────────────────────────────────────────

export interface NotificationConfig {
  ID: string;
  Name: string;
  Type: string;
  WebhookURL: string;
  Secret: string;
  Events: string;
  Enabled: boolean;
  CreatedAt: string;
  UpdatedAt: string;
}

export function useNotificationConfigs() {
  return useSWR<NotificationConfig[]>("/api/notification-configs", fetcher, { refreshInterval: 10000 });
}

export async function createNotificationConfig(body: {
  name: string;
  type: string;
  webhookURL: string;
  secret?: string;
  events?: string;
  enabled: boolean;
}): Promise<NotificationConfig> {
  const res = await fetch("/api/notification-configs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function updateNotificationConfig(id: string, body: {
  name: string;
  type: string;
  webhookURL: string;
  secret?: string;
  events?: string;
  enabled: boolean;
}): Promise<NotificationConfig> {
  const res = await fetch(`/api/notification-configs/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function deleteNotificationConfig(id: string): Promise<void> {
  const res = await fetch(`/api/notification-configs/${id}`, {
    method: "DELETE",
  });
  if (!res.ok) throw new Error(await res.text());
}

export async function testNotificationConfig(id: string): Promise<void> {
  const res = await fetch(`/api/notification-configs/${id}/test`, {
    method: "POST",
  });
  if (!res.ok) throw new Error(await res.text());
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
  retries?: number;
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
