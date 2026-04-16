"use client";

import { useSkills } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { CreateSkillDialog } from "@/components/create-skill-dialog";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";

export default function SkillsPage() {
  const { data: skills, error, isLoading, mutate } = useSkills();
  if (isLoading) return <p className="text-gray-500">Loading skills...</p>;
  if (error) return <p className="text-red-600">Failed to load skills.</p>;

  const total = skills?.length ?? 0;
  const enabled = skills?.filter((s) => s.Enabled).length ?? 0;
  const builtin = skills?.filter((s) => s.Source === "builtin").length ?? 0;
  const custom = skills?.filter((s) => s.Source === "cr").length ?? 0;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Skills</h1>
        <CreateSkillDialog onCreated={() => mutate()} />
      </div>
      <div className="mb-6 grid grid-cols-4 gap-4">
        {[
          { label: "Total", value: total, color: "text-gray-900" },
          { label: "Enabled", value: enabled, color: "text-green-600" },
          { label: "Builtin", value: builtin, color: "text-gray-600" },
          { label: "Custom CR", value: custom, color: "text-blue-600" },
        ].map(({ label, value, color }) => (
          <div key={label} className="rounded-lg border bg-white p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
            <p className={`mt-1 text-2xl font-bold ${color}`}>{value}</p>
          </div>
        ))}
      </div>
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
                try { tools = JSON.parse(skill.ToolsJSON); } catch { /* ignore */ }
                return (
                  <TableRow key={skill.ID}>
                    <TableCell className="font-mono text-sm font-medium">{skill.Name}</TableCell>
                    <TableCell><Badge variant="outline" className="capitalize">{skill.Dimension}</Badge></TableCell>
                    <TableCell><Badge variant={skill.Source === "cr" ? "default" : "secondary"}>{skill.Source}</Badge></TableCell>
                    <TableCell>{skill.Enabled ? <span className="text-green-600">Yes</span> : <span className="text-gray-400">No</span>}</TableCell>
                    <TableCell className="text-sm text-gray-600">{skill.Priority}</TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {tools.map((tool) => (<Badge key={tool} variant="outline" className="text-xs">{tool}</Badge>))}
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
