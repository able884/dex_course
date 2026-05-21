const STORAGE_KEY = 'cpmm-liquidity-cache';
const MAX_AGE_MS = 60 * 1000;
const DEFAULT_POOL_TYPE = 'CPMM';

const normalizePoolType = (val) => {
  if (typeof val !== 'string') return DEFAULT_POOL_TYPE;
  const trimmed = val.trim();
  return trimmed ? trimmed.toUpperCase() : DEFAULT_POOL_TYPE;
};

const safeSession = () => {
  if (typeof window === 'undefined' || !window.sessionStorage) {
    return null;
  }
  return window.sessionStorage;
};

const readCache = () => {
  const store = safeSession();
  if (!store) return {};
  try {
    const raw = store.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch (err) {
    return {};
  }
};

const writeCache = (payload) => {
  const store = safeSession();
  if (!store) return;
  try {
    store.setItem(STORAGE_KEY, JSON.stringify(payload));
  } catch (err) {
    // ignore
  }
};

export const getCachedTokens = (poolType = DEFAULT_POOL_TYPE) => {
  const typeKey = normalizePoolType(poolType);
  const cache = readCache();
  if (!cache.tokens || !cache.tokensUpdatedAt) return null;
  const updatedAt = cache.tokensUpdatedAt[typeKey];
  const tokens = cache.tokens[typeKey];
  if (!updatedAt || !tokens) return null;
  if (Date.now() - updatedAt > MAX_AGE_MS) return null;
  return tokens;
};

export const getCachedFeeTiers = (poolType = DEFAULT_POOL_TYPE) => {
  const typeKey = normalizePoolType(poolType);
  const cache = readCache();
  if (!cache.feeTiers || !cache.feesUpdatedAt) return null;
  const updatedAt = cache.feesUpdatedAt[typeKey];
  const tiers = cache.feeTiers[typeKey];
  if (!updatedAt || !tiers) return null;
  if (Date.now() - updatedAt > MAX_AGE_MS) return null;
  return tiers;
};

export const setCachedTokens = (tokens, poolType = DEFAULT_POOL_TYPE) => {
  const typeKey = normalizePoolType(poolType);
  const cache = readCache();
  cache.tokens = cache.tokens || {};
  cache.tokensUpdatedAt = cache.tokensUpdatedAt || {};
  cache.tokens[typeKey] = tokens;
  cache.tokensUpdatedAt[typeKey] = Date.now();
  writeCache(cache);
};

export const setCachedFeeTiers = (tiers, poolType = DEFAULT_POOL_TYPE) => {
  const typeKey = normalizePoolType(poolType);
  const cache = readCache();
  cache.feeTiers = cache.feeTiers || {};
  cache.feesUpdatedAt = cache.feesUpdatedAt || {};
  cache.feeTiers[typeKey] = tiers;
  cache.feesUpdatedAt[typeKey] = Date.now();
  writeCache(cache);
};

export const clearLiquidityCache = () => {
  const store = safeSession();
  if (!store) return;
  store.removeItem(STORAGE_KEY);
};
