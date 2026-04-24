# Frontend Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Overhaul the dashboard UI to an ops-platform style (Argo CD / Rancher-inspired), with polished dark and light modes, using shadcn theme token updates + per-page rewrites.

**Architecture:** Update CSS custom properties in `globals.css` (shadcn tokens + new semantic tokens) so all shadcn components inherit the new palette automatically; then rewrite Nav, shared badges, and all pages to use the new Tailwind classes.

**Tech Stack:** Next.js 15 App Router, Tailwind CSS v4, shadcn/ui, lucide-react, `next/navigation` (`usePathname`)

---

## File Map

| File | Change type | What changes |
|------|-------------|-------------|
| `dashboard/src/app/globals.css` | Modify | `:root` + `.dark` CSS vars, add semantic tokens |
| `dashboard/src/app/layout.tsx` | Modify | Nav: brand dot, active link, body bg token |
| `dashboard/src/components/phase-badge.tsx` | Modify | Dot-badge style with pulse animation |
| `dashboard/src/components/severity-badge.tsx` | Modify | Dot-badge style matching PhaseBadge |
| `dashboard/src/components/ui/table.tsx` | Modify | Header bg, head text style, row hover |
| `dashboard/src/components/ui/card.tsx` | Modify | CardHeader border-b divider |
| `dashboard/src/app/page.tsx` | Modify | Hero accent line, stat text-3xl, table styles |
| `dashboard/src/app/diagnose/page.tsx` | Modify | Form labels, pill resource selector, chips, button |
| `dashboard/src/app/skills/page.tsx` | Modify | Page header, stat cards, table classes |
| `dashboard/src/app/fixes/page.tsx` | Modify | Page header, stat cards, inline phase badge |
| `dashboard/src/app/events/page.tsx` | Modify | Filter inputs, warning badge, table |
| `dashboard/src/app/clusters/page.tsx` | Modify | Phase dot-badge, table, form inputs, button |
| `dashboard/src/app/modelconfigs/page.tsx` | Modify | Table, form inputs, provider badge, button |
| `dashboard/src/app/about/page.tsx` | Modify | ASCII block colors, step circles, CRD badges |
| `dashboard/src/app/runs/[id]/page.tsx` | Modify | Page header hierarchy, finding cards, suggestion block |
| `dashboard/src/app/fixes/[id]/page.tsx` | Modify | Page header, status prominence |
| `dashboard/src/app/diagnose/[id]/page.tsx` | Modify | Finding cards border/bg, suggestion block, buttons |
| `dashboard/style-preview.html` | Delete | Temporary preview file |

---

## Task 1: Update CSS Design Tokens

**Files:**
- Modify: `dashboard/src/app/globals.css`

- [ ] **Step 1: Open the file and replace the `:root` and `.dark` blocks and add semantic tokens**

Replace the entire `:root { ... }` block with:

```css
:root {
  --background: oklch(0.98 0 0);
  --foreground: oklch(0.145 0 0);
  --card: oklch(1 0 0);
  --card-foreground: oklch(0.145 0 0);
  --popover: oklch(1 0 0);
  --popover-foreground: oklch(0.145 0 0);
  --primary: oklch(0.55 0.18 220);
  --primary-foreground: oklch(0.98 0 0);
  --secondary: oklch(0.97 0 0);
  --secondary-foreground: oklch(0.205 0 0);
  --muted: oklch(0.97 0 0);
  --muted-foreground: oklch(0.52 0 0);
  --accent: oklch(0.97 0 0);
  --accent-foreground: oklch(0.205 0 0);
  --destructive: oklch(0.577 0.245 27.325);
  --border: oklch(0.90 0 0);
  --input: oklch(0.90 0 0);
  --ring: oklch(0.55 0.18 220);
  --chart-1: oklch(0.87 0 0);
  --chart-2: oklch(0.556 0 0);
  --chart-3: oklch(0.439 0 0);
  --chart-4: oklch(0.371 0 0);
  --chart-5: oklch(0.269 0 0);
  --radius: 0.625rem;
  --sidebar: oklch(0.985 0 0);
  --sidebar-foreground: oklch(0.145 0 0);
  --sidebar-primary: oklch(0.55 0.18 220);
  --sidebar-primary-foreground: oklch(0.985 0 0);
  --sidebar-accent: oklch(0.97 0 0);
  --sidebar-accent-foreground: oklch(0.205 0 0);
  --sidebar-border: oklch(0.90 0 0);
  --sidebar-ring: oklch(0.55 0.18 220);
  /* semantic */
  --color-success: #16a34a;
  --color-warning: #ea580c;
  --color-danger: #dc2626;
  --color-info: #0369a1;
}
```

Replace the entire `.dark { ... }` block with:

```css
.dark {
  --background: oklch(0.10 0.01 240);
  --foreground: oklch(0.985 0 0);
  --card: oklch(0.13 0.01 240);
  --card-foreground: oklch(0.985 0 0);
  --popover: oklch(0.13 0.01 240);
  --popover-foreground: oklch(0.985 0 0);
  --primary: oklch(0.75 0.15 220);
  --primary-foreground: oklch(0.10 0.01 240);
  --secondary: oklch(0.269 0 0);
  --secondary-foreground: oklch(0.985 0 0);
  --muted: oklch(0.18 0.01 240);
  --muted-foreground: oklch(0.45 0 0);
  --accent: oklch(0.18 0.01 240);
  --accent-foreground: oklch(0.985 0 0);
  --destructive: oklch(0.704 0.191 22.216);
  --border: oklch(1 0 0 / 8%);
  --input: oklch(1 0 0 / 12%);
  --ring: oklch(0.75 0.15 220);
  --chart-1: oklch(0.87 0 0);
  --chart-2: oklch(0.556 0 0);
  --chart-3: oklch(0.439 0 0);
  --chart-4: oklch(0.371 0 0);
  --chart-5: oklch(0.269 0 0);
  --sidebar: oklch(0.13 0.01 240);
  --sidebar-foreground: oklch(0.985 0 0);
  --sidebar-primary: oklch(0.75 0.15 220);
  --sidebar-primary-foreground: oklch(0.10 0.01 240);
  --sidebar-accent: oklch(0.18 0.01 240);
  --sidebar-accent-foreground: oklch(0.985 0 0);
  --sidebar-border: oklch(1 0 0 / 8%);
  --sidebar-ring: oklch(0.75 0.15 220);
  /* semantic */
  --color-success: #22c55e;
  --color-warning: #fb923c;
  --color-danger: #f87171;
  --color-info: #38bdf8;
}
```

- [ ] **Step 2: Commit**

```bash
git add dashboard/src/app/globals.css
git commit -m "style: update shadcn theme tokens for ops-platform palette"
```

---

## Task 2: Update Global Layout (Nav)

**Files:**
- Modify: `dashboard/src/app/layout.tsx`

- [ ] **Step 1: Update the Nav component**

Replace the entire `Nav` function (lines 14–40) with:

```tsx
function Nav() {
  const { t } = useI18n();
  const pathname = usePathname();

  const links = [
    { href: "/", label: t("nav.runs") },
    { href: "/diagnose", label: t("nav.diagnose") },
    { href: "/skills", label: t("nav.skills") },
    { href: "/fixes", label: t("nav.fixes") },
    { href: "/events", label: t("nav.events") },
    { href: "/modelconfigs", label: t("nav.modelconfigs") },
    { href: "/clusters", label: t("nav.clusters") },
    { href: "/about", label: t("nav.about") },
  ];

  return (
    <nav className="border-b border-border bg-background px-6" style={{ height: "52px", display: "flex", alignItems: "center" }}>
      <div className="mx-auto flex max-w-7xl w-full items-center gap-8">
        <Link href="/" className="flex items-center gap-2 text-[15px] font-bold text-foreground">
          <span className="inline-block size-2 rounded-full bg-primary animate-pulse" />
          {t("nav.brand")}
        </Link>
        <div className="flex flex-1 gap-1 text-sm">
          {links.map((link) => {
            const isActive = link.href === "/" ? pathname === "/" : pathname.startsWith(link.href);
            return (
              <Link
                key={link.href}
                href={link.href}
                className={`rounded-md px-2.5 py-1.5 transition-colors ${
                  isActive
                    ? "bg-primary/10 text-primary font-medium"
                    : "text-muted-foreground hover:text-foreground hover:bg-muted"
                }`}
              >
                {link.label}
              </Link>
            );
          })}
        </div>
        <div className="flex items-center gap-1">
          <ClusterToggle />
          <ThemeToggle />
          <LanguageToggle />
        </div>
      </div>
    </nav>
  );
}
```

- [ ] **Step 2: Add `usePathname` import**

Add to the imports at the top of the file:
```tsx
import { usePathname } from "next/navigation";
```

- [ ] **Step 3: Update body/main classes**

In `RootLayout`, change:
```tsx
<body className="min-h-screen bg-gray-50 dark:bg-gray-950">
```
to:
```tsx
<body className="min-h-screen bg-background">
```

And change:
```tsx
<main className="mx-auto max-w-7xl px-6 py-8 text-gray-900 dark:text-gray-100">{children}</main>
```
to:
```tsx
<main className="mx-auto max-w-7xl px-6 py-6 text-foreground">{children}</main>
```

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/app/layout.tsx
git commit -m "style: redesign nav with active links, brand dot, and semantic bg tokens"
```

---

## Task 3: Update PhaseBadge and SeverityBadge

**Files:**
- Modify: `dashboard/src/components/phase-badge.tsx`
- Modify: `dashboard/src/components/severity-badge.tsx`

- [ ] **Step 1: Rewrite PhaseBadge**

Replace the entire file content with:

```tsx
"use client";

import { useI18n } from "@/i18n/context";

const config: Record<string, { bg: string; text: string; dot: string; pulse?: boolean }> = {
  Pending:   { bg: "bg-slate-500/10",  text: "text-slate-400",  dot: "bg-slate-400" },
  Running:   { bg: "bg-sky-500/10",    text: "text-sky-400",    dot: "bg-sky-400",   pulse: true },
  Succeeded: { bg: "bg-green-500/10",  text: "text-green-400",  dot: "bg-green-400" },
  Failed:    { bg: "bg-red-500/10",    text: "text-red-400",    dot: "bg-red-400" },
  Scheduled: { bg: "bg-purple-500/10", text: "text-purple-400", dot: "bg-purple-400" },
};

interface Props {
  phase: string;
}

export function PhaseBadge({ phase }: Props) {
  const { t } = useI18n();
  const c = config[phase] ?? { bg: "bg-slate-500/10", text: "text-slate-400", dot: "bg-slate-400" };
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-md border border-current/20 px-2 py-0.5 text-xs font-semibold ${c.bg} ${c.text}`}>
      <span className={`size-1.5 rounded-full ${c.dot} ${c.pulse ? "animate-pulse" : ""}`} />
      {t(`phase.${phase}`)}
    </span>
  );
}
```

- [ ] **Step 2: Rewrite SeverityBadge**

Replace the entire file content with:

```tsx
"use client";

import { useI18n } from "@/i18n/context";

const config: Record<string, { bg: string; text: string; dot: string }> = {
  critical: { bg: "bg-red-500/10",    text: "text-red-400",    dot: "bg-red-400" },
  high:     { bg: "bg-orange-500/10", text: "text-orange-400", dot: "bg-orange-400" },
  medium:   { bg: "bg-yellow-500/10", text: "text-yellow-400", dot: "bg-yellow-400" },
  low:      { bg: "bg-sky-500/10",    text: "text-sky-400",    dot: "bg-sky-400" },
};

interface Props {
  severity: string;
}

export function SeverityBadge({ severity }: Props) {
  const { t } = useI18n();
  const c = config[severity] ?? { bg: "bg-slate-500/10", text: "text-slate-400", dot: "bg-slate-400" };
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-md border border-current/20 px-2 py-0.5 text-xs font-semibold ${c.bg} ${c.text}`}>
      <span className={`size-1.5 rounded-full ${c.dot}`} />
      {t(`severity.${severity}`)}
    </span>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/components/phase-badge.tsx dashboard/src/components/severity-badge.tsx
git commit -m "style: upgrade PhaseBadge and SeverityBadge to dot-badge style"
```

---

## Task 4: Update shadcn Table and Card Components

**Files:**
- Modify: `dashboard/src/components/ui/table.tsx`
- Modify: `dashboard/src/components/ui/card.tsx`

- [ ] **Step 1: Read the current table.tsx**

Read `dashboard/src/components/ui/table.tsx` to see the current structure before editing.

- [ ] **Step 2: Update TableHeader, TableHead, TableRow in table.tsx**

In `table.tsx`, find the `TableHeader` component and update its `className` to include `bg-muted/50`:
```tsx
// TableHeader — add bg-muted/50
function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
  return (
    <thead
      data-slot="table-header"
      className={cn("bg-muted/50", className)}
      {...props}
    />
  );
}
```

Find the `TableHead` component and update its className to use uppercase tracking style:
```tsx
function TableHead({ className, ...props }: React.ComponentProps<"th">) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        "text-muted-foreground h-10 px-4 text-left align-middle text-xs font-semibold uppercase tracking-wide [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
        className
      )}
      {...props}
    />
  );
}
```

Find the `TableRow` component and update its hover class to `hover:bg-muted/30`:
```tsx
function TableRow({ className, ...props }: React.ComponentProps<"tr">) {
  return (
    <tr
      data-slot="table-row"
      className={cn(
        "border-b transition-colors hover:bg-muted/30 data-[state=selected]:bg-muted",
        className
      )}
      {...props}
    />
  );
}
```

- [ ] **Step 3: Read card.tsx and add border-b to CardHeader**

Read `dashboard/src/components/ui/card.tsx` first. Then find `CardHeader` and add `border-b border-border pb-4` to its className:

```tsx
function CardHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-header"
      className={cn(
        "flex flex-col gap-1.5 px-6 pt-6 pb-4 border-b border-border",
        className
      )}
      {...props}
    />
  );
}
```

Also update `CardContent` padding to `pt-4` to account for the new header border:
```tsx
function CardContent({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card-content"
      className={cn("px-6 pt-4 pb-6", className)}
      {...props}
    />
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/components/ui/table.tsx dashboard/src/components/ui/card.tsx
git commit -m "style: update Table header/row styles and add CardHeader divider"
```

---

## Task 5: Update Home Page (Runs Overview)

**Files:**
- Modify: `dashboard/src/app/page.tsx`

- [ ] **Step 1: Update the hero section and stat cards, and table wrapper**

Replace the entire `RunsPage` return block with:

```tsx
  return (
    <div>
      {/* Overview hero */}
      <div className="mb-8 relative overflow-hidden rounded-xl border border-border bg-gradient-to-br from-sky-50 to-indigo-50 p-6 dark:from-[#0d1b2e] dark:to-[#130d2e]">
        <div className="absolute inset-x-0 top-0 h-[2px] bg-gradient-to-r from-sky-400 via-indigo-400 to-sky-400" />
        <h1 className="text-2xl font-bold">{t("overview.title")}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t("overview.subtitle")}</p>
        <div className="mt-4 grid grid-cols-3 gap-4">
          {featureCards.map((card) => (
            <Link key={card.title} href={card.href} className="group rounded-lg border border-border bg-background/60 p-4 transition-all hover:border-primary/50 hover:bg-primary/5">
              <card.icon className={`size-5 ${card.color}`} />
              <h3 className="mt-2 text-sm font-semibold">{card.title}</h3>
              <p className="mt-1 text-xs text-muted-foreground line-clamp-2">{card.desc}</p>
            </Link>
          ))}
        </div>
      </div>

      {/* Runs section */}
      <div id="runs" className="mb-6 flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-xl font-semibold">
          {t("runs.title")}
          <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">{total}</span>
        </h2>
        <CreateRunDialog onCreated={() => mutate()} />
      </div>
      <div className="mb-6 grid grid-cols-4 gap-4">
        {[
          { label: t("runs.stat.total"), value: total, color: "text-foreground", trend: null },
          { label: t("runs.stat.running"), value: running, color: "text-sky-400", trend: null },
          { label: t("runs.stat.succeeded"), value: succeeded, color: "text-green-400", trend: total > 0 ? `${Math.round(succeeded / total * 100)}%` : null },
          { label: t("runs.stat.failed"), value: failed, color: "text-red-400", trend: null },
        ].map(({ label, value, color, trend }) => (
          <div key={label} className="rounded-lg border border-border bg-card p-4">
            <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{label}</p>
            <p className={`mt-1 text-3xl font-bold ${color}`}>{value}</p>
            {trend && <p className="mt-1 text-xs text-muted-foreground">{trend}</p>}
          </div>
        ))}
      </div>
      {runs && runs.length === 0 ? (
        <p className="text-muted-foreground">{t("runs.empty")}</p>
      ) : (
        <div className="rounded-lg border border-border bg-card overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("runs.col.id")}</TableHead>
                <TableHead>{t("runs.col.phase")}</TableHead>
                <TableHead>{t("runs.col.created")}</TableHead>
                <TableHead>{t("runs.col.duration")}</TableHead>
                <TableHead>{t("runs.col.target")}</TableHead>
                <TableHead>{t("runs.col.message")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {runs?.map((run) => {
                let target = "-";
                try {
                  const tgt = JSON.parse(run.TargetJSON);
                  target = tgt.namespaces?.join(", ") || tgt.scope || "-";
                } catch {
                  /* ignore */
                }
                return (
                  <TableRow key={run.ID}>
                    <TableCell>
                      <Link href={`/runs/${run.ID}`} className="font-mono text-sm text-primary hover:underline">
                        {run.Name || run.ID.slice(0, 8)}
                      </Link>
                    </TableCell>
                    <TableCell><PhaseBadge phase={run.Status} /></TableCell>
                    <TableCell className="text-sm text-muted-foreground">{formatTime(run.CreatedAt)}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">{duration(run.StartedAt, run.CompletedAt)}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">{target}</TableCell>
                    <TableCell className="max-w-xs truncate text-sm text-muted-foreground" title={run.Message || ""}>
                      {run.Message || "-"}
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
```

- [ ] **Step 2: Commit**

```bash
git add dashboard/src/app/page.tsx
git commit -m "style: polish home page with hero accent line, stat cards, and updated table"
```

---

## Task 6: Update Diagnose Page

**Files:**
- Modify: `dashboard/src/app/diagnose/page.tsx`

- [ ] **Step 1: Update the form card, labels, resource type selector, and submit button**

Replace the main `return` block (the form, not the `createdYAML` success view) — specifically the section from `<div className="rounded-lg border bg-white p-6 shadow-sm ...">` through the submit button — with:

```tsx
      <div className="rounded-lg border border-border bg-card p-6 space-y-6">
        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">{t("diagnose.namespace")}</label>
          <select
            value={namespace}
            onChange={(e) => { setNamespace(e.target.value); setResourceName(""); }}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
          >
            <option value="">{t("diagnose.namespacePlaceholder")}</option>
            {(namespaces || []).map((ns) => (
              <option key={ns.name} value={ns.name}>{ns.name}</option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">{t("diagnose.resourceType")}</label>
          <div className="flex gap-2">
            {RESOURCE_TYPES.map((rt) => (
              <button
                key={rt}
                type="button"
                onClick={() => { setResourceType(rt); setResourceName(""); }}
                className={`rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors ${
                  resourceType === rt
                    ? "border-primary bg-primary/10 text-primary"
                    : "border-border text-muted-foreground hover:border-primary/50 hover:text-foreground"
                }`}
              >
                {rt}
              </button>
            ))}
          </div>
        </div>

        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">
            {t("diagnose.resourceName")}
            <span className="ml-1 normal-case font-normal text-muted-foreground/60">({lang === "zh" ? "可选，留空=全部" : "optional, empty=all"})</span>
          </label>
          <select
            value={resourceName}
            onChange={(e) => setResourceName(e.target.value)}
            disabled={!namespace}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20 disabled:opacity-50"
          >
            <option value="">{t("diagnose.resourceNamePlaceholder")}</option>
            {(resources || []).map((r) => (
              <option key={r.name} value={r.name}>{r.name}</option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-1">{t("diagnose.symptoms")}</label>
          <p className="text-xs text-muted-foreground/70 mb-3">{t("diagnose.symptomsHint")}</p>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {SYMPTOM_PRESETS.map((s) => (
              <label
                key={s.id}
                className={`flex items-center gap-2 rounded-lg border px-3 py-2.5 text-sm cursor-pointer transition-colors ${
                  symptoms.includes(s.id)
                    ? "border-primary bg-primary/10 text-primary"
                    : "border-border text-muted-foreground hover:border-primary/40 hover:text-foreground"
                }`}
              >
                <input
                  type="checkbox"
                  checked={symptoms.includes(s.id)}
                  onChange={() => toggleSymptom(s.id)}
                  className="sr-only"
                />
                <span className={`size-1.5 rounded-full shrink-0 ${symptoms.includes(s.id) ? "bg-primary" : "bg-muted-foreground/40"}`} />
                {lang === "zh" ? s.label_zh : s.label_en}
              </label>
            ))}
          </div>
        </div>

        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">{t("diagnose.outputLanguage")}</label>
          <div className="flex gap-2">
            {(["zh", "en"] as const).map((l) => (
              <button
                key={l}
                type="button"
                onClick={() => setOutputLang(l)}
                className={`rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors ${
                  outputLang === l
                    ? "border-primary bg-primary/10 text-primary"
                    : "border-border text-muted-foreground hover:border-primary/50 hover:text-foreground"
                }`}
              >
                {l === "zh" ? "中文" : "English"}
              </button>
            ))}
          </div>
        </div>

        <div>
          <label className="block text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">{t("diagnose.schedule")}</label>
          <div className="flex flex-wrap gap-2 mb-2">
            {[
              { label: t("diagnose.schedulePreset.none"), value: "" },
              { label: t("diagnose.schedulePreset.hourly"), value: "0 * * * *" },
              { label: t("diagnose.schedulePreset.daily"), value: "0 8 * * *" },
              { label: t("diagnose.schedulePreset.weekly"), value: "0 8 * * 1" },
            ].map((p) => (
              <button
                key={p.value}
                type="button"
                onClick={() => { setSchedule(p.value); setCustomSchedule(false); }}
                className={`rounded-lg border px-3 py-1.5 text-sm transition-colors ${
                  !customSchedule && schedule === p.value
                    ? "border-primary bg-primary/10 text-primary"
                    : "border-border text-muted-foreground hover:border-primary/50"
                }`}
              >
                {p.label}{p.value && <span className="ml-1.5 font-mono text-xs opacity-60">{p.value}</span>}
              </button>
            ))}
            <button
              type="button"
              onClick={() => { setCustomSchedule(true); setSchedule(""); }}
              className={`rounded-lg border px-3 py-1.5 text-sm transition-colors ${
                customSchedule
                  ? "border-primary bg-primary/10 text-primary"
                  : "border-border text-muted-foreground hover:border-primary/50"
              }`}
            >
              {t("diagnose.schedulePreset.custom")}
            </button>
          </div>
          {customSchedule && (
            <input
              type="text"
              value={schedule}
              onChange={(e) => setSchedule(e.target.value)}
              placeholder="*/30 * * * *"
              autoFocus
              className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm font-mono mb-2 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
            />
          )}
          <div className="rounded-lg border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
            <p className="font-medium mb-1">{t("diagnose.cronHelp.title")}</p>
            <code className="block font-mono tracking-wider mb-1">┌─ {t("diagnose.cronHelp.minute")}  (0–59)<br />│ ┌─ {t("diagnose.cronHelp.hour")}    (0–23)<br />│ │ ┌─ {t("diagnose.cronHelp.day")}    (1–31)<br />│ │ │ ┌─ {t("diagnose.cronHelp.month")}  (1–12)<br />│ │ │ │ ┌─ {t("diagnose.cronHelp.weekday")} (0–7)<br />* * * * *</code>
            <p className="mt-1">{t("diagnose.cronHelp.hint")}</p>
          </div>
        </div>

        {error && <p className="text-sm text-red-500">{t("diagnose.error")}: {error}</p>}
        <button
          onClick={handleSubmit}
          disabled={submitting || !namespace || symptoms.length === 0}
          className="rounded-lg bg-primary px-6 py-2 text-sm font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-50 transition-opacity"
        >
          {submitting ? t("diagnose.submitting") : t("diagnose.submit")}
        </button>
      </div>
```

Also update the success view buttons to match. Find `className="rounded bg-blue-600 px-4 py-2 ...` in the `createdYAML` block and change to:
```tsx
className="rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground hover:opacity-90"
```
And the secondary button:
```tsx
className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-muted transition-colors"
```

- [ ] **Step 2: Commit**

```bash
git add dashboard/src/app/diagnose/page.tsx
git commit -m "style: polish diagnose page form with pill selectors and updated chips"
```

---

## Task 7: Update Skills Page

**Files:**
- Modify: `dashboard/src/app/skills/page.tsx`

- [ ] **Step 1: Update stat cards and table wrapper classes**

In the stat cards map, change:
```tsx
// OLD
className="rounded-lg border bg-white p-4 dark:border-gray-800 dark:bg-gray-900"
// NEW
className="rounded-lg border border-border bg-card p-4"
```
And stat value `text-2xl` → `text-3xl`.

Change the table wrapper:
```tsx
// OLD
<div className="rounded-lg border bg-white dark:border-gray-800 dark:bg-gray-900">
// NEW
<div className="rounded-lg border border-border bg-card overflow-hidden">
```

Update the expanded row background:
```tsx
// OLD
<TableCell colSpan={6} className="bg-gray-50 dark:bg-gray-800/30 p-0">
// NEW
<TableCell colSpan={6} className="bg-muted/30 p-0">
```

Update the prompt pre block:
```tsx
// OLD
<pre className="whitespace-pre-wrap text-sm text-gray-700 dark:text-gray-300 rounded-lg bg-gray-900 dark:bg-gray-950 text-gray-100 p-4 max-h-64 overflow-y-auto">
// NEW
<pre className="whitespace-pre-wrap text-sm rounded-lg bg-[#0a0e14] text-slate-200 p-4 max-h-64 overflow-y-auto">
```

Update the row hover class:
```tsx
// OLD
className="cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/50"
// NEW
className="cursor-pointer"
```
(TableRow already has hover:bg-muted/30 from Task 4)

Update the loading/error states:
```tsx
// OLD: className="text-gray-500 dark:text-gray-400"
// NEW: className="text-muted-foreground"
// OLD: className="text-red-600 dark:text-red-400"
// NEW: className="text-destructive"
```

- [ ] **Step 2: Commit**

```bash
git add dashboard/src/app/skills/page.tsx
git commit -m "style: polish skills page stat cards and table"
```

---

## Task 8: Update Fixes Page

**Files:**
- Modify: `dashboard/src/app/fixes/page.tsx`

- [ ] **Step 1: Replace inline `phaseColors` with a dot-badge helper and update classes**

Replace the `phaseColors` record and the `Badge className={phaseColors[fix.Phase]}` usage with a local dot-badge component at the top of the file:

```tsx
const fixPhaseConfig: Record<string, { bg: string; text: string; dot: string }> = {
  PendingApproval: { bg: "bg-yellow-500/10", text: "text-yellow-400", dot: "bg-yellow-400" },
  Approved:        { bg: "bg-sky-500/10",    text: "text-sky-400",    dot: "bg-sky-400" },
  Applying:        { bg: "bg-sky-500/10",    text: "text-sky-400",    dot: "bg-sky-400" },
  Succeeded:       { bg: "bg-green-500/10",  text: "text-green-400",  dot: "bg-green-400" },
  Failed:          { bg: "bg-red-500/10",    text: "text-red-400",    dot: "bg-red-400" },
  RolledBack:      { bg: "bg-orange-500/10", text: "text-orange-400", dot: "bg-orange-400" },
  DryRunComplete:  { bg: "bg-purple-500/10", text: "text-purple-400", dot: "bg-purple-400" },
};

function FixPhaseBadge({ phase }: { phase: string }) {
  const c = fixPhaseConfig[phase] ?? { bg: "bg-slate-500/10", text: "text-slate-400", dot: "bg-slate-400" };
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-md border border-current/20 px-2 py-0.5 text-xs font-semibold ${c.bg} ${c.text}`}>
      <span className={`size-1.5 rounded-full ${c.dot}`} />
      {phase}
    </span>
  );
}
```

Then replace `<Badge className={phaseColors[fix.Phase] || ""}>{t(`phase.${fix.Phase}`)}</Badge>` with `<FixPhaseBadge phase={fix.Phase} />`.

Update stat cards and table wrapper same as Task 7:
- Stat cards: `border-border bg-card`, `text-3xl`
- Table wrapper: `border-border bg-card overflow-hidden`
- Fix name link: `className="font-mono text-sm text-primary hover:underline"`
- Loading/error: `text-muted-foreground` / `text-destructive`

- [ ] **Step 2: Commit**

```bash
git add dashboard/src/app/fixes/page.tsx
git commit -m "style: polish fixes page with dot-badge phase indicator"
```

---

## Task 9: Update Events Page

**Files:**
- Modify: `dashboard/src/app/events/page.tsx`

- [ ] **Step 1: Update filter inputs, warning badge, and table wrapper**

Update filter input and select `className` from:
```tsx
"rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:outline-none dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:placeholder-gray-500"
```
to:
```tsx
"rounded-lg border border-border bg-background px-3 py-1.5 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
```
Apply to both text inputs and the select.

Update the Warning reason badge from:
```tsx
<span className="inline-flex items-center rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700 dark:bg-red-900/30 dark:text-red-400">
```
to:
```tsx
<span className="inline-flex items-center gap-1 rounded-md border border-red-400/20 bg-red-500/10 px-2 py-0.5 text-xs font-semibold text-red-400">
  <span className="size-1.5 rounded-full bg-red-400" />
```
(add a closing `</span>` and close the inner span before the text)

Update table wrapper: `border-border bg-card overflow-hidden` (remove explicit `dark:*` overrides).

Update loading/error/empty states to use `text-muted-foreground` / `text-destructive`.

- [ ] **Step 2: Commit**

```bash
git add dashboard/src/app/events/page.tsx
git commit -m "style: polish events page filter inputs and warning badge"
```

---

## Task 10: Update Clusters and ModelConfigs Pages

**Files:**
- Modify: `dashboard/src/app/clusters/page.tsx`
- Modify: `dashboard/src/app/modelconfigs/page.tsx`

- [ ] **Step 1: Update clusters/page.tsx**

Replace `phaseColors` with a dot-badge helper (same pattern as Task 8):
```tsx
const clusterPhaseConfig: Record<string, { bg: string; text: string; dot: string }> = {
  Connected: { bg: "bg-green-500/10", text: "text-green-400", dot: "bg-green-400" },
  Error:     { bg: "bg-red-500/10",   text: "text-red-400",   dot: "bg-red-400" },
  Pending:   { bg: "bg-yellow-500/10",text: "text-yellow-400",dot: "bg-yellow-400" },
};

function ClusterPhaseBadge({ phase }: { phase: string }) {
  const c = clusterPhaseConfig[phase] ?? { bg: "bg-slate-500/10", text: "text-slate-400", dot: "bg-slate-400" };
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-md border border-current/20 px-2 py-0.5 text-xs font-semibold ${c.bg} ${c.text}`}>
      <span className={`size-1.5 rounded-full ${c.dot}`} />
      {phase}
    </span>
  );
}
```

Replace `<Badge className={phaseColors[c.phase] || ...}>{c.phase}</Badge>` with `<ClusterPhaseBadge phase={c.phase} />`.

Update the raw `<table>` to use shadcn Table components:
```tsx
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
```
Then wrap the table:
```tsx
<div className="mb-8 rounded-lg border border-border bg-card overflow-hidden">
  <Table>
    <TableHeader>
      <TableRow>
        <TableHead>{t("clusters.col.name")}</TableHead>
        <TableHead>{t("clusters.col.phase")}</TableHead>
        <TableHead>{t("clusters.col.prometheus")}</TableHead>
        <TableHead>{t("clusters.col.description")}</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      {clusters.map((c: ClusterItem) => (
        <TableRow key={c.name}>
          <TableCell className="font-medium">{c.name}</TableCell>
          <TableCell><ClusterPhaseBadge phase={c.phase} /></TableCell>
          <TableCell className="font-mono text-xs text-muted-foreground">
            {c.prometheusURL || <span className="italic">-</span>}
          </TableCell>
          <TableCell className="text-muted-foreground">{c.description || "-"}</TableCell>
        </TableRow>
      ))}
    </TableBody>
  </Table>
</div>
```

Update `CreateDialog` input class from:
```tsx
"w-full rounded border border-gray-300 bg-white px-3 py-1.5 text-sm dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100"
```
to:
```tsx
"w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
```

Update form modal background: `bg-white p-6 shadow-xl dark:bg-gray-900` → `bg-card p-6 shadow-xl border border-border`.

Update Cancel/Submit buttons:
```tsx
// Cancel
className="rounded-lg px-4 py-1.5 text-sm text-muted-foreground hover:bg-muted transition-colors"
// Submit
className="rounded-lg bg-primary px-4 py-1.5 text-sm font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-50"
```

Update the "+" new cluster button:
```tsx
className="rounded-lg bg-primary px-4 py-1.5 text-sm font-semibold text-primary-foreground hover:opacity-90"
```

- [ ] **Step 2: Update modelconfigs/page.tsx**

Apply the same pattern: replace raw `<table>` with shadcn `<Table>` components.

Update `provider` cell badge from `rounded bg-blue-100 px-2 py-0.5 text-xs text-blue-800 dark:bg-blue-900 dark:text-blue-200` to:
```tsx
<span className="inline-flex items-center rounded-md border border-sky-400/20 bg-sky-500/10 px-2 py-0.5 text-xs font-semibold text-sky-400">
  {mc.provider}
</span>
```

Update `apiKey` cell: `rounded bg-gray-100 px-2 py-0.5 text-xs dark:bg-gray-800` → `rounded-md bg-muted px-2 py-0.5 text-xs font-mono`.

Update `CreateDialog` with same input/button/modal classes as the clusters dialog above.

Update the "+" new config button to use `bg-primary text-primary-foreground`.

Update all loading/error states to `text-muted-foreground` / `text-destructive`.

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/app/clusters/page.tsx dashboard/src/app/modelconfigs/page.tsx
git commit -m "style: polish clusters and modelconfigs pages with shadcn Table and updated badges"
```

---

## Task 11: Update About Page

**Files:**
- Modify: `dashboard/src/app/about/page.tsx`

- [ ] **Step 1: Update architecture block and flow steps**

In the architecture `<pre>` block, change the outer `<pre>` className from:
```tsx
"overflow-x-auto rounded-lg bg-gray-900 p-4 text-xs text-gray-100 dark:bg-gray-950 leading-relaxed"
```
to:
```tsx
"overflow-x-auto rounded-lg bg-[#0a0e14] p-4 text-xs text-slate-300 leading-relaxed border border-border"
```

Update flow step number circles: find `bg-blue-600` and change to `bg-primary text-primary-foreground`.

Update CRD badge items — find the `.map((crd) => ...)` block and update the card wrapper:
```tsx
// OLD
<div key={crd.name} className="rounded-lg border p-4 dark:border-gray-800">
// NEW
<div key={crd.name} className="rounded-lg border border-border bg-background p-4">
```

Update page title area — add a subtitle under `<h1>`:
```tsx
<div>
  <h1 className="text-2xl font-bold">{t("about.title")}</h1>
  <p className="mt-1 text-sm text-muted-foreground">{t("about.subtitle") || "Kube Agent Helper — AI-powered Kubernetes diagnostics"}</p>
</div>
```
(Use a fallback string if `about.subtitle` is not in the i18n file — it will gracefully show.)

- [ ] **Step 2: Commit**

```bash
git add dashboard/src/app/about/page.tsx
git commit -m "style: polish about page architecture block, step circles, and CRD cards"
```

---

## Task 12: Update Detail Pages

**Files:**
- Modify: `dashboard/src/app/runs/[id]/page.tsx`
- Modify: `dashboard/src/app/fixes/[id]/page.tsx`

- [ ] **Step 1: Update runs/[id]/page.tsx**

Update the back link: `text-blue-600 hover:underline dark:text-blue-400` → `text-primary hover:underline text-sm`.

Update the status message banner. Replace the ternary className block for `run.Message`:
```tsx
// OLD
className={`mt-3 rounded-lg border px-3 py-2 text-sm ${
  run.Status === "Failed"
    ? "border-red-200 bg-red-50 text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300"
    : run.Status === "Running"
      ? "border-yellow-200 bg-yellow-50 text-yellow-800 dark:border-yellow-900 dark:bg-yellow-950 dark:text-yellow-300"
      : "border-gray-200 bg-gray-50 text-gray-700 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-300"
}`}
// NEW
className={`mt-3 rounded-lg border px-3 py-2 text-sm ${
  run.Status === "Failed"
    ? "border-red-500/20 bg-red-500/10 text-red-400"
    : run.Status === "Running"
      ? "border-yellow-500/20 bg-yellow-500/10 text-yellow-400"
      : "border-border bg-muted/30 text-muted-foreground"
}`}
```

Update scheduled run info box: `border-blue-200 bg-blue-50 dark:border-blue-800 dark:bg-blue-950` → `border-primary/20 bg-primary/10`.

Update the suggestion box in finding cards: `rounded bg-blue-50 p-2 text-sm text-blue-800 dark:bg-blue-950 dark:text-blue-200` → `rounded-lg bg-primary/10 border border-primary/20 p-2 text-sm text-primary`.

Update metadata grid text: `text-gray-600 sm:grid-cols-4 dark:text-gray-400` → `text-muted-foreground sm:grid-cols-4`.

Update "View Fix" link: `text-blue-600 hover:underline dark:text-blue-400` → `text-primary hover:underline text-sm`.

- [ ] **Step 2: Read and update fixes/[id]/page.tsx**

Read `dashboard/src/app/fixes/[id]/page.tsx` to see its current structure, then apply the same token replacements:
- `text-blue-600` / `dark:text-blue-400` → `text-primary`
- `bg-white dark:bg-gray-900` / `dark:border-gray-800` → `bg-card border-border`
- `text-gray-600 dark:text-gray-400` → `text-muted-foreground`
- `bg-gray-50 dark:bg-gray-900` → `bg-muted/30`

- [ ] **Step 3: Update diagnose/[id]/page.tsx**

Replace `SEVERITY_STYLES` at the top of the file:

```tsx
const SEVERITY_STYLES: Record<string, { border: string; bg: string }> = {
  critical: { border: "border-red-500/30",    bg: "bg-red-500/10" },
  high:     { border: "border-orange-500/30", bg: "bg-orange-500/10" },
  medium:   { border: "border-yellow-500/30", bg: "bg-yellow-500/10" },
  low:      { border: "border-sky-500/30",    bg: "bg-sky-500/10" },
  info:     { border: "border-border",        bg: "bg-muted/30" },
};
```

Remove the `icon` property from `SEVERITY_STYLES` — update `groupBySeverity` section header to not use `style.icon`:
```tsx
<h2 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
  {t(`severity.${severity}` as Parameters<typeof t>[0]) || severity}
  <span className="rounded-full bg-muted px-2 py-0.5 font-mono">{items.length}</span>
</h2>
```

Update the header card wrapper: `rounded-lg border bg-white p-6 shadow-sm dark:bg-gray-900 dark:border-gray-800` → `rounded-lg border border-border bg-card p-6`.

Update metadata label text: `text-gray-500` → `text-muted-foreground`.

Update Running spinner color: `text-blue-600` → `text-primary`.

Update error box: `rounded bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-400` → `rounded-lg border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-400`.

Update suggestion box: `rounded bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-900/30 dark:text-blue-300` → `rounded-lg border border-primary/20 bg-primary/10 p-3 text-sm text-primary`.

Update back link: `text-sm text-blue-600 hover:underline` → `text-sm text-primary hover:underline`.

Update "View Fix" link: `text-sm text-blue-600 hover:underline` → `text-sm text-primary hover:underline`.

Update generate button: `rounded bg-blue-600 px-3 py-1 text-sm text-white hover:bg-blue-700 disabled:opacity-50` → `rounded-lg bg-primary px-3 py-1 text-sm font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-50`.

Update empty state: `text-sm text-gray-500` → `text-sm text-muted-foreground`.

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/app/runs/[id]/page.tsx dashboard/src/app/fixes/[id]/page.tsx dashboard/src/app/diagnose/[id]/page.tsx
git commit -m "style: polish run, fix, and diagnose detail pages with semantic color tokens"
```

---

## Task 13: Cleanup

**Files:**
- Delete: `dashboard/style-preview.html`

- [ ] **Step 1: Delete the temporary preview file**

```bash
rm dashboard/style-preview.html
```

- [ ] **Step 2: Verify the build passes**

```bash
cd dashboard && npm run build 2>&1 | tail -20
```

Expected: build completes with no TypeScript or import errors.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "chore: remove style preview file"
```
