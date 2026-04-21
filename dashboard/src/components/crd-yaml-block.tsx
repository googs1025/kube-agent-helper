"use client";

import { useState } from "react";
import { useI18n } from "@/i18n/context";

interface Props {
  yaml: string;
  title?: string;
}

export function CRDYamlBlock({ yaml, title }: Props) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const [expanded, setExpanded] = useState(false);

  const copy = () => {
    navigator.clipboard.writeText(yaml);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex w-full items-center justify-between px-4 py-2.5 text-sm font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800/50 rounded-lg"
      >
        <span>{title ?? t("common.crdYaml")}</span>
        <span className="text-gray-400">{expanded ? "▲" : "▼"}</span>
      </button>
      {expanded && (
        <div className="border-t border-gray-200 dark:border-gray-700">
          <div className="flex justify-end px-3 py-1.5 border-b border-gray-100 dark:border-gray-800">
            <button
              type="button"
              onClick={copy}
              className="text-xs text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
            >
              {copied ? t("common.copied") : t("common.copy")}
            </button>
          </div>
          <pre className="overflow-x-auto p-4 text-xs font-mono leading-relaxed text-gray-800 dark:text-gray-200 bg-gray-50 dark:bg-gray-900 rounded-b-lg whitespace-pre">
            {yaml}
          </pre>
        </div>
      )}
    </div>
  );
}
