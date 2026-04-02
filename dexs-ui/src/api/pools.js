import api from './client';

export async function getPools(version) {
  const isCpmm = version === 'V1' || version === 'CPMM';
  const isClmm = version === 'V2' || version === 'CLMM';
  if (!isCpmm && !isClmm) {
    throw new Error(`Invalid version: ${version}`);
  }

  const pool_version = isCpmm ? 3 : 2;
  const params = {
    chain_id: 100000,
    pool_version,
    page_no: 1,
    page_size: 50,
  };

  const endpoint = isCpmm ? '/v1/market/index_cpmm' : '/v1/market/index_clmm';
  const res = await api.get(endpoint, { params });

  const payload = res?.data?.data || {};
  const list = Array.isArray(payload.list) ? payload.list : [];

  const normalize = (it = {}) => ({
    ...it,
    poolState: it.poolState || it.pool_state,
    inputVaultMint: it.inputVaultMint || it.input_vault_mint,
    outputVaultMint: it.outputVaultMint || it.output_vault_mint,
    inputTokenSymbol: it.inputTokenSymbol || it.input_token_symbol,
    outputTokenSymbol: it.outputTokenSymbol || it.output_token_symbol,
    inputTokenIcon: it.inputTokenIcon || it.input_token_icon,
    outputTokenIcon: it.outputTokenIcon || it.output_token_icon,
    tradeFeeRate: it.tradeFeeRate ?? it.trade_fee_rate,
    launchTime: it.launchTime ?? it.launch_time,
    liquidityUsd: it.liquidityUsd ?? it.liquidity_usd,
    txs24h: it.txs24h ?? it.txs_24h,
    vol24h: it.vol24h ?? it.vol_24h,
    apr: it.apr ?? it.apr_24h ?? it.apr,
    poolVersion: it.poolVersion ?? it.pool_version ?? pool_version,
  });

  const items = list.map(normalize);

  return { items };
}

// no default export to keep tree-shaking and linting happy
