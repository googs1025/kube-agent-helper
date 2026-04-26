"use client";

import { useI18n } from "@/i18n/context";
import { Button } from "@/components/ui/button";

export interface FilterField {
  key: string;
  labelKey: string;
  type: "text" | "select";
  options?: { value: string; labelKey: string }[];
  placeholder?: string;
}

interface FilterBarProps {
  fields: FilterField[];
  values: Record<string, string>;
  onChange: (key: string, value: string) => void;
  onClear: () => void;
}

export function FilterBar({ fields, values, onChange, onClear }: FilterBarProps) {
  const { t } = useI18n();
  const hasActive = Object.values(values).some((v) => v !== "");

  return (
    <div className="mb-4 flex flex-wrap items-end gap-3">
      {fields.map((field) => (
        <div key={field.key} className="flex flex-col gap-1">
          <label className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {t(field.labelKey)}
          </label>
          {field.type === "select" ? (
            <select
              value={values[field.key] || ""}
              onChange={(e) => onChange(field.key, e.target.value)}
              className="rounded-lg border border-border bg-background px-3 py-1.5 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
            >
              <option value="">--</option>
              {field.options?.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {t(opt.labelKey)}
                </option>
              ))}
            </select>
          ) : (
            <input
              type="text"
              value={values[field.key] || ""}
              onChange={(e) => onChange(field.key, e.target.value)}
              placeholder={field.placeholder}
              className="rounded-lg border border-border bg-background px-3 py-1.5 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
            />
          )}
        </div>
      ))}
      {hasActive && (
        <Button variant="ghost" size="sm" onClick={onClear} className="text-muted-foreground">
          {t("filter.clear")}
        </Button>
      )}
    </div>
  );
}
