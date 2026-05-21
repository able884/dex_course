import React, { useEffect, useMemo, useRef, useState, useCallback } from 'react';
import { ArrowLeft, ArrowLeftRight, RefreshCw, Wallet2, Settings2, TrendingUp } from 'lucide-react';
import { useConnection, useWallet } from '@solana/wallet-adapter-react';
import { useWalletModal } from '@solana/wallet-adapter-react-ui';
import { LAMPORTS_PER_SOL, PublicKey, Transaction, VersionedTransaction } from '@solana/web3.js';
import { Buffer } from 'buffer';

import TokenPairAvatar from './TokenPairAvatar';
import ClmmLiquidityChart from './ClmmLiquidityChart';
import { Button } from '../UI/Button';
import { Input } from '../UI/input';
import { Badge } from '../UI/badge';
import { Card, CardContent, CardHeader, CardTitle } from '../UI/card';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../UI/dialog';
import { formatCurrencyUSD, formatPercent } from '../../utils/format';
import { useToast } from '../../hooks/use-toast';
import { cn } from '../../lib/utils';
import { useTranslation } from '../../i18n/LanguageContext';
import { getPoolDetail, getClmmPoolDepthData, getClmmPoolPriceRange, buildClmmOpenPositionTx } from '../../api/pools';

const normalizeFeePercent = (tradeFeeRate) => {
  const n = Number(tradeFeeRate);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return n / 10000;
};

const formatAmountForInput = (val) => {
  const num = Number(val);
  if (!Number.isFinite(num) || num <= 0) return '';
  const formatted = Math.abs(num) >= 1 ? num.toFixed(6) : num.toPrecision(6);
  return formatted.replace(/\.?0+$/, '');
};

const sortTokensByMint = (tokenA, tokenB) => {
  const mintA = tokenA?.mint || '';
  const mintB = tokenB?.mint || '';
  if (!mintA || !mintB) return { token0: tokenA, token1: tokenB };
  return mintA < mintB ? { token0: tokenA, token1: tokenB } : { token0: tokenB, token1: tokenA };
};

const TokenInput = ({ token, amount, onChange, balance, onFill, connected, t, label, disabled, overlayMessage }) => {
  const symbol = token?.symbol || t('depositPage.tokenPlaceholder');
  const displayBalance =
    balance === null || balance === undefined ? '--' : Number(balance).toFixed(4);

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
            disabled={!connected || disabled}
            onClick={() => onFill?.(0.5)}
          >
            50%
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 px-2"
            disabled={!connected || disabled}
            onClick={() => onFill?.(1)}
          >
            {t('depositPage.max')}
          </Button>
        </div>
      </div>
      <div className="relative flex items-center gap-4 border rounded-xl bg-card/70 px-4 py-3 border-border/60">
        {disabled && overlayMessage && (
          <div className="absolute inset-0 bg-background/90 backdrop-blur-sm rounded-xl flex items-center justify-center z-10">
            <div className="text-center px-4">
              <p className="text-sm text-muted-foreground">{overlayMessage}</p>
            </div>
          </div>
        )}
        <div className="flex items-center gap-3 min-w-[140px]">
          {renderIcon()}
          <div className="flex flex-col">
            <span className="text-sm text-muted-foreground">{label || symbol}</span>
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
          disabled={disabled}
        />
      </div>
    </div>
  );
};

const ClmmPoolDeposit = ({ pool, onNavigateBack }) => {
  const { publicKey, connected, wallet, sendTransaction, signTransaction: walletSignTransaction } = useWallet();
  const { connection } = useConnection();
  const { toast } = useToast();
  const { t } = useTranslation();
  const { setVisible: setWalletModalVisible } = useWalletModal();

  const [amount0, setAmount0] = useState('');
  const [amount1, setAmount1] = useState('');
  const [slippage, setSlippage] = useState('2.5');
  const [slippageDialogOpen, setSlippageDialogOpen] = useState(false);
  const [priceInverted, setPriceInverted] = useState(false);
  const [timeRange, setTimeRange] = useState('24H');
  const [minPrice, setMinPrice] = useState('');
  const [maxPrice, setMaxPrice] = useState('');
  const [balance0, setBalance0] = useState(null);
  const [balance1, setBalance1] = useState(null);
  const [fetchingBalance, setFetchingBalance] = useState(false);
  const [poolDetail, setPoolDetail] = useState(null);
  const [loadingPool, setLoadingPool] = useState(false);
  const [poolError, setPoolError] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);
  const lastManualInputRef = useRef(null); // 'amount0' | 'amount1' | null

  const poolState = pool?.poolState;
  const poolVersion = pool?.poolVersion;
  const walletAddress = useMemo(
    () => (connected && publicKey ? publicKey?.toBase58?.() || publicKey?.toString?.() : ''),
    [connected, publicKey]
  );

  const resolvedPool = useMemo(() => (poolDetail ? { ...pool, ...poolDetail } : pool), [pool, poolDetail]);

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

  const { token0, token1 } = useMemo(() => sortTokensByMint(tokenA, tokenB), [tokenA, tokenB]);
  const token0Symbol = (priceInverted ? token1 : token0)?.symbol || 'Token0';
  const token1Symbol = (priceInverted ? token0 : token1)?.symbol || 'Token1';

  // 原始价格（未反转）
  const rawCurrentPrice = useMemo(() => {
    const price = Number(
      resolvedPool?.price ||
        resolvedPool?.quotePerBase ||
        resolvedPool?.quote_per_base ||
        resolvedPool?.marketPrice ||
        resolvedPool?.market_price ||
        1
    );
    if (!Number.isFinite(price) || price <= 0) return 1;
    return price;
  }, [resolvedPool]);

  // 显示用的当前价格（考虑反转）
  const currentPrice = useMemo(() => {
    return priceInverted ? 1 / rawCurrentPrice : rawCurrentPrice;
  }, [rawCurrentPrice, priceInverted]);

  // 计算是否需要禁用输入框
  const priceMinNum = Number(minPrice) || 0;
  const priceMaxNum = Number(maxPrice) || 0;
  const shouldDisableToken0 = priceMaxNum > 0 && priceMaxNum < currentPrice;
  const shouldDisableToken1 = priceMinNum > 0 && priceMinNum > currentPrice;
  const overlayMessage = 'The market price is outside your specified price range. Single asset deposit only.';

  const feePercent = normalizeFeePercent(resolvedPool?.tradeFeeRate);
  const fees24h = useMemo(() => {
    const vol = Number(resolvedPool?.vol24h) || 0;
    return vol * (feePercent / 100);
  }, [resolvedPool?.vol24h, feePercent]);

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
        const detail = await getPoolDetail(poolState, poolVersion, walletAddress ? { user_wallet_address: walletAddress } : {});
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
      const resp = await connection.getParsedTokenAccountsByOwner(publicKey, { mint: parsedMint });
      if (!resp?.value?.length) return 0;
      return resp.value.reduce((sum, item) => {
        const amt = item?.account?.data?.parsed?.info?.tokenAmount?.uiAmount ?? 0;
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
      setBalance0(null);
      setBalance1(null);
      return;
    }
    async function run() {
      setFetchingBalance(true);
      try {
        const [b0, b1] = await Promise.all([
          loadBalanceForMint(token0?.mint),
          loadBalanceForMint(token1?.mint),
        ]);
        if (!cancelled) {
          setBalance0(b0);
          setBalance1(b1);
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
  }, [connected, publicKey, connection, token0?.mint, token1?.mint, refreshKey]);

  const handleFill = (side, ratio) => {
    const balance = side === '0' ? balance0 : balance1;
    if (!Number.isFinite(balance)) return;
    const next = balance * ratio;
    const formatted = next >= 1 ? next.toFixed(4) : next.toPrecision(4);
    if (side === '0') {
      setAmount0(formatted.replace(/\.?0+$/, ''));
    } else {
      setAmount1(formatted.replace(/\.?0+$/, ''));
    }
  };

  // 根据 CLMM 公式计算另一个 token 的金额
  // 公式参考：https://docs.uniswap.org/concepts/protocol/understanding-liquidity#calculating-token-amounts-for-a-price-range
  const calculateOtherTokenAmount = useCallback(
    (inputAmount, inputIsToken0) => {
      const priceMin = Number(minPrice);
      const priceMax = Number(maxPrice);
      const price = currentPrice;
      const amount = Number(inputAmount);

      // 验证输入
      if (!Number.isFinite(amount) || amount <= 0) return null;
      if (!Number.isFinite(price) || price <= 0) return null;
      if (!Number.isFinite(priceMin) || priceMin <= 0) return null;
      if (!Number.isFinite(priceMax) || priceMax <= 0) return null;
      if (priceMin >= priceMax) return null;

      // 当前价格必须在价格范围内（不包括边界）
      if (price <= priceMin || price >= priceMax) return null;

      const sqrtP = Math.sqrt(price);
      const sqrtL = Math.sqrt(priceMin);
      const sqrtU = Math.sqrt(priceMax);

      // 避免除零
      if (sqrtU <= sqrtP || sqrtP <= sqrtL) return null;

      let otherAmount;

      if (inputIsToken0) {
        // 给定 token0 金额，计算 token1 金额
        // 参考 Uniswap V3 / Raydium CLMM 公式：
        // L0 = amount0 * sqrtP * sqrtU / (sqrtU - sqrtP)
        // amount1 = L0 * (sqrtP - sqrtL)
        const L0 = (amount * sqrtP * sqrtU) / (sqrtU - sqrtP);
        // req1 = L0 * (sqrtP - sqrtL)
        otherAmount = L0 * (sqrtP - sqrtL);
      } else {
        // 给定 token1 金额，计算 token0 金额
        // 参考 Uniswap V3 / Raydium CLMM 公式：
        // L1 = amount1 / (sqrtP - sqrtL)
        // amount0 = L1 * (sqrtU - sqrtP) / (sqrtU * sqrtP)
        const L1 = amount / (sqrtP - sqrtL);
        // req0 = L1 * (sqrtU - sqrtP) / (sqrtU * sqrtP)
        otherAmount = (L1 * (sqrtU - sqrtP)) / (sqrtU * sqrtP);
      }
      
      // 确保计算结果有足够的精度，避免浮点数误差
      // 使用更高精度的计算，然后四舍五入到合理的小数位数
      if (otherAmount > 0) {
        // 对于大数值，保留更多小数位；对于小数值，使用科学计数法
        const precision = otherAmount >= 1 ? 8 : 12;
        otherAmount = Number(otherAmount.toPrecision(precision));
      }

      if (!Number.isFinite(otherAmount) || otherAmount <= 0) return null;
      return otherAmount;
    },
    [minPrice, maxPrice, currentPrice]
  );

  // 当 amount0 变化时，自动计算 amount1
  useEffect(() => {
    // 如果最后一次手动输入的是 amount1，跳过此更新以避免循环
    if (lastManualInputRef.current === 'amount1') return;
    
    if (!amount0 || amount0 === '0' || amount0 === '') {
      // 如果 amount0 被清空，不清空 amount1，让用户手动输入
      lastManualInputRef.current = null;
      return;
    }

    const calculated = calculateOtherTokenAmount(amount0, true);
    if (calculated !== null) {
      const formatted = calculated >= 1 ? calculated.toFixed(6) : calculated.toPrecision(6);
      lastManualInputRef.current = 'amount0';
      setAmount1(formatted.replace(/\.?0+$/, ''));
    } else {
      lastManualInputRef.current = null;
    }
  }, [amount0, calculateOtherTokenAmount]);

  // 当 amount1 变化时，自动计算 amount0
  useEffect(() => {
    // 如果最后一次手动输入的是 amount0，跳过此更新以避免循环
    if (lastManualInputRef.current === 'amount0') return;
    
    if (!amount1 || amount1 === '0' || amount1 === '') {
      // 如果 amount1 被清空，不清空 amount0，让用户手动输入
      lastManualInputRef.current = null;
      return;
    }

    const calculated = calculateOtherTokenAmount(amount1, false);
    if (calculated !== null) {
      // 使用更高精度格式化，避免精度损失
      const formatted = calculated >= 1 
        ? calculated.toFixed(10).replace(/\.?0+$/, '')
        : calculated.toPrecision(12).replace(/\.?0+$/, '');
      lastManualInputRef.current = 'amount1';
      setAmount0(formatted);
    } else {
      lastManualInputRef.current = null;
    }
  }, [amount1, calculateOtherTokenAmount]);

  // 当价格范围变化时，如果已有输入金额，重新计算另一个 token 的金额
  useEffect(() => {
    // 如果两个金额都为空，不需要重新计算
    if ((!amount0 || amount0 === '0' || amount0 === '') && (!amount1 || amount1 === '0' || amount1 === '')) {
      return;
    }

    // 根据最后一次手动输入的字段重新计算
    if (lastManualInputRef.current === 'amount0' && amount0 && amount0 !== '0' && amount0 !== '') {
      const calculated = calculateOtherTokenAmount(amount0, true);
      if (calculated !== null) {
        // 使用更高精度格式化，避免精度损失
        const formatted = calculated >= 1 
          ? calculated.toFixed(10).replace(/\.?0+$/, '')
          : calculated.toPrecision(12).replace(/\.?0+$/, '');
        setAmount1(formatted);
      }
    } else if (lastManualInputRef.current === 'amount1' && amount1 && amount1 !== '0' && amount1 !== '') {
      const calculated = calculateOtherTokenAmount(amount1, false);
      if (calculated !== null) {
        // 使用更高精度格式化，避免精度损失
        const formatted = calculated >= 1 
          ? calculated.toFixed(10).replace(/\.?0+$/, '')
          : calculated.toPrecision(12).replace(/\.?0+$/, '');
        setAmount0(formatted);
      }
    }
  }, [minPrice, maxPrice, calculateOtherTokenAmount]);

  const handlePricePercentChange = useCallback(
    (percent) => {
      const base = currentPrice;
      if (!Number.isFinite(base) || base <= 0) return;
      if (percent === 'reset') {
        setMinPrice(formatAmountForInput(base * 0.9));
        setMaxPrice(formatAmountForInput(base * 1.1));
        return;
      }
      const num = Number(percent);
      if (!Number.isFinite(num) || num <= 0) return;
      const factor = num / 100;
      setMinPrice(formatAmountForInput(base * (1 - factor)));
      setMaxPrice(formatAmountForInput(base * (1 + factor)));
    },
    [currentPrice]
  );

  const [depthData, setDepthData] = useState(null);
  const [depthDataPoints, setDepthDataPoints] = useState(null);
  
  // 从池子详情中获取历史价格范围数据（24H/7D/30D）
  const historicalPriceRanges = useMemo(() => {
    if (!resolvedPool) return null;
    return {
      '24H': {
        min: resolvedPool.priceRange24hMin ?? resolvedPool.price_range_24h_min,
        max: resolvedPool.priceRange24hMax ?? resolvedPool.price_range_24h_max,
      },
      '7D': {
        min: resolvedPool.priceRange7dMin ?? resolvedPool.price_range_7d_min,
        max: resolvedPool.priceRange7dMax ?? resolvedPool.price_range_7d_max,
      },
      '30D': {
        min: resolvedPool.priceRange30dMin ?? resolvedPool.price_range_30d_min,
        max: resolvedPool.priceRange30dMax ?? resolvedPool.price_range_30d_max,
      },
    };
  }, [resolvedPool]);
  
  // 根据当前选择的时间范围获取对应的价格范围（原始价格，用于传递给图表）
  const currentTimeRangePriceRaw = useMemo(() => {
    if (!historicalPriceRanges || !timeRange) return null;
    return historicalPriceRanges[timeRange] || null;
  }, [historicalPriceRanges, timeRange]);

  // 根据当前选择的时间范围获取对应的价格范围（考虑价格反转，用于UI显示）
  const currentTimeRangePrice = useMemo(() => {
    if (!currentTimeRangePriceRaw) return null;
    // 如果价格反转，需要反转最小和最大价格（并交换位置）
    if (priceInverted) {
      return {
        min: currentTimeRangePriceRaw.max ? 1 / currentTimeRangePriceRaw.max : null,
        max: currentTimeRangePriceRaw.min ? 1 / currentTimeRangePriceRaw.min : null,
      };
    }
    return currentTimeRangePriceRaw;
  }, [currentTimeRangePriceRaw, priceInverted]);

  // 查询池子的流动性深度数据（只传递池子 ID，返回所有 tick 的流动性数据）
  useEffect(() => {
    let cancelled = false;
    (async () => {
      if (!resolvedPool?.poolState) {
        setDepthData(null);
        setDepthDataPoints(null);
        return;
      }
      try {
        const data = await getClmmPoolDepthData(resolvedPool.poolState);
        if (!cancelled && data) {
          setDepthData(data);
        }
      } catch (e) {
        if (!cancelled) {
          console.warn('Failed to get pool depth data', e);
          setDepthData(null);
        setDepthDataPoints(null);
      }
    }
  })();
  return () => {
    cancelled = true;
  };
}, [resolvedPool?.poolState, refreshKey]);

  // 当价格范围变化时，从已加载的数据中过滤数据点
  useEffect(() => {
    if (!depthData?.line) {
      setDepthDataPoints(null);
      return;
    }

    const priceMinNum = Number(minPrice) || 0;
    const priceMaxNum = Number(maxPrice) || 0;

    // 始终使用完整的深度数据，不因滑块范围过滤，避免图形被截断成点
    setDepthDataPoints(depthData.line);
  }, [depthData]);

  const aprData = useMemo(() => {
    const baseApr = Number(resolvedPool?.apr) || 0;
    const timeMultipliers = { '24H': 1, '7D': 7, '30D': 30 };
    return baseApr * (timeMultipliers[timeRange] || 1);
  }, [resolvedPool?.apr, timeRange]);

  const handleAction = async () => {
    if (!connected) {
      if (setWalletModalVisible) {
        setWalletModalVisible(true);
        return;
      }
      wallet?.adapter?.connect?.();
      return;
    }
    if (!resolvedPool?.poolState) {
      toast({ title: 'Missing pool', description: 'Pool state not found', variant: 'destructive' });
      return;
    }
    if (!amount0 || !amount1) {
      toast({ title: 'Invalid amounts', description: 'Please enter amounts for both tokens', variant: 'destructive' });
      return;
    }
    const run = async () => {
      try {
        setSubmitting(true);
        const { token0: t0, token1: t1 } = sortTokensByMint(tokenA, tokenB);
        const token0Amount = t0?.mint === tokenA.mint ? amount0 : amount1;
        const token1Amount = t0?.mint === tokenA.mint ? amount1 : amount0;
        const slippageBps = Math.max(0, Math.min(10000, Math.round((parseFloat(slippage) || 0) * 100)));
        const resp = await buildClmmOpenPositionTx({
          chain_id: 100000,
          pool_state: resolvedPool.poolState,
          token0_amount: token0Amount || '0',
          token1_amount: token1Amount || '0',
          price_min: minPrice || '',
          price_max: maxPrice || '',
          slippage_bps: slippageBps,
          user_wallet_address: walletAddress,
        });
        const txBase64 = resp?.tx_base64 || resp?.txBase64 || resp?.tx;
        if (!txBase64) {
          throw new Error('Backend did not return transaction');
        }
        // 使用 Buffer 反序列化交易（与 PoolCreationNew 保持一致）
        const buffer = Buffer.from(txBase64, 'base64');
        let transaction;
        try {
          transaction = Transaction.from(buffer);
        } catch (legacyError) {
          try {
            transaction = VersionedTransaction.deserialize(buffer);
          } catch (versionedError) {
            throw new Error(`Failed to deserialize transaction: ${legacyError.message}`);
          }
        }
        
        // 确保 feePayer 和 recentBlockhash 正确设置
        if (transaction instanceof Transaction) {
          transaction.feePayer = transaction.feePayer || publicKey;
          if (!transaction.recentBlockhash) {
            const { blockhash } = await connection.getLatestBlockhash();
            transaction.recentBlockhash = blockhash;
          }
        }
        
        // 手动签名并发送（与 PoolCreationNew 保持一致）
        let signedTx;
        const isVersioned = transaction instanceof VersionedTransaction;
        if (isVersioned) {
          if (!wallet?.adapter?.signTransaction) {
            throw new Error('当前钱包不支持签名 versioned 交易');
          }
          signedTx = await wallet.adapter.signTransaction(transaction);
        } else if (walletSignTransaction) {
          // 优先使用 walletSignTransaction（与 PoolCreationNew 保持一致）
          signedTx = await walletSignTransaction(transaction);
        } else if (wallet?.adapter?.signTransaction) {
          signedTx = await wallet.adapter.signTransaction(transaction);
        } else {
          throw new Error('当前钱包无法签名交易');
        }
        
        // 发送已签名的交易
        const rawTx = signedTx.serialize();
        const sig = await connection.sendRawTransaction(rawTx, {
          skipPreflight: false,
          preflightCommitment: 'processed',
          maxRetries: 3,
        });
        
        toast({
          title: 'Deposit submitted',
          description: sig,
        });
        setAmount0('');
        setAmount1('');
        setRefreshKey((k) => k + 1);
      } catch (e) {
        const errorMessage = e?.message || e?.error?.message || e?.cause?.message || (typeof e === 'string' ? e : 'Unknown error');
        console.error('CLMM liquidity transaction failed', {
          amount0,
          amount1,
          wallet: walletAddress,
          error: errorMessage,
          rawError: e,
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

  const poolName = `${token0Symbol}-${token1Symbol}`;
  const totalDeposit = (parseFloat(amount0) || 0) + (parseFloat(amount1) || 0);
  const isActionDisabled =
    loadingPool ||
    fetchingBalance ||
    submitting ||
    !connected ||
    !amount0 ||
    !amount1 ||
    parseFloat(amount0) <= 0 ||
    parseFloat(amount1) <= 0;

  const quickSlippage = [1, 2.5, 3.5, 5];

  return (
    <div className="container mx-auto px-4 py-8 space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" onClick={onNavigateBack}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          {t('depositPage.back')}
        </Button>
        <TokenPairAvatar tokenA={token0} tokenB={token1} />
        <div>
          <div className="text-lg font-semibold">
            {t('depositPage.title')} · {poolName}
          </div>
        </div>
        <div className="ml-auto">
          <Button variant="outline" size="sm" className="flex items-center gap-2" onClick={() => setRefreshKey((k) => k + 1)} disabled={loadingPool}>
            <RefreshCw className={cn('w-4 h-4', loadingPool && 'animate-spin')} />
            {t('tokenList.buttons.refresh') || 'Refresh'}
          </Button>
        </div>
      </div>

      {loadingPool && <div className="text-sm text-muted-foreground">{t('depositPage.syncing') || 'Syncing latest pool data...'}</div>}
      {poolError && (
        <div className="text-sm text-red-500 bg-red-50 border border-red-200 rounded-md px-3 py-2">
          {t('depositPage.errors.loadFailed') || 'Failed to load pool details:'} {poolError.message || t('depositPage.errors.unknown') || 'Unknown error'}
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-[65%_35%] gap-6">
        <Card className="border border-border/60 shadow-sm lg:col-span-2">
          <CardContent className="p-6">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-4">
                <TokenPairAvatar tokenA={token0} tokenB={token1} />
                <div>
                  <div className="text-lg font-semibold">{poolName}</div>
                  <Badge variant="outline" className="mt-1">
                    {feePercent.toFixed(2)}%
                  </Badge>
                </div>
              </div>
              <div className="flex items-center gap-8">
                <div className="text-right">
                  <div className="text-xs text-muted-foreground">{t('depositPage.poolLiquidity')}</div>
                  <div className="text-lg font-semibold">{formatCurrencyUSD(resolvedPool?.liquidityUsd)}</div>
                </div>
                <div className="text-right">
                  <div className="text-xs text-muted-foreground">{t('depositPage.volume24h')}</div>
                  <div className="text-lg font-semibold">{formatCurrencyUSD(resolvedPool?.vol24h)}</div>
                </div>
                <div className="text-right">
                  <div className="text-xs text-muted-foreground">{t('depositPage.fees24h')}</div>
                  <div className="text-lg font-semibold">{formatCurrencyUSD(fees24h)}</div>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-[65%_35%] gap-6">
        <Card className="border border-border/60 shadow-sm">
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle>{t('clmmDeposit.setPriceRange') || 'Set Price Range'}</CardTitle>
              <div className="flex items-center gap-2">
                {['24H', '7D', '30D'].map((range) => (
                  <Button
                    key={range}
                    variant={timeRange === range ? 'default' : 'outline'}
                    size="sm"
                    onClick={() => setTimeRange(range)}
                  >
                    {range}
                  </Button>
                ))}
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="grid grid-cols-1 lg:grid-cols-[67%_33%] gap-6">
              <div>
                <ClmmLiquidityChart
                  currentPrice={rawCurrentPrice}
                  minPrice={minPrice}
                  maxPrice={maxPrice}
                  onMinPriceChange={setMinPrice}
                  onMaxPriceChange={setMaxPrice}
                  priceRangeMin={resolvedPool?.min_price || resolvedPool?.priceRangeMin || rawCurrentPrice * 0.1}
                  priceRangeMax={resolvedPool?.max_price || resolvedPool?.priceRangeMax || rawCurrentPrice * 10}
                  timeRangePriceMin={currentTimeRangePriceRaw?.min}
                  timeRangePriceMax={currentTimeRangePriceRaw?.max}
                  depthDataPoints={depthDataPoints}
                  token0Symbol={token0Symbol}
                  token1Symbol={token1Symbol}
                  inverted={priceInverted}
                />
              </div>
              <div>
                <div className="rounded-xl border border-border/60 bg-card/70 p-4 space-y-3">
                  <div className="text-sm font-semibold text-muted-foreground">{t('clmmDeposit.priceInfo') || 'Price Information'}</div>
                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <div className="w-3 h-3 rounded-full bg-green-500"></div>
                        <span className="text-sm text-muted-foreground">{t('clmmDeposit.currentPrice') || 'Current Price'}</span>
                      </div>
                      <span className="text-sm font-semibold">
                        {currentPrice > 0 ? (currentPrice < 0.01 ? currentPrice.toExponential(3) : currentPrice.toFixed(6)) : '--'}
                      </span>
                    </div>
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <div className="w-3 h-3 rounded-full" style={{ backgroundColor: '#8C6EEF' }}></div>
                        <span className="text-sm text-muted-foreground">
                          {timeRange} {t('clmmDeposit.minPrice') || '最小价格'}
                        </span>
                      </div>
                      <span className="text-sm font-semibold">
                        {currentTimeRangePrice?.min != null && currentTimeRangePrice.min !== 0
                          ? (currentTimeRangePrice.min < 0.01
                              ? currentTimeRangePrice.min.toExponential(3)
                              : currentTimeRangePrice.min.toFixed(6))
                          : '0'}
                      </span>
                    </div>
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <div className="w-3 h-3 rounded-full" style={{ backgroundColor: '#8C6EEF' }}></div>
                        <span className="text-sm text-muted-foreground">
                          {timeRange} {t('clmmDeposit.maxPrice') || '最高价格'}
                        </span>
                      </div>
                      <span className="text-sm font-semibold">
                        {currentTimeRangePrice?.max != null && currentTimeRangePrice.max !== 0
                          ? (currentTimeRangePrice.max < 0.01
                              ? currentTimeRangePrice.max.toExponential(3)
                              : currentTimeRangePrice.max.toFixed(6))
                          : '0'}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="text-sm text-muted-foreground mb-2 block">{t('clmmDeposit.minPrice') || 'Min Price'}</label>
                  <Input
                    type="number"
                    value={minPrice}
                    onChange={(e) => setMinPrice(e.target.value)}
                    placeholder="0.0"
                    className="w-full"
                  />
                </div>
                <div>
                  <label className="text-sm text-muted-foreground mb-2 block">{t('clmmDeposit.maxPrice') || 'Max Price'}</label>
                  <Input
                    type="number"
                    value={maxPrice}
                    onChange={(e) => setMaxPrice(e.target.value)}
                    placeholder="0.0"
                    className="w-full"
                  />
                </div>
              </div>

              <div className="flex items-center gap-2 flex-wrap">
                {[1, 5, 10, 20, 50].map((p) => (
                  <Button
                    key={p}
                    variant="outline"
                    size="sm"
                    onClick={() => handlePricePercentChange(p)}
                  >
                    ±{p}%
                  </Button>
                ))}
                <Button variant="outline" size="sm" onClick={() => handlePricePercentChange('reset')}>
                  {t('clmmDeposit.reset') || 'Reset'}
                </Button>
                <div className="ml-auto">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPriceInverted(!priceInverted)}
                    className="flex items-center gap-2"
                  >
                    <ArrowLeftRight className="w-4 h-4" />
                    {priceInverted ? `${token1Symbol}/${token0Symbol}` : `${token0Symbol}/${token1Symbol}`}
                  </Button>
                </div>
              </div>

              <div className="rounded-xl border border-border/60 bg-muted/20 px-4 py-3 space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">{t('clmmDeposit.estimatedApr') || 'Estimated APR'}</span>
                  <span className="font-semibold">{formatPercent(aprData)}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">{t('depositPage.fees')}</span>
                  <span className="font-semibold">{feePercent.toFixed(2)}%</span>
                </div>
              </div>
            </div>

          </CardContent>
        </Card>

        <Card className="border border-border/60 shadow-sm">
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle>{t('depositPage.title')}</CardTitle>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setSlippageDialogOpen(true)}
                className="flex items-center gap-2"
              >
                <Settings2 className="w-4 h-4" />
                {slippage}%
              </Button>
            </div>
          </CardHeader>
          <CardContent className="space-y-5">
            <TokenInput
              token={priceInverted ? token1 : token0}
              amount={amount0}
              onChange={(value) => {
                lastManualInputRef.current = 'amount0';
                setAmount0(value);
              }}
              balance={priceInverted ? balance1 : balance0}
              onFill={(ratio) => {
                lastManualInputRef.current = 'amount0';
                handleFill('0', ratio);
              }}
              connected={connected}
              t={t}
              label={token0Symbol}
              disabled={shouldDisableToken0}
              overlayMessage={shouldDisableToken0 ? overlayMessage : undefined}
            />
            <TokenInput
              token={priceInverted ? token0 : token1}
              amount={amount1}
              onChange={(value) => {
                lastManualInputRef.current = 'amount1';
                setAmount1(value);
              }}
              balance={priceInverted ? balance0 : balance1}
              onFill={(ratio) => {
                lastManualInputRef.current = 'amount1';
                handleFill('1', ratio);
              }}
              connected={connected}
              t={t}
              label={token1Symbol}
              disabled={shouldDisableToken1}
              overlayMessage={shouldDisableToken1 ? overlayMessage : undefined}
            />

            <div className="rounded-xl border border-border/60 bg-muted/20 px-4 py-3 flex items-center justify-between">
              <div className="text-sm text-muted-foreground">{t('depositPage.totalDeposit')}</div>
              <div className="text-xl font-semibold">
                {totalDeposit > 0 ? totalDeposit.toFixed(4) : '--'}
              </div>
            </div>

            <div className="rounded-xl border border-border/60 bg-muted/20 px-4 py-3 flex items-center justify-between">
              <div className="text-sm text-muted-foreground">{t('clmmDeposit.depositRatio') || 'Deposit Ratio'}</div>
              <div className="text-sm font-semibold">
                {amount0 && amount1
                  ? `${((parseFloat(amount0) / totalDeposit) * 100).toFixed(1)}% / ${((parseFloat(amount1) / totalDeposit) * 100).toFixed(1)}%`
                  : '--'}
              </div>
            </div>

            <Button
              className="w-full h-12 text-base font-semibold"
              disabled={isActionDisabled}
              onClick={handleAction}
            >
              {!connected
                ? t('depositPage.connectWallet')
                : submitting
                ? (t('depositPage.submitting') || 'Submitting...')
                : t('depositPage.actions.submit')}
            </Button>
          </CardContent>
        </Card>
      </div>

      <Dialog open={slippageDialogOpen} onOpenChange={setSlippageDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('clmmDeposit.setSlippageTolerance') || 'Set Slippage Tolerance'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              {quickSlippage.map((opt) => (
                <Button
                  key={opt}
                  size="sm"
                  variant={Number(slippage) === opt ? 'default' : 'outline'}
                  className="h-9 px-3"
                  onClick={() => {
                    setSlippage(String(opt));
                    setSlippageDialogOpen(false);
                  }}
                >
                  {opt}%
                </Button>
              ))}
            </div>
            <div className="flex items-center gap-2">
              <Input
                type="number"
                inputMode="decimal"
                className="flex-1"
                value={slippage}
                onChange={(e) => setSlippage(e.target.value)}
                placeholder="2.5"
              />
              <span className="text-sm text-muted-foreground">%</span>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default ClmmPoolDeposit;
