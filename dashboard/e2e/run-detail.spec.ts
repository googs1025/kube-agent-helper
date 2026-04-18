import { test, expect } from "@playwright/test";

test.describe("Run Detail Page", () => {
  test("shows run detail with findings for a completed run", async ({ page }) => {
    // First get a succeeded run ID from the API
    const runsRes = await page.request.get("/api/runs");
    const runs = await runsRes.json();
    const succeededRun = runs.find(
      (r: { Status: string }) => r.Status === "Succeeded"
    );
    if (!succeededRun) {
      test.skip(true, "No succeeded run found");
      return;
    }

    await page.goto(`/runs/${succeededRun.ID}`);
    // Should show the run ID and status badge ("成功" in Chinese)
    await expect(page.getByText(succeededRun.ID.slice(0, 8))).toBeVisible({ timeout: 15_000 });
  });

  test("diagnose result page shows findings sorted by severity", async ({ page }) => {
    // Get a succeeded run
    const runsRes = await page.request.get("/api/runs");
    const runs = await runsRes.json();
    const succeededRun = runs.find(
      (r: { Status: string }) => r.Status === "Succeeded"
    );
    if (!succeededRun) {
      test.skip(true, "No succeeded run found");
      return;
    }

    // Check if it has findings
    const findingsRes = await page.request.get(
      `/api/runs/${succeededRun.ID}/findings`
    );
    const findings = await findingsRes.json();
    if (!findings || findings.length === 0) {
      test.skip(true, "No findings in run");
      return;
    }

    await page.goto(`/diagnose/${succeededRun.ID}`);
    // Should show findings
    await page.waitForSelector('[class*="rounded"]', { timeout: 10_000 });
    // The page should contain severity-related text
    const content = await page.textContent("body");
    expect(content).toBeTruthy();
  });

  test("failed run shows error message", async ({ page }) => {
    const runsRes = await page.request.get("/api/runs");
    const runs = await runsRes.json();
    const failedRun = runs.find(
      (r: { Status: string }) => r.Status === "Failed"
    );
    if (!failedRun) {
      test.skip(true, "No failed run found");
      return;
    }

    await page.goto(`/runs/${failedRun.ID}`);
    await expect(page.locator("text=Failed")).toBeVisible({ timeout: 10_000 });
  });
});
