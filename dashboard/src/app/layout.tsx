import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: "Kube Agent Helper",
  description: "Kubernetes diagnostic dashboard",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-screen bg-gray-50">
        <nav className="border-b bg-white px-6 py-3">
          <div className="mx-auto flex max-w-7xl items-center gap-8">
            <Link href="/" className="text-lg font-semibold">Kube Agent Helper</Link>
            <div className="flex gap-6 text-sm">
              <Link href="/" className="text-gray-600 hover:text-gray-900">Runs</Link>
              <Link href="/skills" className="text-gray-600 hover:text-gray-900">Skills</Link>
              <Link href="/fixes" className="text-gray-600 hover:text-gray-900">Fixes</Link>
            </div>
          </div>
        </nav>
        <main className="mx-auto max-w-7xl px-6 py-8">{children}</main>
      </body>
    </html>
  );
}
