import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    const apiURL = process.env.API_URL || "http://localhost:8080";
    return [
      {
        source: "/api/:path*",
        destination: `${apiURL}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
