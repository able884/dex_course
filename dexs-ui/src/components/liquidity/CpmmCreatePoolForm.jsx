import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useWallet, useConnection } from '@solana/wallet-adapter-react';
import { Transaction, VersionedTransaction, SendTransactionError } from '@solana/web3.js';
import { WalletSendTransactionError } from '@solana/wallet-adapter-base';
import { Buffer } from 'buffer';
import {
  Loader2,
  ArrowLeftRight,
  Search,
  RefreshCw,
  AlertCircle,
  CheckCircle2,
  Clock,
  Globe2,
  Info,
  ChevronDown,
  Copy,
} from 'lucide-react';
import { Button } from '../UI/Button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '../UI/card';
import { Input } from '../UI/input';
import { Badge } from '../UI/badge';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../UI/select';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '../UI/dialog';
import { useToast } from '../../hooks/use-toast';
import { useCpmmConfig } from '../../hooks/useCpmmConfig';
import { useTranslation } from '../../i18n/LanguageContext';
import { cn } from '../../lib/utils';

const LIQUIDITY_PROXY_BASE = '/api/liquidity/cpmm';
const CPMM_TOKENS_ENDPOINT = `${LIQUIDITY_PROXY_BASE}/tokens`;
const CPMM_FEE_TIER_ENDPOINT = `${LIQUIDITY_PROXY_BASE}/fee-tiers`;
const CPMM_CREATE_ENDPOINT = `${LIQUIDITY_PROXY_BASE}/pools`;
const CHAIN_ID = 100000;

const shortenAddress = (address = '') => {
  if (!address) return '--';
  return `${address.slice(0, 4)}...${address.slice(-4)}`;
};

export const dedupeTokens = (tokens) => {
  const seen = new Set();
  return tokens.filter((token) => {
    if (!token?.mint) return false;
    const key = token.mint.toLowerCase();
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
};

export const normalizeTokenFromApi = (token) => {
  if (!token) return null;
  const mint =
    token.tokenMint ||
    token.token_mint ||
    token.tokenAddress ||
    token.token_address ||
    token.mint ||
    token.pairAddress ||
    '';
  if (!mint) return null;
  const symbol =
    token.tokenSymbol ||
    token.token_symbol ||
    token.symbol ||
    token.base_symbol ||
    token.tokenName ||
    'TOKEN';
  const name = token.tokenName || token.token_name || token.name || symbol;
  const decimals = token.decimals ?? token.tokenDecimals ?? token.token_decimals ?? 9;
  const logo = token.tokenIcon || token.token_icon || token.logo || '';
  return {
    mint,
    symbol: symbol.toUpperCase(),
    name,
    decimals,
    logo,
  };
};

const toDateInputValue = (date) => {
  if (!(date instanceof Date)) return '';
  const pad = (val) => `${val}`.padStart(2, '0');
  const year = date.getFullYear();
  const month = pad(date.getMonth() + 1);
  const day = pad(date.getDate());
  const hours = pad(date.getHours());
  const minutes = pad(date.getMinutes());
  return `${year}-${month}-${day}T${hours}:${minutes}`;
};

const formatUtcString = (date) => {
  if (!(date instanceof Date)) return '--';
  return `${date.toISOString().replace('T', ' ').slice(0, 16)} UTC`;
};

const formatPriceValue = (value) => {
  if (!value || value <= 0) return '';
  if (value >= 1) return value.toFixed(4);
  return value.toPrecision(4);
};

const deriveFriendlyRpcError = (errorMessage, summaryLine, logs) => {
  const allLines = [errorMessage, summaryLine, ...(logs || [])]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
  if (!allLines) {
    return '';
  }
  if (allLines.includes('insufficient funds')) {
    return '余额不足，请确保钱包中有足够的 SOL 与两种代币用于初始化池子。';
  }
  if (allLines.includes('this program may not be used for executing instructions')) {
    return 'Raydium 程序未在当前网络部署或 ProgramID 配置有误，请检查网络设置。';
  }
  if (allLines.includes('constraintraw')) {
    return 'Raydium 校验未通过，请确认选择的两种代币合法且满足 token0 < token1 的要求。';
  }
  if (allLines.includes('blockhash not found')) {
    return '交易已过期，请重新构建并提交。';
  }
  if (allLines.includes('already in use')) {
    return '池子地址已存在，可能已创建过该交易对，请使用已有池子或更换参数后重试。';
  }
  return '';
};

const pickErrorMessage = (error, fallback) => {
  const candidates = [
    error?.message,
    error?.error?.message,
    error?.cause?.message,
    typeof error === 'string' ? error : '',
    error?.cause?.toString?.(),
    error?.error?.toString?.(),
  ];
  for (const msg of candidates) {
    if (typeof msg === 'string' && msg.trim().length > 0) {
      return msg.trim();
    }
  }
  return fallback;
};

const extractRpcLogs = async (error, connection) => {
  if (!error) return [];
  const directLogs =
    error?.logs ||
    error?.error?.logs ||
    error?.cause?.logs;
  if (Array.isArray(directLogs) && directLogs.length) {
    return directLogs;
  }

  const candidates = [error, error?.error, error?.cause].filter(Boolean);
  for (const candidate of candidates) {
    if (typeof candidate?.getLogs === 'function') {
      try {
        const logs = await candidate.getLogs(connection);
        if (Array.isArray(logs) && logs.length) {
          return logs;
        }
      } catch (logErr) {
        console.warn('Failed to fetch RPC logs for CPMM initialize', logErr);
      }
    }
  }
  return [];
};

const summarizeLogs = (logs) => {
  if (!logs?.length) {
    return '';
  }
  return (
    logs.find((line) => line.includes('AnchorError')) ||
    logs.find((line) => line.includes('custom program error')) ||
    logs[logs.length - 1] ||
    ''
  );
};

const simulateTransactionForLogs = async (connection, transaction) => {
  if (!connection || !transaction) return { logs: [], error: null };
  try {
    const simResult = await connection.simulateTransaction(transaction, {
      commitment: 'processed',
      sigVerify: false,
      replaceRecentBlockhash: true,
    });
    return {
      logs: simResult.value?.logs || [],
      error: simResult.value?.err || null,
    };
  } catch (simErr) {
    console.warn('CPMM simulateTransaction failed', simErr);
    return { logs: [], error: null };
  }
};

const findSendTransactionError = (error) => {
  if (!error) return null;
  if (error instanceof SendTransactionError) {
    return error;
  }
  if (error instanceof WalletSendTransactionError && error.error) {
    const nested = error.error;
    if (nested instanceof SendTransactionError) {
      return nested;
    }
  }
  if (error?.error instanceof SendTransactionError) {
    return error.error;
  }
  if (error?.cause instanceof SendTransactionError) {
    return error.cause;
  }
  if (error?.cause instanceof WalletSendTransactionError && error.cause.error instanceof SendTransactionError) {
    return error.cause.error;
  }
  if (error?.error?.cause instanceof SendTransactionError) {
    return error.error.cause;
  }
  if (error?.cause?.error instanceof SendTransactionError) {
    return error.cause.error;
  }
  return null;
};

const deserializeTransaction = (txHash) => {
  const buffer = Buffer.from(txHash, 'base64');
  try {
    return { type: 'legacy', transaction: Transaction.from(buffer) };
  } catch (legacyError) {
    try {
      return { type: 'versioned', transaction: VersionedTransaction.deserialize(buffer) };
    } catch (versionedError) {
      throw new Error('Unable to decode transaction from backend');
    }
  }
};

const TokenInputRow = ({
  label,
  token,
  amount,
  onAmountChange,
  onSelect,
  placeholder,
  t,
}) => {
  const symbol = token?.symbol || t('liquidityPage.form.selectToken');
  const badgeText = symbol?.slice(0, 1)?.toUpperCase() || '?';
  return (
    <div className="space-y-2">
      <label className="text-sm font-medium text-muted-foreground">{label}</label>
      <div className="flex flex-col sm:flex-row sm:items-center gap-3">
        <button
          type="button"
          onClick={onSelect}
          className="flex items-center gap-3 border rounded-2xl px-4 h-14 min-w-[220px] bg-background hover:bg-muted/50 transition-colors text-left"
        >
          <div className="w-10 h-10 rounded-full bg-muted flex items-center justify-center text-base font-semibold text-foreground">
            {badgeText}
          </div>
          <span className="text-lg font-semibold">{symbol}</span>
          <ChevronDown className="w-4 h-4 text-muted-foreground ml-auto" />
        </button>
        <Input
          type="number"
          inputMode="decimal"
          placeholder={placeholder}
          value={amount}
          onChange={(e) => onAmountChange(e.target.value)}
          className="flex-1 h-14 text-right text-2xl font-semibold"
        />
      </div>
    </div>
  );
};

const CpmmCreatePoolForm = () => {
  const { publicKey, connected, wallet, signTransaction: walletSignTransaction } = useWallet();
  const { connection } = useConnection();
  const { toast } = useToast();
  const { t } = useTranslation();

  const [baseToken, setBaseToken] = useState(null);
  const [quoteToken, setQuoteToken] = useState(null);
  const [baseAmount, setBaseAmount] = useState('');
  const [quoteAmount, setQuoteAmount] = useState('');
  const [selectedFeeTier, setSelectedFeeTier] = useState('');
  const {
    tokens: remoteTokens,
    tokenLoading,
    tokenError,
    refreshTokens,
    feeTiers: remoteFeeTiers,
    feeLoading: feeTierLoading,
    feeError: feeTierError,
    refreshFees,
  } = useCpmmConfig({
    tokensEndpoint: `${CPMM_TOKENS_ENDPOINT}?chain_id=${CHAIN_ID}`,
    feeEndpoint: `${CPMM_FEE_TIER_ENDPOINT}?chain_id=${CHAIN_ID}&poolType=cpmm`,
    fallbackTokens: [],
    fallbackFees: [],
  });
  const [txSignature, setTxSignature] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [startMode, setStartMode] = useState('now');
  const [customStartTime, setCustomStartTime] = useState(new Date(Date.now() + 10 * 60 * 1000));
  const [tokenModalOpen, setTokenModalOpen] = useState(false);
  const [tokenModalType, setTokenModalType] = useState('base');
  const [priceView, setPriceView] = useState('quotePerBase');

  const tokenList = useMemo(
    () =>
      dedupeTokens(
        Array.isArray(remoteTokens)
          ? remoteTokens.map(normalizeTokenFromApi).filter(Boolean)
          : []
      ),
    [remoteTokens]
  );

  const normalizedFeeTiers = useMemo(() => {
    if (!Array.isArray(remoteFeeTiers) || !remoteFeeTiers.length) {
      return [];
    }
    return remoteFeeTiers
      .map((tier) => {
        const configIndex = tier?.config_index ?? tier?.configIndex ?? 0;
        const numeric = Number(configIndex);
        const label = tier?.label || `${(numeric / 100).toFixed(2)}%`;
        return {
          value: String(configIndex),
          label,
          description: tier?.description || '',
          tickSpacing: tier?.tickSpacing ?? tier?.tick_spacing,
        };
      })
      .filter(Boolean);
  }, [remoteFeeTiers]);

  useEffect(() => {
    if (!selectedFeeTier && normalizedFeeTiers.length) {
      setSelectedFeeTier(normalizedFeeTiers[0].value);
    }
  }, [normalizedFeeTiers, selectedFeeTier]);

  useEffect(() => {
    if (!tokenList.length) return;
    setBaseToken((prev) => prev || tokenList[0]);
  }, [tokenList]);

  useEffect(() => {
    if (!tokenList.length) return;
    setQuoteToken((prev) => {
      if (prev) return prev;
      const fallback =
        tokenList.find((token) => token.mint !== (baseToken?.mint || tokenList[0].mint)) ||
        tokenList[0];
      return fallback;
    });
  }, [tokenList, baseToken]);

  const priceValue = useMemo(() => {
    const base = parseFloat(baseAmount);
    const quote = parseFloat(quoteAmount);
    if (Number.isFinite(base) && base > 0 && Number.isFinite(quote) && quote > 0) {
      return quote / base;
    }
    return null;
  }, [baseAmount, quoteAmount]);

  const baseSymbol = baseToken?.symbol || t('liquidityPage.form.baseTokenShort');
  const quoteSymbol = quoteToken?.symbol || t('liquidityPage.form.quoteTokenShort');

  const orientedPrice = useMemo(() => {
    if (!priceValue || priceValue <= 0) return null;
    return priceView === 'quotePerBase' ? priceValue : 1 / priceValue;
  }, [priceValue, priceView]);

  const priceInputValue = orientedPrice ? formatPriceValue(orientedPrice) : '';
  const priceHintPrimary = priceView === 'quotePerBase' ? baseSymbol : quoteSymbol;
  const priceHintSecondary = priceView === 'quotePerBase' ? quoteSymbol : baseSymbol;
  const priceHintText = `1 ${priceHintPrimary} ~= ${priceInputValue || '--'} ${priceHintSecondary}`;

  const selectedFeeTierInfo = useMemo(
    () => normalizedFeeTiers.find((tier) => tier.value === selectedFeeTier),
    [normalizedFeeTiers, selectedFeeTier]
  );

  const startTimestamp =
    startMode === 'now'
      ? Math.floor(Date.now() / 1000)
      : Math.floor(customStartTime.getTime() / 1000);

  const tokensAreSame =
    baseToken?.mint && quoteToken?.mint && baseToken.mint === quoteToken.mint;

  const isFormReady =
    !tokensAreSame &&
    baseToken &&
    quoteToken &&
    priceValue &&
    parseFloat(baseAmount) > 0 &&
    parseFloat(quoteAmount) > 0 &&
    selectedFeeTier;

  const handleTogglePriceView = () => {
    setPriceView((prev) => (prev === 'quotePerBase' ? 'basePerQuote' : 'quotePerBase'));
  };

  const handleCreatePool = async () => {
    if (!connected || !publicKey) {
      toast({
        title: t('liquidityPage.toast.connectWalletTitle'),
        description: t('liquidityPage.toast.connectWalletDesc'),
        variant: 'destructive',
      });
      return;
    }

    if (!isFormReady) {
      toast({
        title: t('liquidityPage.toast.incompleteTitle'),
        description: t('liquidityPage.toast.incompleteDesc'),
        variant: 'destructive',
      });
      return;
    }

    setIsSubmitting(true);
    setTxSignature('');

    let preparedTransaction = null;

    try {
      const payload = {
        chain_id: CHAIN_ID,
        pool_type: 'CPMM',
        base_token_mint: baseToken.mint,
        quote_token_mint: quoteToken.mint,
        base_amount: baseAmount,
        quote_amount: quoteAmount,
        initial_price: priceValue.toString(),
        config_index: Number(selectedFeeTier),
        start_time: startTimestamp,
        user_wallet_address: publicKey.toString(),
      };

      const response = await fetch(CPMM_CREATE_ENDPOINT, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        throw new Error(await response.text());
      }

      const result = await response.json();
      const txBase64 =
        result?.data?.txBase64 ||
        result?.data?.tx_base64 ||
        result?.txBase64 ||
        result?.tx_base64 ||
        result?.data?.txHash ||
        result?.data?.tx_hash ||
        result?.txHash;
      if (!txBase64) {
        throw new Error(t('liquidityPage.errors.missingTransaction'));
      }

      const { transaction } = deserializeTransaction(txBase64);

      if (transaction instanceof Transaction) {
        transaction.feePayer = transaction.feePayer || publicKey;
        if (!transaction.recentBlockhash) {
          const { blockhash } = await connection.getLatestBlockhash();
          transaction.recentBlockhash = blockhash;
        }
      }
      preparedTransaction = transaction;

      const supportsVersioned = wallet?.adapter?.supportedTransactionVersions;
      const walletSupportsV0 =
        supportsVersioned === 'all' ||
        (supportsVersioned instanceof Set && supportsVersioned.has(0));

      const isVersionedTx = transaction instanceof VersionedTransaction;

      let signedTx;
      if (isVersionedTx) {
        if (!walletSupportsV0) {
          throw new Error('Current wallet does not support versioned transactions.');
        }
        if (!wallet?.adapter?.signTransaction) {
          throw new Error('Wallet adapter cannot sign versioned transactions.');
        }
        signedTx = await wallet.adapter.signTransaction(transaction);
      } else {
        if (walletSignTransaction) {
          signedTx = await walletSignTransaction(transaction);
        } else if (wallet?.adapter?.signTransaction) {
          signedTx = await wallet.adapter.signTransaction(transaction);
        } else {
          throw new Error('Current wallet cannot sign transactions.');
        }
      }

      preparedTransaction = signedTx;

      const rawTx = signedTx.serialize();
      let signature;
      try {
        signature = await connection.sendRawTransaction(rawTx, {
          skipPreflight: false,
          preflightCommitment: 'processed',
          maxRetries: 5,
        });
      } catch (sendErr) {
        throw sendErr;
      }
      setTxSignature(signature);

      toast({
        title: t('liquidityPage.toast.transactionSent'),
        description: t('liquidityPage.toast.waitingConfirmation'),
      });

      await connection.confirmTransaction(signature, 'confirmed');

      toast({
        title: t('liquidityPage.toast.poolCreated'),
        description: t('liquidityPage.toast.transactionConfirmed', {
          signature: `${signature.slice(0, 4)}...${signature.slice(-4)}`,
        }),
      });
    } catch (err) {
      const defaultMessage = t?.('liquidityPage.errors.transactionFailed') || 'Failed to send transaction';
      const errorMessage = pickErrorMessage(err, defaultMessage);
      let rpcLogs = await extractRpcLogs(err, connection);
      let summaryLine = summarizeLogs(rpcLogs);
      const signature = err?.signature || err?.cause?.signature;
      const sendTxError = findSendTransactionError(err);
      const details = {
        message: errorMessage,
        signature,
        summary: summaryLine,
        logs: rpcLogs,
        cause: err?.cause,
        rawError: err,
        sendError: sendTxError,
      };
      // For direct sendRawTransaction errors, try to add logs via helper similar to BuyModal
      let simErrInfo = null;

      if (!rpcLogs.length && sendTxError) {
        try {
          const logs = await sendTxError.getLogs(connection);
          if (logs?.length) {
            rpcLogs = logs;
            summaryLine = summarizeLogs(logs);
            details.logs = rpcLogs;
            details.summary = summaryLine;
          }
        } catch (logErr) {
          console.warn('Failed to fetch logs from SendTransactionError', logErr);
        }
      }

      let simulationPerformed = false;
      if (!rpcLogs.length && preparedTransaction && connection) {
        const simData = await simulateTransactionForLogs(connection, preparedTransaction);
        simulationPerformed = true;
        if (simData.logs.length) {
          rpcLogs = simData.logs;
          summaryLine = summarizeLogs(rpcLogs);
          details.logs = rpcLogs;
          details.summary = summaryLine;
        }
        if (simData.error) {
          simErrInfo = simData.error;
        }
      }

      console.error('Failed to initialize CPMM pool', details);
      if (rpcLogs.length) {
        const shortLogs = rpcLogs.slice(-12);
        console.error('CPMM initialize RPC logs (last 12 entries):\n', shortLogs.join('\n'));
      }
      if (simErrInfo || simulationPerformed) {
        console.error('CPMM initialize simulation result:', {
          error: simErrInfo,
          logs: rpcLogs,
        });
      }
      const friendlyMessage = deriveFriendlyRpcError(errorMessage, summaryLine, rpcLogs);
      const toastMessage =
        friendlyMessage || (summaryLine ? `${errorMessage}\n${summaryLine}` : errorMessage);
      toast({
        title: t('liquidityPage.toast.createFailed'),
        description: toastMessage,
        variant: 'destructive',
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleTokenSelect = (token) => {
    if (tokenModalType === 'base') {
      setBaseToken(token);
    } else {
      setQuoteToken(token);
    }
    setTokenModalOpen(false);
  };

  return (
    <>
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2">
          <Card className="border border-border/60 shadow-sm">
            <CardContent className="p-6 space-y-6">
              {tokenError && (
                <div className="flex items-center justify-between border border-amber-200 bg-amber-50 text-amber-700 px-4 py-2 rounded-lg text-sm">
                  <div className="flex items-center gap-2">
                    <AlertCircle className="w-4 h-4" />
                    <span>{tokenError}</span>
                  </div>
                  <Button variant="ghost" size="sm" onClick={refreshTokens}>
                    <RefreshCw className="w-4 h-4 mr-1" />
                    {t('liquidityPage.actions.retry')}
                  </Button>
                </div>
              )}

              <div className="space-y-6">
                <TokenInputRow
                  label={t('liquidityPage.form.baseToken')}
                  token={baseToken}
                  amount={baseAmount}
                  onAmountChange={setBaseAmount}
                  onSelect={() => {
                    setTokenModalType('base');
                    setTokenModalOpen(true);
                  }}
                  placeholder="0.0"
                  t={t}
                />
                <TokenInputRow
                  label={t('liquidityPage.form.quoteToken')}
                  token={quoteToken}
                  amount={quoteAmount}
                  onAmountChange={setQuoteAmount}
                  onSelect={() => {
                    setTokenModalType('quote');
                    setTokenModalOpen(true);
                  }}
                  placeholder="0.0"
                  t={t}
                />

                {tokensAreSame && (
                  <div className="flex items-center gap-2 text-sm text-red-600">
                    <AlertCircle className="w-4 h-4" />
                    {t('liquidityPage.form.tokensMustDiffer')}
                  </div>
                )}

                <div className="space-y-4">
                  <div className="flex flex-col gap-4 lg:flex-row">
                    <div className="flex-1 rounded-2xl border border-border/60 bg-card/40 p-4 space-y-3">
                      <div className="flex items-center justify-between text-sm text-muted-foreground">
                        <span>{t('liquidityPage.form.initialPrice')}</span>
                        <span className="font-medium text-foreground">
                          {baseSymbol}/{quoteSymbol}
                        </span>
                      </div>
                      <Input
                        value={priceInputValue}
                        readOnly
                        className="h-14 text-2xl font-semibold"
                      />
                      <button
                        type="button"
                        onClick={handleTogglePriceView}
                        className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition"
                      >
                        <ArrowLeftRight className="w-4 h-4" />
                        {priceHintText}
                      </button>
                    </div>
                    <div className="flex-1 rounded-2xl border border-border/60 bg-card/40 p-4 space-y-3">
                      <div className="flex items-center justify-between text-sm text-muted-foreground">
                        <span>{t('liquidityPage.form.feeTier')}</span>
                        <span className="font-medium text-foreground">
                          {selectedFeeTierInfo?.label || '--'}
                        </span>
                      </div>
                      <div className="flex flex-col sm:flex-row gap-3 sm:items-stretch">
                        <Select value={selectedFeeTier} onValueChange={setSelectedFeeTier}>
                          <SelectTrigger className="h-14 min-w-[200px] justify-between text-2xl font-semibold rounded-2xl">
                            {feeTierLoading ? (
                              <div className="flex items-center gap-2">
                                <Loader2 className="w-4 h-4 animate-spin" />
                                {t('liquidityPage.form.loadingFees')}
                              </div>
                            ) : (
                              <SelectValue placeholder={t('liquidityPage.form.selectFeeTier')} />
                            )}
                          </SelectTrigger>
                          <SelectContent>
                            {normalizedFeeTiers.map((tier) => (
                              <SelectItem key={tier.value} value={tier.value}>
                                {tier.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        {feeTierError && (
                          <Button
                            type="button"
                            variant="outline"
                            onClick={refreshFees}
                            className="h-14 min-w-[140px] rounded-2xl flex items-center justify-center gap-2 text-sm font-medium"
                          >
                            <RefreshCw className="w-4 h-4" />
                            {t('liquidityPage.actions.retry')}
                          </Button>
                        )}
                      </div>
                      <p className="text-sm text-muted-foreground flex items-center gap-2">
                        <Info className="w-4 h-4" />
                        {selectedFeeTierInfo?.description || t('liquidityPage.form.feeDescription')}
                      </p>
                    </div>
                  </div>

                  <div className="rounded-2xl border border-border/60 bg-card/40 p-4 space-y-3">
                    <div className="inline-flex rounded-full border border-border/60 bg-background p-1">
                      <button
                        type="button"
                        className={cn(
                          'px-4 py-1 text-sm font-medium rounded-full transition',
                          startMode === 'now'
                            ? 'bg-primary text-white shadow'
                            : 'text-muted-foreground'
                        )}
                        onClick={() => setStartMode('now')}
                      >
                        {t('liquidityPage.form.startNow')}
                      </button>
                      <button
                        type="button"
                        className={cn(
                          'px-4 py-1 text-sm font-medium rounded-full transition',
                          startMode === 'custom'
                            ? 'bg-primary text-white shadow'
                            : 'text-muted-foreground'
                        )}
                        onClick={() => setStartMode('custom')}
                      >
                        {t('liquidityPage.form.custom')}
                      </button>
                    </div>
                    {startMode === 'custom' ? (
                      <div className="space-y-2">
                        <Input
                          type="datetime-local"
                          value={toDateInputValue(customStartTime)}
                          onChange={(e) => {
                            const next = new Date(e.target.value);
                            if (!Number.isNaN(next.getTime())) {
                              setCustomStartTime(next);
                            }
                          }}
                        />
                        <p className="text-xs text-muted-foreground flex items-center gap-1">
                          <Globe2 className="w-3 h-3" />
                          {formatUtcString(customStartTime)}
                        </p>
                      </div>
                    ) : (
                      <p className="text-xs text-muted-foreground flex items-center gap-1">
                        <Clock className="w-3 h-3" />
                        {t('liquidityPage.form.startImmediate')}
                      </p>
                    )}
                  </div>
                </div>

                <Button
                  onClick={handleCreatePool}
                  disabled={!isFormReady || isSubmitting}
                  className="w-full h-12 text-base font-semibold"
                >
                  {isSubmitting ? (
                    <>
                      <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                      {t('liquidityPage.actions.initializing')}
                    </>
                  ) : (
                    t('liquidityPage.actions.initialize')
                  )}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>

        <div className="space-y-6 lg:flex lg:flex-col">
          <Card className="lg:flex-1">
            <CardHeader>
              <CardTitle>{t('liquidityPage.summary.title')}</CardTitle>
              <CardDescription>{t('liquidityPage.summary.subtitle')}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">{t('liquidityPage.summary.network')}</span>
                <div className="flex items-center gap-2">
                  <Badge variant="outline">Solana Devnet</Badge>
                </div>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">{t('liquidityPage.summary.wallet')}</span>
                <span className="font-mono text-xs">
                  {publicKey ? shortenAddress(publicKey.toBase58()) : t('liquidityPage.summary.notConnected')}
                </span>
              </div>
              <div className="border-t pt-4 space-y-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('liquidityPage.summary.base')}</span>
                  <span className="font-semibold">
                    {baseAmount || '--'} {baseToken?.symbol || ''}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('liquidityPage.summary.quote')}</span>
                  <span className="font-semibold">
                    {quoteAmount || '--'} {quoteToken?.symbol || ''}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('liquidityPage.summary.fee')}</span>
                  <span className="font-semibold">
                    {selectedFeeTierInfo?.label || '--'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('liquidityPage.summary.start')}</span>
                  <span className="font-semibold">
                    {startMode === 'now'
                      ? t('liquidityPage.summary.startImmediate')
                      : formatUtcString(customStartTime)}
                  </span>
                </div>
              </div>
            </CardContent>
          </Card>

          {txSignature && (
            <Card className="border border-green-200 bg-green-50 dark:bg-green-950/40 dark:border-green-900">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-green-700 dark:text-green-300">
                  <CheckCircle2 className="w-5 h-5" />
                  {t('liquidityPage.summary.successTitle')}
                </CardTitle>
                <CardDescription>
                  {t('liquidityPage.summary.successDesc')}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="text-xs break-all bg-background/80 px-3 py-2 rounded-lg flex items-start justify-between gap-2">
                  <span>{txSignature}</span>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-6 w-6"
                    onClick={() => navigator.clipboard?.writeText(txSignature)}
                  >
                    <Copy className="w-3 h-3" />
                  </Button>
                </div>
                <Button
                  variant="outline"
                  className="w-full"
                  asChild
                >
                  <a
                    href={`https://solscan.io/tx/${txSignature}?cluster=devnet`}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {t('liquidityPage.summary.viewOnExplorer')}
                  </a>
                </Button>
              </CardContent>
            </Card>
          )}
        </div>
      </div>

      <TokenSelectorModal
        open={tokenModalOpen}
        mode={tokenModalType}
        tokens={tokenList}
        loading={tokenLoading}
        selectedMint={tokenModalType === 'base' ? baseToken?.mint : quoteToken?.mint}
        onClose={() => setTokenModalOpen(false)}
        onSelect={handleTokenSelect}
        t={t}
      />
    </>
  );
};

const TOKEN_ITEM_HEIGHT = 72;
const VISIBLE_ROWS = 12;
const BUFFER_ROWS = 6;

const TokenSelectorModal = ({
  open,
  mode,
  tokens,
  loading,
  selectedMint,
  onClose,
  onSelect,
  t,
}) => {
  const [search, setSearch] = useState('');
  const [customMint, setCustomMint] = useState('');
  const [customSymbol, setCustomSymbol] = useState('');
  const [scrollTop, setScrollTop] = useState(0);

  useEffect(() => {
    if (!open) {
      setSearch('');
      setCustomMint('');
      setCustomSymbol('');
      setScrollTop(0);
    }
  }, [open]);

  const filteredTokens = useMemo(() => {
    if (!search) return tokens;
    return tokens.filter(
      (token) =>
        token.symbol?.toLowerCase().includes(search.toLowerCase()) ||
        token.name?.toLowerCase().includes(search.toLowerCase()) ||
        token.mint?.toLowerCase().includes(search.toLowerCase())
    );
  }, [tokens, search]);

  const handleCustomSelect = () => {
    const trimmedMint = customMint.trim();
    if (!trimmedMint) return;
    onSelect({
      symbol: customSymbol || shortenAddress(trimmedMint),
      name: customSymbol || 'Custom Token',
      mint: trimmedMint,
      decimals: 9,
    });
  };

  const handleScrollCapture = useCallback((event) => {
    event.stopPropagation();
  }, []);

  const handleVirtualScroll = useCallback((event) => {
    setScrollTop(event.currentTarget.scrollTop);
  }, []);

  const totalItems = filteredTokens.length;
  const virtualStart = Math.max(
    0,
    Math.floor(scrollTop / TOKEN_ITEM_HEIGHT) - BUFFER_ROWS
  );
  const virtualEnd = Math.min(
    totalItems,
    virtualStart + VISIBLE_ROWS + BUFFER_ROWS * 2
  );
  const visibleTokens = filteredTokens.slice(virtualStart, virtualEnd);
  const paddingTop = virtualStart * TOKEN_ITEM_HEIGHT;
  const paddingBottom = Math.max(
    0,
    (totalItems - virtualEnd) * TOKEN_ITEM_HEIGHT
  );

  return (
    <Dialog open={open} onOpenChange={(value) => !value && onClose()}>
      <DialogContent className="max-h-[85vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>
            {mode === 'base'
              ? t('liquidityPage.tokenModal.selectBase')
              : t('liquidityPage.tokenModal.selectQuote')}
          </DialogTitle>
          <DialogDescription>
            {t('liquidityPage.tokenModal.description')}
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-center gap-2">
          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-3 h-4 w-4 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t('liquidityPage.tokenModal.searchPlaceholder')}
              className="pl-8"
            />
          </div>
          {search && (
            <Button variant="ghost" size="sm" onClick={() => setSearch('')}>
              {t('liquidityPage.actions.clear')}
            </Button>
          )}
        </div>

        <div
          className="flex-1 overflow-y-auto mt-4 pr-1 overscroll-contain"
          onWheelCapture={handleScrollCapture}
          onTouchMoveCapture={handleScrollCapture}
          onScroll={handleVirtualScroll}
        >
          {loading ? (
            <div className="flex items-center justify-center py-8 text-muted-foreground">
              <Loader2 className="w-4 h-4 mr-2 animate-spin" />
              {t('liquidityPage.tokenModal.loading')}
            </div>
          ) : filteredTokens.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              {t('liquidityPage.tokenModal.empty')}
            </div>
          ) : (
            <div style={{ height: totalItems * TOKEN_ITEM_HEIGHT }}>
              <div
                className="space-y-2"
                style={{
                  paddingTop,
                  paddingBottom,
                }}
              >
                {visibleTokens.map((token) => {
                  const active = token.mint === selectedMint;
                  return (
                    <button
                      key={token.mint}
                      type="button"
                      onClick={() => onSelect(token)}
                      className={cn(
                        'w-full border rounded-xl px-4 py-3 text-left hover:bg-muted/70 transition-colors',
                        active && 'border-primary bg-primary/10'
                      )}
                    >
                      <div className="flex items-center justify-between">
                        <div>
                          <p className="font-semibold">{token.symbol}</p>
                          <p className="text-xs text-muted-foreground">{token.name}</p>
                        </div>
                        <Badge variant="outline">{shortenAddress(token.mint)}</Badge>
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>
          )}
        </div>

        <div className="border-t pt-4 mt-4 space-y-3">
          <p className="text-sm font-medium">{t('liquidityPage.tokenModal.manualTitle')}</p>
          <Input
            value={customMint}
            onChange={(e) => setCustomMint(e.target.value)}
            placeholder={t('liquidityPage.tokenModal.manualMint')}
          />
          <Input
            value={customSymbol}
            onChange={(e) => setCustomSymbol(e.target.value)}
            placeholder={t('liquidityPage.tokenModal.manualSymbol')}
          />
          <Button onClick={handleCustomSelect} disabled={!customMint}>
            {t('liquidityPage.tokenModal.addCustom')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
};

export default CpmmCreatePoolForm;
