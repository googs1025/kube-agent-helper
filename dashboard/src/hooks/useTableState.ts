/**
 * 通用表格状态 hook（分页 / 排序 / 过滤 / 多选 / URL 同步）。
 *
 * 所有列表页（runs / fixes / events 等）共享这个 hook，避免重复实现：
 *
 *   const t = useTableState({ pageSize: 20 }, { syncURL: true, urlPrefix: "runs" });
 *   const { data } = useRunsPaginated(t.params);  // 自动包含 page/sort/filter
 *   <Table ... selected={t.selected} onToggle={t.toggleSelect} />
 *
 * 设计要点：
 *   - selected 用 Set<string>，支持跨页保留（不持久化到 URL，刷新即清）
 *   - filters 是平铺 string map，传给后端拼成 ?xxx=yyy 查询串
 *   - syncURL=true 时把 page / pageSize / sortBy / sortOrder / filters 写到 URL，
 *     用 history.replaceState 不污染浏览器历史；同一 layout 下多张表用 urlPrefix 区分
 */
"use client";

import { useState, useCallback, useEffect } from "react";
import type { ListParams } from "@/lib/types";

export interface TableState {
  page: number;
  setPage: (p: number) => void;
  pageSize: number;
  setPageSize: (ps: number) => void;
  sortBy: string;
  setSortBy: (s: string) => void;
  sortOrder: "asc" | "desc";
  setSortOrder: (o: "asc" | "desc") => void;
  filters: Record<string, string>;
  setFilter: (key: string, value: string) => void;
  clearFilters: () => void;
  selected: Set<string>;
  toggleSelect: (id: string) => void;
  selectAll: (ids: string[]) => void;
  clearSelection: () => void;
  params: ListParams;
}

export interface UseTableStateOptions {
  /** 把状态同步到 URL query string，刷新页面后能恢复，链接可分享 */
  syncURL?: boolean;
  /** 同一 layout 下多张表共存时用前缀避免 query key 冲突，例如 "runs" → ?runs.page=2 */
  urlPrefix?: string;
}

const RESERVED_KEYS = new Set(["page", "pageSize", "sortBy", "sortOrder"]);

function buildKey(prefix: string | undefined, key: string): string {
  return prefix ? `${prefix}.${key}` : key;
}

function readInitialFromURL(prefix: string | undefined, defaults: Partial<ListParams>) {
  if (typeof window === "undefined") {
    return {
      page: defaults.page ?? 1,
      pageSize: defaults.pageSize ?? 20,
      sortBy: defaults.sortBy ?? "created_at",
      sortOrder: (defaults.sortOrder as "asc" | "desc") ?? "desc",
      filters: {} as Record<string, string>,
    };
  }
  const sp = new URLSearchParams(window.location.search);
  const get = (k: string) => sp.get(buildKey(prefix, k));
  const filters: Record<string, string> = {};
  sp.forEach((value, fullKey) => {
    const key = prefix && fullKey.startsWith(`${prefix}.`)
      ? fullKey.slice(prefix.length + 1)
      : !prefix && !fullKey.includes(".")
        ? fullKey
        : null;
    if (!key || RESERVED_KEYS.has(key)) return;
    filters[key] = value;
  });
  return {
    page: Number(get("page")) || defaults.page || 1,
    pageSize: Number(get("pageSize")) || defaults.pageSize || 20,
    sortBy: get("sortBy") || defaults.sortBy || "created_at",
    sortOrder: ((get("sortOrder") as "asc" | "desc") || (defaults.sortOrder as "asc" | "desc") || "desc"),
    filters,
  };
}

export function useTableState(
  defaults?: Partial<ListParams>,
  options?: UseTableStateOptions,
): TableState {
  const syncURL = options?.syncURL ?? false;
  const urlPrefix = options?.urlPrefix;

  // useState lazy initializer runs once on mount — safe to read window.location.
  const init = () =>
    syncURL
      ? readInitialFromURL(urlPrefix, defaults ?? {})
      : {
          page: defaults?.page ?? 1,
          pageSize: defaults?.pageSize ?? 20,
          sortBy: defaults?.sortBy ?? "created_at",
          sortOrder: (defaults?.sortOrder as "asc" | "desc") ?? "desc",
          filters: {} as Record<string, string>,
        };

  const [page, setPageRaw] = useState(() => init().page);
  const [pageSize, setPageSizeRaw] = useState(() => init().pageSize);
  const [sortBy, setSortBy] = useState(() => init().sortBy);
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">(() => init().sortOrder);
  const [filters, setFilters] = useState<Record<string, string>>(() => init().filters);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // Push state into URL when any tracked field changes.
  useEffect(() => {
    if (!syncURL || typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    // Wipe our keys first so removed filters drop from the URL.
    Array.from(sp.keys()).forEach((k) => {
      const stripped = urlPrefix && k.startsWith(`${urlPrefix}.`)
        ? k.slice(urlPrefix.length + 1)
        : !urlPrefix && !k.includes(".")
          ? k
          : null;
      if (stripped !== null) sp.delete(k);
    });
    sp.set(buildKey(urlPrefix, "page"), String(page));
    sp.set(buildKey(urlPrefix, "pageSize"), String(pageSize));
    if (sortBy) sp.set(buildKey(urlPrefix, "sortBy"), sortBy);
    if (sortOrder) sp.set(buildKey(urlPrefix, "sortOrder"), sortOrder);
    Object.entries(filters).forEach(([k, v]) => {
      if (v) sp.set(buildKey(urlPrefix, k), v);
    });
    const next = `${window.location.pathname}?${sp.toString()}`;
    window.history.replaceState(null, "", next);
  }, [syncURL, urlPrefix, page, pageSize, sortBy, sortOrder, filters]);

  const setPage = useCallback((p: number) => setPageRaw(p), []);
  const setPageSize = useCallback((ps: number) => {
    setPageSizeRaw(ps);
    setPageRaw(1);
  }, []);

  const setFilter = useCallback((key: string, value: string) => {
    setFilters((prev) => {
      if (value === "") {
        const next = { ...prev };
        delete next[key];
        return next;
      }
      return { ...prev, [key]: value };
    });
    setPageRaw(1);
  }, []);

  const clearFilters = useCallback(() => {
    setFilters({});
    setPageRaw(1);
  }, []);

  const toggleSelect = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const selectAll = useCallback((ids: string[]) => {
    setSelected((prev) => {
      if (prev.size === ids.length) return new Set();
      return new Set(ids);
    });
  }, []);

  const clearSelection = useCallback(() => setSelected(new Set()), []);

  const params: ListParams = {
    page,
    pageSize,
    sortBy,
    sortOrder,
    ...filters,
  };

  return {
    page,
    setPage,
    pageSize,
    setPageSize,
    sortBy,
    setSortBy,
    sortOrder,
    setSortOrder,
    filters,
    setFilter,
    clearFilters,
    selected,
    toggleSelect,
    selectAll,
    clearSelection,
    params,
  };
}
