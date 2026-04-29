import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

import { I18nProvider } from "@/i18n/context";
import { ModelConfigPicker } from "../model-config-picker";
import type { ModelConfig } from "@/lib/types";

// jsdom localStorage mock so I18nProvider's useEffect doesn't crash
function makeLocalStorageMock() {
  const store: Record<string, string> = {};
  return {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => { store[key] = value; }),
    removeItem: vi.fn((key: string) => { delete store[key]; }),
    clear: vi.fn(() => { for (const k of Object.keys(store)) delete store[k]; }),
  };
}

beforeEach(() => {
  vi.stubGlobal("localStorage", makeLocalStorageMock());
});

const configs: ModelConfig[] = [
  {
    name: "primary",
    namespace: "default",
    provider: "anthropic",
    model: "sonnet",
    secretRef: "p-secret",
    secretKey: "apiKey",
    apiKey: "****",
  },
  {
    name: "backup-1",
    namespace: "default",
    provider: "anthropic",
    model: "haiku",
    secretRef: "b1-secret",
    secretKey: "apiKey",
    apiKey: "****",
  },
  {
    name: "backup-2",
    namespace: "default",
    provider: "anthropic",
    model: "opus",
    secretRef: "b2-secret",
    secretKey: "apiKey",
    apiKey: "****",
  },
];

function renderPicker(props: Partial<React.ComponentProps<typeof ModelConfigPicker>> = {}) {
  const onChange = vi.fn();
  const ui = render(
    <I18nProvider>
      <ModelConfigPicker
        configs={configs}
        primary={props.primary ?? "primary"}
        fallbacks={props.fallbacks ?? []}
        onChange={props.onChange ?? onChange}
      />
    </I18nProvider>
  );
  return { ...ui, onChange };
}

describe("ModelConfigPicker", () => {
  it("renders all configs in primary dropdown", () => {
    renderPicker({ primary: "primary" });
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    const options = Array.from(select.querySelectorAll("option")).map((o) => o.value);
    expect(options).toEqual(["primary", "backup-1", "backup-2"]);
  });

  it("changing primary fires onChange and removes that name from fallbacks", () => {
    const onChange = vi.fn();
    renderPicker({ primary: "primary", fallbacks: ["backup-1"], onChange });

    const select = screen.getByRole("combobox") as HTMLSelectElement;
    fireEvent.change(select, { target: { value: "backup-1" } });

    // backup-1 was selected as primary → should be removed from fallbacks
    expect(onChange).toHaveBeenCalledWith("backup-1", []);
  });

  it("clicking + add reveals candidate dropdown excluding primary and existing fallbacks", () => {
    const onChange = vi.fn();
    renderPicker({ primary: "primary", fallbacks: ["backup-1"], onChange });

    fireEvent.click(screen.getByRole("button", { name: /Add fallback|添加备选/ }));

    const selects = screen.getAllByRole("combobox") as HTMLSelectElement[];
    const adder = selects[selects.length - 1];
    const options = Array.from(adder.querySelectorAll("option"))
      .map((o) => o.value)
      .filter((v) => v !== "");
    expect(options).toEqual(["backup-2"]);
  });

  it("selecting from add dropdown appends to fallbacks", () => {
    const onChange = vi.fn();
    renderPicker({ primary: "primary", fallbacks: [], onChange });

    fireEvent.click(screen.getByRole("button", { name: /Add fallback|添加备选/ }));
    const selects = screen.getAllByRole("combobox") as HTMLSelectElement[];
    fireEvent.change(selects[selects.length - 1], { target: { value: "backup-2" } });

    expect(onChange).toHaveBeenCalledWith("primary", ["backup-2"]);
  });

  it("removing a fallback chip fires onChange without that name", () => {
    const onChange = vi.fn();
    renderPicker({ primary: "primary", fallbacks: ["backup-1", "backup-2"], onChange });

    const removeButtons = screen.getAllByRole("button", { name: /Remove backup-1|移除 backup-1/ });
    fireEvent.click(removeButtons[0]);

    expect(onChange).toHaveBeenCalledWith("primary", ["backup-2"]);
  });

  it("move-up button on second fallback swaps order", () => {
    const onChange = vi.fn();
    renderPicker({ primary: "primary", fallbacks: ["backup-1", "backup-2"], onChange });

    const upButton = screen.getByRole("button", { name: /Move up backup-2|上移 backup-2/ });
    fireEvent.click(upButton);

    expect(onChange).toHaveBeenCalledWith("primary", ["backup-2", "backup-1"]);
  });

  it("first fallback has no move-up button", () => {
    renderPicker({ primary: "primary", fallbacks: ["backup-1"] });
    const upButtons = screen.queryAllByRole("button", { name: /Move up|上移/ });
    expect(upButtons).toHaveLength(0);
  });

  it("renders empty state when no configs", () => {
    render(
      <I18nProvider>
        <ModelConfigPicker
          configs={[]}
          primary=""
          fallbacks={[]}
          onChange={() => {}}
        />
      </I18nProvider>
    );
    expect(
      screen.getByText(/Not configured|未配置/)
    ).toBeInTheDocument();
  });

  it("namespace filter limits visible configs", () => {
    const mixed: ModelConfig[] = [
      ...configs,
      {
        name: "other-ns-config",
        namespace: "other",
        provider: "anthropic",
        model: "sonnet",
        secretRef: "x",
        secretKey: "apiKey",
        apiKey: "****",
      },
    ];
    render(
      <I18nProvider>
        <ModelConfigPicker
          configs={mixed}
          primary="primary"
          fallbacks={[]}
          onChange={() => {}}
          namespace="default"
        />
      </I18nProvider>
    );
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    const options = Array.from(select.querySelectorAll("option")).map((o) => o.value);
    expect(options).not.toContain("other-ns-config");
  });
});
