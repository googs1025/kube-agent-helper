/**
 * Next.js 全量 API 代理。
 *
 * 路由匹配 /api/* 所有路径（[...proxy] 是 catch-all 段），把请求原样转发给
 * 后端 controller HTTP server（默认 http://localhost:8080，可由 API_URL 覆盖）。
 *
 * 关键能力：
 *   - 普通 JSON：原样透传 status + body
 *   - SSE 流（Content-Type: text/event-stream）：用 Response.body 直传
 *     ReadableStream，避免 Next.js 缓冲导致 LogViewer 收不到实时事件
 *
 * 为什么需要这层代理：
 *   1. 浏览器同源 — Dashboard 跑 :3000，controller 跑 :8080，跨域要 CORS
 *   2. Helm 部署时前端只暴露 :3000，后端可以仅 ClusterIP，安全
 *   3. 后端地址变化（端口 / 域名）只改 API_URL 环境变量
 */
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

  const contentType = res.headers.get("Content-Type") || "application/json";

  // SSE pass-through: stream the response body without buffering
  if (contentType.includes("text/event-stream")) {
    return new Response(res.body, {
      status: res.status,
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
        "Connection": "keep-alive",
        "X-Accel-Buffering": "no",
      },
    });
  }

  return new Response(res.body, {
    status: res.status,
    headers: { "Content-Type": contentType },
  });
}

export const GET = handler;
export const POST = handler;
export const PUT = handler;
export const PATCH = handler;
export const DELETE = handler;
