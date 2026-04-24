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
          <label className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
            {t("events.filter.namespace")}
          </label>
          <input
            type="text"
            value={namespace}
            onChange={(e) => setNamespace(e.target.value)}
            placeholder={t("events.filter.namespace")}
            className="rounded-lg border border-border bg-background px-3 py-1.5 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
          />
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
            {t("events.filter.name")}
          </label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("events.filter.name")}
            className="rounded-lg border border-border bg-background px-3 py-1.5 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
          />
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
            {t("events.filter.since")}
          </label>
          <select
            value={since}
            onChange={(e) => setSince(Number(e.target.value))}
            className="rounded-lg border border-border bg-background px-3 py-1.5 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
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
        <p className="text-muted-foreground">{t("events.loading")}</p>
      )}
      {error && (
        <p className="text-destructive">{t("common.loadFailed")}</p>
      )}
      {!isLoading && !error && events && events.length === 0 && (
        <p className="text-muted-foreground">{t("events.empty")}</p>
      )}

      {/* Events table */}
      {!isLoading && !error && events && events.length > 0 && (
        <div className="rounded-lg border border-border bg-card overflow-hidden">
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
                  <TableCell className="text-xs text-muted-foreground whitespace-nowrap">
                    {formatTime(ev.LastTime)}
                  </TableCell>
                  <TableCell className="text-sm">{ev.Namespace}</TableCell>
                  <TableCell className="text-sm">
                    <span className="font-medium">{ev.Kind}</span>
                    <span className="text-muted-foreground">/</span>
                    {ev.Name}
                  </TableCell>
                  <TableCell className="text-sm">
                    {ev.Type === "Warning" ? (
                      <span className="inline-flex items-center gap-1 rounded-md border border-red-400/20 bg-red-500/10 px-2 py-0.5 text-xs font-semibold text-red-400">
                        <span className="size-1.5 rounded-full bg-red-400" />
                        {ev.Reason}
                      </span>
                    ) : (
                      <span>{ev.Reason}</span>
                    )}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground max-w-xs truncate" title={ev.Message}>
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
