import { test, expect } from "@playwright/test";

test.describe("Diagnose Page", () => {
  test("loads and shows form elements", async ({ page }) => {
    await page.goto("/diagnose");
    // Title
    await expect(page.locator("h1")).toBeVisible();
    // Namespace selector
    await expect(page.locator("select").first()).toBeVisible();
    // Resource type radio buttons
    await expect(page.locator('input[type="radio"][value="Deployment"]')).toBeVisible();
    await expect(page.locator('input[type="radio"][value="Pod"]')).toBeVisible();
  });

  test("namespace dropdown populates from cluster", async ({ page }) => {
    await page.goto("/diagnose");
    // Wait for the namespace API to return
    await page.waitForResponse(
      (res) => res.url().includes("/api/k8s/resources") && res.url().includes("Namespace") && res.status() === 200,
      { timeout: 10_000 }
    );
    // Open namespace select and check options
    const select = page.locator("select").first();
    const options = select.locator("option");
    // Should have at least placeholder + some namespaces
    expect(await options.count()).toBeGreaterThan(1);
  });

  test("symptom checkboxes are interactive", async ({ page }) => {
    await page.goto("/diagnose");
    // Find symptom labels (they contain checkbox inputs with sr-only class)
    const symptoms = page.locator('label:has(input[type="checkbox"])');
    expect(await symptoms.count()).toBe(8);

    // Click first symptom
    await symptoms.first().click();
    // The label should now have the selected styling (border-blue-500)
    await expect(symptoms.first()).toHaveClass(/border-blue/);
  });

  test("submit button disabled without namespace and symptoms", async ({ page }) => {
    await page.goto("/diagnose");
    const submitBtn = page.locator('button:has-text("开始诊断"), button:has-text("Start")');
    await expect(submitBtn).toBeDisabled();
  });

  test("can select namespace and symptoms to enable submit", async ({ page }) => {
    await page.goto("/diagnose");
    // Wait for namespaces to load
    await page.waitForResponse(
      (res) => res.url().includes("/api/k8s/resources") && res.url().includes("Namespace"),
      { timeout: 10_000 }
    );

    // Select first real namespace (skip placeholder)
    const select = page.locator("select").first();
    const options = select.locator("option");
    const count = await options.count();
    if (count > 1) {
      const value = await options.nth(1).getAttribute("value");
      if (value) await select.selectOption(value);
    }

    // Click a symptom
    const symptoms = page.locator('label:has(input[type="checkbox"])');
    await symptoms.first().click();

    // Submit button should now be enabled
    const submitBtn = page.locator('button:has-text("开始诊断"), button:has-text("Start")');
    await expect(submitBtn).toBeEnabled();
  });
});
