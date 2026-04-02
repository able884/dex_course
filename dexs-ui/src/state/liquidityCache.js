const STORAGE_KEY = 'cpmm-liquidity-cache';
const MAX_AGE_MS = 60 * 1000;

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

export const getCachedTokens = () => {
  const cache = readCache();
  if (!cache.tokens || !cache.tokensUpdatedAt) return null;
  if (Date.now() - cache.tokensUpdatedAt > MAX_AGE_MS) return null;
  return cache.tokens;
};

export const getCachedFeeTiers = () => {
  const cache = readCache();
  if (!cache.feeTiers || !cache.feesUpdatedAt) return null;
  if (Date.now() - cache.feesUpdatedAt > MAX_AGE_MS) return null;
  return cache.feeTiers;
};

export const setCachedTokens = (tokens) => {
  const cache = readCache();
  cache.tokens = tokens;
  cache.tokensUpdatedAt = Date.now();
  writeCache(cache);
};

export const setCachedFeeTiers = (tiers) => {
  const cache = readCache();
  cache.feeTiers = tiers;
  cache.feesUpdatedAt = Date.now();
  writeCache(cache);
};

export const clearLiquidityCache = () => {
  const store = safeSession();
  if (!store) return;
  store.removeItem(STORAGE_KEY);
};
