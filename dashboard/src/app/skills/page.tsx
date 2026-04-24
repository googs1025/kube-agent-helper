"use client";

import { useState, Fragment } from "react";
import { useSkills } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import { Badge } from "@/components/ui/badge";
import { CreateSkillDialog } from "@/components/create-skill-dialog";
import { ChevronDown, ChevronRight } from "lucide-react";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";

export default function SkillsPage() {
  const { t } = useI18n();
  const { data: skills, error, isLoading, mutate } = useSkills();
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  function toggleExpand(id: string) {
    setExpanded((prev) => ({ ...prev, [id]: !prev[id] }));
  }
  if (isLoading) return <p className="text-muted-foreground">{t("common.loading")}</p>;
  if (error) return <p className="text-destructive">{t("common.loadFailed")}</p>;

  const total = skills?.length ?? 0;
  const enabled = skills?.filter((s) => s.Enabled).length ?? 0;
  const builtin = skills?.filter((s) => s.Source === "builtin").length ?? 0;
  const custom = skills?.filter((s) => s.Source === "cr").length ?? 0;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t("skills.title")}</h1>
        <CreateSkillDialog onCreated={() => mutate()} />
      </div>
      <div className="mb-6 grid grid-cols-4 gap-4">
        {[
          { label: t("skills.stat.total"), value: total, color: "text-foreground" },
          { label: t("skills.stat.enabled"), value: enabled, color: "text-green-400" },
          { label: t("skills.stat.builtin"), value: builtin, color: "text-muted-foreground" },
          { label: t("skills.stat.custom"), value: custom, color: "text-primary" },
        ].map(({ label, value, color }) => (
          <div key={label} className="rounded-lg border border-border bg-card p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</p>
            <p className={`mt-1 text-3xl font-bold ${color}`}>{value}</p>
          </div>
        ))}
      </div>
      {skills && skills.length === 0 ? (
        <p className="text-muted-foreground">{t("skills.empty")}</p>
      ) : (
        <div className="rounded-lg border border-border bg-card overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("skills.col.name")}</TableHead>
                <TableHead>{t("skills.col.dimension")}</TableHead>
                <TableHead>{t("skills.col.source")}</TableHead>
                <TableHead>{t("skills.col.enabled")}</TableHead>
                <TableHead>{t("skills.col.priority")}</TableHead>
                <TableHead>{t("skills.col.tools")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {skills?.map((skill) => {
                let tools: string[] = [];
                try { tools = JSON.parse(skill.ToolsJSON); } catch { /* ignore */ }
                let requiresData: string[] = [];
                try { requiresData = JSON.parse(skill.RequiresDataJSON); } catch { /* ignore */ }
                const isOpen = expanded[skill.ID] ?? false;
                return (
                  <Fragment key={skill.ID}>
                    <TableRow className="cursor-pointer" onClick={() => toggleExpand(skill.ID)}>
                      <TableCell className="font-mono text-sm font-medium">
                        <span className="inline-flex items-center gap-1.5">
                          {isOpen ? <ChevronDown className="size-3.5 text-muted-foreground" /> : <ChevronRight className="size-3.5 text-muted-foreground" />}
                          {skill.Name}
                        </span>
                      </TableCell>
                      <TableCell><Badge variant="outline">{t(`dimension.${skill.Dimension}`)}</Badge></TableCell>
                      <TableCell><Badge variant={skill.Source === "cr" ? "default" : "secondary"}>{t(`skills.source.${skill.Source}`)}</Badge></TableCell>
                      <TableCell>{skill.Enabled ? <span className="text-green-400">{t("common.yes")}</span> : <span className="text-muted-foreground">{t("common.no")}</span>}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{skill.Priority}</TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-1">
                          {tools.map((tool) => (<Badge key={tool} variant="outline" className="text-xs">{tool}</Badge>))}
                        </div>
                      </TableCell>
                    </TableRow>
                    {isOpen && (
                      <TableRow>
                        <TableCell colSpan={6} className="bg-muted/30 p-0">
                          <div className="px-6 py-4 space-y-3">
                            <div>
                              <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground mb-1">Prompt</p>
                              <pre className="whitespace-pre-wrap text-sm rounded-lg bg-[#0a0e14] text-slate-200 p-4 max-h-64 overflow-y-auto">{skill.Prompt}</pre>
                            </div>
                            {requiresData && requiresData.length > 0 && (
                              <div>
                                <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground mb-1">{t("skills.form.requiresData")}</p>
                                <div className="flex flex-wrap gap-1.5">
                                  {requiresData.map((d) => (<Badge key={d} variant="outline" className="text-xs">{d}</Badge>))}
                                </div>
                              </div>
                            )}
                            <div className="flex gap-6 text-xs text-muted-foreground">
                              <span>ID: <code className="font-mono">{skill.ID}</code></span>
                              <span>{t("skills.col.source")}: {t(`skills.source.${skill.Source}`)}</span>
                              <span>{t("common.updated")}: {new Date(skill.UpdatedAt).toLocaleString()}</span>
                            </div>
                          </div>
                        </TableCell>
                      </TableRow>
                    )}
                  </Fragment>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
