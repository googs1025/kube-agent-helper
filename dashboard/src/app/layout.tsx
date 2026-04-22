"use client";

import Link from "next/link";
import "./globals.css";
import { ClientProviders } from "@/components/client-providers";
import { ErrorBoundary } from "@/components/error-boundary";
import { ThemeToggle } from "@/components/theme-toggle";
import { LanguageToggle } from "@/components/language-toggle";
import { useI18n } from "@/i18n/context";
import { preHydrationScript } from "@/theme/context";

function Nav() {
  const { t } = useI18n();
  return (
    <nav className="border-b bg-white px-6 py-3 dark:bg-gray-900 dark:border-gray-800">
      <div className="mx-auto flex max-w-7xl items-center gap-8">
        <Link href="/" className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          {t("nav.brand")}
        </Link>
        <div className="flex flex-1 gap-6 text-sm">
          <Link href="/diagnose" className="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">{t("nav.diagnose")}</Link>
          <Link href="/" className="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">{t("nav.runs")}</Link>
          <Link href="/skills" className="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">{t("nav.skills")}</Link>
          <Link href="/fixes" className="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">{t("nav.fixes")}</Link>
          <Link href="/events" className="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">{t("nav.events")}</Link>
          <Link href="/modelconfigs" className="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">{t("nav.modelconfigs")}</Link>
          <Link href="/about" className="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">{t("nav.about")}</Link>
        </div>
        <div className="flex items-center gap-1">
          <ThemeToggle />
          <LanguageToggle />
        </div>
      </div>
    </nav>
  );
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh" suppressHydrationWarning>
      <head>
        <title>Kube Agent Helper</title>
        <script dangerouslySetInnerHTML={{ __html: preHydrationScript }} />
      </head>
      <body className="min-h-screen bg-gray-50 dark:bg-gray-950">
        <ClientProviders>
          <Nav />
          <ErrorBoundary>
            <main className="mx-auto max-w-7xl px-6 py-8 text-gray-900 dark:text-gray-100">{children}</main>
          </ErrorBoundary>
        </ClientProviders>
      </body>
    </html>
  );
}
