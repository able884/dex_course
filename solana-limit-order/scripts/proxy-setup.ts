/**
 * Proxy Setup Utility
 *
 * This module provides proxy configuration for Solana RPC requests.
 * It supports HTTP, HTTPS, and SOCKS5 proxies.
 *
 * Usage:
 * 1. Set HTTP_PROXY or HTTPS_PROXY environment variable
 * 2. Call setupProxyForSolana() before creating Connection
 *
 * Example .env:
 * HTTP_PROXY=http://127.0.0.1:7890
 * HTTPS_PROXY=http://127.0.0.1:7890
 */

import { HttpsProxyAgent } from "https-proxy-agent";

/**
 * Setup proxy for Solana web3.js Connection
 * This patches the global fetch to use the configured proxy
 *
 * @returns ProxyAgent if proxy is configured, null otherwise
 */
export function setupProxyForSolana() {
  const proxyUrl = process.env.HTTPS_PROXY || process.env.HTTP_PROXY;

  if (!proxyUrl) {
    console.log("ℹ️  No proxy configured");
    console.log("   Set HTTP_PROXY or HTTPS_PROXY in .env if you need proxy access");
    return null;
  }

  console.log(`🔒 Configuring proxy: ${proxyUrl}`);

  try {
    // Create proxy agent
    const agent = new HttpsProxyAgent(proxyUrl);

    // Patch global fetch to use proxy
    const originalFetch = global.fetch;

    global.fetch = function(url: any, options: any = {}) {
      // Add agent to fetch options for Node.js fetch
      const fetchOptions = {
        ...options,
        agent: agent,
      };

      return originalFetch(url, fetchOptions);
    } as any;

    console.log("✅ Proxy configured successfully");
    return agent;
  } catch (error: any) {
    console.error(`❌ Failed to setup proxy: ${error.message}`);
    console.error("   Please check your proxy URL format");
    console.error("   Expected format: http://host:port or https://host:port");
    throw error;
  }
}

/**
 * Test proxy connection
 * Makes a simple request to verify proxy is working
 */
export async function testProxyConnection(testUrl: string = "https://api.devnet.solana.com") {
  console.log(`\n🔍 Testing proxy connection to ${testUrl}...`);

  try {
    const response = await fetch(testUrl, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        jsonrpc: "2.0",
        id: 1,
        method: "getHealth",
      }),
    });

    if (response.ok) {
      const data = await response.json();
      console.log("✅ Proxy connection test successful");
      console.log(`   Response: ${JSON.stringify(data)}`);
      return true;
    } else {
      console.error(`❌ Proxy connection test failed: ${response.status} ${response.statusText}`);
      return false;
    }
  } catch (error: any) {
    console.error(`❌ Proxy connection test failed: ${error.message}`);
    return false;
  }
}

/**
 * Get proxy info for debugging
 */
export function getProxyInfo() {
  const httpProxy = process.env.HTTP_PROXY;
  const httpsProxy = process.env.HTTPS_PROXY;
  const noProxy = process.env.NO_PROXY;

  return {
    httpProxy: httpProxy || "Not set",
    httpsProxy: httpsProxy || "Not set",
    noProxy: noProxy || "Not set",
    isConfigured: !!(httpProxy || httpsProxy),
  };
}
