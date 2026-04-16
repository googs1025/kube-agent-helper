"use client";

import { ReactNode } from "react";
import { I18nProvider } from "@/i18n/context";
import { ThemeProvider } from "@/theme/context";

export function ClientProviders({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider>
      <I18nProvider>{children}</I18nProvider>
    </ThemeProvider>
  );
}
