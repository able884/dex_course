/**
 * Shared Proxy Configuration Utility
 *
 * This module provides proxy configuration for all Solana scripts.
 * Import and call setupSolanaProxy() before creating any Connection.
 *
 * Usage:
 * ```typescript
 * import { setupSolanaProxy, createProxyConnection } from './utils/proxy-config';
 *
 * // Setup proxy first
 * const proxyAgent = setupSolanaProxy();
 *
 * // Create connection with proxy
 * const connection = createProxyConnection(rpcUrl, proxyAgent);
 * ```
 */

import { Connection, ConnectionConfig, SendOptions, TransactionSignature } from "@solana/web3.js";
import { ProxyAgent } from "undici";

/**
 * Setup proxy for Solana operations
 * Reads HTTP_PROXY or HTTPS_PROXY from environment
 *
 * @returns ProxyAgent if proxy is configured, null otherwise
 */
export function setupSolanaProxy(): ProxyAgent | null {
  const proxyUrl = process.env.HTTPS_PROXY || process.env.HTTP_PROXY;

  if (!proxyUrl) {
    console.log("ℹ️  No proxy configured");
    return null;
  }

  console.log(`🔒 Using proxy: ${proxyUrl}`);

  try {
    const proxyAgent = new ProxyAgent(proxyUrl);
    console.log("✅ Proxy configured successfully");
    return proxyAgent;
  } catch (error: any) {
    console.error(`❌ Failed to setup proxy: ${error.message}`);
    console.error("   Falling back to direct connection...");
    return null;
  }
}

/**
 * Create a Solana Connection with proxy support and polling-based confirmation
 *
 * @param rpcUrl - Solana RPC endpoint URL
 * @param proxyAgent - ProxyAgent from setupSolanaProxy()
 * @param config - Additional connection configuration
 * @returns Connection instance with proxy support
 */
export function createProxyConnection(
  rpcUrl: string,
  proxyAgent: ProxyAgent | null,
  config?: ConnectionConfig
): Connection {
  const connectionConfig: ConnectionConfig = {
    commitment: "confirmed",
    // Use polling instead of WebSocket for confirmation (WebSocket doesn't work well with proxies)
    wsEndpoint: undefined, // Disable WebSocket
    disableRetryOnRateLimit: false,
    ...config,
  };

  // Add custom fetch with proxy if proxy is configured
  if (proxyAgent) {
    connectionConfig.fetch = async (url: any, options: any) => {
      // Add timeout to prevent hanging
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 30000); // 30 second timeout

      try {
        return await fetch(url, {
          ...options,
          dispatcher: proxyAgent,
          signal: controller.signal,
        });
      } finally {
        clearTimeout(timeoutId);
      }
    };
  }

  return new Connection(rpcUrl, connectionConfig);
}

/**
 * Send and confirm transaction using polling (works better with proxies)
 *
 * @param connection - Solana Connection
 * @param rawTransaction - Signed transaction buffer
 * @param options - Send options
 * @returns Transaction signature
 */
export async function sendAndConfirmTransactionWithPolling(
  connection: Connection,
  rawTransaction: Buffer,
  options?: SendOptions
): Promise<TransactionSignature> {
  // Send transaction
  const signature = await connection.sendRawTransaction(rawTransaction, {
    skipPreflight: false,
    preflightCommitment: "confirmed",
    ...options,
  });

  console.log(`   Transaction sent: ${signature}`);
  console.log(`   Explorer: https://explorer.solana.com/tx/${signature}?cluster=devnet`);

  // Poll for confirmation instead of using WebSocket
  const startTime = Date.now();
  const timeout = 120000; // 120 seconds
  const pollInterval = 2000; // Poll every 2 seconds

  while (Date.now() - startTime < timeout) {
    try {
      const status = await connection.getSignatureStatus(signature);

      if (status?.value?.confirmationStatus === "confirmed" ||
          status?.value?.confirmationStatus === "finalized") {
        if (status.value.err) {
          throw new Error(`Transaction failed: ${JSON.stringify(status.value.err)}`);
        }
        console.log(`   ✅ Transaction confirmed in ${((Date.now() - startTime) / 1000).toFixed(1)}s`);
        return signature;
      }

      // Check if transaction failed
      if (status?.value?.err) {
        throw new Error(`Transaction failed: ${JSON.stringify(status.value.err)}`);
      }

      // Wait before next poll
      await new Promise(resolve => setTimeout(resolve, pollInterval));
    } catch (error: any) {
      // If it's a "not found" error, transaction might still be processing
      if (error.message?.includes("not found")) {
        await new Promise(resolve => setTimeout(resolve, pollInterval));
        continue;
      }
      throw error;
    }
  }

  // Timeout - but transaction might still succeed
  console.warn(`   ⚠️  Confirmation timeout after ${timeout / 1000}s`);
  console.warn(`   Transaction may still succeed. Check explorer:`);
  console.warn(`   https://explorer.solana.com/tx/${signature}?cluster=devnet`);

  return signature;
}

/**
 * Test connection to Solana RPC
 *
 * @param connection - Solana Connection instance
 * @returns true if connection successful, false otherwise
 */
export async function testSolanaConnection(connection: Connection): Promise<boolean> {
  console.log("\n🔍 Testing Solana RPC connection...");

  try {
    const version = await connection.getVersion();
    console.log(`✅ Connected to Solana ${version["solana-core"]}`);
    return true;
  } catch (error: any) {
    console.error(`❌ Connection test failed: ${error.message}`);
    return false;
  }
}

/**
 * Get proxy info for debugging
 */
export function getProxyInfo() {
  return {
    httpProxy: process.env.HTTP_PROXY || "Not set",
    httpsProxy: process.env.HTTPS_PROXY || "Not set",
    isConfigured: !!(process.env.HTTP_PROXY || process.env.HTTPS_PROXY),
  };
}
