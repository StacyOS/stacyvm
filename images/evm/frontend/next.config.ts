import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  /* config options here */
  logging: {
    fetches: {
      fullUrl: true,
    },
    browserToTerminal: false,
    incomingRequests: true,
  },
};

export default nextConfig;
