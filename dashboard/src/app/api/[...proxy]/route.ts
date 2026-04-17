import { NextRequest } from "next/server";

export const dynamic = "force-dynamic";

async function handler(req: NextRequest) {
  const apiURL = process.env.API_URL || "http://localhost:8080";
  const path = req.nextUrl.pathname;
  const search = req.nextUrl.search;
  const url = `${apiURL}${path}${search}`;

  const res = await fetch(url, {
    method: req.method,
    headers: { "Content-Type": "application/json" },
    body: req.method !== "GET" && req.method !== "HEAD" ? await req.text() : undefined,
  });

  return new Response(res.body, {
    status: res.status,
    headers: { "Content-Type": res.headers.get("Content-Type") || "application/json" },
  });
}

export const GET = handler;
export const POST = handler;
export const PATCH = handler;
