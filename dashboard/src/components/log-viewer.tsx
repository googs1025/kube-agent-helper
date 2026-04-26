/**
 * 实时日志查看器（用在 /runs/[id] 详情页）。
 *
 * 数据源切换：
 *   - 运行中：EventSource('/api/runs/{id}/logs?follow=true') ── SSE 流
 *     后端从 Pod stdout 直读，每行一个 JSON 推送
 *   - 已完成：fetch('/api/runs/{id}/logs')                  ── 一次性 JSON 数组
 *     后端从 SQLite run_logs 表读取（reconciler 在 Pod 退出时已采集）
 *
 * 切换时机：phase ∈ {Pending, Running} 用 SSE，否则用 fetch。
 *
 * UI 行为：
 *   - 默认自动滚动到底部
 *   - 鼠标悬停时暂停滚动（避免阅读时跳走）
 *   - 按 type 着色：step/finding/fix/error/info 用不同颜色
 */
"use client";

import { useEffect, useRef, useState } from "react";
import type { RunLogEntry } from "@/lib/types";
import { useI18n } from "@/i18n/context";

const typeColors: Record<string, string> = {
  step: "text-sky-400",
  finding: "text-yellow-400",
  fix: "text-green-400",
  error: "text-red-400",
  info: "text-gray-400",
};

const typeBadgeColors: Record<string, string> = {
  step: "bg-sky-500/20 text-sky-400",
  finding: "bg-yellow-500/20 text-yellow-400",
  fix: "bg-green-500/20 text-green-400",
  error: "bg-red-500/20 text-red-400",
  info: "bg-gray-500/20 text-gray-400",
};

function formatTimestamp(ts: string): string {
  try {
    const d = new Date(ts);
    return d.toLocaleTimeString([], { hour12: false, fractionalSecondDigits: 3 });
  } catch {
    return ts;
  }
}

interface LogViewerProps {
  runId: string;
  isRunning: boolean;
}

export function LogViewer({ runId, isRunning }: LogViewerProps) {
  const { t } = useI18n();
  const [logs, setLogs] = useState<RunLogEntry[]>([]);
  const [autoScroll, setAutoScroll] = useState(true);
  const [isFollowing, setIsFollowing] = useState(false);
  const [expanded, setExpanded] = useState(true);
  const containerRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);

  // Load historical logs or stream via SSE
  useEffect(() => {
    if (!expanded) return;

    if (!isRunning) {
      // Completed run: fetch all logs at once
      fetch(`/api/runs/${runId}/logs`)
        .then((r) => r.json())
        .then((data) => {
          if (Array.isArray(data)) setLogs(data);
        })
        .catch(() => {});
      return;
    }

    // Running: use SSE for live streaming
    const es = new EventSource(`/api/runs/${runId}/logs?follow=true`);

    es.onopen = () => {
      setIsFollowing(true);
    };

    es.onmessage = (e) => {
      try {
        const entry: RunLogEntry = JSON.parse(e.data);
        setLogs((prev) => [...prev, entry]);
      } catch {
        // ignore malformed messages
      }
    };

    es.addEventListener("done", () => {
      setIsFollowing(false);
      es.close();
    });

    es.onerror = () => {
      setIsFollowing(false);
      es.close();
    };

    return () => {
      es.close();
      setIsFollowing(false);
    };
  }, [runId, isRunning, expanded]);

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (autoScroll && bottomRef.current) {
      bottomRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [logs, autoScroll]);

  // Pause auto-scroll on hover
  const handleMouseEnter = () => setAutoScroll(false);
  const handleMouseLeave = () => setAutoScroll(true);

  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex w-full items-center justify-between px-4 py-2.5 text-sm font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800/50 rounded-lg"
      >
        <span className="flex items-center gap-2">
          <span>{t("logs.title")}</span>
          {isFollowing && (
            <span className="inline-flex items-center gap-1 text-xs font-normal text-green-400">
              <span className="h-2 w-2 rounded-full bg-green-400 animate-pulse" />
              {t("logs.live")}
            </span>
          )}
          {logs.length > 0 && (
            <span className="text-xs font-normal text-gray-500">({logs.length})</span>
          )}
        </span>
        <span className="text-gray-400">{expanded ? "\u25B2" : "\u25BC"}</span>
      </button>
      {expanded && (
        <div className="border-t border-gray-200 dark:border-gray-700">
          <div
            ref={containerRef}
            onMouseEnter={handleMouseEnter}
            onMouseLeave={handleMouseLeave}
            className="overflow-y-auto max-h-96 bg-gray-50 dark:bg-[#0a0e14] p-3 font-mono text-xs leading-relaxed rounded-b-lg"
          >
            {logs.length === 0 && (
              <p className="text-gray-500 text-center py-4">{t("logs.empty")}</p>
            )}
            {logs.map((log) => (
              <div key={log.id} className="flex gap-2 py-0.5 hover:bg-gray-100 dark:hover:bg-gray-800/30 px-1 rounded">
                <span className="text-gray-500 shrink-0 select-none">
                  {formatTimestamp(log.timestamp)}
                </span>
                <span
                  className={`shrink-0 rounded px-1.5 py-0 text-[10px] font-semibold uppercase ${
                    typeBadgeColors[log.type] || typeBadgeColors.info
                  }`}
                >
                  {log.type}
                </span>
                <span className={typeColors[log.type] || "text-gray-300"}>
                  {log.message}
                </span>
              </div>
            ))}
            <div ref={bottomRef} />
          </div>
        </div>
      )}
    </div>
  );
}
