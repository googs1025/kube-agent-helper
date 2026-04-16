"use client";

import { createContext, useCallback, useContext, useEffect, useState, ReactNode } from "react";

export type Theme = "dark" | "light";

interface ThemeCtx {
  theme: Theme;
  setTheme: (t: Theme) => void;
}

const Ctx = createContext<ThemeCtx | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>("dark");

  useEffect(() => {
    const stored = typeof window !== "undefined" ? localStorage.getItem("theme") : null;
    const initial: Theme = stored === "light" ? "light" : "dark";
    setThemeState(initial);
    applyClass(initial);
  }, []);

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t);
    if (typeof window !== "undefined") localStorage.setItem("theme", t);
    applyClass(t);
  }, []);

  return <Ctx.Provider value={{ theme, setTheme }}>{children}</Ctx.Provider>;
}

function applyClass(t: Theme) {
  if (typeof document === "undefined") return;
  const el = document.documentElement;
  if (t === "dark") el.classList.add("dark");
  else el.classList.remove("dark");
}

export function useTheme(): ThemeCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error("useTheme must be used inside ThemeProvider");
  return v;
}

/**
 * Returns the script body to inject synchronously in <head> before React
 * hydrates. Reads localStorage.theme (defaults to "dark") and sets
 * document.documentElement class so the first paint is correct.
 */
export const preHydrationScript = `
(function() {
  try {
    var t = localStorage.getItem("theme") || "dark";
    if (t === "dark") document.documentElement.classList.add("dark");
  } catch (e) {}
})();
`;
