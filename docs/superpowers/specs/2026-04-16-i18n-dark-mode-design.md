# Dashboard i18n + Dark Mode Design

**Status:** Approved
**Date:** 2026-04-16
**Goal:** Add internationalization (Chinese default, English toggle) and dark-mode (dark default, light toggle) to the Next.js dashboard, plus per-Run control over the language the diagnostic agent outputs findings in.

---

## 1. Architecture

Two independent React Contexts, both mounted in `dashboard/src/app/layout.tsx`:

### 1.1 `I18nContext`

- Holds current language: `"zh" | "en"`
- Exposes `t(key: string): string` lookup against in-memory dictionary
- Reads/writes `localStorage.lang` (default `"zh"` on first load)
- Dictionaries live at `dashboard/src/i18n/zh.json` and `dashboard/src/i18n/en.json`

Dictionary shape (nested by page/component, dot-separated lookup):

```json
{
  "nav": { "runs": "诊断任务", "skills": "技能", "fixes": "修复建议" },
  "runs": {
    "title": "诊断任务",
    "createButton": "创建任务",
    "col.id": "ID",
    "col.phase": "阶段",
    "col.created": "创建时间",
    "col.duration": "耗时",
    "col.target": "目标",
    "col.message": "消息"
  },
  "phase": { "Pending": "待处理", "Running": "执行中", "Succeeded": "成功", "Failed": "失败" },
  "severity": { "critical": "严重", "high": "高", "medium": "中", "low": "低" },
  "dimension": { "health": "健康", "security": "安全", "cost": "成本", "reliability": "可靠性" },
  "fixes": { "title": "修复建议", "phase.PendingApproval": "待审批", ... },
  "common": { "cancel": "取消", "create": "创建", "loading": "加载中...", ... }
}
```

### 1.2 `ThemeContext`

- Holds current theme: `"dark" | "light"`
- Toggles `class="dark"` on `<html>`
- Reads/writes `localStorage.theme` (default `"dark"` on first load)
- Tailwind configured with `darkMode: "class"` so `dark:` variants activate when `<html class="dark">`

### 1.3 Client-only concern

Both contexts are client-state. The app already uses `"use client"` on every page. The providers are declared in a `ClientProviders.tsx` component and the `RootLayout` wraps `{children}` with it. Initial render reads `localStorage` in a `useEffect` — to avoid a flash of wrong theme on first paint, we inject a small inline script in `<head>` that synchronously reads `localStorage.theme` and adds the `dark` class before hydration.

---

## 2. Toggle UI

Top nav bar gets two right-aligned icon buttons (after existing nav links):

```
Kube Agent Helper   Runs  Skills  Fixes                   🌙  中
                                                          ↑   ↑
                                                          │   └── Language button
                                                          └────── Theme button
```

### 2.1 Theme button

- Shows icon of the *opposite* state (`☀️` when currently `dark`, `🌙` when currently `light`)
- `lucide-react` icons: `Moon` / `Sun`
- Click toggles theme + writes localStorage

### 2.2 Language button

- Shows *opposite* label text: `EN` when current is `zh`, `中` when current is `en`
- Click toggles language + writes localStorage

### 2.3 Styling

- Both buttons: `h-8 w-8 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800`
- Small and unobtrusive, matching the minimal nav aesthetic

---

## 3. Per-Run Output Language

Users control (per Run) what language the LLM writes findings in.

### 3.1 CRD spec field

Add to `DiagnosticRunSpec`:

```go
// +optional
// +kubebuilder:validation:Enum=zh;en
OutputLanguage string `json:"outputLanguage,omitempty"`
```

When omitted, defaults to `"en"` (backward compatible with existing CRs).

### 3.2 Flow

1. **CreateRunDialog** reads current UI language from `useI18n()`, uses it as the default for a new "Output Language" select in the form (user can override)
2. **POST /api/runs** accepts `outputLanguage: "zh" | "en"`, passes to CR
3. **Translator** adds `OUTPUT_LANGUAGE` env var to the agent Job container, sourced from `cr.Spec.OutputLanguage` (fallback `"en"` if empty)
4. **orchestrator.py** reads `OUTPUT_LANGUAGE`, chooses one of two literal strings to append to the system prompt:

```python
OUTPUT_LANG = os.environ.get("OUTPUT_LANGUAGE", "en")

LANG_INSTRUCTION = {
    "zh": "Output title/description/suggestion fields in Simplified Chinese (简体中文).",
    "en": "Output title/description/suggestion fields in English.",
}.get(OUTPUT_LANG, LANG_INSTRUCTION_EN)

SYSTEM_PROMPT = f"""You are a Kubernetes diagnostic agent.
{LANG_INSTRUCTION}
Other fields (dimension, severity, resource_kind/namespace/name) keep as English enum values.
... (existing content) ...
"""
```

### 3.3 Enum fields stay English in storage

`dimension`, `severity`, `phase` etc. remain English enum values end-to-end (CRD, store, LLM output). The dashboard renders them through the i18n dictionary: `t(\`dimension.${finding.dimension}\`)` → `健康` or `Health`.

This means switching the UI language *after* a run completes instantly re-renders all labels in the new language, while free-text (title/description/suggestion) stays in whatever language the LLM produced for that run.

---

## 4. Dark Mode Coverage

Every page and component that uses a color class needs a `dark:` counterpart.

### 4.1 Scope (files that need updates)

- `app/layout.tsx` — nav bar + body background
- `app/page.tsx` (Runs list) — table + stat cards + message cells
- `app/runs/[id]/page.tsx` — detail page + finding cards + message banner
- `app/skills/page.tsx` — stat cards + table + badges
- `app/fixes/page.tsx` — stat cards + table
- `app/fixes/[id]/page.tsx` — detail page + patch code block
- `components/ui/button.tsx` — primary/outline variants
- `components/ui/badge.tsx` — all variants
- `components/ui/card.tsx`
- `components/ui/dialog.tsx` — backdrop + popup
- `components/ui/table.tsx` — border colors
- `components/ui/separator.tsx`
- `components/tag-input.tsx` — chip + input styling
- `components/create-run-dialog.tsx` + `create-skill-dialog.tsx` — form inputs
- `components/phase-badge.tsx`, `severity-badge.tsx` — status chips

### 4.2 Color conventions

| Light class | Dark class |
|-------------|------------|
| `bg-white` | `dark:bg-gray-900` |
| `bg-gray-50` | `dark:bg-gray-950` |
| `border-gray-200` | `dark:border-gray-800` |
| `text-gray-900` | `dark:text-gray-100` |
| `text-gray-600` | `dark:text-gray-400` |
| `text-gray-500` | `dark:text-gray-500` (unchanged, still readable) |
| `bg-blue-50` (message boxes) | `dark:bg-blue-950` |
| `text-blue-700` | `dark:text-blue-300` |
| (same pattern for red/yellow/green/purple/orange) |

### 4.3 Tailwind config

The project already uses Tailwind v4 (`dashboard/src/app/globals.css`) with `@custom-variant dark (&:is(.dark *))` configured — `dark:` variants activate when `<html class="dark">` is set. No config change needed.

shadcn components use semantic CSS variables (`bg-background`, `text-foreground`, etc.) that already swap in dark mode. Only pages and custom components using direct color classes (`bg-white`, `text-gray-600`) need `dark:` variants added.

---

## 5. Data Flow Summary

```
User clicks 🌙 toggle
  ↓
ThemeContext.setTheme("dark")
  ↓
  ├─ localStorage.theme = "dark"
  └─ document.documentElement.classList.add("dark")
          ↓
          All dark: variants activate

User clicks 中 toggle
  ↓
I18nContext.setLang("en")
  ↓
  ├─ localStorage.lang = "en"
  └─ Re-render — every t("...") call now reads en.json

User opens CreateRunDialog when UI lang = zh
  ↓
Form prefills outputLanguage = "zh"
  ↓
User submits → POST /api/runs { ..., outputLanguage: "zh" }
  ↓
httpserver creates CR with spec.outputLanguage = "zh"
  ↓
Reconciler → Translator → Job env { OUTPUT_LANGUAGE: "zh" }
  ↓
Agent pod orchestrator.py injects Chinese instruction to system prompt
  ↓
LLM returns findings with Chinese title/description/suggestion
  ↓
Dashboard displays:
  - enum fields through i18n dict (render-time language)
  - free-text fields as stored (fixed at run-time)
```

---

## 6. Backward Compatibility

- **Existing CRs** without `spec.outputLanguage` → reconciler defaults to `"en"` → agent behaves as before
- **Existing findings** in store → remain in whatever language they were produced in; dashboard renders as-is
- **Users not touching toggles** → see zh / dark by default (new behavior — acceptable since this is internal-facing)

---

## 7. Non-Goals

- Runtime language switching *during* an in-flight run (the language is fixed at CR creation time)
- Translating the LLM's historical findings after-the-fact
- Language-specific skill prompts (the skill files stay English; only the instruction appended by orchestrator changes)
- RTL languages, accessibility improvements, or font changes

---

## 8. Testing Strategy

### Unit tests
- `I18nContext` — key lookup, fallback to key on missing entry, localStorage persistence
- `ThemeContext` — DOM class toggle, localStorage persistence
- `httpserver_test.go` — POST /api/runs with `outputLanguage` is forwarded to CR
- `translator_test.go` — CR with `spec.outputLanguage="zh"` produces Job with `OUTPUT_LANGUAGE=zh` env

### Manual QA checklist
- Open dashboard — verify zh + dark
- Toggle theme → light mode renders all pages correctly
- Toggle language → all UI labels switch to English
- Create Run while zh — confirm Findings come back in Chinese
- Switch to English, view old zh run — free-text stays zh, labels are English

---

## 9. File Structure After Change

```
dashboard/src/
├── i18n/
│   ├── zh.json                   (new — default dictionary)
│   ├── en.json                   (new)
│   └── context.tsx               (new — I18nContext + useI18n hook)
├── theme/
│   └── context.tsx               (new — ThemeContext + useTheme hook)
├── components/
│   ├── client-providers.tsx      (new — wraps both contexts)
│   ├── theme-toggle.tsx          (new)
│   ├── language-toggle.tsx       (new)
│   └── ... (existing — modified for dark: variants + t() calls)
├── app/
│   ├── layout.tsx                (modified — add inline pre-hydration script + providers)
│   └── ... (every page — replace hardcoded strings + add dark: variants)
└── lib/
    └── types.ts                  (modified — add outputLanguage to CreateRunRequest)

internal/controller/
├── api/v1alpha1/types.go         (modified — add OutputLanguage to DiagnosticRunSpec)
├── api/v1alpha1/zz_generated.deepcopy.go  (regen unnecessary — string field)
├── translator/translator.go      (modified — inject OUTPUT_LANGUAGE env)
├── translator/translator_test.go (modified — test env injection)
├── httpserver/server.go          (modified — handleAPIRunsPost accepts outputLanguage)
└── httpserver/server_test.go     (modified — test new field)

deploy/helm/templates/crds/k8sai.io_diagnosticruns.yaml  (modified — add outputLanguage schema)

agent-runtime/runtime/orchestrator.py  (modified — read OUTPUT_LANGUAGE, inject lang instruction)
```

---

## 10. Risks / Open Questions

None blocking. The translation key coverage is tedious but mechanical. The LLM output language change is a single-line concat in orchestrator.py — low risk.
