import { test, expect } from "@playwright/test";

test.describe("Navigation", () => {
  test("homepage loads and shows run list", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("h1")).toBeVisible();
    // Should show stat cards with run counts
    await expect(page.locator("text=全部").first()).toBeVisible();
  });

  test("nav links are present", async ({ page }) => {
    await page.goto("/");
    const nav = page.locator("nav");
    await expect(nav.locator('a[href="/diagnose"]')).toBeVisible();
    await expect(nav.locator('a[href="/skills"]')).toBeVisible();
    await expect(nav.locator('a[href="/fixes"]')).toBeVisible();
    await expect(nav.locator('a[href="/about"]')).toBeVisible();
  });

  test("skills page loads and shows 10 builtin skills", async ({ page }) => {
    await page.goto("/skills");
    await expect(page.locator("h1")).toBeVisible();
    // Wait for data to load
    await page.waitForResponse((res) =>
      res.url().includes("/api/skills") && res.status() === 200
    );
    // Should show at least 10 rows in the table (the builtin skills)
    const rows = page.locator("table tbody tr");
    await expect(rows).toHaveCount(10, { timeout: 10_000 });
  });

  test("fixes page loads", async ({ page }) => {
    await page.goto("/fixes");
    await expect(page.locator("h1")).toBeVisible();
  });

  test("about page loads and shows tool count", async ({ page }) => {
    await page.goto("/about");
    await expect(page.locator("h1")).toBeVisible();
    // Should mention MCP tools
    await expect(page.locator("text=MCP").first()).toBeVisible();
  });
});
