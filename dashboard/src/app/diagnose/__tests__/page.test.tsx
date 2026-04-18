import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { I18nProvider } from "@/i18n/context";
import { SYMPTOM_PRESETS } from "@/lib/symptoms";
import DiagnosePage from "../page";

// Ensure localStorage is available in the jsdom environment
const localStorageMock = (() => {
  const store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => { store[key] = value; },
    removeItem: (key: string) => { delete store[key]; },
    clear: () => { Object.keys(store).forEach((k) => delete store[k]); },
  };
})();

Object.defineProperty(globalThis, "localStorage", {
  value: localStorageMock,
  writable: true,
});

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useParams: () => ({}),
}));

vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

vi.mock("@/lib/api", () => ({
  useK8sNamespaces: () => ({ data: [{ name: "default" }, { name: "kube-system" }] }),
  useK8sResources: () => ({ data: [{ name: "nginx", namespace: "default" }] }),
  useRuns: () => ({ data: [] }),
  createRun: vi.fn().mockResolvedValue("uuid-123"),
  getK8sResourceDetail: vi.fn().mockResolvedValue({}),
}));

vi.mock("@/components/phase-badge", () => ({
  PhaseBadge: ({ phase }: { phase: string }) => <span>{phase}</span>,
}));

function renderPage() {
  return render(
    <I18nProvider>
      <DiagnosePage />
    </I18nProvider>
  );
}

describe("DiagnosePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders title and form", () => {
    renderPage();
    // The i18n key "diagnose.title" should render something (or fall back to key)
    const heading = screen.getByRole("heading", { level: 1 });
    expect(heading).toBeDefined();

    // Namespace select should be present
    const selects = screen.getAllByRole("combobox");
    expect(selects.length).toBeGreaterThanOrEqual(1);
  });

  it("shows namespace options", () => {
    renderPage();
    expect(screen.getByText("default")).toBeDefined();
    expect(screen.getByText("kube-system")).toBeDefined();
  });

  it("shows symptom checkboxes", () => {
    renderPage();
    // All 8 symptom presets should appear — check by label_zh (default lang is zh)
    for (const preset of SYMPTOM_PRESETS) {
      expect(screen.getByText(preset.label_zh)).toBeDefined();
    }
    expect(SYMPTOM_PRESETS).toHaveLength(8);
  });

  it("submit button disabled without selection", () => {
    renderPage();
    // Button is disabled when namespace is empty or no symptoms selected
    const button = screen.getByRole("button");
    expect((button as HTMLButtonElement).disabled).toBe(true);
  });

  it("full-check clears other symptoms", () => {
    renderPage();
    // First select a non-full-check symptom
    const cpuLabel = screen.getByText("CPU 利用率高");
    fireEvent.click(cpuLabel);

    // Then click full-check
    const fullCheckLabel = screen.getByText("全面体检");
    fireEvent.click(fullCheckLabel);

    // full-check checkbox should be checked
    const checkboxes = document.querySelectorAll('input[type="checkbox"]');
    const fullCheckIdx = SYMPTOM_PRESETS.findIndex((p) => p.id === "full-check");
    const cpuIdx = SYMPTOM_PRESETS.findIndex((p) => p.id === "cpu-high");

    const fullCheckBox = checkboxes[fullCheckIdx] as HTMLInputElement;
    const cpuBox = checkboxes[cpuIdx] as HTMLInputElement;

    expect(fullCheckBox.checked).toBe(true);
    expect(cpuBox.checked).toBe(false);
  });
});
