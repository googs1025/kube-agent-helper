"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DialogRoot, DialogTrigger, DialogPortal, DialogBackdrop, DialogPopup, DialogTitle, DialogClose } from "@/components/ui/dialog";
import { TagInput } from "@/components/tag-input";
import { createSkill } from "@/lib/api";
import type { CreateSkillRequest } from "@/lib/types";

const AVAILABLE_TOOLS = ["kubectl_get", "kubectl_describe", "events_list", "logs_get"];
const DIMENSIONS = ["health", "security", "cost", "reliability"] as const;

interface Props {
  onCreated: () => void;
}

export function CreateSkillDialog({ onCreated }: Props) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [name, setName] = useState("");
  const [namespace, setNamespace] = useState("kube-agent-helper");
  const [dimension, setDimension] = useState<CreateSkillRequest["dimension"]>("health");
  const [description, setDescription] = useState("");
  const [prompt, setPrompt] = useState("");
  const [tools, setTools] = useState<string[]>([]);
  const [requiresData, setRequiresData] = useState<string[]>([]);
  const [enabled, setEnabled] = useState(true);
  const [priority, setPriority] = useState(100);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (tools.length === 0) { setError("至少选择一个 Tool"); return; }

    const body: CreateSkillRequest = {
      name, namespace, dimension, description, prompt, tools,
      requiresData: requiresData.length > 0 ? requiresData : undefined,
      enabled, priority,
    };
    setLoading(true);
    try {
      await createSkill(body);
      setOpen(false);
      onCreated();
      setName(""); setDescription(""); setPrompt(""); setTools([]); setRequiresData([]);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "创建失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <DialogRoot open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm"><Plus className="size-4" />创建 Skill</Button>} />
      <DialogPortal>
        <DialogBackdrop />
        <DialogPopup>
          <form onSubmit={handleSubmit} className="max-h-[85vh] overflow-y-auto p-6 space-y-4">
            <DialogTitle>新建 DiagnosticSkill</DialogTitle>

            {error && <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</div>}

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Name * <span className="font-normal normal-case text-gray-400">（小写+连字符）</span></label>
                <input required value={name} onChange={(e) => setName(e.target.value)} placeholder="my-security-analyst"
                  pattern="[a-z0-9][a-z0-9\-]*"
                  className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20" />
              </div>
              <div className="space-y-1.5">
                <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Namespace *</label>
                <input required value={namespace} onChange={(e) => setNamespace(e.target.value)} placeholder="kube-agent-helper"
                  className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20" />
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Dimension *</label>
              <div className="flex flex-wrap gap-2">
                {DIMENSIONS.map((d) => (
                  <button key={d} type="button" onClick={() => setDimension(d)}
                    className={`rounded-lg px-4 py-1.5 text-sm font-medium capitalize transition-colors ${dimension === d ? "bg-blue-600 text-white" : "border border-gray-200 text-gray-600 hover:border-blue-300"}`}>
                    {d}
                  </button>
                ))}
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Description *</label>
              <input required value={description} onChange={(e) => setDescription(e.target.value)} placeholder="分析 Pod 的健康状态"
                className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20" />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Prompt *</label>
              <textarea required rows={4} value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder="你是一个 K8s 健康分析专家..."
                className="w-full resize-y rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20" />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Tools * （至少一个）</label>
              <TagInput value={tools} onChange={setTools} suggestions={AVAILABLE_TOOLS} />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">RequiresData <span className="font-normal normal-case text-gray-400">（可选）</span></label>
              <p className="text-xs text-gray-400">声明 skill 需要的外部数据源，如 <code className="rounded bg-gray-100 px-1">workflows</code>、<code className="rounded bg-gray-100 px-1">logs</code></p>
              <TagInput value={requiresData} onChange={setRequiresData} placeholder="输入数据源，回车添加" />
            </div>

            <div className="flex items-center justify-between">
              <label className="flex cursor-pointer items-center gap-2">
                <button type="button" role="switch" aria-checked={enabled} onClick={() => setEnabled(!enabled)}
                  className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${enabled ? "bg-blue-600" : "bg-gray-200"}`}>
                  <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${enabled ? "translate-x-4" : "translate-x-1"}`} />
                </button>
                <span className="text-sm text-gray-700">Enabled</span>
              </label>
              <label className="flex items-center gap-2">
                <span className="text-xs text-gray-500">Priority</span>
                <input type="number" value={priority} onChange={(e) => setPriority(Number(e.target.value))}
                  className="w-16 rounded-lg border border-gray-200 px-2 py-1 text-center text-sm outline-none focus:border-blue-400" />
              </label>
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <DialogClose render={<Button type="button" variant="outline" disabled={loading}>取消</Button>} />
              <Button type="submit" disabled={loading}>{loading ? "创建中..." : "创建 Skill"}</Button>
            </div>
          </form>
        </DialogPopup>
      </DialogPortal>
    </DialogRoot>
  );
}