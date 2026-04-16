"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DialogRoot, DialogTrigger, DialogPortal, DialogBackdrop, DialogPopup, DialogTitle, DialogClose } from "@/components/ui/dialog";
import { TagInput } from "@/components/tag-input";
import { createRun } from "@/lib/api";
import type { CreateRunRequest } from "@/lib/types";

interface Props {
  onCreated: () => void;
}

export function CreateRunDialog({ onCreated }: Props) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [name, setName] = useState("");
  const [namespace, setNamespace] = useState("kube-agent-helper");
  const [scope, setScope] = useState<"namespace" | "cluster">("namespace");
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [labelSelector, setLabelSelector] = useState<string[]>([]);
  const [skills, setSkills] = useState<string[]>([]);
  const [modelConfigRef, setModelConfigRef] = useState("anthropic-credentials");
  const [timeoutSeconds, setTimeoutSeconds] = useState<string>("");

  function parseLabelSelector(tags: string[]): Record<string, string> {
    const result: Record<string, string> = {};
    for (const tag of tags) {
      const idx = tag.indexOf("=");
      if (idx > 0) result[tag.slice(0, idx)] = tag.slice(idx + 1);
    }
    return result;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    const body: CreateRunRequest = {
      name: name || undefined,
      namespace,
      target: {
        scope,
        namespaces: scope === "namespace" && namespaces.length > 0 ? namespaces : undefined,
        labelSelector: labelSelector.length > 0 ? parseLabelSelector(labelSelector) : undefined,
      },
      skills: skills.length > 0 ? skills : undefined,
      modelConfigRef,
      timeoutSeconds: timeoutSeconds ? Number(timeoutSeconds) : undefined,
    };
    setLoading(true);
    try {
      await createRun(body);
      setOpen(false);
      onCreated();
      setName(""); setNamespaces([]); setLabelSelector([]); setSkills([]); setTimeoutSeconds("");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "创建失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <DialogRoot open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm"><Plus className="size-4" />创建 Run</Button>} />
      <DialogPortal>
        <DialogBackdrop />
        <DialogPopup>
          <form onSubmit={handleSubmit} className="max-h-[85vh] overflow-y-auto p-6 space-y-4">
            <DialogTitle>新建 DiagnosticRun</DialogTitle>

            {error && <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</div>}

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Name <span className="font-normal normal-case text-gray-400">（留空自动生成）</span></label>
              <input value={name} onChange={(e) => setName(e.target.value)} placeholder="run-20260415"
                className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20" />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Namespace *</label>
              <input required value={namespace} onChange={(e) => setNamespace(e.target.value)} placeholder="kube-agent-helper"
                className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20" />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Scope *</label>
              <div className="flex gap-2">
                {(["namespace", "cluster"] as const).map((s) => (
                  <button key={s} type="button" onClick={() => setScope(s)}
                    className={`rounded-lg px-4 py-1.5 text-sm font-medium transition-colors ${scope === s ? "bg-blue-600 text-white" : "border border-gray-200 text-gray-600 hover:border-blue-300"}`}>
                    {s}
                  </button>
                ))}
              </div>
              <p className="text-xs text-gray-400">
                <strong className="text-gray-500">namespace</strong> — 只扫描指定 namespace &nbsp;·&nbsp;
                <strong className="text-gray-500">cluster</strong> — 扫描整个集群
              </p>
            </div>

            {scope === "namespace" && (
              <div className="space-y-1.5">
                <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Namespaces <span className="font-normal normal-case text-gray-400">（留空 = 全部）</span></label>
                <TagInput value={namespaces} onChange={setNamespaces} placeholder="输入 namespace，回车添加" />
              </div>
            )}

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Label Selector <span className="font-normal normal-case text-gray-400">（可选）</span></label>
              <p className="text-xs text-gray-400">只诊断带指定 label 的资源，如 <code className="rounded bg-gray-100 px-1">app=nginx</code>，留空 = 不过滤</p>
              <TagInput value={labelSelector} onChange={setLabelSelector} placeholder="输入 key=value，回车添加" />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Skills <span className="font-normal normal-case text-gray-400">（留空 = 全部启用的 skill）</span></label>
              <TagInput value={skills} onChange={setSkills} placeholder="输入 skill 名称，回车添加" />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">ModelConfigRef *</label>
              <input required value={modelConfigRef} onChange={(e) => setModelConfigRef(e.target.value)} placeholder="anthropic-credentials"
                className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20" />
              <p className="text-xs text-gray-400">引用集群中 ModelConfig CR 的名称</p>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">
                Timeout <span className="font-normal normal-case text-gray-400">（秒，留空 = 不超时）</span>
              </label>
              <input type="number" min={0} value={timeoutSeconds} onChange={(e) => setTimeoutSeconds(e.target.value)}
                placeholder="600"
                className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20" />
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <DialogClose render={<Button type="button" variant="outline" disabled={loading}>取消</Button>} />
              <Button type="submit" disabled={loading}>{loading ? "创建中..." : "创建 Run"}</Button>
            </div>
          </form>
        </DialogPopup>
      </DialogPortal>
    </DialogRoot>
  );
}
