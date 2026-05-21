import React, { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, ArrowLeft, ArrowLeftRight, Info, Lock, Wallet2, RefreshCw } from 'lucide-react';
import { useConnection, useWallet } from '@solana/wallet-adapter-react';
import { useWalletModal } from '@solana/wallet-adapter-react-ui';
import { LAMPORTS_PER_SOL, PublicKey, Transaction } from '@solana/web3.js';

import TokenPairAvatar from './TokenPairAvatar';
import { Button } from '../UI/Button';
import { Input } from '../UI/input';
import { Badge } from '../UI/badge';
import { Card, CardContent, CardHeader, CardTitle } from '../UI/card';
import { formatCurrencyUSD, formatPercent } from '../../utils/format';
import { useToast } from '../../hooks/use-toast';
import { cn } from '../../lib/utils';
import { useTranslation } from '../../i18n/LanguageContext';
import { getPoolDetail, buildCpmmAddLiquidityTx, buildCpmmRemoveLiquidityTx } from '../../api/pools';

const normalizeFeePercent = (tradeFeeRate) => {
  const n = Number(tradeFeeRate);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return n / 10000;
};

const pickNumber = (...values) => {
  for (const v of values) {
    const n = Number(v);
    if (Number.isFinite(n) && n > 0) return n;
  }
  return null;
};

const pickNumberAllowZero = (...values) => {
  for (const v of values) {
    const n = Number(v);
    if (Number.isFinite(n) && n >= 0) return n;
  }
  return null;
};

const formatUserMetric = (value) => {
  if (value === null || value === undefined || Number.isNaN(Number(value))) {
    return '--';
  }
  const num = Number(value);
  if (!Number.isFinite(num)) return '--';
  if (num === 0) return '0';
  if (Math.abs(num) >= 1) {
    return num.toLocaleString(undefined, { maximumFractionDigits: 4 });
  }
  return num.toPrecision(4);
};

const sortTokensByMint = (tokenA, tokenB) => {
  const mintA = tokenA?.mint || '';
  const mintB = tokenB?.mint || '';
  if (!mintA || !mintB) return { token0: tokenA, token1: tokenB };
  return mintA < mintB ? { token0: tokenA, token1: tokenB } : { token0: tokenB, token1: tokenA };
};

const shortenAddress = (address = '', chars = 4) => {
  if (!address || typeof address !== 'string') return '--';
  if (address.length <= chars * 2 + 3) return address;
  return `${address.slice(0, chars)}...${address.slice(-chars)}`;
};

const formatAmountForInput = (val) => {
  const num = Number(val);
  if (!Number.isFinite(num) || num <= 0) return '';
  const formatted =
    Math.abs(num) >= 1 ? num.toFixed(6) : num.toPrecision(6);
  return formatted.replace(/\.?0+$/, '');
};

const TokenInput = ({ token, amount, onChange, balance, onFill, connected, t, hasError, label }) => {
  const symbol = token?.symbol || t('depositPage.tokenPlaceholder');
  const displayBalance =
    balance === null || balance === undefined
      ? '--'
      : Number(balance).toFixed(4);

  const renderIcon = () => {
    if (token?.icon) {
      return (
        <img
          src={token.icon}
          alt={symbol}
          className="w-10 h-10 rounded-full border border-border object-cover"
          onError={(e) => {
            e.currentTarget.style.display = 'none';
          }}
        />
      );
    }
    return (
      <div className="w-10 h-10 rounded-full bg-muted flex items-center justify-center text-sm font-semibold">
        {symbol.slice(0, 2).toUpperCase()}
      </div>
    );
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <div className="flex items-center gap-2">
          <Wallet2 className="w-4 h-4" />
          <span>
            {t('depositPage.balance')}: {displayBalance} {symbol}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 px-2"
            disabled={!connected}
            onClick={() => onFill?.(0.5)}
          >
            {t('depositPage.half')}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 px-2"
            disabled={!connected}
            onClick={() => onFill?.(1)}
          >
            {t('depositPage.max')}
          </Button>
        </div>
      </div>
      <div
        className={cn(
          'flex items-center gap-4 border rounded-xl bg-card/70 px-4 py-3',
          hasError ? 'border-red-400' : 'border-border/60'
        )}
      >
        <div className="flex items-center gap-3 min-w-[140px]">
          {renderIcon()}
          <div className="flex flex-col">
            <span className="text-sm text-muted-foreground">{label || t('depositPage.tokenLabel')}</span>
            <span className="text-lg font-semibold">{symbol}</span>
          </div>
        </div>
        <Input
          type="number"
          inputMode="decimal"
          placeholder="0.0"
          value={amount}
          onChange={(e) => onChange?.(e.target.value)}
          className="h-12 text-right text-xl font-semibold flex-1"
        />
      </div>
    </div>
  );
};

const StatRow = ({ label, value, hint }) => (
  <div className="flex items-start justify-between text-sm">
    <div className="text-muted-foreground flex items-center gap-1">
      <span>{label}</span>
      {hint && <Info className="w-3 h-3" />}
    </div>
    <div className="text-right">
      <div className="font-semibold">{value}</div>
      {hint && <div className="text-xs text-muted-foreground">{hint}</div>}
    </div>
  </div>
);

const AddressWithTooltip = ({ label, address, className, onCopy }) => {
  const truncated = shortenAddress(address);
  const [open, setOpen] = useState(false);
  const timerRef = useRef(null);

  const show = () => {
    if (timerRef.current) clearTimeout(timerRef.current);
    setOpen(true);
  };
  const hide = () => {
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => setOpen(false), 200);
  };

  return (
    <div
      className={cn('relative inline-flex items-center gap-1 cursor-pointer', className)}
      onMouseEnter={show}
      onMouseLeave={hide}
    >
      <span className="font-medium">{truncated}</span>
      {open && (
        <div
          className="absolute left-0 top-full mt-1 z-30 bg-popover text-popover-foreground border border-border/60 rounded-md shadow-lg px-3 py-2 text-xs text-left whitespace-pre-wrap min-w-[220px]"
          onMouseEnter={show}
          onMouseLeave={hide}
        >
          {label ? `${label}: ${address || '--'}` : address || '--'}
          <div className="mt-2 flex justify-end">
            <Button
              size="sm"
              variant="ghost"
              className="h-7 px-2"
              onClick={() => onCopy && address && onCopy(address)}
            >
              Copy
            </Button>
          </div>
        </div>
      )}
    </div>
  );
};

const PoolDeposit = ({ pool, onNavigateBack }) => {
  const { publicKey, connected, wallet, sendTransaction } = useWallet();
  const { connection } = useConnection();
  const { toast } = useToast();
  const { t } = useTranslation();
  const { setVisible: setWalletModalVisible } = useWalletModal();

  const [mode, setMode] = useState('deposit');
  const [amountA, setAmountA] = useState('');
  const [amountB, setAmountB] = useState('');
  const [slippage, setSlippage] = useState('0.5');
  const [priceDirection, setPriceDirection] = useState('AtoB');
  const [balanceA, setBalanceA] = useState(null);
  const [balanceB, setBalanceB] = useState(null);
  const [fetchingBalance, setFetchingBalance] = useState(false);
  const [showTooltip, setShowTooltip] = useState(false);
  const tooltipTimer = useRef(null);
  const [poolDetail, setPoolDetail] = useState(null);
  const [loadingPool, setLoadingPool] = useState(false);
  const [poolError, setPoolError] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);
  const [lastChanged, setLastChanged] = useState(null);

  const poolState = pool?.poolState;
  const poolVersion = pool?.poolVersion;
  const walletAddress = useMemo(
    () =>
      connected && publicKey
        ? publicKey?.toBase58?.() || publicKey?.toString?.()
        : '',
    [connected, publicKey]
  );

  const resolvedPool = useMemo(
    () => (poolDetail ? { ...pool, ...poolDetail } : pool),
    [pool, poolDetail]
  );

  const tokenA = {
    symbol: resolvedPool?.inputTokenSymbol,
    icon: resolvedPool?.inputTokenIcon,
    mint: resolvedPool?.inputVaultMint,
  };
  const tokenB = {
    symbol: resolvedPool?.outputTokenSymbol,
    icon: resolvedPool?.outputTokenIcon,
    mint: resolvedPool?.outputVaultMint,
  };

  const tokenASymbol = tokenA.symbol || 'A';
  const tokenBSymbol = tokenB.symbol || 'B';

  const poolName = `${tokenASymbol}-${tokenBSymbol}`;
  const feePercent = normalizeFeePercent(resolvedPool?.tradeFeeRate);
  const aprValue = Number(resolvedPool?.apr) || 0;
  const lockedPercent = Number(resolvedPool?.lockedPercent || resolvedPool?.lockPercent || 0);
  const pooledA = pickNumber(
    resolvedPool?.inputAmount,
    resolvedPool?.input_amount,
    resolvedPool?.baseReserve,
    resolvedPool?.base_reserve,
    resolvedPool?.inputReserve
  );
  const pooledB = pickNumber(
    resolvedPool?.outputAmount,
    resolvedPool?.output_amount,
    resolvedPool?.quoteReserve,
    resolvedPool?.quote_reserve,
    resolvedPool?.outputReserve
  );
  const poolReserveA = useMemo(
    () =>
      pickNumberAllowZero(
        resolvedPool?.inputAmount,
        resolvedPool?.input_amount,
        resolvedPool?.baseReserve,
        resolvedPool?.base_reserve,
        resolvedPool?.inputReserve
      ),
    [resolvedPool]
  );
  const poolReserveB = useMemo(
    () =>
      pickNumberAllowZero(
        resolvedPool?.outputAmount,
        resolvedPool?.output_amount,
        resolvedPool?.quoteReserve,
        resolvedPool?.quote_reserve,
        resolvedPool?.outputReserve
      ),
    [resolvedPool]
  );

  const poolRatioAB = useMemo(() => {
    const a = Number(poolReserveA);
    const b = Number(poolReserveB);
    if (Number.isFinite(a) && a > 0 && Number.isFinite(b) && b >= 0) {
      return b / a;
    }
    const fallback =
      Number(
        resolvedPool?.price ||
          resolvedPool?.quotePerBase ||
          resolvedPool?.marketPrice ||
          resolvedPool?.quotePrice ||
          0
      );
    return Number.isFinite(fallback) && fallback > 0 ? fallback : null;
  }, [poolReserveA, poolReserveB, resolvedPool]);

  const computedPrice = useMemo(() => {
    const a = parseFloat(amountA);
    const b = parseFloat(amountB);
    if (Number.isFinite(a) && Number.isFinite(b) && a > 0) {
      return b / a;
    }
    if (Number.isFinite(poolRatioAB) && poolRatioAB > 0) {
      return poolRatioAB;
    }
    const fallback =
      Number(
        resolvedPool?.price ||
          resolvedPool?.quotePerBase ||
          resolvedPool?.marketPrice ||
          0
      ) || 1;
    return fallback;
  }, [amountA, amountB, poolRatioAB, resolvedPool]);

  const displayPrice =
    priceDirection === 'AtoB'
      ? computedPrice
      : computedPrice > 0
      ? 1 / computedPrice
      : 0;

  const priceLabel =
    priceDirection === 'AtoB'
      ? `1 ${tokenASymbol} ~= ${displayPrice.toFixed(4)} ${tokenBSymbol}`
      : `1 ${tokenBSymbol} ~= ${displayPrice.toFixed(4)} ${tokenASymbol}`;

  const totalAmount =
    mode === 'deposit'
      ? (parseFloat(amountA) || 0) + (parseFloat(amountB) || 0)
      : parseFloat(amountA) || 0;

  const userMetrics = useMemo(() => {
    const position = resolvedPool?.userPosition || resolvedPool?.user_position || {};
    return {
      pooledA: pickNumberAllowZero(
        resolvedPool?.userPooledInput,
        position?.userPooledInput,
        position?.inputAmount,
        position?.baseAmount
      ),
      pooledB: pickNumberAllowZero(
        resolvedPool?.userPooledOutput,
        position?.userPooledOutput,
        position?.outputAmount,
        position?.quoteAmount
      ),
      stakedLp: pickNumberAllowZero(
        resolvedPool?.userStakedLp,
        position?.userStakedLp,
        position?.stakedLp,
        position?.stakeLp,
        position?.stake_lp
      ),
      unstakedLp: pickNumberAllowZero(
        resolvedPool?.userUnstakedLp,
        position?.userUnstakedLp,
        position?.unstakedLp,
        position?.lpBalance,
        position?.lp_balance,
        position?.unstakeLp,
        position?.unstake_lp
      ),
    };
  }, [resolvedPool]);
  const availableWithdrawRatio = useMemo(() => {
    const unstaked = Number(userMetrics.unstakedLp) || 0;
    const staked = Number(userMetrics.stakedLp) || 0;
    const total = unstaked + staked;
    if (total <= 0) return 0;
    return unstaked / total;
  }, [userMetrics]);
  const availableWithdrawLp = useMemo(() => Number(userMetrics.unstakedLp) || 0, [userMetrics]);
  const availableWithdrawA = useMemo(() => {
    const pooled = Number(userMetrics.pooledA);
    if (!Number.isFinite(pooled)) return 0;
    return pooled * availableWithdrawRatio;
  }, [userMetrics, availableWithdrawRatio]);
  const availableWithdrawB = useMemo(() => {
    const pooled = Number(userMetrics.pooledB);
    if (!Number.isFinite(pooled)) return 0;
    return pooled * availableWithdrawRatio;
  }, [userMetrics, availableWithdrawRatio]);
  const estimatedWithdraw = useMemo(() => {
    const lpInput = parseFloat(amountA) || 0;
    if (lpInput <= 0 || availableWithdrawLp <= 0) {
      return { outA: 0, outB: 0 };
    }
    const factor = Math.min(1, lpInput / availableWithdrawLp);
    return {
      outA: (availableWithdrawA || 0) * factor,
      outB: (availableWithdrawB || 0) * factor,
    };
  }, [amountA, availableWithdrawA, availableWithdrawB, availableWithdrawLp]);
  const isWithdrawExceed = useMemo(() => {
    if (mode !== 'withdraw') return false;
    const lpNum = parseFloat(amountA) || 0;
    if (lpNum <= 0) return true;
    return lpNum - availableWithdrawLp > 1e-9;
  }, [mode, amountA, availableWithdrawLp]);

  const isActionDisabled =
    loadingPool ||
    fetchingBalance ||
    submitting ||
    !connected ||
    (mode === 'deposit' ? !(amountA && amountB) : !amountA) ||
    isWithdrawExceed;

  useEffect(() => {
    let cancelled = false;
    if (!poolState) {
      setPoolDetail(null);
      setPoolError(null);
      setLoadingPool(false);
      return;
    }

    setPoolDetail(null);
    setPoolError(null);
    setLoadingPool(true);
    async function run() {
      try {
        const detail = await getPoolDetail(
          poolState,
          poolVersion,
          walletAddress
            ? { user_wallet_address: walletAddress }
            : {}
        );
        if (!cancelled) {
          setPoolDetail(detail);
        }
      } catch (e) {
        if (!cancelled) {
          console.warn('Failed to load pool detail', e);
          setPoolError(e);
        }
      } finally {
        if (!cancelled) {
          setLoadingPool(false);
        }
      }
    }
    run();
    return () => {
      cancelled = true;
    };
  }, [poolState, poolVersion, walletAddress, refreshKey]);

  const loadBalanceForMint = async (mint) => {
    if (!connection || !publicKey || !mint) return 0;
    try {
      if (mint === 'So11111111111111111111111111111111111111112') {
        const lamports = await connection.getBalance(publicKey);
        return lamports / LAMPORTS_PER_SOL;
      }
      const parsedMint = new PublicKey(mint);
      const resp = await connection.getParsedTokenAccountsByOwner(publicKey, {
        mint: parsedMint,
      });
      if (!resp?.value?.length) return 0;
      return resp.value.reduce((sum, item) => {
        const amt =
          item?.account?.data?.parsed?.info?.tokenAmount?.uiAmount ??
          0;
        return sum + (Number(amt) || 0);
      }, 0);
    } catch (e) {
      console.warn('Failed to fetch balance', e);
      return 0;
    }
  };

  useEffect(() => {
    let cancelled = false;
    if (!connected || !publicKey) {
      setBalanceA(null);
      setBalanceB(null);
      return;
    }
    async function run() {
      setFetchingBalance(true);
      try {
        const [a, b] = await Promise.all([
          loadBalanceForMint(tokenA.mint),
          loadBalanceForMint(tokenB.mint),
        ]);
        if (!cancelled) {
          setBalanceA(a);
          setBalanceB(b);
        }
      } finally {
        if (!cancelled) {
          setFetchingBalance(false);
        }
      }
    }
    run();
    return () => {
      cancelled = true;
    };
  }, [connected, publicKey, connection, tokenA.mint, tokenB.mint, refreshKey]);

  const syncAmountByRatio = (side, rawVal) => {
    const ratio = Number(poolRatioAB);
    const num = parseFloat(rawVal);
    if (!Number.isFinite(num) || num <= 0 || !Number.isFinite(ratio) || ratio <= 0) {
      if (side === 'A') {
        setAmountB('');
      } else {
        setAmountA('');
      }
      return;
    }
    if (side === 'A') {
      const calc = num * ratio;
      setAmountB(formatAmountForInput(calc));
    } else {
      const calc = num / ratio;
      setAmountA(formatAmountForInput(calc));
    }
  };

  const handleAmountAChange = (val) => {
    setLastChanged('A');
    setAmountA(val);
    syncAmountByRatio('A', val);
  };

  const handleAmountBChange = (val) => {
    setLastChanged('B');
    setAmountB(val);
    syncAmountByRatio('B', val);
  };

  useEffect(() => {
    if (!Number.isFinite(poolRatioAB) || poolRatioAB <= 0) return;
    if (lastChanged === 'A' && amountA) {
      syncAmountByRatio('A', amountA);
    } else if (lastChanged === 'B' && amountB) {
      syncAmountByRatio('B', amountB);
    }
  }, [amountA, amountB, lastChanged, mode, poolRatioAB]);

  const handleFill = (side, ratio) => {
    let balance = side === 'A' ? balanceA : balanceB;
    if (mode === 'withdraw') {
      balance = side === 'A' ? availableWithdrawA : availableWithdrawB;
    }
    if (!Number.isFinite(balance)) return;
    const next = balance * ratio;
    const formatted =
      next >= 1 ? next.toFixed(4) : next.toPrecision(4);
    if (side === 'A') {
      handleAmountAChange(formatted.replace(/\.?0+$/, ''));
    } else {
      handleAmountBChange(formatted.replace(/\.?0+$/, ''));
    }
  };

  const computeWithdrawMins = (lpAmountUi) => {
    const lp = parseFloat(lpAmountUi) || 0;
    if (lp <= 0 || availableWithdrawLp <= 0) {
      return { min0: '0', min1: '0' };
    }
    const ratio = Math.min(1, lp / availableWithdrawLp);
    const expected0 = (availableWithdrawA || 0) * ratio;
    const expected1 = (availableWithdrawB || 0) * ratio;
    const slippageBps = Math.max(0, Math.min(10000, Math.round((parseFloat(slippage) || 0) * 100)));
    const factor = 1 - slippageBps / 10000;
    return {
      min0: formatAmountForInput(expected0 * factor) || '0',
      min1: formatAmountForInput(expected1 * factor) || '0',
    };
  };

  const handleAction = () => {
    if (!connected) {
      if (setWalletModalVisible) {
        setWalletModalVisible(true);
        return;
      }
      wallet?.adapter?.connect?.();
      return;
    }
    if (!resolvedPool?.poolState) {
      toast({ title: 'Missing pool', description: 'Pool state not found' });
      return;
    }
    const run = async () => {
      try {
        setSubmitting(true);
        const { token0, token1 } = sortTokensByMint(tokenA, tokenB);
        const token0Amount = token0?.mint === tokenA.mint ? amountA : amountB;
        const token1Amount = token0?.mint === tokenA.mint ? amountB : amountA;
        const slippageBps = Math.max(
          0,
          Math.min(10000, Math.round((parseFloat(slippage) || 0) * 100))
        );
        let txBase64 = '';
        if (mode === 'withdraw') {
          const { min0, min1 } = computeWithdrawMins(amountA);
          const resp = await buildCpmmRemoveLiquidityTx({
            chain_id: 100000,
            pool_state: resolvedPool.poolState,
            lp_amount: amountA || '0',
            min_token0_amount: min0,
            min_token1_amount: min1,
            user_wallet_address: walletAddress,
          });
          txBase64 = resp?.tx_base64 || resp?.txBase64 || resp?.tx;
        } else {
          const resp = await buildCpmmAddLiquidityTx({
            chain_id: 100000,
            pool_state: resolvedPool.poolState,
            token0_amount: token0Amount || '0',
            token1_amount: token1Amount || '0',
            slippage_bps: slippageBps,
            user_wallet_address: walletAddress,
          });
          txBase64 = resp?.tx_base64 || resp?.txBase64 || resp?.tx;
        }
        if (!txBase64) {
          throw new Error('Backend did not return transaction');
        }
        const raw = Uint8Array.from(atob(txBase64), (c) => c.charCodeAt(0));
        const tx = Transaction.from(raw);
        const sig = await sendTransaction(tx, connection);
        toast({
          title: mode === 'withdraw' ? 'Withdraw submitted' : 'Deposit submitted',
          description: sig,
        });
        setAmountA('');
        setAmountB('');
        setRefreshKey((k) => k + 1);
      } catch (e) {
        const errorMessage =
          e?.message ||
          e?.error?.message ||
          e?.cause?.message ||
          (typeof e === 'string' ? e : 'Unknown error');
        let logs = e?.logs || e?.error?.logs || e?.cause?.logs || [];
        if ((!logs || !logs.length) && typeof e?.getLogs === 'function') {
          try {
            logs = await e.getLogs(connection);
          } catch (logErr) {
            console.warn('Failed to fetch logs for liquidity tx', logErr);
          }
        }
        console.error('CPMM liquidity transaction failed', {
          mode,
          amountA,
          amountB,
          wallet: walletAddress,
          error: errorMessage,
          rawError: e,
          rawErrorMessage: e?.error || e?.cause || e?.message,
          logs,
        });
        toast({
          title: 'Transaction failed',
          description: errorMessage || String(e),
          variant: 'destructive',
        });
      } finally {
        setSubmitting(false);
      }
    };
    run();
  };

  if (!resolvedPool) {
    return (
      <div className="container mx-auto px-4 py-10">
        <Button variant="ghost" onClick={onNavigateBack}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          {t('depositPage.back')}
        </Button>
        <div className="mt-6 text-muted-foreground">{t('depositPage.missingPool')}</div>
      </div>
    );
  }

  const truncatedPool = shortenAddress(resolvedPool?.poolState);
  const truncatedTokenA = shortenAddress(tokenA.mint);
  const truncatedTokenB = shortenAddress(tokenB.mint);
  const handleCopy = (val) => {
    if (!val) return;
    navigator.clipboard?.writeText(val);
    toast({ title: 'Copied', description: 'Address copied' });
  };

  const handleRefresh = () => {
    setRefreshKey((k) => k + 1);
  };

  const tooltipShow = () => {
    if (tooltipTimer.current) clearTimeout(tooltipTimer.current);
    setShowTooltip(true);
  };
  const tooltipHide = () => {
    if (tooltipTimer.current) clearTimeout(tooltipTimer.current);
    tooltipTimer.current = setTimeout(() => setShowTooltip(false), 200);
  };

  return (
    <div className="container mx-auto px-4 py-8 space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" onClick={onNavigateBack}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          {t('depositPage.back')}
        </Button>
        <TokenPairAvatar tokenA={tokenA} tokenB={tokenB} />
        <div>
          <div className="text-lg font-semibold">
            {t('depositPage.title')} · {poolName}
          </div>
          <div className="text-xs text-muted-foreground flex items-center gap-2 relative">
            <span>{t('depositPage.poolAddress')}:</span>
            <AddressWithTooltip label={t('depositPage.poolAddress')} address={resolvedPool?.poolState} onCopy={handleCopy} />
            <button
              type="button"
              className="flex items-center text-amber-500 hover:text-amber-600"
              onMouseEnter={tooltipShow}
              onMouseLeave={tooltipHide}
              aria-label={t('depositPage.tooltip.poolLabel')}
            >
              <AlertCircle className="w-4 h-4" />
            </button>
            {showTooltip && (
              <div
                className="absolute top-6 left-0 z-20 bg-popover text-popover-foreground border border-border/60 rounded-md shadow-lg px-3 py-2 text-xs space-y-1 min-w-[240px] text-left"
                onMouseEnter={tooltipShow}
                onMouseLeave={tooltipHide}
              >
                <div className="font-semibold">{poolName}</div>
                <div className="flex items-center justify-between gap-2">
                  <span>{t('depositPage.tooltip.poolLabel')}: {resolvedPool?.poolState || '--'}</span>
                  <Button size="sm" variant="ghost" className="h-7 px-2" onClick={() => handleCopy(resolvedPool?.poolState)}>Copy</Button>
                </div>
                <div className="flex items-center justify-between gap-2">
                  <span>{`${tokenASymbol} Mint`}: {tokenA.mint || '--'}</span>
                  <Button size="sm" variant="ghost" className="h-7 px-2" onClick={() => handleCopy(tokenA.mint)}>Copy</Button>
                </div>
                <div className="flex items-center justify-between gap-2">
                  <span>{`${tokenBSymbol} Mint`}: {tokenB.mint || '--'}</span>
                  <Button size="sm" variant="ghost" className="h-7 px-2" onClick={() => handleCopy(tokenB.mint)}>Copy</Button>
                </div>
              </div>
            )}
          </div>
        </div>
        <div className="ml-auto">
          <Button variant="outline" size="sm" className="flex items-center gap-2" onClick={handleRefresh} disabled={loadingPool}>
            <RefreshCw className={cn('w-4 h-4', loadingPool && 'animate-spin')} />
            {t('tokenList.buttons.refresh') || 'Refresh'}
          </Button>
        </div>
      </div>

      {loadingPool && (
        <div className="text-sm text-muted-foreground">
          Syncing latest pool data...
        </div>
      )}
      {poolError && (
        <div className="text-sm text-red-500 bg-red-50 border border-red-200 rounded-md px-3 py-2">
          Failed to load pool details: {poolError.message || 'Unknown error'}
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 items-start lg:items-stretch">
        <div className="space-y-6 lg:flex lg:flex-col lg:h-full">
          <Card className="border border-border/60 shadow-sm flex-1 h-full">
            <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <CardTitle className="flex items-center gap-2 text-xl">
                {t('depositPage.title')}
              </CardTitle>
              <div className="flex items-center gap-2">
                <div className="inline-flex rounded-full border border-border/60 p-1 bg-muted/30">
                  {['deposit', 'withdraw'].map((key) => (
                    <button
                      key={key}
                      type="button"
                      onClick={() => setMode(key)}
                      className={cn(
                        'px-4 py-1 text-sm font-medium rounded-full transition',
                        mode === key
                          ? 'bg-primary text-primary-foreground shadow-sm'
                          : 'text-muted-foreground hover:bg-background'
                      )}
                    >
                      {key === 'deposit'
                        ? t('depositPage.tabs.deposit')
                        : t('depositPage.tabs.withdraw')}
                    </button>
                  ))}
                </div>
                {connected ? (
                  <Badge variant="outline" className="flex items-center gap-1">
                    <Wallet2 className="w-3 h-3" />
                    {t('depositPage.connected')}
                  </Badge>
                ) : (
                  <Badge variant="secondary">{t('depositPage.notConnected')}</Badge>
                )}
              </div>
            </CardHeader>

            <CardContent className="space-y-5">
              {mode === 'deposit' ? (
                <>
                  <TokenInput
                    token={tokenA}
                    amount={amountA}
                    onChange={handleAmountAChange}
                    balance={balanceA}
                    connected={connected}
                    onFill={(ratio) => handleFill('A', ratio)}
                    t={t}
                  />
                  <TokenInput
                    token={tokenB}
                    amount={amountB}
                    onChange={handleAmountBChange}
                    balance={balanceB}
                    connected={connected}
                    onFill={(ratio) => handleFill('B', ratio)}
                    t={t}
                  />
                </>
              ) : (
                <>
                  <TokenInput
                    token={{ symbol: 'LP', mint: resolvedPool?.lpMint }}
                    amount={amountA}
                    onChange={handleAmountAChange}
                    balance={availableWithdrawLp}
                    connected={connected}
                    onFill={(ratio) => handleFill('A', ratio)}
                    t={t}
                    hasError={mode === 'withdraw' && isWithdrawExceed && amountA}
                    label="LP Tokens"
                  />
                  <div className="rounded-xl border border-border/60 bg-muted/10 px-4 py-3 text-sm space-y-1">
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground">{`Est. ${tokenASymbol}`}</span>
                      <span className="font-semibold">
                        {estimatedWithdraw.outA ? estimatedWithdraw.outA.toFixed(6) : '--'}
                      </span>
                    </div>
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground">{`Est. ${tokenBSymbol}`}</span>
                      <span className="font-semibold">
                        {estimatedWithdraw.outB ? estimatedWithdraw.outB.toFixed(6) : '--'}
                      </span>
                    </div>
                  </div>
                </>
              )}

              <div className="rounded-xl border border-border/60 bg-muted/20 px-4 py-3 flex items-center justify-between">
                <div className="text-sm text-muted-foreground">
                  {mode === 'deposit'
                    ? t('depositPage.totalDeposit')
                    : t('depositPage.totalWithdraw')}
                </div>
                <div className="text-xl font-semibold">
                  {totalAmount > 0 ? totalAmount.toFixed(4) : '--'}
                  {mode === 'withdraw' ? ' LP' : ''}
                </div>
              </div>

              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <button
                  type="button"
                  onClick={() =>
                    setPriceDirection((prev) => (prev === 'AtoB' ? 'BtoA' : 'AtoB'))
                  }
                  className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition"
                >
                  <ArrowLeftRight className="w-4 h-4" />
                  {priceLabel}
                </button>
                <div className="flex items-center gap-2">
                  <span className="text-sm text-muted-foreground">{t('depositPage.slippage')}</span>
                  <div className="relative flex items-center border border-border/60 rounded-lg px-3 py-1.5">
                    <Input
                      type="number"
                      min="0"
                      max="100"
                      step="0.1"
                      value={slippage}
                      onChange={(e) => setSlippage(e.target.value)}
                      className="w-20 pr-8 text-right border-0 focus-visible:ring-0 h-8"
                    />
                    <span className="absolute right-3 text-sm text-muted-foreground">%</span>
                  </div>
                </div>
              </div>

              <Button
                className="w-full h-12 text-base font-semibold"
                disabled={isActionDisabled}
                onClick={handleAction}
              >
                {!connected
                  ? t('depositPage.connectWallet')
                  : mode === 'withdraw'
                  ? t('depositPage.actions.withdraw')
                  : t('depositPage.actions.submit')}
              </Button>
            </CardContent>
          </Card>

          <Card className="border border-border/60 flex-1 h-full">
            <CardHeader>
              <CardTitle className="flex items-center justify-between">
                <span>{t('depositPage.myPosition')}</span>
                <Badge variant="outline">{t('depositPage.lpBalances')}</Badge>
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div className="rounded-xl border border-border/60 bg-card/60 p-3">
                  <div className="text-xs text-muted-foreground mb-1">{t('depositPage.staked')}</div>
                  <div className="text-lg font-semibold">{formatUserMetric(userMetrics.stakedLp)}</div>
                  <div className="text-xs text-muted-foreground">{t('depositPage.lpTokens')}</div>
                </div>
                <div className="rounded-xl border border-border/60 bg-card/60 p-3">
                  <div className="text-xs text-muted-foreground mb-1">{t('depositPage.unstaked')}</div>
                  <div className="text-lg font-semibold">{formatUserMetric(userMetrics.unstakedLp)}</div>
                  <div className="text-xs text-muted-foreground">{t('depositPage.lpTokens')}</div>
                </div>
                <div className="rounded-xl border border-border/60 bg-card/60 p-3">
                  <div className="text-xs text-muted-foreground mb-1">
                    {`Pooled ${tokenASymbol}`}
                  </div>
                  <div className="text-lg font-semibold">{formatUserMetric(userMetrics.pooledA)}</div>
                </div>
                <div className="rounded-xl border border-border/60 bg-card/60 p-3">
                  <div className="text-xs text-muted-foreground mb-1">
                    {`Pooled ${tokenBSymbol}`}
                  </div>
                  <div className="text-lg font-semibold">{formatUserMetric(userMetrics.pooledB)}</div>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>

        <div className="space-y-6 lg:flex lg:flex-col lg:h-full">
          <Card className="border border-border/60 flex flex-col lg:flex-1 h-full">
            <CardHeader>
              <CardTitle className="flex items-center justify-between">
                <span>{t('depositPage.totalApr')}</span>
                <Badge variant="outline">{formatPercent(aprValue)}</Badge>
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-baseline justify-between">
                <div className="text-3xl font-bold">
                  {formatPercent(aprValue)}
                </div>
                <span className="text-sm text-muted-foreground">7D</span>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Badge className="bg-amber-500/20 text-amber-600 border-amber-500/40">
                    {t('depositPage.fees')}
                  </Badge>
                  <span className="text-sm text-muted-foreground">{t('depositPage.feesDesc')}</span>
                </div>
                <span className="font-semibold">{formatPercent(aprValue)}</span>
              </div>

              <div className="space-y-3 pt-2">
                <StatRow
                  label={t('depositPage.poolLiquidity')}
                  value={formatCurrencyUSD(resolvedPool?.liquidityUsd)}
                />
                <StatRow
                  label={`Pooled ${tokenASymbol}`}
                  value={pooledA !== null ? pooledA.toLocaleString() : '--'}
                />
                <StatRow
                  label={`Pooled ${tokenBSymbol}`}
                  value={pooledB !== null ? pooledB.toLocaleString() : '--'}
                />
                <StatRow
                  label={t('depositPage.permanentLock')}
                  value={`${lockedPercent.toFixed(2)}% permanently locked`}
                  hint={t('depositPage.permanentLockHint')}
                />
              </div>
            </CardContent>
          </Card>

          <Card className="border border-border/60 flex-1 h-full">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Lock className="w-4 h-4" />
                {t('depositPage.poolDetails')}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">{t('depositPage.volume24h')}</span>
                <span className="font-medium">{formatCurrencyUSD(resolvedPool?.vol24h)}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">{t('depositPage.fees24h')}</span>
                <span className="font-medium">
                  {formatCurrencyUSD((Number(resolvedPool?.vol24h) || 0) * (feePercent / 100))}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">{t('depositPage.poolAddress')}</span>
                <AddressWithTooltip label={t('depositPage.poolAddress')} address={resolvedPool?.poolState} onCopy={handleCopy} />
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">{`${tokenASymbol} Address`}</span>
                <AddressWithTooltip label={`${tokenASymbol} Address`} address={tokenA.mint} onCopy={handleCopy} />
              </div>
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">{`${tokenBSymbol} Address`}</span>
                <AddressWithTooltip label={`${tokenBSymbol} Address`} address={tokenB.mint} onCopy={handleCopy} />
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
};

export default PoolDeposit;
