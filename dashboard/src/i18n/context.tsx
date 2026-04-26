/**
 * 双语 i18n 上下文（中文 / English）。
 *
 * 字典文件：zh.json / en.json，结构是嵌套 JSON（runs.form.name 之类的点路径）。
 * 也支持半扁平 key（runs.stat.total 既可以是 dict.runs.stat.total，也可以
 * 是 dict.runs["stat.total"]）— 这是为了兼容渐进迁移。
 *
 * 用法：
 *   const { t, lang, setLang } = useI18n();
 *   t("runs.form.name")     // "Run name" 或 "运行名称"
 *
 * 切换语言会写 localStorage["kah-lang"]，并通过 storage 事件跨标签页同步。
 */
"use client";

import { createContext, useCallback, useContext, useEffect, useState, ReactNode } from "react";
import zh from "./zh.json";
import en from "./en.json";

export type Lang = "zh" | "en";

const dictionaries: Record<Lang, Record<string, unknown>> = { zh, en };

/**
 * Look up `key` in `dict`. Supports both fully-nested keys (runs.form.name
 * walks runs → form → name) and partially-flat keys (runs.stat.total can
 * resolve to dict.runs["stat.total"] if the nested walk fails).
 */
function lookup(dict: Record<string, unknown>, key: string): string {
  const parts = key.split(".");
  // Try progressively shorter walks, greedy from the left.
  // At each level, attempt to consume the remaining joined key as a single leaf.
  let cur: unknown = dict;
  for (let i = 0; i < parts.length; i++) {
    if (!cur || typeof cur !== "object") return key;
    const obj = cur as Record<string, unknown>;
    // Try the joined tail as a leaf key first
    const tail = parts.slice(i).join(".");
    if (tail in obj && typeof obj[tail] === "string") {
      return obj[tail] as string;
    }
    // Otherwise recurse one segment
    if (parts[i] in obj) {
      cur = obj[parts[i]];
    } else {
      return key;
    }
  }
  return typeof cur === "string" ? cur : key;
}

interface I18nCtx {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: (key: string) => string;
}

const Ctx = createContext<I18nCtx | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  // Always start with "zh" to match SSR; read localStorage after hydration
  const [lang, setLangState] = useState<Lang>("zh");

  useEffect(() => {
    const apply = (v: string | null) => {
      if (v === "zh" || v === "en") setLangState(v);
    };
    apply(localStorage.getItem("lang"));
    const handler = (e: StorageEvent) => { if (e.key === "lang") apply(e.newValue); };
    window.addEventListener("storage", handler);
    return () => window.removeEventListener("storage", handler);
  }, []);

  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    if (typeof window !== "undefined") localStorage.setItem("lang", l);
  }, []);

  const t = useCallback((key: string) => lookup(dictionaries[lang], key), [lang]);

  return <Ctx.Provider value={{ lang, setLang, t }}>{children}</Ctx.Provider>;
}

export function useI18n(): I18nCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error("useI18n must be used inside I18nProvider");
  return v;
}
