"use client";

import { useState } from "react";
import { useI18n } from "@/i18n/context";
import {
  useNotificationConfigs,
  createNotificationConfig,
  updateNotificationConfig,
  deleteNotificationConfig,
  testNotificationConfig,
} from "@/lib/api";
import type { NotificationConfig } from "@/lib/api";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

const EVENT_TYPES = [
  "run.completed",
  "run.failed",
  "finding.critical",
  "fix.applied",
  "fix.failed",
  "fix.approved",
  "fix.rejected",
];

const CHANNEL_TYPES = ["webhook", "slack", "dingtalk", "feishu"];

interface FormState {
  name: string;
  type: string;
  webhookURL: string;
  secret: string;
  events: string[];
  enabled: boolean;
}

const emptyForm: FormState = {
  name: "",
  type: "webhook",
  webhookURL: "",
  secret: "",
  events: [],
  enabled: true,
};

function ConfigDialog({
  initial,
  editId,
  onClose,
}: {
  initial: FormState;
  editId?: string;
  onClose: () => void;
}) {
  const { t } = useI18n();
  const [form, setForm] = useState<FormState>(initial);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const toggleEvent = (ev: string) => {
    setForm((f) => ({
      ...f,
      events: f.events.includes(ev)
        ? f.events.filter((e) => e !== ev)
        : [...f.events, ev],
    }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      const body = {
        name: form.name,
        type: form.type,
        webhookURL: form.webhookURL,
        secret: form.secret || undefined,
        events: form.events.join(","),
        enabled: form.enabled,
      };
      if (editId) {
        await updateNotificationConfig(editId, body);
      } else {
        await createNotificationConfig(body);
      }
      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally {
      setSubmitting(false);
    }
  };

  const inputClass =
    "w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-lg rounded-lg bg-card border border-border p-6 shadow-xl max-h-[90vh] overflow-y-auto"
      >
        <h2 className="mb-4 text-lg font-semibold">
          {editId ? t("notifications.edit.title") : t("notifications.create.title")}
        </h2>
        {error && <p className="mb-3 text-sm text-red-500">{error}</p>}

        <div className="space-y-3 mb-4">
          <div>
            <label className="block text-xs mb-1 text-muted-foreground">
              {t("notifications.form.name")}
            </label>
            <input
              className={inputClass}
              required
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>

          <div>
            <label className="block text-xs mb-1 text-muted-foreground">
              {t("notifications.form.type")}
            </label>
            <select
              className={inputClass}
              value={form.type}
              onChange={(e) => setForm({ ...form, type: e.target.value })}
            >
              {CHANNEL_TYPES.map((ct) => (
                <option key={ct} value={ct}>
                  {ct}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs mb-1 text-muted-foreground">
              {t("notifications.form.webhookURL")}
            </label>
            <input
              className={inputClass}
              required
              placeholder="https://hooks.slack.com/..."
              value={form.webhookURL}
              onChange={(e) => setForm({ ...form, webhookURL: e.target.value })}
            />
          </div>

          <div>
            <label className="block text-xs mb-1 text-muted-foreground">
              {t("notifications.form.secret")}
            </label>
            <input
              className={inputClass}
              placeholder={t("notifications.form.secretPlaceholder")}
              value={form.secret}
              onChange={(e) => setForm({ ...form, secret: e.target.value })}
            />
          </div>

          <div>
            <label className="block text-xs mb-1 text-muted-foreground">
              {t("notifications.form.events")}
            </label>
            <p className="text-xs text-muted-foreground mb-2">
              {t("notifications.form.eventsHint")}
            </p>
            <div className="flex flex-wrap gap-2">
              {EVENT_TYPES.map((ev) => (
                <label
                  key={ev}
                  className={`inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs cursor-pointer transition-colors ${
                    form.events.includes(ev)
                      ? "border-primary bg-primary/10 text-primary"
                      : "border-border text-muted-foreground hover:bg-muted"
                  }`}
                >
                  <input
                    type="checkbox"
                    className="sr-only"
                    checked={form.events.includes(ev)}
                    onChange={() => toggleEvent(ev)}
                  />
                  {ev}
                </label>
              ))}
            </div>
          </div>

          <div className="flex items-center gap-2">
            <label className="text-xs text-muted-foreground">
              {t("notifications.form.enabled")}
            </label>
            <button
              type="button"
              onClick={() => setForm({ ...form, enabled: !form.enabled })}
              className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                form.enabled ? "bg-primary" : "bg-muted"
              }`}
            >
              <span
                className={`inline-block size-3.5 rounded-full bg-white transition-transform ${
                  form.enabled ? "translate-x-4.5" : "translate-x-0.5"
                }`}
              />
            </button>
          </div>
        </div>

        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg px-4 py-1.5 text-sm text-muted-foreground hover:bg-muted transition-colors"
          >
            {t("common.cancel")}
          </button>
          <button
            type="submit"
            disabled={submitting || !form.name || !form.webhookURL}
            className="rounded-lg bg-primary px-4 py-1.5 text-sm font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-50"
          >
            {editId ? t("notifications.form.save") : t("notifications.form.create")}
          </button>
        </div>
      </form>
    </div>
  );
}

export default function NotificationsPage() {
  const { t } = useI18n();
  const { data: configs, isLoading, mutate } = useNotificationConfigs();
  const [showDialog, setShowDialog] = useState(false);
  const [editConfig, setEditConfig] = useState<NotificationConfig | null>(null);
  const [testingId, setTestingId] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{ id: string; ok: boolean; msg: string } | null>(null);

  const handleEdit = (cfg: NotificationConfig) => {
    setEditConfig(cfg);
    setShowDialog(true);
  };

  const handleDelete = async (id: string) => {
    if (!confirm(t("notifications.confirmDelete"))) return;
    try {
      await deleteNotificationConfig(id);
      mutate();
    } catch {
      // ignore
    }
  };

  const handleTest = async (id: string) => {
    setTestingId(id);
    setTestResult(null);
    try {
      await testNotificationConfig(id);
      setTestResult({ id, ok: true, msg: t("notifications.testSuccess") });
    } catch (err: unknown) {
      setTestResult({
        id,
        ok: false,
        msg: err instanceof Error ? err.message : "Failed",
      });
    } finally {
      setTestingId(null);
    }
  };

  const dialogInitial: FormState = editConfig
    ? {
        name: editConfig.Name,
        type: editConfig.Type,
        webhookURL: editConfig.WebhookURL,
        secret: editConfig.Secret,
        events: editConfig.Events ? editConfig.Events.split(",").filter(Boolean) : [],
        enabled: editConfig.Enabled,
      }
    : emptyForm;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t("notifications.title")}</h1>
        <button
          onClick={() => {
            setEditConfig(null);
            setShowDialog(true);
          }}
          className="rounded-lg bg-primary px-4 py-1.5 text-sm font-semibold text-primary-foreground hover:opacity-90"
        >
          + {t("notifications.create.title")}
        </button>
      </div>

      {isLoading && <p className="text-muted-foreground">{t("common.loading")}</p>}

      {!isLoading && (!configs || configs.length === 0) && (
        <p className="text-muted-foreground">{t("notifications.empty")}</p>
      )}

      {configs && configs.length > 0 && (
        <div className="rounded-lg border border-border bg-card overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("notifications.col.name")}</TableHead>
                <TableHead>{t("notifications.col.type")}</TableHead>
                <TableHead>{t("notifications.col.webhookURL")}</TableHead>
                <TableHead>{t("notifications.col.events")}</TableHead>
                <TableHead>{t("notifications.col.enabled")}</TableHead>
                <TableHead>{t("notifications.col.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {configs.map((cfg) => (
                <TableRow key={cfg.ID}>
                  <TableCell className="font-medium">{cfg.Name}</TableCell>
                  <TableCell>
                    <span className="inline-flex items-center rounded-md border border-border px-2 py-0.5 text-xs font-semibold">
                      {cfg.Type}
                    </span>
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground max-w-[200px] truncate">
                    {cfg.WebhookURL}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground max-w-[200px]">
                    {cfg.Events ? (
                      <div className="flex flex-wrap gap-1">
                        {cfg.Events.split(",").map((ev) => (
                          <span
                            key={ev}
                            className="rounded bg-muted px-1.5 py-0.5 text-[10px]"
                          >
                            {ev}
                          </span>
                        ))}
                      </div>
                    ) : (
                      <span className="italic">{t("notifications.allEvents")}</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <span
                      className={`inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-semibold ${
                        cfg.Enabled
                          ? "bg-green-500/10 text-green-500"
                          : "bg-slate-500/10 text-slate-400"
                      }`}
                    >
                      <span
                        className={`size-1.5 rounded-full ${
                          cfg.Enabled ? "bg-green-500" : "bg-slate-400"
                        }`}
                      />
                      {cfg.Enabled ? t("common.yes") : t("common.no")}
                    </span>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <button
                        onClick={() => handleTest(cfg.ID)}
                        disabled={testingId === cfg.ID}
                        className="rounded px-2 py-1 text-xs text-primary hover:bg-primary/10 transition-colors disabled:opacity-50"
                      >
                        {testingId === cfg.ID
                          ? t("notifications.testing")
                          : t("notifications.test")}
                      </button>
                      <button
                        onClick={() => handleEdit(cfg)}
                        className="rounded px-2 py-1 text-xs text-muted-foreground hover:bg-muted transition-colors"
                      >
                        {t("notifications.edit.button")}
                      </button>
                      <button
                        onClick={() => handleDelete(cfg.ID)}
                        className="rounded px-2 py-1 text-xs text-red-500 hover:bg-red-500/10 transition-colors"
                      >
                        {t("notifications.delete")}
                      </button>
                    </div>
                    {testResult && testResult.id === cfg.ID && (
                      <p
                        className={`mt-1 text-xs ${
                          testResult.ok ? "text-green-500" : "text-red-500"
                        }`}
                      >
                        {testResult.msg}
                      </p>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {showDialog && (
        <ConfigDialog
          initial={dialogInitial}
          editId={editConfig?.ID}
          onClose={() => {
            setShowDialog(false);
            setEditConfig(null);
            mutate();
          }}
        />
      )}
    </div>
  );
}
