import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useWallet, useConnection } from '@solana/wallet-adapter-react';
import { PublicKey, Transaction } from '@solana/web3.js';
import {
  ArrowLeft,
  ArrowDownUp,
  ChevronDown,
  Loader2,
  RefreshCw,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
} from 'lucide-react';
import { Button } from '../UI/Button';
import { Card, CardContent, CardHeader, CardTitle } from '../UI/card';
import { Input } from '../UI/input';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../UI/dialog';
import { useToast } from '../../hooks/use-toast';
import { useTranslation } from '../../i18n/LanguageContext';
import api from '../../api/client';
import { dedupeTokens, normalizeTokenFromApi } from '../liquidity/CpmmCreatePoolForm';
import { quoteCpmmSwap, buildCpmmSwapTx } from '../../api/pools';

const CHAIN_ID = 100000;
const TOKENS_ENDPOINT = '/api/liquidity/cpmm/tokens';

const formatNumber = (value, digits = 6) => {
  const num = Number(value);
  if (!Number.isFinite(num)) return '';
  return num.toFixed(digits).replace(/\.?0+$/, '');
};

const deriveTokenFromPool = (pool, side = 'input') => {
  if (!pool) return null;
  const isInput = side === 'input';
  const mint = isInput
    ? pool?.inputVaultMint || pool?.input_vault_mint || pool?.inputMint || pool?.input_mint
    : pool?.outputVaultMint || pool?.output_vault_mint || pool?.outputMint || pool?.output_mint;
  const symbol = isInput ? pool?.inputTokenSymbol : pool?.outputTokenSymbol;
  const icon = isInput ? pool?.inputTokenIcon : pool?.outputTokenIcon;
  const decimals =
    (isInput ? pool?.inputDecimals || pool?.input_decimals : pool?.outputDecimals || pool?.output_decimals) ??
    9;
  if (!mint && !symbol) return null;
  return {
    mint: mint || '',
    symbol: symbol || '',
    name: symbol || '',
    logo: icon || '',
    decimals,
  };
};

const TokenButton = ({ token, onClick, disabled = false }) => {
  const badge = token?.symbol?.slice(0, 1)?.toUpperCase() || '?';
  return (
    <Button
      type="button"
      variant="outline"
      className={`flex items-center gap-2 rounded-2xl px-3 h-12 border-border/60 bg-background/60 hover:bg-background/90 ${disabled ? 'pointer-events-none opacity-60' : ''}`}
      onClick={disabled ? undefined : onClick}
    >
      <div className="w-8 h-8 rounded-full bg-muted flex items-center justify-center overflow-hidden text-sm font-semibold">
        {token?.logo ? (
          <img src={token.logo} alt={token.symbol || 'token'} className="w-full h-full object-cover" />
        ) : (
          <span>{badge}</span>
        )}
      </div>
      <span className="text-lg font-semibold">{token?.symbol || 'Select'}</span>
      <ChevronDown className="w-4 h-4 text-muted-foreground" />
    </Button>
  );
};

const TokenSelectDialog = ({ open, onClose, tokens, onSelect, title }) => {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');

  const filtered = useMemo(() => {
    if (!search) return tokens;
    const key = search.toLowerCase();
    return tokens.filter(
      (token) =>
        token.symbol?.toLowerCase().includes(key) ||
        token.name?.toLowerCase().includes(key) ||
        token.mint?.toLowerCase().includes(key)
    );
  }, [tokens, search]);

  useEffect(() => {
    if (!open) {
      setSearch('');
    }
  }, [open]);

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader className="space-y-2">
          <DialogTitle className="text-xl font-semibold">{title}</DialogTitle>
          <Input
            placeholder={t('swapPage.searchToken')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full"
          />
        </DialogHeader>
        <div className="max-h-[360px] overflow-y-auto space-y-2">
          {filtered.length === 0 && (
            <div className="text-center text-muted-foreground py-6 text-sm">{t('swapPage.tokensEmpty')}</div>
          )}
          {filtered.map((token) => (
            <button
              key={token.mint}
              onClick={() => {
                onSelect(token);
                onClose(false);
              }}
              className="w-full flex items-center gap-3 p-3 rounded-xl hover:bg-muted/60 transition-colors text-left border border-border/60"
            >
              <div className="w-10 h-10 rounded-full bg-muted flex items-center justify-center overflow-hidden text-base font-semibold">
                {token.logo ? (
                  <img src={token.logo} alt={token.symbol} className="w-full h-full object-cover" />
                ) : (
                  token.symbol?.slice(0, 1)?.toUpperCase() || '?'
                )}
              </div>
              <div className="flex flex-col">
                <span className="font-semibold text-foreground">{token.symbol || 'TOKEN'}</span>
                <span className="text-xs text-muted-foreground">{token.mint}</span>
              </div>
            </button>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
};

const buildPoolRate = (pool) => {
  if (!pool) return null;
  const inputReserve = Number(pool?.inputReserve ?? pool?.baseReserve);
  const outputReserve = Number(pool?.outputReserve ?? pool?.quoteReserve);
  if (Number.isFinite(inputReserve) && Number.isFinite(outputReserve) && inputReserve > 0 && outputReserve > 0) {
    return outputReserve / inputReserve;
  }
  const price =
    Number(pool?.quotePerBase) ||
    Number(pool?.price) ||
    Number(pool?.marketPrice) ||
    Number(pool?.quote_per_base) ||
    Number(pool?.market_price);
  if (Number.isFinite(price) && price > 0) {
    return price;
  }
  return null;
};

const SwapPage = ({ pool, onNavigateBack }) => {
  const { publicKey, connected, sendTransaction, wallet } = useWallet();
  const { connection: solConnection } = useConnection();
  const { toast } = useToast();
  const { t } = useTranslation();
  const [tokenIn, setTokenIn] = useState(() => deriveTokenFromPool(pool, 'input'));
  const [tokenOut, setTokenOut] = useState(() => deriveTokenFromPool(pool, 'output'));
  const [amountIn, setAmountIn] = useState('');
  const [amountOut, setAmountOut] = useState('');
  const [slippage, setSlippage] = useState('0.5');
  const [invertRate, setInvertRate] = useState(false);
  const [tokenModal, setTokenModal] = useState({ open: false, target: 'in' });
  const [tokenList, setTokenList] = useState([]);
  const [tokenLoading, setTokenLoading] = useState(false);
  const [balanceIn, setBalanceIn] = useState('--');
  const [balanceOut, setBalanceOut] = useState('--');
  const [priceImpactPct, setPriceImpactPct] = useState('');
  const [minReceiveAmount, setMinReceiveAmount] = useState('');
  const [swapping, setSwapping] = useState(false);
  const requestSeq = useRef(0);

  const poolRate = useMemo(() => buildPoolRate(pool), [pool]);

  const normalizedTokens = useMemo(() => {
    const defaults = [deriveTokenFromPool(pool, 'input'), deriveTokenFromPool(pool, 'output')].filter(Boolean);
    const combined = dedupeTokens([...(tokenList || []), ...defaults].map(normalizeTokenFromApi).filter(Boolean));
    return combined;
  }, [tokenList, pool]);

  const currentRate = useMemo(() => {
    if (!tokenIn?.mint || !tokenOut?.mint) return null;
    const inMint = tokenIn.mint.toLowerCase();
    const outMint = tokenOut.mint.toLowerCase();
    const poolIn = (pool?.inputVaultMint || pool?.input_vault_mint || '').toLowerCase();
    const poolOut = (pool?.outputVaultMint || pool?.output_vault_mint || '').toLowerCase();
    if (poolIn && poolOut && poolRate) {
      if (inMint === poolIn && outMint === poolOut) return poolRate;
      if (inMint === poolOut && outMint === poolIn && poolRate > 0) return 1 / poolRate;
    }
    return poolRate;
  }, [tokenIn?.mint, tokenOut?.mint, poolRate, pool]);

  useEffect(() => {
    if (pool) {
      setTokenIn(deriveTokenFromPool(pool, 'input'));
      setTokenOut(deriveTokenFromPool(pool, 'output'));
      setInvertRate(false);
      if (tokenModal.open) {
        setTokenModal({ open: false, target: 'in' });
      }
    }
  }, [pool, tokenModal.open]);

  const displayRate = useMemo(() => {
    if (!currentRate || currentRate <= 0) return null;
    if (invertRate) {
      return currentRate > 0 ? 1 / currentRate : null;
    }
    return currentRate;
  }, [currentRate, invertRate]);

  const refreshTokens = useCallback(async () => {
    setTokenLoading(true);
    try {
      const res = await api.get(TOKENS_ENDPOINT, { params: { chain_id: CHAIN_ID, page_size: 200 } });
      const payload = res?.data?.data || res?.data || {};
      const list = Array.isArray(payload?.tokens) ? payload.tokens : Array.isArray(payload) ? payload : [];
      const normalized = dedupeTokens(list.map(normalizeTokenFromApi).filter(Boolean));
      setTokenList(normalized);
    } catch (err) {
      const message = err?.message || 'Token list unavailable';
      toast({
        title: t('common.error') || 'Error',
        description: message,
        variant: 'destructive',
      });
    } finally {
      setTokenLoading(false);
    }
  }, [toast, t]);

  useEffect(() => {
    refreshTokens();
  }, [refreshTokens]);

  const requestQuote = useCallback(
    async (val, source) => {
      const poolState = pool?.poolState || pool?.pool_state;
      if (!poolState) return;
      const numVal = Number(val);
      if (!Number.isFinite(numVal) || numVal <= 0) {
        if (source === 'in') setAmountOut('');
        else setAmountIn('');
        setPriceImpactPct('');
        setMinReceiveAmount('');
        return;
      }
      const seq = ++requestSeq.current;
      const slippageNum = Number(slippage);
      const slippageBps = Number.isFinite(slippageNum) ? Math.max(0, Math.round(slippageNum * 100)) : 0;
      const params = {
        chain_id: CHAIN_ID,
        pool_state: poolState,
        input_mint: tokenIn?.mint,
        output_mint: tokenOut?.mint,
        slippage_bps: slippageBps,
      };
      if (source === 'in') {
        params.amount_in = String(numVal);
      } else {
        params.amount_out = String(numVal);
      }
      try {
        const resp = await quoteCpmmSwap(params);
        if (seq !== requestSeq.current) return;
        const pay = resp?.payAmount || resp?.pay_amount;
        const recv = resp?.receiveAmount || resp?.receive_amount;
        const minRecv = resp?.minReceiveAmount || resp?.min_receive_amount;
        const impact = resp?.priceImpactPct || resp?.price_impact_pct;
        const impactText =
          impact && Number.isFinite(Number(impact)) ? Number(impact).toFixed(2) : impact || '';
        if (source === 'in') {
          setAmountOut(recv ? formatNumber(recv) : '');
        } else {
          setAmountIn(pay ? formatNumber(pay) : '');
        }
        setMinReceiveAmount(minRecv || '');
        setPriceImpactPct(impactText);
      } catch (err) {
        if (seq !== requestSeq.current) return;
        setPriceImpactPct('');
        setMinReceiveAmount('');
      }
    },
    [pool?.poolState, pool?.pool_state, slippage, tokenIn?.mint, tokenOut?.mint]
  );

  const handleAmountInChange = (val) => {
    setAmountIn(val);
    requestQuote(val, 'in');
  };

  const handleAmountOutChange = (val) => {
    setAmountOut(val);
    requestQuote(val, 'out');
  };

  useEffect(() => {
    if (slippage === undefined || slippage === null) return;
    if (amountIn) {
      requestQuote(amountIn, 'in');
    } else if (amountOut) {
      requestQuote(amountOut, 'out');
    }
  }, [slippage]);

  const handleSwap = () => {
    if (!connected || !publicKey) {
      if (wallet?.adapter?.connect) {
        wallet.adapter.connect().catch(() => {
          /* ignore */
        });
      }
      toast({
        title: t('common.warning') || 'Warning',
        description: t('swapPage.connectHint'),
      });
      return;
    }
    if (!tokenIn || !tokenOut || !amountIn || Number(amountIn) <= 0) {
      toast({
        title: t('common.warning') || 'Warning',
        description: t('swapPage.fillAmount'),
      });
      return;
    }
    const poolState = pool?.poolState || pool?.pool_state;
    if (!poolState) {
      toast({
        title: t('common.error') || 'Error',
        description: 'Pool state not found',
        variant: 'destructive',
      });
      return;
    }
    const run = async () => {
      try {
        setSwapping(true);
        const slippageNum = Number(slippage);
        const slippageBps = Number.isFinite(slippageNum)
          ? Math.max(0, Math.min(10000, Math.round(slippageNum * 100)))
          : 0;
        const params = {
          chain_id: CHAIN_ID,
          pool_state: poolState,
          input_mint: tokenIn?.mint,
          output_mint: tokenOut?.mint,
          amount_in: amountIn,
          slippage_bps: slippageBps,
          user_wallet_address: publicKey?.toBase58(),
        };
        const resp = await buildCpmmSwapTx(params);
        const txBase64 = resp?.tx_base64 || resp?.txBase64 || resp?.tx;
        if (!txBase64) {
          throw new Error('Backend did not return transaction');
        }
        const raw = Uint8Array.from(atob(txBase64), (c) => c.charCodeAt(0));
        const tx = Transaction.from(raw);
        const sig = await sendTransaction(tx, solConnection);
        toast({
          title: t('swapPage.swapSubmitted') || 'Swap submitted',
          description: sig,
        });
        setAmountIn('');
        setAmountOut('');
        setPriceImpactPct('');
        setMinReceiveAmount('');
      } catch (e) {
        console.log(e)
        const rejected =
          e?.code === 4001 ||
          e?.code === 'ACTION_REJECTED' ||
          /reject/i.test(e?.message || '') ||
          /user rejected/i.test(e?.error || '') ||
          /user rejected/i.test(e?.cause || '');
        const baseMessage =
          e?.error ||
          e?.cause?.message ||
          (typeof e === 'string' ? e : 'Unknown error');
        const friendlyMessage = rejected
          ? t('swapPage.walletRejected') || 'User rejected the transaction'
          : baseMessage;
        let logs = e?.logs || e?.error?.logs || e?.cause?.logs || [];
        if ((!logs || !logs.length) && typeof e?.getLogs === 'function') {
          try {
            logs = await e.getLogs(solConnection);
          } catch (logErr) {
            console.warn('Failed to fetch logs for swap tx', logErr);
          }
        }
        console.error('CPMM swap failed', {
          amountIn,
          poolState,
          tokenIn,
          tokenOut,
          wallet: publicKey?.toBase58(),
          error: friendlyMessage,
          rawError: e,
          rawErrorMessage: e?.error || e?.cause || e?.message,
          rawErrorError: e?.error?.error,
          logs,
        });
        toast({
          title: t('swapPage.swapFailed') || 'Swap failed',
          description: friendlyMessage || String(e),
          variant: 'destructive',
        });
      } finally {
        setSwapping(false);
      }
    };
    run();
  };

  const poolName = `${tokenIn?.symbol || pool?.inputTokenSymbol || 'Token A'} / ${
    tokenOut?.symbol || pool?.outputTokenSymbol || 'Token B'
  }`;

  const quickSlippage = [0.1, 0.5, 1, 2];
  const feeText = pool?.tradeFeeRate ? `${(Number(pool.tradeFeeRate) / 10000).toFixed(2)}%` : '--';
  const minReceiveNum = Number(minReceiveAmount);

  const openTokenModal = (target) => {
    // 固定池子模式下禁用切换
    return setTokenModal({ open: false, target });
  };

  const fetchBalance = useCallback(
    async (mint, decimals = 9) => {
      if (!connected || !publicKey || !mint) return '--';
      const connection = solConnection;
      if (!connection) return '--';
      try {
        if (mint.toLowerCase() === 'sol' || mint === 'So11111111111111111111111111111111111111112') {
          const lamports = await connection.getBalance(publicKey);
          return formatNumber(lamports / 10 ** decimals, 6);
        }
        const mintPk = new PublicKey(mint);
        const resp = await connection.getParsedTokenAccountsByOwner(publicKey, {
          mint: mintPk,
        });
        const amount = resp?.value?.[0]?.account?.data?.parsed?.info?.tokenAmount;
        if (!amount) return '0';
        const ui = Number(amount.uiAmount);
        if (!Number.isFinite(ui)) return '0';
        return formatNumber(ui, 6);
      } catch (e) {
        console.warn('fetch balance failed', e);
        return '--';
      }
    },
    [solConnection, connected, publicKey]
  );

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const inBal = await fetchBalance(tokenIn?.mint, tokenIn?.decimals || 9);
      const outBal = await fetchBalance(tokenOut?.mint, tokenOut?.decimals || 9);
      if (!cancelled) {
        setBalanceIn(inBal);
        setBalanceOut(outBal);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [fetchBalance, tokenIn?.mint, tokenOut?.mint, tokenIn?.decimals, tokenOut?.decimals]);

  return (
    <div className="relative overflow-hidden">
      <div className="absolute inset-0 bg-gradient-to-br from-sky-500/10 via-indigo-600/10 to-purple-500/10 blur-3xl" />
      <div className="absolute -left-32 top-10 w-72 h-72 bg-cyan-400/20 blur-[120px] rounded-full" />
      <div className="absolute -right-24 bottom-10 w-80 h-80 bg-purple-500/20 blur-[120px] rounded-full" />
      <div className="relative z-10 container mx-auto px-4 py-6 max-w-5xl space-y-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Button
              variant="ghost"
              size="sm"
              onClick={onNavigateBack}
              className="h-10 px-3 rounded-full border border-border/60"
            >
              <ArrowLeft className="w-5 h-5 mr-2" />
              <span>{t('swapPage.back') || t('liquidityPage.back')}</span>
            </Button>
            <div>
              <h1 className="text-3xl font-bold text-foreground">{t('swapPage.title')}</h1>
              <p className="text-muted-foreground">{t('swapPage.subtitle')}</p>
            </div>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={refreshTokens}
            disabled={tokenLoading}
            className="flex items-center gap-2"
          >
            {tokenLoading ? <Loader2 className="w-4 h-4 animate-spin" /> : <RefreshCw className="w-4 h-4" />}
            <span>{t('tokenList.buttons.refresh')}</span>
          </Button>
        </div>

        <div className="grid lg:grid-cols-3 gap-4">
          <Card className="lg:col-span-2 bg-background/60 border border-white/10 shadow-xl backdrop-blur-xl relative overflow-hidden">
            <div className="absolute inset-0 bg-gradient-to-br from-white/5 via-transparent to-sky-500/5 pointer-events-none" />
            <CardHeader className="flex flex-row items-center justify-between">
              <div className="flex items-center gap-2">
                <Sparkles className="w-5 h-5 text-sky-400" />
                <CardTitle>{poolName}</CardTitle>
              </div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <ShieldCheck className="w-4 h-4" />
                <span>{t('swapPage.ratioHint')}</span>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="p-4 rounded-2xl bg-white/5 border border-white/10 space-y-3">
                <div className="flex items-center justify-between text-sm text-muted-foreground">
                  <span>{t('swapPage.fromLabel')}</span>
                  <div className="flex items-center gap-2">
                    <span>
                      {t('swapPage.balance')}: {balanceIn}
                    </span>
                    <Button variant="ghost" size="sm" className="h-7 px-2" onClick={() => handleAmountInChange(balanceIn)}>
                      MAX
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 px-2"
                      onClick={() => handleAmountInChange((Number(balanceIn) || 0) / 2)}
                    >
                      50%
                    </Button>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <TokenButton
                    token={tokenIn || { symbol: t('swapPage.selectToken') }}
                    onClick={() => openTokenModal('in')}
                    disabled
                  />
                  <div className="flex-1">
                    <Input
                      type="number"
                      inputMode="decimal"
                      placeholder={t('swapPage.placeholder')}
                      value={amountIn}
                      onChange={(e) => handleAmountInChange(e.target.value)}
                      className="h-14 text-2xl font-semibold text-right"
                    />
                  </div>
                </div>
              </div>

              <div className="p-4 rounded-2xl bg-white/5 border border-white/10 space-y-3">
                <div className="flex items-center justify-between text-sm text-muted-foreground">
                  <span>{t('swapPage.toLabel')}</span>
                  <div className="flex items-center gap-2">
                    <span>
                      {t('swapPage.balance')}: {balanceOut}
                    </span>
                    <Button variant="ghost" size="sm" className="h-7 px-2" onClick={() => handleAmountOutChange(balanceOut)}>
                      MAX
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 px-2"
                      onClick={() => handleAmountOutChange((Number(balanceOut) || 0) / 2)}
                    >
                      50%
                    </Button>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <TokenButton
                    token={tokenOut || { symbol: t('swapPage.selectToken') }}
                    onClick={() => openTokenModal('out')}
                    disabled
                  />
                  <div className="flex-1">
                    <Input
                      type="number"
                      inputMode="decimal"
                      placeholder={t('swapPage.placeholder')}
                      value={amountOut}
                      onChange={(e) => handleAmountOutChange(e.target.value)}
                      className="h-14 text-2xl font-semibold text-right"
                    />
                  </div>
                </div>
              </div>

              <div className="p-4 rounded-2xl bg-muted/40 flex flex-col gap-3 text-sm border border-border/60">
                <div className="flex items-center justify-between text-muted-foreground">
                  <div className="flex items-center gap-2">
                    <SlidersHorizontal className="w-4 h-4" />
                    <span>{t('swapPage.slippage')}</span>
                  </div>
                  <span className="text-foreground/70">{t('swapPage.autoSlippageHint')}</span>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  {quickSlippage.map((opt) => (
                    <Button
                      key={opt}
                      size="sm"
                      variant={Number(slippage) === opt ? 'default' : 'outline'}
                      className="h-9 px-3"
                      onClick={() => setSlippage(String(opt))}
                    >
                      {opt}%
                    </Button>
                  ))}
                  <Input
                    type="number"
                    inputMode="decimal"
                    className="w-24 h-9 text-right"
                    value={slippage}
                    onChange={(e) => setSlippage(e.target.value)}
                    placeholder="0.5"
                  />
                </div>
              </div>

              <Button
                className="w-full h-12 text-base font-semibold rounded-2xl shadow-lg shadow-sky-500/10"
                onClick={handleSwap}
                disabled={!tokenIn || !tokenOut || swapping}
              >
                {swapping
                  ? t('swapPage.submitting') || 'Submitting...'
                  : connected
                  ? t('swapPage.swapAction')
                  : t('header.wallet.connect')}
              </Button>
            </CardContent>
          </Card>

          <div className="space-y-3">
            <Card className="bg-background/60 border border-white/10 backdrop-blur-xl">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-lg">
                  <Sparkles className="w-5 h-5 text-amber-400" />
                  {t('swapPage.swapOverview')}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3 text-sm">
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground">{t('swapPage.minimumReceived')}</span>
                  <span className="font-semibold">
                    {Number.isFinite(minReceiveNum) && minReceiveNum > 0
                      ? `${formatNumber(minReceiveNum)} ${tokenOut?.symbol || '--'}`
                      : '--'}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground">{t('swapPage.priceImpact')}</span>
                  <span className="font-semibold">
                    {priceImpactPct ? `${priceImpactPct}%` : t('swapPage.priceImpactPending')}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground">{t('swapPage.feeLabel')}</span>
                  <span className="font-semibold">{feeText}</span>
                </div>
              </CardContent>
            </Card>
            <Card className="bg-background/60 border border-white/10 backdrop-blur-xl">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-lg">
                  <ArrowDownUp className="w-5 h-5 text-sky-400" />
                  {t('swapPage.routeHint')}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3 text-sm">
                <p className="text-muted-foreground">{t('swapPage.fixedDesc')}</p>
                <div className="flex items-center gap-2 text-foreground font-semibold">
                  <span>{tokenIn?.symbol || 'Token A'}</span>
                  <ArrowDownUp className="w-4 h-4" />
                  <span>{tokenOut?.symbol || 'Token B'}</span>
                </div>
                <button
                  type="button"
                  onClick={() => setInvertRate((v) => !v)}
                  className="w-full flex items-center justify-between rounded-xl border border-border/60 px-3 py-2 hover:bg-muted/40 transition-colors"
                >
                  <span className="text-muted-foreground">{t('swapPage.rateLabel')}</span>
                  <span className="font-semibold text-foreground flex items-center gap-2">
                    {displayRate && displayRate > 0
                      ? `1 ${invertRate ? tokenOut?.symbol || 'Token B' : tokenIn?.symbol || 'Token A'} ~= ${formatNumber(displayRate)} ${
                          invertRate ? tokenIn?.symbol || 'Token A' : tokenOut?.symbol || 'Token B'
                        }`
                      : t('swapPage.rateUnavailable')}
                    <ArrowDownUp className="w-4 h-4 text-muted-foreground" />
                  </span>
                </button>
              </CardContent>
            </Card>
          </div>
        </div>
      </div>

      <TokenSelectDialog
        open={tokenModal.open}
        onClose={(open) => setTokenModal((prev) => ({ ...prev, open }))}
        tokens={normalizedTokens}
        title={t('swapPage.searchToken')}
        onSelect={(token) => {
          if (tokenModal.target === 'in') {
            setTokenIn(token);
          } else {
            setTokenOut(token);
          }
        }}
      />
    </div>
  );
};

export default SwapPage;
