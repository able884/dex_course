package cpmm

import (
	ag_solanago "github.com/gagliardetto/solana-go"
	"richcode.cc/dex/pkg/raydium/cpmm/idl/generated/raydium_cp_swap"
)

// InitializePoolPara contains all parameters and accounts required to create a CPMM pool.
type InitializePoolPara struct {
	// Parameters:
	InitAmount0 uint64 // Initial deposit amount for token0
	InitAmount1 uint64 // Initial deposit amount for token1
	OpenTime    uint64 // Timestamp when swaps become available

	// Accounts:
	Creator                ag_solanago.PublicKey // Wallet paying to create the pool
	AmmConfig              ag_solanago.PublicKey // AMM config account
	Authority              ag_solanago.PublicKey // Vault + LP mint authority PDA
	PoolState              ag_solanago.PublicKey // Pool state PDA
	Token0Mint             ag_solanago.PublicKey // Token0 mint address
	Token1Mint             ag_solanago.PublicKey // Token1 mint address
	LpMint                 ag_solanago.PublicKey // LP mint PDA
	CreatorToken0          ag_solanago.PublicKey // Creator's token0 ATA
	CreatorToken1          ag_solanago.PublicKey // Creator's token1 ATA
	CreatorLpToken         ag_solanago.PublicKey // Creator's LP token ATA
	Token0Vault            ag_solanago.PublicKey // Token0 vault PDA
	Token1Vault            ag_solanago.PublicKey // Token1 vault PDA
	CreatePoolFee          ag_solanago.PublicKey // Create-pool fee account
	ObservationState       ag_solanago.PublicKey // Observation/oracle PDA
	TokenProgram           ag_solanago.PublicKey // SPL token program
	Token0Program          ag_solanago.PublicKey // Token0 program (SPL or Token2022)
	Token1Program          ag_solanago.PublicKey // Token1 program (SPL or Token2022)
	AssociatedTokenProgram ag_solanago.PublicKey // Associated token account program
	SystemProgram          ag_solanago.PublicKey // System program
	Rent                   ag_solanago.PublicKey // Rent sysvar
}

// NewInitializeInstruction builds a Raydium CPMM initialize instruction.
func NewInitializeInstruction(para *InitializePoolPara) (ag_solanago.Instruction, error) {
	ins := raydium_cp_swap.NewInitializeInstructionBuilder().
		SetInitAmount0(para.InitAmount0).
		SetInitAmount1(para.InitAmount1).
		SetOpenTime(para.OpenTime).
		SetCreatorAccount(para.Creator).
		SetAmmConfigAccount(para.AmmConfig).
		SetAuthorityAccount(para.Authority).
		SetPoolStateAccount(para.PoolState).
		SetToken0MintAccount(para.Token0Mint).
		SetToken1MintAccount(para.Token1Mint).
		SetLpMintAccount(para.LpMint).
		SetCreatorToken0Account(para.CreatorToken0).
		SetCreatorToken1Account(para.CreatorToken1).
		SetCreatorLpTokenAccount(para.CreatorLpToken).
		SetToken0VaultAccount(para.Token0Vault).
		SetToken1VaultAccount(para.Token1Vault).
		SetCreatePoolFeeAccount(para.CreatePoolFee).
		SetObservationStateAccount(para.ObservationState).
		SetTokenProgramAccount(para.TokenProgram).
		SetToken0ProgramAccount(para.Token0Program).
		SetToken1ProgramAccount(para.Token1Program).
		SetAssociatedTokenProgramAccount(para.AssociatedTokenProgram).
		SetSystemProgramAccount(para.SystemProgram).
		SetRentAccount(para.Rent)

	return ins.ValidateAndBuild()
}

// DeriveCpmmPoolPDAs derives every PDA required by the Raydium CPMM program.
func DeriveCpmmPoolPDAs(
	ammConfig ag_solanago.PublicKey,
	token0Mint ag_solanago.PublicKey,
	token1Mint ag_solanago.PublicKey,
	programID ag_solanago.PublicKey,
) (
	poolState ag_solanago.PublicKey,
	lpMint ag_solanago.PublicKey,
	authority ag_solanago.PublicKey,
	token0Vault ag_solanago.PublicKey,
	token1Vault ag_solanago.PublicKey,
	observationState ag_solanago.PublicKey,
	err error,
) {
	// Raydium requires token0 < token1 lexicographically.
	if token0Mint.String() > token1Mint.String() {
		token0Mint, token1Mint = token1Mint, token0Mint
	}

	// 1. Pool state PDA: ["pool", amm_config, token0_mint, token1_mint]
	poolStateSeeds := [][]byte{
		[]byte("pool"),
		ammConfig[:],
		token0Mint[:],
		token1Mint[:],
	}
	poolState, _, err = ag_solanago.FindProgramAddress(poolStateSeeds, programID)
	if err != nil {
		return
	}

	// 2. LP mint PDA: ["pool_lp_mint", pool_state]
	lpMintSeeds := [][]byte{
		[]byte("pool_lp_mint"),
		poolState[:],
	}
	lpMint, _, err = ag_solanago.FindProgramAddress(lpMintSeeds, programID)
	if err != nil {
		return
	}

	// 3. Vault/LP authority PDA: ["vault_and_lp_mint_auth_seed"]
	authoritySeeds := [][]byte{
		[]byte("vault_and_lp_mint_auth_seed"),
	}
	authority, _, err = ag_solanago.FindProgramAddress(authoritySeeds, programID)
	if err != nil {
		return
	}

	// 4. Token0 vault PDA: ["pool_vault", pool_state, token0_mint]
	token0VaultSeeds := [][]byte{
		[]byte("pool_vault"),
		poolState[:],
		token0Mint[:],
	}
	token0Vault, _, err = ag_solanago.FindProgramAddress(token0VaultSeeds, programID)
	if err != nil {
		return
	}

	// 5. Token1 vault PDA: ["pool_vault", pool_state, token1_mint]
	token1VaultSeeds := [][]byte{
		[]byte("pool_vault"),
		poolState[:],
		token1Mint[:],
	}
	token1Vault, _, err = ag_solanago.FindProgramAddress(token1VaultSeeds, programID)
	if err != nil {
		return
	}

	// 6. Observation PDA: ["observation", pool_state]
	observationStateSeeds := [][]byte{
		[]byte("observation"),
		poolState[:],
	}
	observationState, _, err = ag_solanago.FindProgramAddress(observationStateSeeds, programID)
	if err != nil {
		return
	}

	return poolState, lpMint, authority, token0Vault, token1Vault, observationState, nil
}
