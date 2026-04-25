"use client";

import { useState, useCallback } from "react";
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

export function useTableState(defaults?: Partial<ListParams>): TableState {
  const [page, setPageRaw] = useState(defaults?.page ?? 1);
  const [pageSize, setPageSizeRaw] = useState(defaults?.pageSize ?? 20);
  const [sortBy, setSortBy] = useState(defaults?.sortBy ?? "created_at");
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">(
    (defaults?.sortOrder as "asc" | "desc") ?? "desc"
  );
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // Reset to page 1 when changing page size or filters
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
