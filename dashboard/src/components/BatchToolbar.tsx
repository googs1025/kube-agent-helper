"use client";

import { useI18n } from "@/i18n/context";
import { Button } from "@/components/ui/button";

interface BatchAction {
  labelKey: string;
  variant?: "default" | "destructive" | "outline" | "secondary" | "ghost" | "link";
  onClick: () => void;
}

interface BatchToolbarProps {
  count: number;
  actions: BatchAction[];
  onClear: () => void;
}

export function BatchToolbar({ count, actions, onClear }: BatchToolbarProps) {
  const { t } = useI18n();

  if (count === 0) return null;

  return (
    <div className="sticky bottom-4 z-10 mx-auto w-fit animate-in slide-in-from-bottom-4 fade-in">
      <div className="flex items-center gap-3 rounded-lg border border-border bg-card px-4 py-2 shadow-lg">
        <span className="text-sm font-medium">
          {t("batch.selected").replace("{count}", String(count))}
        </span>
        <div className="h-4 w-px bg-border" />
        {actions.map((action) => (
          <Button
            key={action.labelKey}
            variant={action.variant ?? "default"}
            size="sm"
            onClick={action.onClick}
          >
            {t(action.labelKey)}
          </Button>
        ))}
        <Button variant="ghost" size="sm" onClick={onClear}>
          {t("filter.clear")}
        </Button>
      </div>
    </div>
  );
}
