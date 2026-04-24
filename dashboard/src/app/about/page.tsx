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
        <p className="mt-1 text-sm text-muted-foreground">Kube Agent Helper — AI-powered Kubernetes diagnostics</p>
      </div>

      {/* Architecture */}
      <Card>
        <CardHeader>
          <CardTitle>{t("about.arch.title")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">{t("about.arch.desc")}</p>
          <pre className="overflow-x-auto rounded-lg bg-[#0a0e14] border border-border p-4 text-xs text-slate-300 leading-relaxed">{`┌────────────────────────────────────────────────────────────────────────────┐
│  User: Dashboard (Next.js :3000) / kubectl / REST API (:8080)             │
│  5 CRDs: DiagnosticRun · DiagnosticFix · DiagnosticSkill · ModelConfig    │
│          · ClusterConfig                                                  │
└────────┬──────────────────────┬────────────────────────────────────────────┘
         │ CR apply             │ /api/*
         ▼                      ▼
┌────────────────────────────────────────────────────────────────────────────┐
│  Controller (Go)                                                           │
│  ┌─────────────────────┐  ┌────────────────┐  ┌────────────────────────┐  │
│  │ 6 Reconcilers        │  │ HTTP Server     │  │ Translator             │  │
│  │ DiagnosticRun        │  │ /api/runs       │  │ CR → Job + SA + RBAC   │  │
│  │ DiagnosticFix        │  │ /api/skills     │  │ + ConfigMap            │  │
│  │ DiagnosticSkill      │  │ /api/fixes      │  │ ClusterRef → target    │  │
│  │ ModelConfig          │  │ /api/events     │  │ cluster routing        │  │
│  │ ScheduledRun         │  │ /api/modelconfigs│  └────────────────────────┘  │
│  │ ClusterConfig        │  │ /api/clusters   │                              │
│  └─────────────────────┘  └────────────────┘                               │
│  ┌─────────────────────────────────┐  ┌─────────────────────────────────┐  │
│  │ SQLite                           │  │ EventCollector                   │  │
│  │ runs/findings/fixes/events       │  │ K8s Warning + Prom Snapshots     │  │
│  │ (cluster_name filter)            │  └─────────────────────────────────┘  │
│  └─────────────────────────────────┘  ┌─────────────────────────────────┐  │
│                                        │ ClusterClientRegistry           │  │
│                                        │ kubeconfig → remote K8s client  │  │
│                                        └─────────────────────────────────┘  │
└────────┬────────────────────────┬──────────────────────────────────────────┘
         │ creates Job            │ creates Job (on target cluster)
         ▼                        ▼
┌──────────────────────────┐   ┌────────────────────────────┐
│ Diagnostic Agent Pod      │   │ Fix Generator Pod           │
│ python -m runtime.main    │   │ single LLM call → patch JSON│
│ multi-turn Claude loop    │   │ strategy: merge/create      │
│ ┌──────────────────────┐ │   └────────────────────────────┘
│ │ k8s-mcp-server (Go)  │ │         │
│ │ 16 MCP Tools         │ │         ▼
│ │ kubectl · prometheus  │ │   ┌──────────────┐
│ │ events · metrics · …  │ │   │ Claude API   │
│ └──────────────────────┘ │   │ (Anthropic)  │
│ POST findings → Controller│   └──────────────┘
└──────────────────────────┘`}</pre>
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
              { name: "DiagnosticRun", color: "bg-sky-500/10 text-sky-400", desc: t("about.crd.run") },
              { name: "DiagnosticSkill", color: "bg-green-500/10 text-green-400", desc: t("about.crd.skill") },
              { name: "ModelConfig", color: "bg-purple-500/10 text-purple-400", desc: t("about.crd.model") },
              { name: "DiagnosticFix", color: "bg-orange-500/10 text-orange-400", desc: t("about.crd.fix") },
              { name: "ClusterConfig", color: "bg-cyan-500/10 text-cyan-400", desc: t("about.crd.cluster") },
            ].map((crd) => (
              <div key={crd.name} className="rounded-lg border border-border bg-background p-4">
                <Badge className={`${crd.color} border border-current/20`}>{crd.name}</Badge>
                <p className="mt-2 text-sm text-muted-foreground">{crd.desc}</p>
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
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary text-sm font-bold text-primary-foreground">
                  {step}
                </div>
                <div>
                  <p className="font-medium text-sm">{t(`about.flow.step${step}`)}</p>
                  <p className="text-sm text-muted-foreground">{t(`about.flow.step${step}.desc`)}</p>
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
          <p className="text-sm text-muted-foreground mb-3">{t("about.tools.desc")}</p>
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
