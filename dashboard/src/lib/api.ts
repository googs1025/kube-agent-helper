import useSWR from "swr";
import type { DiagnosticRun, Finding, Skill, CreateRunRequest, CreateSkillRequest, Fix } from "./types";

const fetcher = (url: string) => fetch(url).then((res) => res.json());

export function useRuns() {
  return useSWR<DiagnosticRun[]>("/api/runs", fetcher, { refreshInterval: 5000 });
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

export async function createRun(body: CreateRunRequest): Promise<string> {
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
  // Backend returns K8s object; uid is stored as ID in the run store
  return (obj?.metadata?.uid as string) ?? "";
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

export function useFixes() {
  return useSWR<Fix[]>("/api/fixes", fetcher, { refreshInterval: 5000 });
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
