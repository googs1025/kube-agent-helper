import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { renderHook } from "@testing-library/react";
import { I18nProvider, useI18n } from "../context";

// Build a minimal in-memory localStorage mock that satisfies the Web Storage API
function makeLocalStorageMock() {
  let store: Record<string, string> = {};
  return {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => { store[key] = value; }),
    removeItem: vi.fn((key: string) => { delete store[key]; }),
    clear: vi.fn(() => { store = {}; }),
  };
}

// Wrapper helper so renderHook tests have the provider
function wrapper({ children }: { children: React.ReactNode }) {
  return <I18nProvider>{children}</I18nProvider>;
}

describe("I18nProvider", () => {
  let localStorageMock: ReturnType<typeof makeLocalStorageMock>;

  beforeEach(() => {
    localStorageMock = makeLocalStorageMock();
    vi.stubGlobal("localStorage", localStorageMock);
  });

  it("defaults to zh", () => {
    function TestComponent() {
      const { lang } = useI18n();
      return <div data-testid="lang">{lang}</div>;
    }

    render(
      <I18nProvider>
        <TestComponent />
      </I18nProvider>
    );

    expect(screen.getByTestId("lang").textContent).toBe("zh");
  });

  it("t() resolves nested keys", () => {
    const { result } = renderHook(() => useI18n(), { wrapper });

    // zh common.loading = "加载中..."
    expect(result.current.t("common.loading")).toBe("加载中...");
  });

  it("t() returns key for unknown keys", () => {
    const { result } = renderHook(() => useI18n(), { wrapper });

    expect(result.current.t("nonexistent.key")).toBe("nonexistent.key");
  });

  it("setLang switches language", () => {
    const { result } = renderHook(() => useI18n(), { wrapper });

    act(() => {
      result.current.setLang("en");
    });

    // After switching to en, common.loading should be the English string
    expect(result.current.lang).toBe("en");
    expect(result.current.t("common.loading")).toBe("Loading...");
  });

  it("setLang persists to localStorage", () => {
    const { result } = renderHook(() => useI18n(), { wrapper });

    act(() => {
      result.current.setLang("en");
    });

    expect(localStorageMock.setItem).toHaveBeenCalledWith("lang", "en");
  });
});
