import api from './client';

const normalizePool = (it = {}, fallbackVersion) => {
  const userPosition = it.userPosition || it.user_position;
  const price =
    it.price ??
    it.quotePerBase ??
    it.quote_per_base ??
    it.marketPrice ??
    it.market_price;

  return {
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
    poolVersion: it.poolVersion ?? it.pool_version ?? fallbackVersion,
    lockedPercent: it.lockedPercent ?? it.lock_percent,
    inputAmount: it.inputAmount ?? it.input_amount,
    outputAmount: it.outputAmount ?? it.output_amount,
    baseReserve: it.baseReserve ?? it.base_reserve,
    quoteReserve: it.quoteReserve ?? it.quote_reserve,
    inputReserve: it.inputReserve ?? it.input_reserve,
    outputReserve: it.outputReserve ?? it.output_reserve,
    price,
    quotePerBase: it.quotePerBase ?? it.quote_per_base ?? price,
    marketPrice: it.marketPrice ?? it.market_price ?? price,
    // user-specific fields (populated when wallet address is provided)
    userPosition,
    userPooledInput:
      it.userPooledInput ??
      it.user_pooled_input ??
      it.userInputAmount ??
      it.user_input_amount ??
      it.inputAmountUser ??
      it.input_amount_user ??
      it.baseAmountUser ??
      it.base_amount_user ??
      userPosition?.userPooledInput ??
      userPosition?.user_pooled_input ??
      userPosition?.inputAmount ??
      userPosition?.input_amount ??
      userPosition?.baseAmount ??
      userPosition?.base_amount,
    userPooledOutput:
      it.userPooledOutput ??
      it.user_pooled_output ??
      it.userOutputAmount ??
      it.user_output_amount ??
      it.outputAmountUser ??
      it.output_amount_user ??
      it.quoteAmountUser ??
      it.quote_amount_user ??
      userPosition?.userPooledOutput ??
      userPosition?.user_pooled_output ??
      userPosition?.outputAmount ??
      userPosition?.output_amount ??
      userPosition?.quoteAmount ??
      userPosition?.quote_amount,
    userStakedLp:
      it.userStakedLp ??
      it.user_staked_lp ??
      it.stakedLp ??
      it.staked_lp ??
      it.stakeLp ??
      it.stake_lp ??
      it.stakeLpAmount ??
      it.stake_lp_amount ??
      userPosition?.userStakedLp ??
      userPosition?.user_staked_lp ??
      userPosition?.stakedLp ??
      userPosition?.staked_lp ??
      userPosition?.stakeLp ??
      userPosition?.stake_lp ??
      userPosition?.stakeLpAmount ??
      userPosition?.stake_lp_amount,
    userUnstakedLp:
      it.userUnstakedLp ??
      it.user_unstaked_lp ??
      it.unstakedLp ??
      it.unstaked_lp ??
      it.lpBalance ??
      it.lp_balance ??
      it.unstakeLp ??
      it.unstake_lp ??
      it.lpAmount ??
      it.lp_amount ??
      userPosition?.userUnstakedLp ??
      userPosition?.user_unstaked_lp ??
      userPosition?.unstakedLp ??
      userPosition?.unstaked_lp ??
      userPosition?.lpBalance ??
      userPosition?.lp_balance ??
      userPosition?.unstakeLp ??
      userPosition?.unstake_lp ??
      userPosition?.lpAmount ??
      userPosition?.lp_amount,
    // Historical price ranges
    priceRange24hMin: it.priceRange24hMin ?? it.price_range_24h_min,
    priceRange24hMax: it.priceRange24hMax ?? it.price_range_24h_max,
    priceRange7dMin: it.priceRange7dMin ?? it.price_range_7d_min,
    priceRange7dMax: it.priceRange7dMax ?? it.price_range_7d_max,
    priceRange30dMin: it.priceRange30dMin ?? it.price_range_30d_min,
    priceRange30dMax: it.priceRange30dMax ?? it.price_range_30d_max,
  };
};

export async function getPools(version) {
  const isCpmm = version === 'V1' || version === 'CPMM';
  const isClmm = version === 'V2' || version === 'CLMM';
  if (!isCpmm && !isClmm) {
    throw new Error(`Invalid version: ${version}`);
  }

  const pool_version = isCpmm ? 3 : 1; // CPMM use v3, CLMM should send v1
  const params = {
    chain_id: 100000,
    pool_version,
    page_no: 1,
    page_size: 10,
  };

  const endpoint = isCpmm ? '/v1/market/index_cpmm' : '/v1/market/index_clmm';
  const res = await api.get(endpoint, { params });

  const payload = res?.data?.data || {};
  const list = Array.isArray(payload.list) ? payload.list : [];

  const items = list.map((item) => normalizePool(item, pool_version));

  return { items };
}

export async function getPoolDetail(poolState, poolVersion, extraParams = {}) {
  if (!poolState) {
    throw new Error('poolState is required to fetch pool detail');
  }

  const pool_version = poolVersion ?? extraParams.pool_version;
  const params = {
    chain_id: 100000,
    pool_state: poolState,
    ...(pool_version ? { pool_version } : {}),
    ...extraParams,
  };

  const res = await api.get('/v1/market/pool_detail', { params });
  const payload = res?.data?.data ?? res?.data ?? {};

  return normalizePool(payload, pool_version);
}

export async function buildCpmmAddLiquidityTx(params) {
  const res = await api.post('/v1/trade/add_cpmm_liquidity', params);
  return res?.data?.data || res?.data;
}

export async function buildCpmmRemoveLiquidityTx(params) {
  const res = await api.post('/v1/trade/remove_cpmm_liquidity', params);
  return res?.data?.data || res?.data;
}

export async function buildCpmmSwapTx(params) {
  const res = await api.post('/v1/trade/cpmm_swap', params);
  return res?.data?.data || res?.data;
}

export async function quoteCpmmSwap(params) {
  const res = await api.post('/v1/market/quote_cpmm', params);
  return res?.data?.data || res?.data;
}

// CLMM pool liquidity APIs
export async function buildClmmOpenPositionTx(params) {
  const res = await api.post('/v1/trade/open_position', params);
  return res?.data?.data || res?.data;
}

export async function getClmmPoolDepthData(poolState) {
  if (!poolState) return null;
  
  const params = {
    chain_id: 100000,
    pool_state: poolState,
  };
  
  const res = await api.get('/v1/market/clmm_pool_depth', { params });
  const data = res?.data?.data || res?.data;
  
  // 转换数据格式以匹配前端期望的结构
  if (data?.line && Array.isArray(data.line)) {
    return {
      count: data.count || data.line.length,
      line: data.line.map((point) => ({
        price: Number(point.price) || 0,
        liquidity: Number(point.liquidity) || 0, // 前端期望数字类型
        tick: point.tick || 0,
      })),
    };
  }
  
  return data;
}

export async function getClmmPoolPriceRange(poolState, timeRange = '24H') {
  // TODO: Implement backend API endpoint
  // Placeholder: Returns mock price range data
  // When backend is ready, replace with actual API call
  
  if (!poolState) return null;
  
  const currentPrice = 1.0; // This should come from pool data
  const multipliers = {
    '24H': { min: 0.8, max: 1.2 },
    '7D': { min: 0.5, max: 2.0 },
    '30D': { min: 0.3, max: 3.0 },
  };
  
  const multiplier = multipliers[timeRange] || multipliers['24H'];
  
  return {
    min: currentPrice * multiplier.min,
    max: currentPrice * multiplier.max,
    current: currentPrice,
  };
}

// no default export to keep tree-shaking and linting happy
