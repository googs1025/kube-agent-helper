"use client";

import { useI18n } from "@/i18n/context";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

export default function AboutPage() {
  const { t } = useI18n();

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold">{t("about.title")}</h1>
      </div>

      {/* Architecture */}
      <Card>
        <CardHeader>
          <CardTitle>{t("about.arch.title")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-gray-700 dark:text-gray-300">{t("about.arch.desc")}</p>
          <pre className="overflow-x-auto rounded-lg bg-gray-900 p-4 text-xs text-gray-100 dark:bg-gray-950 leading-relaxed">{`┌─────────────────────────────────────────────────────────────────┐
│  User: Dashboard (Next.js) / kubectl / REST API                 │
└────────┬──────────────────────┬─────────────────────────────────┘
         │ CR apply             │ /api/*
         ▼                      ▼
┌─────────────────────────────────────────────────────────────────┐
│  Controller (Go)                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────────────┐ │
│  │Run Reconciler │  │Fix Reconciler │  │ HTTP Server           │ │
│  │Skill Reconciler│ │ apply/create │  │ /api/runs,skills,fixes│ │
│  │Pod status     │  │ auto-rollback│  │ /api/findings/*/fix   │ │
│  └──────┬────────┘  └──────────────┘  └───────────────────────┘ │
│         │ Translator                                             │
│         ▼                                                        │
│  ┌──────────────────────────────────────────────┐               │
│  │ SQLite (runs, findings, skills, fixes)        │               │
│  └─────────���────────────────────────────────────┘               │
└────────┬──────────────────────────┬─────────────────────────────┘
         │ creates Job              │ creates Job
         ▼                          ▼
┌────────────────────┐   ┌─────────────────────────┐
│ Diagnostic Agent   │   │ Fix Generator Pod        │
│ multi-turn LLM     │   │ single LLM call          │
│ ┌────────────────┐ │   │ kubectl_get → snapshot   │
│ │k8s-mcp-server  │ │   │ LLM → patch JSON         │
│ │9 read-only     │ │   │ POST /internal/fixes     │
│ │MCP tools       │ │   └─────────────────────────┘
│ └────────────────┘ │
│ POST findings→ctrl │
└────────────────────┘`}</pre>
        </CardContent>
      </Card>

      {/* CRD Overview */}
      <Card>
        <CardHeader>
          <CardTitle>{t("about.crd.title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            {[
              { name: "DiagnosticRun", color: "bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300", desc: t("about.crd.run") },
              { name: "DiagnosticSkill", color: "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300", desc: t("about.crd.skill") },
              { name: "ModelConfig", color: "bg-purple-100 text-purple-800 dark:bg-purple-950 dark:text-purple-300", desc: t("about.crd.model") },
              { name: "DiagnosticFix", color: "bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-300", desc: t("about.crd.fix") },
            ].map((crd) => (
              <div key={crd.name} className="rounded-lg border p-4 dark:border-gray-800">
                <Badge className={crd.color}>{crd.name}</Badge>
                <p className="mt-2 text-sm text-gray-600 dark:text-gray-400">{crd.desc}</p>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Flow */}
      <Card>
        <CardHeader>
          <CardTitle>{t("about.flow.title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {[1, 2, 3, 4, 5].map((step) => (
              <div key={step} className="flex gap-4">
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-blue-600 text-sm font-bold text-white">
                  {step}
                </div>
                <div>
                  <p className="font-medium text-sm">{t(`about.flow.step${step}`)}</p>
                  <p className="text-sm text-gray-500 dark:text-gray-400">{t(`about.flow.step${step}.desc`)}</p>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* MCP Tools */}
      <Card>
        <CardHeader>
          <CardTitle>{t("about.tools.title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-gray-700 dark:text-gray-300 mb-3">{t("about.tools.desc")}</p>
          <div className="flex flex-wrap gap-2">
            {t("about.tools.list").split(" · ").map((tool) => (
              <Badge key={tool} variant="outline" className="font-mono text-xs">{tool}</Badge>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
