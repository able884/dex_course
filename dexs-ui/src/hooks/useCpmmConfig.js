import { useCallback, useEffect, useRef, useState } from 'react';
import { getCachedFeeTiers, getCachedTokens, setCachedFeeTiers, setCachedTokens } from '../state/liquidityCache';

const unwrapPayload = (payload) => {
  if (!payload) return null;
  if (payload.data && payload.data.data) {
    return payload.data.data;
  }
  if (payload.data) {
    return payload.data;
  }
  return payload;
};

const buildError = (error, fallbackMessage = 'Unexpected error') => {
  if (!error) return fallbackMessage;
  if (typeof error === 'string') return error;
  return error.message || fallbackMessage;
};

const formatErrorMessage = (error) => {
  const message = buildError(error);
  if (error && error.traceId) {
    return `${message} (trace: ${error.traceId})`;
  }
  return message;
};

const parseResponse = async (response) => {
  const raw = await response.json().catch(() => null);
  if (!raw) {
    const err = new Error(response.statusText);
    throw err;
  }
  const data = unwrapPayload(raw);
  const traceId = data?.traceId || raw?.data?.traceId || raw?.traceId;
  const code = raw?.code ?? (response.ok ? 10000 : 0);
  if (code !== 10000) {
    const err = new Error(data?.error || raw?.message || 'Request failed');
    err.traceId = traceId;
    throw err;
  }
  return { data, traceId };
};

export function useCpmmConfig({
  tokensEndpoint,
  feeEndpoint,
  fallbackTokens = [],
  fallbackFees = [],
}) {
  const mountedRef = useRef(true);
  const fallbackTokensRef = useRef(fallbackTokens);
  const fallbackFeesRef = useRef(fallbackFees);
  useEffect(() => {
    fallbackTokensRef.current = fallbackTokens;
  }, [fallbackTokens]);
  useEffect(() => {
    fallbackFeesRef.current = fallbackFees;
  }, [fallbackFees]);
  const cachedTokens = getCachedTokens();
  const cachedFees = getCachedFeeTiers();
  const [tokenState, setTokenState] = useState({
    items: cachedTokens && cachedTokens.length ? cachedTokens : fallbackTokens,
    loading: false,
    error: '',
  });
  const [feeState, setFeeState] = useState({
    items: cachedFees && cachedFees.length ? cachedFees : fallbackFees,
    loading: false,
    error: '',
  });

  useEffect(() => () => {
    mountedRef.current = false;
  }, []);

  const fetchTokens = useCallback(async () => {
    setTokenState((prev) => ({ ...prev, loading: true, error: '' }));
    try {
      const resp = await fetch(`${tokensEndpoint}&page_size=200`);
      const { data } = await parseResponse(resp);
      const list =
        Array.isArray(data?.tokens) && data.tokens.length
          ? data.tokens
          : fallbackTokensRef.current;
      if (!mountedRef.current) return;
      setCachedTokens(list);
      setTokenState({ items: list, loading: false, error: '' });
    } catch (err) {
      if (!mountedRef.current) return;
      setTokenState({
        items: fallbackTokensRef.current,
        loading: false,
        error: formatErrorMessage(err),
      });
    }
  }, [tokensEndpoint]);

  const fetchFees = useCallback(async () => {
    setFeeState((prev) => ({ ...prev, loading: true, error: '' }));
    try {
      const resp = await fetch(feeEndpoint);
      const { data } = await parseResponse(resp);
      const tiers =
        Array.isArray(data?.tiers) && data.tiers.length
          ? data.tiers
          : fallbackFeesRef.current;
      if (!mountedRef.current) return;
      setCachedFeeTiers(tiers);
      setFeeState({ items: tiers, loading: false, error: '' });
    } catch (err) {
      if (!mountedRef.current) return;
      setFeeState({
        items: fallbackFeesRef.current,
        loading: false,
        error: formatErrorMessage(err),
      });
    }
  }, [feeEndpoint]);

  useEffect(() => {
    fetchTokens();
    fetchFees();
  }, [fetchTokens, fetchFees]);

  return {
    tokens: tokenState.items,
    tokenLoading: tokenState.loading,
    tokenError: tokenState.error,
    refreshTokens: fetchTokens,
    feeTiers: feeState.items,
    feeLoading: feeState.loading,
    feeError: feeState.error,
    refreshFees: fetchFees,
  };
}
