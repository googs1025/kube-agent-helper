import useSWR from "swr";
import type { DiagnosticRun, Finding, Skill, CreateRunRequest, CreateSkillRequest } from "./types";

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
