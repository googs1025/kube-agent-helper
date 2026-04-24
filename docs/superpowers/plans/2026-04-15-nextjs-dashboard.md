# Next.js Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Next.js dashboard for visualizing DiagnosticRun results, findings, and skills — consuming the existing REST API at `/api/runs`, `/api/runs/{id}/findings`, and `/api/skills`.

**Architecture:** Standalone Next.js 14 app (App Router) with Tailwind CSS and shadcn/ui. Lives in `dashboard/` at repo root. Fetches data from the controller HTTP API via configurable base URL. Three pages: Runs list, Run detail with findings, Skills list. Auto-refresh via polling. Deployable as a separate container or statically served.

**Tech Stack:** Next.js 14 (App Router), TypeScript, Tailwind CSS, shadcn/ui, SWR for data fetching

---

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `dashboard/package.json` | Project config and dependencies |
| Create | `dashboard/tsconfig.json` | TypeScript config |
| Create | `dashboard/tailwind.config.ts` | Tailwind configuration |
| Create | `dashboard/next.config.ts` | Next.js config with API proxy |
| Create | `dashboard/src/app/layout.tsx` | Root layout with nav |
| Create | `dashboard/src/app/page.tsx` | Runs list page |
| Create | `dashboard/src/app/runs/[id]/page.tsx` | Run detail + findings page |
| Create | `dashboard/src/app/skills/page.tsx` | Skills list page |
| Create | `dashboard/src/lib/api.ts` | API client functions |
| Create | `dashboard/src/lib/types.ts` | TypeScript types matching API responses |
| Create | `dashboard/src/components/severity-badge.tsx` | Severity color badge |
| Create | `dashboard/src/components/phase-badge.tsx` | Phase status badge |
| Create | `dashboard/Dockerfile` | Container build |

---

### Task 1: Initialize Next.js project

**Files:**
- Create: `dashboard/package.json`
- Create: `dashboard/tsconfig.json`
- Create: `dashboard/next.config.ts`
- Create: `dashboard/tailwind.config.ts`
- Create: `dashboard/postcss.config.mjs`
- Create: `dashboard/src/app/globals.css`

- [ ] **Step 1: Scaffold the Next.js project**

Run:
```bash
cd /Users/zhenyu.jiang/kube-agent-helper
npx create-next-app@latest dashboard \
  --typescript --tailwind --eslint --app --src-dir \
  --no-turbopack --import-alias "@/*" \
  --use-npm
```

- [ ] **Step 2: Install shadcn/ui**

Run:
```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npx shadcn@latest init -d
```

- [ ] **Step 3: Add SWR for data fetching**

Run:
```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npm install swr
```

- [ ] **Step 4: Add shadcn components we'll need**

Run:
```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npx shadcn@latest add table badge card separator
```

- [ ] **Step 5: Configure API proxy in next.config.ts**

Replace `dashboard/next.config.ts`:

```typescript
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    const apiURL = process.env.API_URL || "http://localhost:8080";
    return [
      {
        source: "/api/:path*",
        destination: `${apiURL}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
```

- [ ] **Step 6: Verify dev server starts**

Run:
```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npm run dev &
sleep 3
curl -s http://localhost:3000 | head -5
kill %1
```
Expected: HTML response from Next.js

- [ ] **Step 7: Commit**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
git add dashboard/
git commit -m "feat(dashboard): scaffold Next.js 14 project with Tailwind + shadcn/ui"
```

---

### Task 2: Define TypeScript types and API client

**Files:**
- Create: `dashboard/src/lib/types.ts`
- Create: `dashboard/src/lib/api.ts`

- [ ] **Step 1: Create type definitions**

Create `dashboard/src/lib/types.ts`:

```typescript
export interface DiagnosticRun {
  ID: string;
  TargetJSON: string;
  SkillsJSON: string;
  Status: "Pending" | "Running" | "Succeeded" | "Failed";
  Message: string;
  StartedAt: string | null;
  CompletedAt: string | null;
  CreatedAt: string;
}

export interface Finding {
  ID: string;
  RunID: string;
  Dimension: string;
  Severity: "critical" | "high" | "medium" | "low";
  Title: string;
  Description: string;
  ResourceKind: string;
  ResourceNamespace: string;
  ResourceName: string;
  Suggestion: string;
  CreatedAt: string;
}

export interface Skill {
  ID: string;
  Name: string;
  Dimension: string;
  Prompt: string;
  ToolsJSON: string;
  RequiresDataJSON: string;
  Source: "builtin" | "cr";
  Enabled: boolean;
  Priority: number;
  UpdatedAt: string;
}
```

- [ ] **Step 2: Create API client with SWR**

Create `dashboard/src/lib/api.ts`:

```typescript
import useSWR from "swr";
import type { DiagnosticRun, Finding, Skill } from "./types";

const fetcher = (url: string) => fetch(url).then((res) => res.json());

export function useRuns() {
  return useSWR<DiagnosticRun[]>("/api/runs", fetcher, {
    refreshInterval: 5000,
  });
}

export function useRun(id: string) {
  return useSWR<DiagnosticRun>(`/api/runs/${id}`, fetcher, {
    refreshInterval: 5000,
  });
}

export function useFindings(runId: string) {
  return useSWR<Finding[]>(`/api/runs/${runId}/findings`, fetcher, {
    refreshInterval: 5000,
  });
}

export function useSkills() {
  return useSWR<Skill[]>("/api/skills", fetcher, {
    refreshInterval: 10000,
  });
}
```

- [ ] **Step 3: Commit**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
git add dashboard/src/lib/
git commit -m "feat(dashboard): add TypeScript types and SWR-based API client"
```

---

### Task 3: Create shared badge components

**Files:**
- Create: `dashboard/src/components/phase-badge.tsx`
- Create: `dashboard/src/components/severity-badge.tsx`

- [ ] **Step 1: Create PhaseBadge component**

Create `dashboard/src/components/phase-badge.tsx`:

```tsx
import { Badge } from "@/components/ui/badge";

const phaseStyles: Record<string, string> = {
  Pending: "bg-yellow-100 text-yellow-800 hover:bg-yellow-100",
  Running: "bg-blue-100 text-blue-800 hover:bg-blue-100",
  Succeeded: "bg-green-100 text-green-800 hover:bg-green-100",
  Failed: "bg-red-100 text-red-800 hover:bg-red-100",
};

export function PhaseBadge({ phase }: { phase: string }) {
  return (
    <Badge variant="outline" className={phaseStyles[phase] || ""}>
      {phase || "Unknown"}
    </Badge>
  );
}
```

- [ ] **Step 2: Create SeverityBadge component**

Create `dashboard/src/components/severity-badge.tsx`:

```tsx
import { Badge } from "@/components/ui/badge";

const severityStyles: Record<string, string> = {
  critical: "bg-red-600 text-white hover:bg-red-600",
  high: "bg-orange-500 text-white hover:bg-orange-500",
  medium: "bg-yellow-500 text-white hover:bg-yellow-500",
  low: "bg-blue-400 text-white hover:bg-blue-400",
};

export function SeverityBadge({ severity }: { severity: string }) {
  return (
    <Badge className={severityStyles[severity] || ""}>
      {severity}
    </Badge>
  );
}
```

- [ ] **Step 3: Commit**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
git add dashboard/src/components/phase-badge.tsx dashboard/src/components/severity-badge.tsx
git commit -m "feat(dashboard): add PhaseBadge and SeverityBadge components"
```

---

### Task 4: Build root layout with navigation

**Files:**
- Modify: `dashboard/src/app/layout.tsx`

- [ ] **Step 1: Replace the root layout**

Replace `dashboard/src/app/layout.tsx`:

```tsx
import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: "Kube Agent Helper",
  description: "Kubernetes diagnostic dashboard",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="min-h-screen bg-gray-50">
        <nav className="border-b bg-white px-6 py-3">
          <div className="mx-auto flex max-w-7xl items-center gap-8">
            <Link href="/" className="text-lg font-semibold">
              Kube Agent Helper
            </Link>
            <div className="flex gap-6 text-sm">
              <Link
                href="/"
                className="text-gray-600 hover:text-gray-900"
              >
                Runs
              </Link>
              <Link
                href="/skills"
                className="text-gray-600 hover:text-gray-900"
              >
                Skills
              </Link>
            </div>
          </div>
        </nav>
        <main className="mx-auto max-w-7xl px-6 py-8">{children}</main>
      </body>
    </html>
  );
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
git add dashboard/src/app/layout.tsx
git commit -m "feat(dashboard): add root layout with navigation bar"
```

---

### Task 5: Build Runs list page

**Files:**
- Modify: `dashboard/src/app/page.tsx`

- [ ] **Step 1: Implement the Runs list page**

Replace `dashboard/src/app/page.tsx`:

```tsx
"use client";

import Link from "next/link";
import { useRuns } from "@/lib/api";
import { PhaseBadge } from "@/components/phase-badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

function formatTime(iso: string | null): string {
  if (!iso) return "-";
  return new Date(iso).toLocaleString();
}

function duration(start: string | null, end: string | null): string {
  if (!start) return "-";
  const s = new Date(start).getTime();
  const e = end ? new Date(end).getTime() : Date.now();
  const sec = Math.round((e - s) / 1000);
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

export default function RunsPage() {
  const { data: runs, error, isLoading } = useRuns();

  if (isLoading) return <p className="text-gray-500">Loading runs...</p>;
  if (error) return <p className="text-red-600">Failed to load runs.</p>;

  return (
    <div>
      <h1 className="mb-6 text-2xl font-bold">Diagnostic Runs</h1>
      {runs && runs.length === 0 ? (
        <p className="text-gray-500">No runs yet.</p>
      ) : (
        <div className="rounded-lg border bg-white">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Phase</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Duration</TableHead>
                <TableHead>Target</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {runs?.map((run) => {
                let target = "-";
                try {
                  const t = JSON.parse(run.TargetJSON);
                  target = t.namespaces?.join(", ") || t.scope || "-";
                } catch {
                  /* ignore */
                }
                return (
                  <TableRow key={run.ID}>
                    <TableCell>
                      <Link
                        href={`/runs/${run.ID}`}
                        className="font-mono text-sm text-blue-600 hover:underline"
                      >
                        {run.ID.slice(0, 8)}...
                      </Link>
                    </TableCell>
                    <TableCell>
                      <PhaseBadge phase={run.Status} />
                    </TableCell>
                    <TableCell className="text-sm text-gray-600">
                      {formatTime(run.CreatedAt)}
                    </TableCell>
                    <TableCell className="text-sm text-gray-600">
                      {duration(run.StartedAt, run.CompletedAt)}
                    </TableCell>
                    <TableCell className="text-sm text-gray-600">
                      {target}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify build**

Run:
```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npm run build 2>&1 | tail -5
```
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
git add dashboard/src/app/page.tsx
git commit -m "feat(dashboard): add Runs list page with phase badges and auto-refresh"
```

---

### Task 6: Build Run detail + findings page

**Files:**
- Create: `dashboard/src/app/runs/[id]/page.tsx`

- [ ] **Step 1: Implement the Run detail page**

Create `dashboard/src/app/runs/[id]/page.tsx`:

```tsx
"use client";

import { use } from "react";
import Link from "next/link";
import { useRun, useFindings } from "@/lib/api";
import { PhaseBadge } from "@/components/phase-badge";
import { SeverityBadge } from "@/components/severity-badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";

function formatTime(iso: string | null): string {
  if (!iso) return "-";
  return new Date(iso).toLocaleString();
}

const dimensionLabels: Record<string, string> = {
  health: "Health",
  security: "Security",
  cost: "Cost",
  reliability: "Reliability",
};

export default function RunDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = use(params);
  const { data: run, error: runErr, isLoading: runLoading } = useRun(id);
  const { data: findings, error: findErr, isLoading: findLoading } = useFindings(id);

  if (runLoading) return <p className="text-gray-500">Loading run...</p>;
  if (runErr) return <p className="text-red-600">Failed to load run.</p>;
  if (!run) return <p className="text-gray-500">Run not found.</p>;

  // Group findings by dimension
  const grouped: Record<string, typeof findings> = {};
  findings?.forEach((f) => {
    const dim = f.Dimension || "other";
    if (!grouped[dim]) grouped[dim] = [];
    grouped[dim]!.push(f);
  });

  // Sort dimensions: health, security, cost, reliability, other
  const dimOrder = ["health", "security", "cost", "reliability"];
  const sortedDims = Object.keys(grouped).sort(
    (a, b) => (dimOrder.indexOf(a) === -1 ? 99 : dimOrder.indexOf(a)) -
              (dimOrder.indexOf(b) === -1 ? 99 : dimOrder.indexOf(b))
  );

  return (
    <div>
      <Link href="/" className="text-sm text-blue-600 hover:underline">
        &larr; Back to Runs
      </Link>

      <div className="mt-4 mb-6">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold font-mono">{run.ID.slice(0, 8)}</h1>
          <PhaseBadge phase={run.Status} />
        </div>
        <div className="mt-2 grid grid-cols-2 gap-4 text-sm text-gray-600 sm:grid-cols-4">
          <div>
            <span className="font-medium">Created:</span>{" "}
            {formatTime(run.CreatedAt)}
          </div>
          <div>
            <span className="font-medium">Started:</span>{" "}
            {formatTime(run.StartedAt)}
          </div>
          <div>
            <span className="font-medium">Completed:</span>{" "}
            {formatTime(run.CompletedAt)}
          </div>
          <div>
            <span className="font-medium">Findings:</span>{" "}
            {findings?.length ?? 0}
          </div>
        </div>
        {run.Message && (
          <p className="mt-2 text-sm text-gray-700">{run.Message}</p>
        )}
      </div>

      <Separator className="mb-6" />

      <h2 className="mb-4 text-xl font-semibold">Findings</h2>

      {findLoading && <p className="text-gray-500">Loading findings...</p>}
      {findErr && <p className="text-red-600">Failed to load findings.</p>}
      {findings && findings.length === 0 && (
        <p className="text-gray-500">No findings for this run.</p>
      )}

      <div className="space-y-6">
        {sortedDims.map((dim) => (
          <div key={dim}>
            <h3 className="mb-3 text-lg font-medium capitalize">
              {dimensionLabels[dim] || dim}
            </h3>
            <div className="space-y-3">
              {grouped[dim]?.map((f) => (
                <Card key={f.ID}>
                  <CardHeader className="pb-2">
                    <div className="flex items-center justify-between">
                      <CardTitle className="text-base">{f.Title}</CardTitle>
                      <SeverityBadge severity={f.Severity} />
                    </div>
                  </CardHeader>
                  <CardContent>
                    <p className="text-sm text-gray-700">{f.Description}</p>
                    {f.ResourceKind && (
                      <p className="mt-2 font-mono text-xs text-gray-500">
                        {f.ResourceKind}/{f.ResourceNamespace}/{f.ResourceName}
                      </p>
                    )}
                    {f.Suggestion && (
                      <div className="mt-2 rounded bg-blue-50 p-2 text-sm text-blue-800">
                        <span className="font-medium">Suggestion: </span>
                        {f.Suggestion}
                      </div>
                    )}
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Verify build**

Run:
```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npm run build 2>&1 | tail -5
```
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
git add dashboard/src/app/runs/
git commit -m "feat(dashboard): add Run detail page with findings grouped by dimension"
```

---

### Task 7: Build Skills list page

**Files:**
- Create: `dashboard/src/app/skills/page.tsx`

- [ ] **Step 1: Implement the Skills page**

Create `dashboard/src/app/skills/page.tsx`:

```tsx
"use client";

import { useSkills } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

export default function SkillsPage() {
  const { data: skills, error, isLoading } = useSkills();

  if (isLoading) return <p className="text-gray-500">Loading skills...</p>;
  if (error) return <p className="text-red-600">Failed to load skills.</p>;

  return (
    <div>
      <h1 className="mb-6 text-2xl font-bold">Skills</h1>
      {skills && skills.length === 0 ? (
        <p className="text-gray-500">No skills registered.</p>
      ) : (
        <div className="rounded-lg border bg-white">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Dimension</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Enabled</TableHead>
                <TableHead>Priority</TableHead>
                <TableHead>Tools</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {skills?.map((skill) => {
                let tools: string[] = [];
                try {
                  tools = JSON.parse(skill.ToolsJSON);
                } catch {
                  /* ignore */
                }
                return (
                  <TableRow key={skill.ID}>
                    <TableCell className="font-mono text-sm font-medium">
                      {skill.Name}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline" className="capitalize">
                        {skill.Dimension}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={skill.Source === "cr" ? "default" : "secondary"}
                      >
                        {skill.Source}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {skill.Enabled ? (
                        <span className="text-green-600">Yes</span>
                      ) : (
                        <span className="text-gray-400">No</span>
                      )}
                    </TableCell>
                    <TableCell className="text-sm text-gray-600">
                      {skill.Priority}
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {tools.map((tool) => (
                          <Badge
                            key={tool}
                            variant="outline"
                            className="text-xs"
                          >
                            {tool}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify build**

Run:
```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npm run build 2>&1 | tail -5
```
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
git add dashboard/src/app/skills/
git commit -m "feat(dashboard): add Skills list page showing registered skills"
```

---

### Task 8: Add Dockerfile and .gitignore

**Files:**
- Create: `dashboard/Dockerfile`
- Create: `dashboard/.gitignore`

- [ ] **Step 1: Create Dockerfile**

Create `dashboard/Dockerfile`:

```dockerfile
FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
COPY --from=builder /app/public ./public
EXPOSE 3000
CMD ["node", "server.js"]
```

- [ ] **Step 2: Create .gitignore**

Create `dashboard/.gitignore`:

```
node_modules/
.next/
out/
```

- [ ] **Step 3: Final build verification**

Run:
```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npm run build
```
Expected: Build succeeds with no errors

- [ ] **Step 4: Commit**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
git add dashboard/Dockerfile dashboard/.gitignore
git commit -m "feat(dashboard): add Dockerfile for standalone deployment"
```