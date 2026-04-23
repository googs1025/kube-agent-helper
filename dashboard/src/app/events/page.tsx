"use client";

import { useState } from "react";
import { useEvents } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import { useCluster } from "@/cluster/context";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";

function formatTime(iso: string | null | undefined): string {
  if (!iso) return "-";
  return new Date(iso).toLocaleString();
}

const SINCE_OPTIONS = [
  { value: 15, labelKey: "events.filter.since.15m" },
  { value: 30, labelKey: "events.filter.since.30m" },
  { value: 60, labelKey: "events.filter.since.1h" },
  { value: 360, labelKey: "events.filter.since.6h" },
  { value: 1440, labelKey: "events.filter.since.1d" },
] as const;

export default function EventsPage() {
  const { t } = useI18n();
  const { cluster } = useCluster();
  const [namespace, setNamespace] = useState("");
  const [name, setName] = useState("");
  const [since, setSince] = useState<number>(60);

  const { data: events, error, isLoading } = useEvents({
    namespace: namespace.trim() || undefined,
    name: name.trim() || undefined,
    since,
    cluster,
  });

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t("events.title")}</h1>
      </div>

      {/* Filter bar */}
      <div className="mb-6 flex flex-wrap gap-4">
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">
            {t("events.filter.namespace")}
          </label>
          <input
            type="text"
            value={namespace}
            onChange={(e) => setNamespace(e.target.value)}
            placeholder={t("events.filter.namespace")}
            className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:outline-none dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:placeholder-gray-500"
          />
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">
            {t("events.filter.name")}
          </label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("events.filter.name")}
            className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:outline-none dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100 dark:placeholder-gray-500"
          />
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wide">
            {t("events.filter.since")}
          </label>
          <select
            value={since}
            onChange={(e) => setSince(Number(e.target.value))}
            className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-900 focus:border-blue-500 focus:outline-none dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100"
          >
            {SINCE_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {t(opt.labelKey)}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* Loading / error / empty states */}
      {isLoading && (
        <p className="text-gray-500 dark:text-gray-400">{t("events.loading")}</p>
      )}
      {error && (
        <p className="text-red-600 dark:text-red-400">{t("common.loadFailed")}</p>
      )}
      {!isLoading && !error && events && events.length === 0 && (
        <p className="text-gray-500 dark:text-gray-400">{t("events.empty")}</p>
      )}

      {/* Events table */}
      {!isLoading && !error && events && events.length > 0 && (
        <div className="rounded-lg border bg-white dark:border-gray-800 dark:bg-gray-900 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-40">{t("events.col.time")}</TableHead>
                <TableHead className="w-32">{t("events.col.namespace")}</TableHead>
                <TableHead>{t("events.col.resource")}</TableHead>
                <TableHead className="w-32">{t("events.col.reason")}</TableHead>
                <TableHead>{t("events.col.message")}</TableHead>
                <TableHead className="w-16 text-right">{t("events.col.count")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {events.map((ev) => (
                <TableRow key={ev.ID}>
                  <TableCell className="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">
                    {formatTime(ev.LastTime)}
                  </TableCell>
                  <TableCell className="text-sm">{ev.Namespace}</TableCell>
                  <TableCell className="text-sm">
                    <span className="font-medium">{ev.Kind}</span>
                    <span className="text-gray-400 dark:text-gray-500">/</span>
                    {ev.Name}
                  </TableCell>
                  <TableCell className="text-sm">
                    {ev.Type === "Warning" ? (
                      <span className="inline-flex items-center rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700 dark:bg-red-900/30 dark:text-red-400">
                        {ev.Reason}
                      </span>
                    ) : (
                      <span>{ev.Reason}</span>
                    )}
                  </TableCell>
                  <TableCell className="text-sm text-gray-600 dark:text-gray-400 max-w-xs truncate" title={ev.Message}>
                    {ev.Message}
                  </TableCell>
                  <TableCell className="text-sm text-right">{ev.Count}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
