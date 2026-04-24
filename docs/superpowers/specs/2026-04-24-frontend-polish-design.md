# Frontend Polish Design

**Date:** 2026-04-24
**Branch:** feat/multi-cluster-support
**Scope:** Full visual overhaul of dashboard UI

## Goal

Elevate the dashboard from a functional but utilitarian UI to a polished ops-platform style (inspired by Argo CD / Rancher), while preserving all existing functionality. Both light and dark modes must be well-designed.

## Approach

**Method: shadcn theme token overhaul + page-level rewrites**

Update CSS variables in `globals.css` for both `:root` (light) and `.dark` (dark) to define the new color palette. Since the project uses shadcn/ui throughout, all shadcn components automatically inherit the new theme. Then rewrite page layouts and Tailwind class names page by page.

## Design System

### Color Palette

#### Light mode (`:root`)

| Token | New value | Notes |
|-------|-----------|-------|
| `--background` | `oklch(0.98 0 0)` | Slightly off-white, less glaring than pure white |
| `--card` | `oklch(1 0 0)` | White cards |
| `--primary` | `oklch(0.55 0.18 220)` | Sky-600 blue, brand color |
| `--primary-foreground` | `oklch(0.98 0 0)` | White text on primary |
| `--border` | `oklch(0.90 0 0)` | Slightly darker border |
| `--muted-foreground` | `oklch(0.52 0 0)` | Secondary text |

#### Dark mode (`.dark`)

| Token | New value | Notes |
|-------|-----------|-------|
| `--background` | `oklch(0.10 0.01 240)` | `#0f1117`, blue-tinted near-black |
| `--card` | `oklch(0.13 0.01 240)` | `#141920`, slightly lighter card bg |
| `--primary` | `oklch(0.75 0.15 220)` | Sky-400 `#38bdf8`, bright brand accent |
| `--primary-foreground` | `oklch(0.10 0.01 240)` | Dark text on primary buttons |
| `--border` | `oklch(1 0 0 / 8%)` | Softer border than current 10% |
| `--muted-foreground` | `oklch(0.45 0 0)` | Dimmer secondary text |

#### Semantic additions (new CSS custom properties)

```css
--color-success:  #22c55e;  /* green-500 */
--color-warning:  #fb923c;  /* orange-400 */
--color-danger:   #f87171;  /* red-400 */
--color-info:     #38bdf8;  /* sky-400 */
```

Light mode counterparts:
```css
--color-success:  #16a34a;
--color-warning:  #ea580c;
--color-danger:   #dc2626;
--color-info:     #0369a1;
```

## Global Layout

### Nav (`layout.tsx`)

- Brand logo: add small pulsing blue dot (`animate-pulse`) left of text to convey "live"
- Active nav link: use `usePathname()` to detect current route; active link gets sky-blue text + subtle background (`bg-sky-500/10 text-sky-400`); inactive links remain muted
- Cluster toggle badge: styled as a pill with a green/red status dot indicating cluster health
- Nav height: ~52px (currently ~48px)
- `border-b` uses new `--border` token (softer in dark mode)

### Body

- `bg-gray-50` / `dark:bg-gray-950` → `bg-background` (single semantic token)
- `main` padding: `py-8` → `py-6`

## Shared Components

### PhaseBadge

Upgrade from plain colored text to dot-badge style:

| Phase | Background | Text | Dot |
|-------|-----------|------|-----|
| Running | `bg-sky-500/10` | `text-sky-400` | Pulsing blue dot |
| Succeeded | `bg-green-500/10` | `text-green-400` | Static green dot |
| Failed | `bg-red-500/10` | `text-red-400` | Static red dot |
| Pending | `bg-slate-500/10` | `text-slate-400` | Static gray dot |

Structure: `<span class="badge"><span class="dot" /> {phase}</span>`

### SeverityBadge

Same dot-badge pattern:
- Critical → red
- Warning → orange
- Info → sky blue

### Card (shadcn)

- Auto-inherits new `--card` token
- `CardHeader`: add `border-b border-border` divider line (currently absent)
- Card border uses new `--border` token

### Table (shadcn)

- `TableHeader`: `bg-muted/50` background to distinguish from body
- `TableHead`: `text-xs uppercase tracking-wide text-muted-foreground`
- `TableRow` hover: `hover:bg-muted/30`
- `TableCell`: `text-sm`

### Button (shadcn primary)

- Light: `bg-sky-600 hover:bg-sky-700 text-white`
- Dark: `bg-sky-700 hover:bg-sky-600 text-white`
- Defined via `--primary` token update

## Pages

### Home (`/` — Runs overview)

**Hero section:**
- Gradient: light → `from-sky-50 to-indigo-50`; dark → `from-[#0d1b2e] to-[#130d2e]`
- Add 2px top accent line: `linear-gradient(90deg, #38bdf8, #818cf8)`
- Feature cards: on hover, `border-sky-500` + `bg-sky-500/5`

**Stats cards:**
- Value font size: `text-2xl` → `text-3xl`
- Add trend line below value (e.g., success rate percentage)

**Runs table:**
- Apply new Table styles
- Run name: `font-mono text-sm`
- Run ID link: `text-primary hover:underline`

### Diagnose (`/diagnose`)

- Form card: remove `shadow-sm`, use new border token
- Label style: `text-xs font-semibold uppercase tracking-wide text-muted-foreground`
- Resource type selector: replace radio inputs with pill button group (tab-style)
- Symptom chips: selected state → sky border + bg + left colored dot
- Submit button: sky-blue primary style

### Skills / Fixes / Events / ModelConfigs

- Unified page header structure: `<h1>` title + subtitle text + right-aligned action button
- Apply new Table styles to all list tables

### Clusters (`/clusters`)

- Cluster cards: add health status dot (green=healthy, red=error, gray=unknown)
- Connection status uses new badge style

### About (`/about`)

- Architecture ASCII block: deeper background (`#0a0e14`), layered text colors (keywords sky-blue, comments slate)
- Flow step numbers: sky-blue circle (`bg-sky-600`)
- CRD badges: keep colorful, standardize padding/radius

### Detail pages (`/runs/[id]`, `/fixes/[id]`, `/diagnose/[id]`)

- Page header: status badge prominently displayed next to title
- Improve visual hierarchy between metadata, logs, and action areas

## Implementation Notes

- All pages already import from `@/components/ui/*` — no new shadcn components needed
- `globals.css` changes cascade automatically to all pages
- Nav active-link detection requires `usePathname()` from `next/navigation` (already client component)
- Semantic color tokens (`--color-success` etc.) added as CSS custom properties alongside shadcn tokens; referenced as `text-[var(--color-success)]` in Tailwind v4
- Preview file `dashboard/style-preview.html` can be deleted after implementation
