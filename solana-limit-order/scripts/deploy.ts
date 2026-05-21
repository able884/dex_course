import * as dotenv from "dotenv";
dotenv.config();

import * as anchor from "@coral-xyz/anchor";
import { Program, AnchorProvider, Wallet } from "@coral-xyz/anchor";
import { LimitOrder } from "../target/types/limit_order";
import { PublicKey, Keypair, SystemProgram, Connection, clusterApiUrl, SYSVAR_RENT_PUBKEY } from "@solana/web3.js";
import { TOKEN_PROGRAM_ID } from "@solana/spl-token";
import { readFileSync, writeFileSync } from "fs";
import * as path from "path";
import { setupSolanaProxy, createProxyConnection, testSolanaConnection, sendAndConfirmTransactionWithPolling } from "./utils/proxy-config";

interface DeploymentConfig {
  admin: PublicKey;
  treasury: PublicKey;
  matcher?: PublicKey;
  pythConfidenceTolBps: number;
  pythMaxStalenessSlots: number;
  feeBpsLimit: number;
}

interface MarketConfig {
  baseMint: PublicKey;
  quoteMint: PublicKey;
  tickSize: number;
  minBaseLot: number;
  minQuoteLot: number;
  makerFeeBps: number;
  takerFeeBps: number;
  pythPriceFeed?: PublicKey;
}

async function initializeProgram(
  program: Program<LimitOrder>,
  connection: Connection,
  config: DeploymentConfig,
  admin: Keypair
): Promise<PublicKey> {
  const [configPda] = PublicKey.findProgramAddressSync(
    [Buffer.from("config")],
    program.programId
  );

  console.log("\n=== Program Config ===");
  console.log(`Config PDA: ${configPda.toBase58()}`);

  // Check if config already exists
  const configAccount = await connection.getAccountInfo(configPda);

  if (configAccount) {
    console.log("✅ Program config already exists, skipping initialization");
    return configPda;
  }

  console.log("Initializing program config...");
  const whitelistRoot = Buffer.alloc(32, 0); // Whitelist disabled

  try {
    // Build transaction
    const tx = await program.methods
      .initProgram({
        admin: config.admin,
        treasury: config.treasury,
        whitelistRoot: Array.from(whitelistRoot),
        whitelistVersion: new anchor.BN(0),
        pythConfidenceTolBps: config.pythConfidenceTolBps,
        pythMaxStalenessSlots: new anchor.BN(config.pythMaxStalenessSlots),
        feeBpsLimit: config.feeBpsLimit,
        matcher: config.matcher || PublicKey.default,
      })
      .accounts({
        payer: admin.publicKey,
      })
      .signers([admin])
      .transaction();

    // Get recent blockhash
    const { blockhash, lastValidBlockHeight } = await connection.getLatestBlockhash();
    tx.recentBlockhash = blockhash;
    tx.lastValidBlockHeight = lastValidBlockHeight;
    tx.feePayer = admin.publicKey;

    // Sign transaction
    tx.sign(admin);

    // Send and confirm with polling
    await sendAndConfirmTransactionWithPolling(connection, tx.serialize());

    console.log("✅ Program config initialized successfully");
  } catch (error: any) {
    console.error("❌ Failed to initialize program config:", error.message);
    throw error;
  }

  return configPda;
}

async function initializeMarket(
  program: Program<LimitOrder>,
  connection: Connection,
  configPda: PublicKey,
  marketConfig: MarketConfig,
  admin: Keypair
): Promise<{ marketPda: PublicKey; baseVault: PublicKey; quoteVault: PublicKey }> {
  // Market PDA - Seeds: ["LimitOrderMarket", base_mint, quote_mint]
  const [marketPda] = PublicKey.findProgramAddressSync(
    [
      Buffer.from("LimitOrderMarket"),
      marketConfig.baseMint.toBuffer(),
      marketConfig.quoteMint.toBuffer(),
    ],
    program.programId
  );

  // Vault PDAs - Seeds: ["MarketVault", market, mint]
  const [baseVaultPda] = PublicKey.findProgramAddressSync(
    [Buffer.from("MarketVault"), marketPda.toBuffer(), marketConfig.baseMint.toBuffer()],
    program.programId
  );

  const [quoteVaultPda] = PublicKey.findProgramAddressSync(
    [Buffer.from("MarketVault"), marketPda.toBuffer(), marketConfig.quoteMint.toBuffer()],
    program.programId
  );

  console.log("\n=== Market Initialization ===");
  console.log(`Base Mint: ${marketConfig.baseMint.toBase58()}`);
  console.log(`Quote Mint: ${marketConfig.quoteMint.toBase58()}`);
  console.log(`Market PDA: ${marketPda.toBase58()}`);
  console.log(`Base Vault: ${baseVaultPda.toBase58()}`);
  console.log(`Quote Vault: ${quoteVaultPda.toBase58()}`);

  // Check if market already exists
  let marketAccount = null;
  let baseVaultAccount = null;
  let quoteVaultAccount = null;

  try {
    marketAccount = await connection.getAccountInfo(marketPda);
    // Add delay to ensure vaults are synced
    await new Promise(resolve => setTimeout(resolve, 1000));
    baseVaultAccount = await connection.getAccountInfo(baseVaultPda);
    quoteVaultAccount = await connection.getAccountInfo(quoteVaultPda);
  } catch (error) {
    console.log("⚠️  Account info query failed, will attempt initialization");
  }

  // Initialize market if it doesn't exist
  if (marketAccount) {
    console.log("✅ Market already exists, skipping initialization");

    // Verify market data matches config
    try {
      const marketData = await program.account.market.fetch(marketPda);
      console.log(`   Market base_mint: ${marketData.baseMint.toBase58()}`);
      console.log(`   Market quote_mint: ${marketData.quoteMint.toBase58()}`);

      if (!marketData.baseMint.equals(marketConfig.baseMint) ||
          !marketData.quoteMint.equals(marketConfig.quoteMint)) {
        console.log("⚠️  WARNING: Market mints don't match config!");
        console.log("   This market was created with different mints.");
        console.log("   Vaults belong to the old market and cannot be reused.");
        console.log("\n   Options:");
        console.log("   1. Use the original mints in your config");
        console.log("   2. Create a new market with new mints (will have different addresses)");
      }
    } catch (error) {
      console.log("⚠️  Could not fetch market data");
    }
  } else {
    console.log("Initializing market...");
    try {
      // Determine oracle type based on pythPriceFeed
      const oracleType = marketConfig.pythPriceFeed && !marketConfig.pythPriceFeed.equals(PublicKey.default)
        ? { pyth: {} }
        : { none: {} };

      // Convert pythPriceFeed PublicKey to array of bytes
      const pythPriceFeedId = marketConfig.pythPriceFeed && !marketConfig.pythPriceFeed.equals(PublicKey.default)
        ? Array.from(marketConfig.pythPriceFeed.toBytes())
        : Array.from(PublicKey.default.toBytes());

      // Build transaction
      const tx = await program.methods
        .initMarket({
          tickSize: new anchor.BN(marketConfig.tickSize),
          minBaseLot: new anchor.BN(marketConfig.minBaseLot),
          minQuoteLot: new anchor.BN(marketConfig.minQuoteLot),
          makerFeeBps: marketConfig.makerFeeBps,
          takerFeeBps: marketConfig.takerFeeBps,
          oracleType: oracleType,
          pythPriceFeedId: pythPriceFeedId,
        })
        .accounts({
          admin: admin.publicKey,
          baseMint: marketConfig.baseMint,
          quoteMint: marketConfig.quoteMint,
        })
        .signers([admin])
        .transaction();

      // Get recent blockhash
      const { blockhash, lastValidBlockHeight } = await connection.getLatestBlockhash();
      tx.recentBlockhash = blockhash;
      tx.lastValidBlockHeight = lastValidBlockHeight;
      tx.feePayer = admin.publicKey;

      // Sign transaction
      tx.sign(admin);

      // Send and confirm with polling
      await sendAndConfirmTransactionWithPolling(connection, tx.serialize());

      console.log("✅ Market initialized successfully");
    } catch (error: any) {
      console.error("❌ Failed to initialize market:", error.message);
      throw error;
    }
  }

  // Try to fetch token accounts to verify vaults exist
  let vaultsVerified = false;
  try {
    const TOKEN_PROGRAM = new PublicKey("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA");
    const baseVaultInfo = await connection.getParsedAccountInfo(baseVaultPda);
    const quoteVaultInfo = await connection.getParsedAccountInfo(quoteVaultPda);

    if (baseVaultInfo.value && quoteVaultInfo.value) {
      console.log(`Base Vault exists: ✅ (owner: ${baseVaultInfo.value.owner.toBase58()})`);
      console.log(`Quote Vault exists: ✅ (owner: ${quoteVaultInfo.value.owner.toBase58()})`);
      vaultsVerified = true;
    }
  } catch (error) {
    console.log("Vault verification via getParsedAccountInfo failed, trying basic check...");
  }

  // Additional check: try to fetch as token accounts using program.account
  if (!vaultsVerified && marketAccount) {
    console.log("Checking vaults via direct RPC call...");
    try {
      const baseVaultData = await connection.getAccountInfo(baseVaultPda);
      const quoteVaultData = await connection.getAccountInfo(quoteVaultPda);

      if (baseVaultData && quoteVaultData) {
        // Check if they are token accounts (owner should be Token Program)
        const TOKEN_PROGRAM_ID_KEY = new PublicKey("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA");
        if (baseVaultData.owner.equals(TOKEN_PROGRAM_ID_KEY) &&
            quoteVaultData.owner.equals(TOKEN_PROGRAM_ID_KEY)) {
          console.log(`Base Vault exists: ✅ (verified as token account)`);
          console.log(`Quote Vault exists: ✅ (verified as token account)`);
          vaultsVerified = true;
          baseVaultAccount = baseVaultData;
          quoteVaultAccount = quoteVaultData;
        }
      }
    } catch (error) {
      console.log(`Direct RPC check failed: ${error}`);
    }
  }

  if (!vaultsVerified) {
    console.log(`Base Vault exists: ${baseVaultAccount ? "✅" : "❌"}`);
    console.log(`Quote Vault exists: ${quoteVaultAccount ? "✅" : "❌"}`);
  }

  // Initialize vaults if they don't exist
  if (vaultsVerified || (baseVaultAccount && quoteVaultAccount)) {
    console.log("✅ Vaults already exist, skipping initialization");
  } else if (!baseVaultAccount && !quoteVaultAccount) {
    console.log("⚠️  Vaults are created automatically with init_market instruction");
    console.log("⚠️  No separate vault initialization needed");
  } else {
    console.log("⚠️  Vaults in inconsistent state (one exists, one doesn't)");
    console.log("⚠️  This should not happen with the new init_market implementation");
  }

  return {
    marketPda,
    baseVault: baseVaultPda,
    quoteVault: quoteVaultPda,
  };
}

async function main() {
  console.log("=== Solana Limit Order Deployment ===\n");

  // Setup proxy first (before any network requests)
  const proxyAgent = setupSolanaProxy();

  // Read command line arguments
  const args = process.argv.slice(2);
  if (args.length < 1) {
    console.error(
      "Usage: ts-node deploy.ts <CONFIG_FILE> [--network mainnet|devnet|localnet]"
    );
    process.exit(1);
  }

  const configFile = args[0];
  const network = args[2] || "localnet";

  // Load configuration
  const rawConfig: {
    program: {
      admin: string;
      treasury: string;
      matcher?: string | null;
      pythConfidenceTolBps: number;
      pythMaxStalenessSlots: number;
      feeBpsLimit: number;
    };
    markets: {
      baseMint: string;
      quoteMint: string;
      tickSize: number;
      minBaseLot: number;
      minQuoteLot: number;
      makerFeeBps: number;
      takerFeeBps: number;
      pythPriceFeed?: string | null;
    }[];
  } = JSON.parse(readFileSync(configFile, "utf-8"));

  // Convert string addresses to PublicKey
  const deployConfig = {
    program: {
      admin: new PublicKey(rawConfig.program.admin),
      treasury: new PublicKey(rawConfig.program.treasury),
      matcher: rawConfig.program.matcher ? new PublicKey(rawConfig.program.matcher) : undefined,
      pythConfidenceTolBps: rawConfig.program.pythConfidenceTolBps,
      pythMaxStalenessSlots: rawConfig.program.pythMaxStalenessSlots,
      feeBpsLimit: rawConfig.program.feeBpsLimit,
    },
    markets: rawConfig.markets.map((m) => ({
      baseMint: new PublicKey(m.baseMint),
      quoteMint: new PublicKey(m.quoteMint),
      tickSize: m.tickSize,
      minBaseLot: m.minBaseLot,
      minQuoteLot: m.minQuoteLot,
      makerFeeBps: m.makerFeeBps,
      takerFeeBps: m.takerFeeBps,
      pythPriceFeed: m.pythPriceFeed ? new PublicKey(m.pythPriceFeed) : undefined,
    })),
  };

  // Setup RPC URL based on network
  let rpcUrl: string;
  if (network === "localnet") {
    rpcUrl = process.env.LOCALNET_RPC_URL || "http://127.0.0.1:8899";
  } else if (network === "devnet") {
    // Use custom RPC if provided, otherwise use default
    rpcUrl = process.env.DEVNET_RPC_URL || "https://api.devnet.solana.com";
  } else if (network === "mainnet") {
    rpcUrl = process.env.MAINNET_RPC_URL || clusterApiUrl("mainnet-beta");
  } else {
    throw new Error(`Unknown network: ${network}`);
  }

  console.log(`Using RPC URL: ${rpcUrl}`);

  // Load wallet
  const walletPath = path.join("config", "admin.json");
  const walletKeypair = Keypair.fromSecretKey(
    Buffer.from(JSON.parse(readFileSync(walletPath, "utf-8")))
  );

  // Create connection with proxy support
  const connection = createProxyConnection(rpcUrl, proxyAgent);

  // Test connection before proceeding
  const connectionWorks = await testSolanaConnection(connection);
  if (!connectionWorks) {
    console.error("\n❌ Failed to connect to Solana RPC!");
    console.error("   Please check:");
    console.error("   1. Your VPN/proxy is running");
    console.error("   2. Proxy URL is correct in .env");
    console.error("   3. RPC endpoint is accessible");
    throw new Error("Connection test failed");
  }

  const wallet = new Wallet(walletKeypair);
  const provider = new AnchorProvider(connection, wallet, {
    commitment: "confirmed",
    preflightCommitment: "confirmed",
  });
  anchor.setProvider(provider);

  // Load program
  const idl = JSON.parse(
    readFileSync("./target/idl/limit_order.json", "utf-8")
  );
  const program = new Program(idl, provider) as Program<LimitOrder>;
  const admin = walletKeypair;

  console.log("\n=== Deployment Info ===");
  console.log(`Network: ${network}`);
  console.log(`Program ID: ${program.programId.toBase58()}`);
  console.log(`Admin: ${admin.publicKey.toBase58()}`);

  // Initialize program
  const configPda = await initializeProgram(
    program,
    connection,
    deployConfig.program,
    admin
  );

  // Save deployment info
  const deployment = {
    network,
    programId: program.programId.toBase58(),
    configPda: configPda.toBase58(),
    admin: admin.publicKey.toBase58(),
    timestamp: new Date().toISOString(),
  };

  const deploymentFile = `deployment-${network}.json`;
  writeFileSync(deploymentFile, JSON.stringify(deployment, null, 2));

  console.log("\n=== Deployment Complete ===");
  console.log(`✅ Program ID: ${program.programId.toBase58()}`);
  console.log(`✅ Config PDA: ${configPda.toBase58()}`);
  console.log(`✅ Deployment info saved to: ${deploymentFile}`);
  console.log(`\nView on Explorer: https://explorer.solana.com/address/${program.programId.toBase58()}?cluster=${network}`);
  console.log("\nNext steps:");
  console.log("  1. npm run test-integration    - Run integration tests");
  console.log("  2. npm run setup-tokens        - Create test tokens if needed");
}

main().catch((error) => {
  console.error("Deployment failed:", error);
  process.exit(1);
});
