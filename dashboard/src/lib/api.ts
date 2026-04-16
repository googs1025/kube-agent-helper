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

export async function createRun(body: CreateRunRequest): Promise<void> {
  const res = await fetch("/api/runs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
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
