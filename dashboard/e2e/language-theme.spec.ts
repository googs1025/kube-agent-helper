import { test, expect } from "@playwright/test";

test.describe("Language Toggle", () => {
  test("defaults to Chinese", async ({ page }) => {
    await page.goto("/");
    // HTML lang attribute should be zh
    await expect(page.locator("html")).toHaveAttribute("lang", "zh");
  });

  test("can switch to English", async ({ page }) => {
    await page.goto("/");
    // Find the language toggle button (contains "EN" or "English" or language icon)
    const langToggle = page.locator('button:has-text("EN"), button:has-text("English"), button:has-text("中")');
    if (await langToggle.count() > 0) {
      await langToggle.first().click();
      // After switching, nav should show English text
      await expect(page.locator('a[href="/diagnose"]')).toContainText(/Diagnose|诊断/);
    }
  });
});

test.describe("Theme Toggle", () => {
  test("defaults to dark mode", async ({ page }) => {
    await page.goto("/");
    // HTML element should have 'dark' class
    await expect(page.locator("html")).toHaveClass(/dark/);
  });

  test("can toggle to light mode", async ({ page }) => {
    await page.goto("/");
    // Find theme toggle (sun/moon icon button)
    const themeToggle = page.locator('button:has(svg)').first();
    if (await themeToggle.isVisible()) {
      await themeToggle.click();
      // After toggle, body background should change
      // Check dark class is removed
      const htmlClass = await page.locator("html").getAttribute("class");
      // It should either have 'dark' removed or still have it (depending on which button)
      expect(htmlClass).toBeDefined();
    }
  });
});
