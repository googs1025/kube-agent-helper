"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import "./globals.css";
import { ClientProviders } from "@/components/client-providers";
import { ErrorBoundary } from "@/components/error-boundary";
import { ThemeToggle } from "@/components/theme-toggle";
import { LanguageToggle } from "@/components/language-toggle";
import { ClusterToggle } from "@/components/cluster-toggle";
import { useI18n } from "@/i18n/context";
import { preHydrationScript } from "@/theme/context";
import { ClusterProvider } from "@/cluster/context";

function Nav() {
  const { t } = useI18n();
  const pathname = usePathname();

  const links = [
    { href: "/", label: t("nav.runs") },
    { href: "/diagnose", label: t("nav.diagnose") },
    { href: "/skills", label: t("nav.skills") },
    { href: "/fixes", label: t("nav.fixes") },
    { href: "/events", label: t("nav.events") },
    { href: "/modelconfigs", label: t("nav.modelconfigs") },
    { href: "/clusters", label: t("nav.clusters") },
    { href: "/about", label: t("nav.about") },
  ];

  return (
    <nav className="border-b border-border bg-background px-6" style={{ height: "52px", display: "flex", alignItems: "center" }}>
      <div className="mx-auto flex max-w-7xl w-full items-center gap-8">
        <Link href="/" className="flex items-center gap-2 text-[15px] font-bold text-foreground">
          <span className="inline-block size-2 rounded-full bg-primary animate-pulse" />
          {t("nav.brand")}
        </Link>
        <div className="flex flex-1 gap-1 text-sm">
          {links.map((link) => {
            const isActive = link.href === "/" ? pathname === "/" : pathname.startsWith(link.href);
            return (
              <Link
                key={link.href}
                href={link.href}
                className={`rounded-md px-2.5 py-1.5 transition-colors ${
                  isActive
                    ? "bg-primary/10 text-primary font-medium"
                    : "text-muted-foreground hover:text-foreground hover:bg-muted"
                }`}
              >
                {link.label}
              </Link>
            );
          })}
        </div>
        <div className="flex items-center gap-1">
          <ClusterToggle />
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
      <body className="min-h-screen bg-background">
        <ClientProviders>
          <ClusterProvider>
            <Nav />
            <ErrorBoundary>
              <main className="mx-auto max-w-7xl px-6 py-6 text-foreground">{children}</main>
            </ErrorBoundary>
          </ClusterProvider>
        </ClientProviders>
      </body>
    </html>
  );
}
