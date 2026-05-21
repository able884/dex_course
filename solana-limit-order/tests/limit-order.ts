import * as dotenv from "dotenv";
dotenv.config();

import * as anchor from "@coral-xyz/anchor";
import { Program } from "@coral-xyz/anchor";
import { LimitOrder } from "../target/types/limit_order";
import { PublicKey, Keypair, SystemProgram, SYSVAR_RENT_PUBKEY } from "@solana/web3.js";
import { TOKEN_PROGRAM_ID, createMint, createAccount, mintTo, getAccount } from "@solana/spl-token";
import { assert } from "chai";
import { keccak_256 } from "js-sha3";
import { setupSolanaProxy } from "../scripts/utils/proxy-config";

// Setup proxy before tests run
before(function() {
  setupSolanaProxy();
});

describe("limit-order", () => {
  const provider = anchor.AnchorProvider.env();
  anchor.setProvider(provider);

  const program = anchor.workspace.LimitOrder as Program<LimitOrder>;
  const payer = provider.wallet as anchor.Wallet;

  let configPda: PublicKey;
  let marketPda: PublicKey;
  let baseMint: PublicKey;
  let quoteMint: PublicKey;
  let baseVaultPda: PublicKey;
  let quoteVaultPda: PublicKey;

  let admin = Keypair.generate();
  let treasury = Keypair.generate();
  let user1 = Keypair.generate();
  let user2 = Keypair.generate();

  let user1BaseToken: PublicKey;
  let user1QuoteToken: PublicKey;
  let user2BaseToken: PublicKey;
  let user2QuoteToken: PublicKey;

  before(async () => {
    // Airdrop SOL to test accounts
    const airdropAmount = 10 * anchor.web3.LAMPORTS_PER_SOL;
    await provider.connection.confirmTransaction(
      await provider.connection.requestAirdrop(admin.publicKey, airdropAmount)
    );
    await provider.connection.confirmTransaction(
      await provider.connection.requestAirdrop(user1.publicKey, airdropAmount)
    );
    await provider.connection.confirmTransaction(
      await provider.connection.requestAirdrop(user2.publicKey, airdropAmount)
    );

    // Create mints
    baseMint = await createMint(
      provider.connection,
      payer.payer,
      payer.publicKey,
      null,
      6
    );

    quoteMint = await createMint(
      provider.connection,
      payer.payer,
      payer.publicKey,
      null,
      6
    );

    // Create user token accounts
    user1BaseToken = await createAccount(
      provider.connection,
      payer.payer,
      baseMint,
      user1.publicKey
    );

    user1QuoteToken = await createAccount(
      provider.connection,
      payer.payer,
      quoteMint,
      user1.publicKey
    );

    user2BaseToken = await createAccount(
      provider.connection,
      payer.payer,
      baseMint,
      user2.publicKey
    );

    user2QuoteToken = await createAccount(
      provider.connection,
      payer.payer,
      quoteMint,
      user2.publicKey
    );

    // Mint tokens to users
    await mintTo(
      provider.connection,
      payer.payer,
      baseMint,
      user1BaseToken,
      payer.publicKey,
      1_000_000_000 // 1000 tokens
    );

    await mintTo(
      provider.connection,
      payer.payer,
      quoteMint,
      user1QuoteToken,
      payer.publicKey,
      10_000_000_000 // 10000 tokens
    );

    await mintTo(
      provider.connection,
      payer.payer,
      baseMint,
      user2BaseToken,
      payer.publicKey,
      1_000_000_000 // 1000 tokens
    );

    await mintTo(
      provider.connection,
      payer.payer,
      quoteMint,
      user2QuoteToken,
      payer.publicKey,
      10_000_000_000 // 10000 tokens
    );

    // Derive PDAs
    [configPda] = PublicKey.findProgramAddressSync(
      [Buffer.from("config")],
      program.programId
    );

    // Market PDA - Seeds: ["LimitOrderMarket", base_mint, quote_mint]
    [marketPda] = PublicKey.findProgramAddressSync(
      [Buffer.from("LimitOrderMarket"), baseMint.toBuffer(), quoteMint.toBuffer()],
      program.programId
    );

    // Vault PDAs - Seeds: ["MarketVault", market, mint]
    [baseVaultPda] = PublicKey.findProgramAddressSync(
      [Buffer.from("MarketVault"), marketPda.toBuffer(), baseMint.toBuffer()],
      program.programId
    );

    [quoteVaultPda] = PublicKey.findProgramAddressSync(
      [Buffer.from("MarketVault"), marketPda.toBuffer(), quoteMint.toBuffer()],
      program.programId
    );
  });

  it("Initializes program config", async () => {
    const whitelistRoot = Buffer.alloc(32); // All zeros = disabled

    await program.methods
      .initProgram({
        admin: admin.publicKey,
        treasury: treasury.publicKey,
        whitelistRoot: Array.from(whitelistRoot),
        whitelistVersion: new anchor.BN(0),
        pythConfidenceTolBps: 500, // 5%
        pythMaxStalenessSlots: new anchor.BN(100),
        feeBpsLimit: 5000, // 50% max
        matcher: PublicKey.default, // Open to everyone
      })
      .accounts({
        config: configPda,
        payer: payer.publicKey,
        systemProgram: SystemProgram.programId,
      })
      .rpc();

    const config = await program.account.programConfig.fetch(configPda);
    assert.ok(config.admin.equals(admin.publicKey));
    assert.ok(config.treasury.equals(treasury.publicKey));
    assert.equal(config.feeBpsLimit, 5000);
    assert.equal(config.paused, false);
  });

  it("Initializes market", async () => {
    await program.methods
      .initMarket({
        tickSize: new anchor.BN(100),
        minBaseLot: new anchor.BN(1_000_000), // 1 token min
        minQuoteLot: new anchor.BN(1_000_000),
        makerFeeBps: 0,
        takerFeeBps: 30, // 0.3%
        oracleType: { none: {} }, // No oracle
        pythPriceFeedId: Array.from(PublicKey.default.toBytes()), // 32 bytes array
      })
      .accounts({
        config: configPda,
        admin: admin.publicKey,
        market: marketPda,
        baseMint: baseMint,
        quoteMint: quoteMint,
        baseVault: baseVaultPda,
        quoteVault: quoteVaultPda,
        tokenProgram: TOKEN_PROGRAM_ID,
        systemProgram: SystemProgram.programId,
      })
      .signers([admin])
      .rpc();

    const market = await program.account.market.fetch(marketPda);
    assert.ok(market.baseMint.equals(baseMint));
    assert.ok(market.quoteMint.equals(quoteMint));
    assert.equal(market.takerFeeBps, 30);
    assert.equal(market.makerFeeBps, 0);
  });

  it("Creates user margin account", async () => {
    const [marginPda] = PublicKey.findProgramAddressSync(
      [Buffer.from("margin"), marketPda.toBuffer(), user1.publicKey.toBuffer()],
      program.programId
    );

    await program.methods
      .createMargin([]) // Empty whitelist proof (whitelist disabled)
      .accounts({
        config: configPda,
        market: marketPda,
        margin: marginPda,
        owner: user1.publicKey,
        systemProgram: SystemProgram.programId,
      })
      .signers([user1])
      .rpc();

    const margin = await program.account.userMargin.fetch(marginPda);
    assert.ok(margin.owner.equals(user1.publicKey));
    assert.ok(margin.market.equals(marketPda));
    assert.equal(margin.baseFree.toNumber(), 0);
    assert.equal(margin.quoteFree.toNumber(), 0);
  });

  it("Deposits base tokens", async () => {
    const [marginPda] = PublicKey.findProgramAddressSync(
      [Buffer.from("margin"), marketPda.toBuffer(), user1.publicKey.toBuffer()],
      program.programId
    );

    const depositAmount = 100_000_000; // 100 tokens

    await program.methods
      .deposit(new anchor.BN(depositAmount), { base: {} }, [])
      .accounts({
        config: configPda,
        market: marketPda,
        margin: marginPda,
        owner: user1.publicKey,
        baseMint: baseMint,
        quoteMint: quoteMint,
        userBaseToken: user1BaseToken,
        userQuoteToken: user1QuoteToken,
        baseVault: baseVaultPda,
        quoteVault: quoteVaultPda,
        tokenProgram: TOKEN_PROGRAM_ID,
      })
      .signers([user1])
      .rpc();

    const margin = await program.account.userMargin.fetch(marginPda);
    assert.equal(margin.baseFree.toNumber(), depositAmount);
  });

  it("Deposits quote tokens", async () => {
    const [marginPda] = PublicKey.findProgramAddressSync(
      [Buffer.from("margin"), marketPda.toBuffer(), user1.publicKey.toBuffer()],
      program.programId
    );

    const depositAmount = 1_000_000_000; // 1000 tokens

    await program.methods
      .deposit(new anchor.BN(depositAmount), { quote: {} }, [])
      .accounts({
        config: configPda,
        market: marketPda,
        margin: marginPda,
        owner: user1.publicKey,
        baseMint: baseMint,
        quoteMint: quoteMint,
        userBaseToken: user1BaseToken,
        userQuoteToken: user1QuoteToken,
        baseVault: baseVaultPda,
        quoteVault: quoteVaultPda,
        tokenProgram: TOKEN_PROGRAM_ID,
      })
      .signers([user1])
      .rpc();

    const margin = await program.account.userMargin.fetch(marginPda);
    assert.equal(margin.quoteFree.toNumber(), depositAmount);
  });

  it("Places a bid order", async () => {
    const [marginPda] = PublicKey.findProgramAddressSync(
      [Buffer.from("margin"), marketPda.toBuffer(), user1.publicKey.toBuffer()],
      program.programId
    );

    const market = await program.account.market.fetch(marketPda);
    const nextSeq = market.seqNum.toNumber() + 1;

    const [orderPda] = PublicKey.findProgramAddressSync(
      [
        Buffer.from("order"),
        marketPda.toBuffer(),
        user1.publicKey.toBuffer(),
        Buffer.from(new anchor.BN(nextSeq).toArrayLike(Buffer, "le", 8)),
      ],
      program.programId
    );

    const currentSlot = await provider.connection.getSlot();

    await program.methods
      .placeOrder(
        {
          side: { bid: {} },
          priceLots: new anchor.BN(10_000), // Price = 10
          qtyLots: new anchor.BN(5_000_000), // 5 tokens
          expirySlot: new anchor.BN(currentSlot + 1000),
          minFillBps: null,
          selfTradeBehavior: { decrementTake: {} },
        },
        []
      )
      .accounts({
        config: configPda,
        market: marketPda,
        margin: marginPda,
        order: orderPda,
        owner: user1.publicKey,
        systemProgram: SystemProgram.programId,
      })
      .signers([user1])
      .rpc();

    const order = await program.account.order.fetch(orderPda);
    assert.ok(order.owner.equals(user1.publicKey));
    assert.equal(order.priceLots.toNumber(), 10_000);
    assert.equal(order.qtyLots.toNumber(), 5_000_000);

    const margin = await program.account.userMargin.fetch(marginPda);
    assert.equal(margin.quoteLocked.toNumber(), 10_000 * 5_000_000);
  });

  it("Cancels an order", async () => {
    const [marginPda] = PublicKey.findProgramAddressSync(
      [Buffer.from("margin"), marketPda.toBuffer(), user1.publicKey.toBuffer()],
      program.programId
    );

    const market = await program.account.market.fetch(marketPda);
    const orderSeq = market.seqNum.toNumber();

    const [orderPda] = PublicKey.findProgramAddressSync(
      [
        Buffer.from("order"),
        marketPda.toBuffer(),
        user1.publicKey.toBuffer(),
        Buffer.from(new anchor.BN(orderSeq).toArrayLike(Buffer, "le", 8)),
      ],
      program.programId
    );

    const marginBefore = await program.account.userMargin.fetch(marginPda);

    await program.methods
      .cancelOrder()
      .accounts({
        config: configPda,
        market: marketPda,
        margin: marginPda,
        order: orderPda,
        owner: user1.publicKey,
      })
      .signers([user1])
      .rpc();

    const order = await program.account.order.fetch(orderPda);
    assert.deepEqual(order.status, { cancelled: {} });

    const marginAfter = await program.account.userMargin.fetch(marginPda);
    assert.equal(marginAfter.quoteLocked.toNumber(), 0);
    assert.ok(marginAfter.quoteFree.gt(marginBefore.quoteFree));
  });

  it("Withdraws tokens", async () => {
    const [marginPda] = PublicKey.findProgramAddressSync(
      [Buffer.from("margin"), marketPda.toBuffer(), user1.publicKey.toBuffer()],
      program.programId
    );

    const withdrawAmount = 50_000_000; // 50 base tokens

    const userTokenBefore = await getAccount(provider.connection, user1BaseToken);

    await program.methods
      .withdraw(new anchor.BN(withdrawAmount), { base: {} })
      .accounts({
        config: configPda,
        market: marketPda,
        margin: marginPda,
        owner: user1.publicKey,
        userBaseToken: user1BaseToken,
        userQuoteToken: user1QuoteToken,
        baseVault: baseVaultPda,
        quoteVault: quoteVaultPda,
        tokenProgram: TOKEN_PROGRAM_ID,
      })
      .signers([user1])
      .rpc();

    const userTokenAfter = await getAccount(provider.connection, user1BaseToken);
    const diff = Number(userTokenAfter.amount) - Number(userTokenBefore.amount);
    assert.equal(diff, withdrawAmount);
  });
});
