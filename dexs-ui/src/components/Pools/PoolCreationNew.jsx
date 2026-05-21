
import React, { useEffect, useMemo, useState, useCallback } from 'react';
import { useWallet, useConnection } from '@solana/wallet-adapter-react';
import { PublicKey, Transaction, VersionedTransaction } from '@solana/web3.js';
import { motion } from 'framer-motion';
import { Buffer } from 'buffer';
import {
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  Edit3,
  Info,
  Loader2,
  Shield,
  Wallet,
  Zap,
  RefreshCw
} from 'lucide-react';
import { Button } from '../UI/Button';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '../UI/card';
import { Input } from '../UI/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../UI/select';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../UI/dialog';
import { useToast } from '../../hooks/use-toast';
import { useTranslation } from '../../i18n/LanguageContext';
import api from '../../api/client';
import { cn } from '../../lib/utils';

const CHAIN_ID = 100000;
const TOKENS_ENDPOINT = '/api/liquidity/cpmm/tokens';
const CREATE_ENDPOINT = '/api/liquidity/cpmm/pools';
const FALLBACK_FEE_TIERS = [
  { fee_bps: 1 },
  { fee_bps: 5 },
  { fee_bps: 30 },
  { fee_bps: 100 },
];

const normalizeToken = (token) => {
  if (!token) return null;
  const mint =
    token.tokenMint ||
    token.token_mint ||
    token.tokenAddress ||
    token.token_address ||
    token.mint ||
    '';
  if (!mint) return null;
  return {
    mint,
    symbol: (token.symbol || token.tokenSymbol || token.token_symbol || 'TOKEN').toUpperCase(),
    name: token.name || token.tokenName || token.token_name || token.symbol || 'Token',
    logo: token.logo || token.tokenIcon || token.token_icon || '',
    decimals: token.decimals ?? token.tokenDecimals ?? token.token_decimals ?? 9,
    program:
      token.program ||
      token.tokenProgram ||
      token.token_program ||
      token.mintProgram ||
      token.mint_program ||
      '',
  };
};
const TokenSelectDialog = ({ open, onOpenChange, tokens, onSelect, title }) => {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');

  const filtered = useMemo(() => {
    const key = search.trim().toLowerCase();
    if (!key) return tokens;
    return tokens.filter(
      (tk) =>
        tk.symbol.toLowerCase().includes(key) ||
        tk.name.toLowerCase().includes(key) ||
        tk.mint.toLowerCase().includes(key)
    );
  }, [tokens, search]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <Input
            placeholder={t('clmmCreation.searchToken')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div className="max-h-[360px] overflow-y-auto space-y-2">
            {filtered.length === 0 && (
              <div className="text-center text-muted-foreground text-sm py-4">
                {t('clmmCreation.noToken')}
              </div>
            )}
            {filtered.map((tk) => (
              <button
                key={tk.mint}
                type="button"
                onClick={() => {
                  onSelect(tk);
                  onOpenChange(false);
                }}
                className="w-full flex items-center gap-3 p-3 rounded-xl border border-border/60 hover:bg-muted/50 transition-colors"
              >
                <div className="w-10 h-10 rounded-full bg-muted flex items-center justify-center overflow-hidden text-base font-semibold">
                  {tk.logo ? (
                    <img src={tk.logo} alt={tk.symbol} className="w-full h-full object-cover" />
                  ) : (
                    tk.symbol.slice(0, 1)
                  )}
                </div>
                <div className="flex flex-col text-left">
                  <span className="font-semibold">{tk.symbol}</span>
                  <span className="text-xs text-muted-foreground">{tk.mint}</span>
                </div>
              </button>
            ))}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
};

const StepItem = ({ index, title, active, completed, description }) => {
  return (
    <div className="flex gap-3 items-start">
      <div
        className={cn(
          'w-8 h-8 rounded-full flex items-center justify-center border',
          completed ? 'bg-primary text-primary-foreground border-primary' : '',
          active && !completed ? 'border-primary text-primary' : 'border-border text-muted-foreground'
        )}
      >
        {completed ? <CheckCircle2 className="w-5 h-5" /> : <span className="text-sm">{index}</span>}
      </div>
      <div>
        <p className="font-semibold">{title}</p>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
    </div>
  );
};

const formatNumber = (val, digits = 6) => {
  const num = Number(val);
  if (!Number.isFinite(num)) return '--';
  return num.toFixed(digits).replace(/\.?0+$/, '');
};
const PoolCreationNew = ({ onNavigateBack }) => {
  const { publicKey, connected, wallet, signTransaction: walletSignTransaction } = useWallet();
  const { connection } = useConnection();
  const { toast } = useToast();
  const { t } = useTranslation();

  const [step, setStep] = useState(1);
  const [tokenModal, setTokenModal] = useState({ open: false, target: 'base' });
  const [tokens, setTokens] = useState([]);
  const [loadingTokens, setLoadingTokens] = useState(false);
  const [feeTiers, setFeeTiers] = useState([]);
  const [feeLoading, setFeeLoading] = useState(false);
  const [feeError, setFeeError] = useState('');

  const [baseToken, setBaseToken] = useState(null);
  const [quoteToken, setQuoteToken] = useState(null);
  const [feeTier, setFeeTier] = useState('');

  const [priceMode, setPriceMode] = useState('token0');
  const [initialPrice, setInitialPrice] = useState('');
  const [rangeMode, setRangeMode] = useState('full');
  const [priceMin, setPriceMin] = useState('');
  const [priceMax, setPriceMax] = useState('');
  const [showPriceToken0, setShowPriceToken0] = useState(true);

  const [amount0, setAmount0, getAmount0] = useState('');
  const [amount1, setAmount1, getAmount1] = useState('');
  const [balance0, setBalance0] = useState('--');
  const [balance1, setBalance1] = useState('--');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [txSignature, setTxSignature] = useState('');

  const normalizedFeeTiers = useMemo(() => {
    const source = feeTiers?.length ? feeTiers : FALLBACK_FEE_TIERS;
    return source
      .map((tier) => {
        if (tier?.value && tier?.label) return tier;
        const raw =
          tier?.value ??
          tier?.valueBps ??
          tier?.value_bps ??
          tier?.value_bp ??
          tier?.bps ??
          tier?.fee_bps ??
          tier?.feeBps ??
          tier?.fee ??
          tier?.fee_tier_bps;
        if (raw === undefined || raw === null) return null;
        const numeric = Number(raw);
        if (!Number.isFinite(numeric)) return null;
        const label = tier?.label || `${(numeric / 100).toFixed(2)}%`;
        return {
          value: String(raw),
          label,
          description: tier?.description || '',
          tickSpacing: tier?.tickSpacing ?? tier?.tick_spacing,
          configIndex: tier?.configIndex ?? tier?.config_index ?? tier?.configindex,
          programAddress: tier?.programAddress ?? tier?.program_address,
          feeBps: numeric,
        };
      })
      .filter(Boolean);
  }, [feeTiers]);

  const token0 = baseToken;
  const token1 = quoteToken;
  const currentFeeInfo = useMemo(
    () => normalizedFeeTiers.find((tier) => tier.value === feeTier),
    [normalizedFeeTiers, feeTier]
  );

  const canStep1Next = baseToken && quoteToken && feeTier && baseToken.mint !== quoteToken.mint;
  const canStep2Next = useMemo(() => {
    const price = Number(initialPrice);
    if (!Number.isFinite(price) || price <= 0) return false;
    if (rangeMode === 'custom') {
      const min = Number(priceMin);
      const max = Number(priceMax);
      if (!Number.isFinite(min) || !Number.isFinite(max) || min <= 0 || max <= 0) return false;
      if (min >= max) return false;
    }
    return true;
  }, [initialPrice, rangeMode, priceMin, priceMax]);

  const canCreate = useMemo(() => {
    const a0 = Number(amount0);
    const a1 = Number(amount1);
    return canStep2Next && Number.isFinite(a0) && Number.isFinite(a1) && a0 > 0 && a1 > 0;
  }, [amount0, amount1, canStep2Next]);

  const derivedPrice = useMemo(() => {
    const p = Number(initialPrice);
    if (!Number.isFinite(p) || p <= 0 || !token0 || !token1) return null;
    // 统一转换成 token0 per token1，便于第三步联动金额
    return priceMode === 'token0' ? p : 1 / p;
  }, [initialPrice, token0, token1, priceMode]);

  
  // 将价格按 tick spacing 对齐
  const alignPrice = useCallback(
    (p, upward) => {
      const priceNum = Number(p);
      const spacing = Number(currentFeeInfo?.tickSpacing || currentFeeInfo?.tick_spacing || 1);
      if (!Number.isFinite(priceNum) || priceNum <= 0) return null;
      const tick = Math.log(priceNum) / Math.log(1.0001);
      const sp = spacing || 1;
      let t = Math.floor(tick);
      const rem = t % sp;
      if (rem !== 0) {
        if (upward) {
          t = t >= 0 ? t + (sp - rem) : t - rem;
        } else {
          t = t >= 0 ? t - rem : t - rem - sp;
        }
      }
      return Math.pow(1.0001, t);
    },
    [currentFeeInfo]
  );

  // 同步计算另一侧金额
  // 根据用户输入的金额和设置的价格，计算另一个代币的金额
  // 对于价格区间内的流动性，使用 CLMM 公式计算
  const syncAmounts = useCallback(
    (changedField, rawValue) => {
      if (!derivedPrice || !token0 || !token1) return;
      const num = Number(rawValue);
      if (!Number.isFinite(num) || num <= 0) {
        setAmount0(changedField === 'amount0' ? rawValue : '');
        setAmount1(changedField === 'amount1' ? rawValue : '');
        return;
      }
      const p = Number(derivedPrice);
      
      // 如果价格区间无效或使用全区间，按价格比例直接计算
      const calculateSimple = () => {
        if (changedField === 'amount0') {
          setAmount0(rawValue);
          setAmount1(formatNumber(num * p, 6));
        } else {
          setAmount1(rawValue);
          setAmount0(formatNumber(num / p, 6));
        }
      };

      // 如果是全区间模式，直接按价格比例计算
      if (rangeMode !== 'custom' || !priceMin || !priceMax) {
        calculateSimple();
        return;
      }

      const pMin = Number(priceMin);
      const pMax = Number(priceMax);
      const low = alignPrice(Math.min(pMin, pMax), false);
      const high = alignPrice(Math.max(pMin, pMax), true);

      // 如果区间无效或价格不在区间内，按价格比例计算
      if (!(low && high && p > low && p < high)) {
        calculateSimple();
        return;
      }

      const sqrtP = Math.sqrt(p);
      const sqrtL = Math.sqrt(low);
      const sqrtU = Math.sqrt(high);

      // 根据用户输入的金额计算流动性，然后计算另一个金额
      // 不应用滑点，保持用户输入的值不变
      if (changedField === 'amount0') {
        // 用户输入了 amount0
        // L0 = amount0 * sqrtP * sqrtU / (sqrtU - sqrtP)
        const L = (num * sqrtP * sqrtU) / (sqrtU - sqrtP);
        // amount1 = L * (sqrtP - sqrtL)
        const calculatedAmount1 = L * (sqrtP - sqrtL);
        setAmount0(rawValue); // 保持用户输入的值
        setAmount1(formatNumber(calculatedAmount1, 6));
      } else {
        // 用户输入了 amount1
        // L1 = amount1 / (sqrtP - sqrtL)
        const L = num / (sqrtP - sqrtL);
        // amount0 = L * (sqrtU - sqrtP) / (sqrtU * sqrtP)
        const calculatedAmount0 = (L * (sqrtU - sqrtP)) / (sqrtU * sqrtP);
        setAmount0(formatNumber(calculatedAmount0, 6));
        setAmount1(rawValue); // 保持用户输入的值
      }
    },
    [derivedPrice, token0, token1, priceMin, priceMax, rangeMode, alignPrice]
  );

  // 当切到自定义区间时，基于当前价格自动设置对齐后的 min/max
  useEffect(() => {
    if (rangeMode !== 'custom') return;
    if (!derivedPrice) return;
    const base = Number(derivedPrice);
    if (!Number.isFinite(base) || base <= 0) return;
    const factor = 10; // 默认上下十倍
    const alignedMin = alignPrice(base / factor, false);
    const alignedMax = alignPrice(base * factor, true);
    if (alignedMin && alignedMax) {
      setPriceMin(formatNumber(alignedMin, 6));
      setPriceMax(formatNumber(alignedMax, 6));
    }
  }, [rangeMode, derivedPrice, alignPrice]);

  // 对用户输入的 min/max 进行对齐，避免链上 seed 校验失败
  useEffect(() => {
    if (rangeMode !== 'custom') return;
    const minNum = Number(priceMin);
    const maxNum = Number(priceMax);
    if (!Number.isFinite(minNum) || !Number.isFinite(maxNum) || minNum <= 0 || maxNum <= 0) return;
    const alignedMin = alignPrice(minNum, false);
    const alignedMax = alignPrice(maxNum, true);
    if (alignedMin && Math.abs(alignedMin - minNum) / minNum > 1e-6) {
      setPriceMin(formatNumber(alignedMin, 6));
    }
    if (alignedMax && Math.abs(alignedMax - maxNum) / maxNum > 1e-6) {
      setPriceMax(formatNumber(alignedMax, 6));
    }
  }, [priceMin, priceMax, rangeMode, alignPrice]);

const fetchTokens = useCallback(async () => {
    setLoadingTokens(true);
    try {
      const res = await api.get(TOKENS_ENDPOINT, {
        params: { chain_id: CHAIN_ID, page_size: 200, pool_type: 'CLMM' },
      });
      const payload = res?.data?.data || res?.data || {};
      const list = Array.isArray(payload?.tokens) ? payload.tokens : Array.isArray(payload) ? payload : [];
      const normalized = list
        .map(normalizeToken)
        .filter(Boolean)
        .filter((tk, idx, arr) => arr.findIndex((x) => x.mint === tk.mint) === idx);
      setTokens(normalized);
    } catch (err) {
      toast({
        title: t('common.error'),
        description: err?.message || 'Failed to load tokens',
        variant: 'destructive',
      });
    } finally {
      setLoadingTokens(false);
    }
  }, [toast, t]);

  useEffect(() => {
    fetchTokens();
  }, [fetchTokens]);

  const fetchFees = useCallback(async () => {
    setFeeLoading(true);
    setFeeError('');
    try {
      const res = await api.get('/api/liquidity/cpmm/fee-tiers', {
        params: { chain_id: CHAIN_ID, pool_type: 'CLMM' },
      });
      const payload = res?.data?.data || res?.data || {};
      const tiers = Array.isArray(payload?.tiers) ? payload.tiers : Array.isArray(payload) ? payload : [];
      setFeeTiers(tiers.length ? tiers : FALLBACK_FEE_TIERS);
    } catch (err) {
      setFeeError(err?.message || 'Failed to load fee tiers');
      setFeeTiers(FALLBACK_FEE_TIERS);
      toast({
        title: t('common.error'),
        description: err?.message || 'Failed to load fee tiers',
        variant: 'destructive',
      });
    } finally {
      setFeeLoading(false);
    }
  }, [toast, t]);

  useEffect(() => {
    fetchFees();
  }, [fetchFees]);

  const handleRefresh = () => {
    fetchTokens();
    fetchFees();
  };

  const fetchBalance = useCallback(
    async (mint, decimals, program) => {
      if (!connection || !publicKey || !mint) return '--';
      try {
        const mintKey = new PublicKey(mint);
        // Native SOL
        if (mint === 'So11111111111111111111111111111111111111112') {
          const lamports = await connection.getBalance(publicKey);
          return formatNumber(lamports / 10 ** (decimals || 9));
        }
        let programId = new PublicKey('TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA');
        if (program) {
          try {
            programId = new PublicKey(program);
          } catch (err) {
            console.warn('Invalid token program', program, err);
          }
        }
        const resp = await connection.getParsedTokenAccountsByOwner(publicKey, {
          programId,
        });
        const total = resp.value
          .map((acc) => {
            const info = acc.account?.data?.parsed?.info;
            if (!info) return 0;
            if (info.mint !== mintKey.toBase58()) return 0;
            return Number(info.tokenAmount?.uiAmount) || 0;
          })
          .reduce((a, b) => a + b, 0);
        return formatNumber(total, 6);
      } catch (err) {
        console.warn('Fetch balance failed', err);
        return '--';
      }
    },
    [connection, publicKey]
  );
  const loadBalances = useCallback(async () => {
    if (!token0 || !token1) return;
    const b0 = await fetchBalance(token0.mint, token0.decimals, token0.program);
    const b1 = await fetchBalance(token1.mint, token1.decimals, token1.program);
    setBalance0(b0);
    setBalance1(b1);
  }, [fetchBalance, token0, token1]);

  useEffect(() => {
    loadBalances();
  }, [loadBalances]);

  useEffect(() => {
    if (step === 3) {
      loadBalances();
    }
  }, [step, loadBalances]);

  const switchToStep = (target) => {
    setStep(target);
  };

  useEffect(() => {
    if (!feeTier && normalizedFeeTiers.length) {
      setFeeTier(normalizedFeeTiers[0].value);
    }
  }, [normalizedFeeTiers, feeTier]);

  const deserializeTransaction = useCallback((txBase64) => {
    const buffer = Buffer.from(txBase64, 'base64');
    try {
      return { transaction: Transaction.from(buffer) };
    } catch (legacyError) {
      try {
        return { transaction: VersionedTransaction.deserialize(buffer) };
      } catch (versionedError) {
        throw new Error('无法从后端解析交易数据');
      }
    }
  }, []);

  const handleCreate = useCallback(async () => {
    if (!connected || !publicKey) {
      toast({
        title: t('common.warning'),
        description: t('clmmCreation.connectWallet'),
        variant: 'destructive',
      });
      return;
    }
    if (!canCreate || !token0 || !token1) {
      toast({
        title: t('common.warning'),
        description: t('clmmCreation.fillAll'),
      });
      return;
    }

    setIsSubmitting(true);
    setTxSignature('');

    console.log('amount: ', amount0, amount1)

    try {
      const payload = {
        chain_id: CHAIN_ID,
        pool_type: 'CLMM',
        base_token_mint: token0.mint,
        quote_token_mint: token1.mint,
        initial_price: derivedPrice ? derivedPrice.toString() : initialPrice,
        fee_tier_bps: Number(feeTier) || 0,
        config_index: currentFeeInfo?.configIndex ?? 0,
        start_time: Math.floor(Date.now() / 1000),
        user_wallet_address: publicKey.toString(),
        price_mode: priceMode,
        price_min: priceMin,
        price_max: priceMax,
        amount_0: amount0 || '0',
        amount_1: amount1 || '0',
        range_mode: rangeMode,
      };

      const res = await api.post(CREATE_ENDPOINT, payload);
      const data = res?.data?.data || res?.data || {};
      const txBase64 =
        data.txBase64 ||
        data.tx_base64 ||
        data.txHash ||
        data.tx_hash;
      if (!txBase64) {
        throw new Error(t('liquidityPage.errors.missingTransaction') || '缺少交易数据');
      }

      const { transaction } = deserializeTransaction(txBase64);
      if (transaction instanceof Transaction) {
        transaction.feePayer = transaction.feePayer || publicKey;
        if (!transaction.recentBlockhash) {
          const { blockhash } = await connection.getLatestBlockhash();
          transaction.recentBlockhash = blockhash;
        }
      }

      let signedTx;
      const isVersioned = transaction instanceof VersionedTransaction;
      if (isVersioned) {
        if (!wallet?.adapter?.signTransaction) {
          throw new Error('当前钱包不支持签名 versioned 交易');
        }
        signedTx = await wallet.adapter.signTransaction(transaction);
      } else if (walletSignTransaction) {
        signedTx = await walletSignTransaction(transaction);
      } else if (wallet?.adapter?.signTransaction) {
        signedTx = await wallet.adapter.signTransaction(transaction);
      } else {
        throw new Error('当前钱包无法签名交易');
      }

      const sig = await connection.sendRawTransaction(signedTx.serialize(), {
        skipPreflight: false,
        preflightCommitment: 'processed',
        maxRetries: 3,
      });
      setTxSignature(sig);
      toast({
        title: t('liquidityPage.toast.transactionSent'),
        description: t('liquidityPage.toast.transactionConfirmed', { signature: sig }),
      });
    } catch (err) {
      console.error('Create CLMM pool failed', err);
      const logs =
        (Array.isArray(err?.logs) && err.logs) ||
        (Array.isArray(err?.error?.logs) && err.error.logs) ||
        [];
      const rawMsg =
        err?.message ||
        err?.error?.message ||
        t('clmmCreation.submitFailed') ||
        '??????';
      const friendly = (() => {
        const text = `${rawMsg} ${logs.join(' ')}`.toLowerCase();
        if (text.includes('accountownedbywrongprogram') || text.includes('error: 0xbbf')) {
          return 'AMM ????????????????????????????ID???';
        }
        if (text.includes('blockhash not found')) {
          return '?????????????';
        }
        if (text.includes('slippage') || text.includes('0x1785') || text.includes('priceslippagecheck')) {
          return '???????????????????????????';
        }
        return rawMsg;
      })();

toast({
        title: t('common.error'),
        description: friendly,
        variant: 'destructive',
      });
toast({
        title: t('common.error'),
        description: friendly,
        variant: 'destructive',
      });
    } finally {
      setIsSubmitting(false);
    }
  }, [
    connected,
    publicKey,
    canCreate,
    token0,
    token1,
    derivedPrice,
    initialPrice,
    feeTier,
    currentFeeInfo,
    priceMode,
    priceMin,
    priceMax,
    rangeMode,
    amount0,
    amount1,
    wallet,
    walletSignTransaction,
    connection,
    toast,
    t,
    deserializeTransaction,
  ]);

  const priceLabel =
    priceMode === 'token0'
      ? `${token0?.symbol || 'Token0'} ${t('clmmCreation.per')} ${token1?.symbol || 'Token1'}`
      : `${token1?.symbol || 'Token1'} ${t('clmmCreation.per')} ${token0?.symbol || 'Token0'}`;

  const currentPriceText =
    showPriceToken0 && token0 && token1
      ? `${t('clmmCreation.currentPrice')}: ${initialPrice || '--'} ${token0.symbol}/${token1.symbol}`
      : `${t('clmmCreation.currentPrice')}: ${initialPrice || '--'} ${token1?.symbol}/${token0?.symbol}`;

  const ratioText = () => {
    const a0 = Number(amount0);
    const a1 = Number(amount1);
    if (!Number.isFinite(a0) || !Number.isFinite(a1) || a0 <= 0 || a1 <= 0) return '--';
    const sum = a0 + a1;
    const p0 = ((a0 / sum) * 100).toFixed(2);
    const p1 = ((a1 / sum) * 100).toFixed(2);
    return `${p0}% / ${p1}%`;
  };

  // 保留手动输入，不自动覆盖对侧金额，避免误传。

  // 给出建议金额（基于价格=derivedPrice, 区间=priceMin~priceMax, 使用当前 tickSpacing 对齐后估算的所需比例）
  const suggestAmounts = useMemo(() => {
    const price = Number(derivedPrice);
    const pMin = Number(priceMin);
    const pMax = Number(priceMax);
    const spacing = Number(currentFeeInfo?.tickSpacing || currentFeeInfo?.tick_spacing || 1);
    if (!Number.isFinite(price) || price <= 0 || !Number.isFinite(pMin) || !Number.isFinite(pMax) || pMin <= 0 || pMax <= 0 || pMin >= pMax) {
      return null;
    }
    const align = (p, up) => {
      // tick = ln(p)/ln(1.0001)
      const tick = Math.log(p) / Math.log(1.0001);
      const sp = spacing || 1;
      const rem = tick % sp;
      let alignedTick = tick;
      if (rem !== 0) {
        alignedTick = up ? tick + (sp - rem) : tick - rem;
      }
      return Math.pow(1.0001, alignedTick);
    };
    const low = align(pMin, false);
    const high = align(pMax, true);
    if (!(low > 0 && high > 0 && price > 0 && price > low && price < high)) return null;
    const sqrtP = Math.sqrt(price);
    const sqrtL = Math.sqrt(low);
    const sqrtU = Math.sqrt(high);
    // 设 amount0=1，求所需 amount1
    const L0 = (1 * sqrtP * sqrtU) / (sqrtU - sqrtP);
    const req1 = L0 * (sqrtP - sqrtL);
    // 设 amount1=1，求所需 amount0
    const L1 = 1 / (sqrtP - sqrtL);
    const req0 = (L1 * (sqrtU - sqrtP)) / (sqrtU * sqrtP);
    return {
      amount0Per1: req0, // amount0 需要的倍数（当 amount1=1 时）
      amount1Per1: req1, // amount1 需要的倍数（当 amount0=1 时）
      alignedMin: low,
      alignedMax: high,
    };
  }, [derivedPrice, priceMin, priceMax, currentFeeInfo]);


  const renderTokenIcon = (token, size = 'w-10 h-10') => {
    const cls = `${size} rounded-full border border-border bg-muted flex items-center justify-center overflow-hidden text-base font-semibold`;
    if (token?.logo) {
      return <img src={token.logo} alt={token.symbol} className={`${cls} object-cover`} />;
    }
    return <div className={cls}>{token?.symbol?.slice(0, 1) || '?'}</div>;
  };

  const tokenButton = (token, placeholder, onClick) => (
    <button
      type="button"
      onClick={onClick}
      className="w-full flex items-center justify-between border border-border/60 rounded-xl px-3 py-2 hover:bg-muted/50 transition-colors"
    >
      <div className="flex items-center gap-2">
        {renderTokenIcon(token)}
        <div className="flex flex-col items-start">
          <span className="font-semibold text-lg">{token?.symbol || placeholder}</span>
          <span className="text-xs text-muted-foreground truncate max-w-[180px]">{token?.mint || '--'}</span>
        </div>
      </div>
      <ArrowRight className="w-4 h-4 text-muted-foreground" />
    </button>
  );
  if (!connected) {
    return (
      <div className="container mx-auto px-4 py-8">
        <Card className="max-w-lg mx-auto text-center">
          <CardHeader>
            <div className="w-16 h-16 mx-auto bg-primary/10 rounded-full flex items-center justify-center mb-4">
              <Wallet className="w-8 h-8 text-primary" />
            </div>
            <CardTitle>{t('header.wallet.connect')}</CardTitle>
            <CardDescription>{t('clmmCreation.connectWallet')}</CardDescription>
          </CardHeader>
        </Card>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-8">
      <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} className="max-w-6xl mx-auto space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold">{t('clmmCreation.title')}</h1>
            <p className="text-muted-foreground">{t('clmmCreation.subtitle')}</p>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={handleRefresh}>
              <RefreshCw className="w-4 h-4 mr-1" />
              {t('tokenList.buttons.refresh')}
            </Button>
            {onNavigateBack && (
              <Button variant="outline" onClick={onNavigateBack}>
                <ArrowLeft className="w-4 h-4 mr-2" />
                {t('clmmCreation.back')}
              </Button>
            )}
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{t('clmmCreation.stepsTitle')}</CardTitle>
                <CardDescription>{t('clmmCreation.stepsDesc')}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <StepItem
                  index={1}
                  title={t('clmmCreation.step1Title')}
                  description={t('clmmCreation.step1Desc')}
                  active={step === 1}
                  completed={step > 1}
                />
                <StepItem
                  index={2}
                  title={t('clmmCreation.step2Title')}
                  description={t('clmmCreation.step2Desc')}
                  active={step === 2}
                  completed={step > 2}
                />
                <StepItem
                  index={3}
                  title={t('clmmCreation.step3Title')}
                  description={t('clmmCreation.step3Desc')}
                  active={step === 3}
                  completed={step > 3}
                />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base flex items-center gap-2">
                  <Info className="w-4 h-4" />
                  {t('clmmCreation.summaryTitle')}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('clmmCreation.baseToken')}</span>
                  <span>{baseToken?.symbol || '--'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('clmmCreation.quoteToken')}</span>
                  <span>{quoteToken?.symbol || '--'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('clmmCreation.feeTier')}</span>
                  <span>{feeTier ? `${feeTier} bps` : '--'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('clmmCreation.priceSummary')}</span>
                  <span>{initialPrice || '--'}</span>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base flex items-center gap-2">
                  <Shield className="w-4 h-4" />
                  {t('clmmCreation.tipsTitle')}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-2 text-sm text-muted-foreground">
                <p>{t('clmmCreation.tipTokens')}</p>
                <p>{t('clmmCreation.tipPrice')}</p>
                <p>{t('clmmCreation.tipRange')}</p>
              </CardContent>
            </Card>
          </div>
          <div className="lg:col-span-2 space-y-4">
            {step === 1 && (
              <Card>
                <CardHeader>
                  <CardTitle>{t('clmmCreation.step1Title')}</CardTitle>
                  <CardDescription>{t('clmmCreation.step1Desc')}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-6">
                  <div className="space-y-4">
                    <div className="flex items-center justify-between">
                      <span className="text-sm text-muted-foreground">{t('clmmCreation.tokens')}</span>
                      <Button size="sm" variant="ghost" onClick={fetchTokens} disabled={loadingTokens}>
                        {loadingTokens ? <Loader2 className="w-4 h-4 animate-spin" /> : <RefreshCw className="w-4 h-4" />}
                      </Button>
                    </div>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                      {tokenButton(baseToken, t('clmmCreation.selectBase'), () =>
                        setTokenModal({ open: true, target: 'base' })
                      )}
                      {tokenButton(quoteToken, t('clmmCreation.selectQuote'), () =>
                        setTokenModal({ open: true, target: 'quote' })
                      )}
                    </div>
                  </div>

                  <div className="space-y-2">
                    <span className="text-sm font-semibold">{t('clmmCreation.feeTier')}</span>
                    <Select value={feeTier} onValueChange={setFeeTier} disabled={feeLoading}>
                      <SelectTrigger className="h-11">
                        <SelectValue
                          placeholder={
                            feeLoading
                              ? t('liquidityPage.form.loadingFees') || t('common.loading')
                              : t('liquidityPage.form.selectFeeTier') || t('clmmCreation.feePlaceholder')
                          }
                        />
                      </SelectTrigger>
                      <SelectContent>
                        {normalizedFeeTiers.map((tier) => (
                          <SelectItem key={tier.value} value={tier.value}>
                            <div className="flex items-center justify-between w-full">
                              <span>{tier.label}</span>
                              <span className="text-xs text-muted-foreground">
                                {tier.description || (tier.tickSpacing ? `Tick ${tier.tickSpacing}` : '')}
                              </span>
                            </div>
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    {feeError && (
                      <p className="text-xs text-red-500">{feeError}</p>
                    )}
                  </div>

                  <div className="flex justify-end">
                    <Button disabled={!canStep1Next} onClick={() => setStep(2)}>
                      {t('clmmCreation.continue')}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )}

            {step === 2 && (
              <Card>
                <CardHeader className="flex flex-col gap-2">
                  <CardTitle>{t('clmmCreation.step2Title')}</CardTitle>
                  <CardDescription>{t('clmmCreation.step2Desc')}</CardDescription>
                <div className="flex items-center justify-between p-3 rounded-xl border border-border/60 bg-muted/40">
                  <div className="flex items-center gap-2">
                    <div className="-space-x-2 flex items-center">
                      {renderTokenIcon(token0, 'w-9 h-9')}
                      {renderTokenIcon(token1, 'w-9 h-9')}
                    </div>
                    <div className="flex flex-col">
                      <span className="font-semibold">{token0?.symbol || 'Token0'}/{token1?.symbol || 'Token1'}</span>
                      <span className="text-xs text-muted-foreground">
                        {t('clmmCreation.feeTier')}: {currentFeeInfo?.label || (feeTier ? `${(Number(feeTier) / 100).toFixed(2)}%` : '--')}
                      </span>
                    </div>
                  </div>
                    <Button variant="ghost" size="sm" onClick={() => switchToStep(1)}>
                      <Edit3 className="w-4 h-4 mr-1" />
                      {t('clmmCreation.edit')}
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="space-y-6">
                  <div className="rounded-2xl border border-border/60 p-4 space-y-4">
                    <div className="flex items-center justify-between">
                      <span className="font-semibold">{t('clmmCreation.priceSetting')}</span>
                      <div className="inline-flex rounded-full border border-border/60 bg-muted/30 p-1">
                        {['token0', 'token1'].map((key) => (
                          <button
                            key={key}
                            type="button"
                            onClick={() => setPriceMode(key)}
                            className={cn(
                              'px-4 py-1 text-sm font-medium rounded-full transition',
                              priceMode === key
                                ? 'bg-primary text-primary-foreground shadow-sm'
                                : 'text-muted-foreground hover:text-foreground'
                            )}
                          >
                            {key === 'token0' ? token0?.symbol || 'Token0' : token1?.symbol || 'Token1'}
                          </button>
                        ))}
                      </div>
                    </div>
                    <div className="space-y-2">
                      <div className="flex items-center justify-between text-sm text-muted-foreground">
                        <span>{t('clmmCreation.initialPrice')}</span>
                        <span className="font-medium text-foreground">{priceLabel}</span>
                      </div>
                      <Input
                        type="number"
                        inputMode="decimal"
                        value={initialPrice}
                        onChange={(e) => setInitialPrice(e.target.value)}
                        placeholder="0.0000"
                      />
                      <button
                        type="button"
                        onClick={() => setShowPriceToken0((v) => !v)}
                        className="text-xs text-primary hover:underline inline-flex items-center gap-1"
                      >
                        {currentPriceText}
                        <ArrowLeft className="w-3 h-3" />
                      </button>
                    </div>
                  </div>

                  <div className="rounded-2xl border border-border/60 p-4 space-y-4">
                    <div className="flex items-center gap-3">
                      <span className="font-semibold">{t('clmmCreation.priceRange')}</span>
                      <div className="inline-flex rounded-full border border-border/60 bg-muted/30 p-1">
                        {['full', 'custom'].map((key) => (
                          <button
                            key={key}
                            type="button"
                            onClick={() => setRangeMode(key)}
                            className={cn(
                              'px-4 py-1 text-sm font-medium rounded-full transition',
                              rangeMode === key
                                ? 'bg-primary text-primary-foreground shadow-sm'
                                : 'text-muted-foreground hover:text-foreground'
                            )}
                          >
                            {key === 'full' ? t('clmmCreation.fullRange') : t('clmmCreation.customRange')}
                          </button>
                        ))}
                      </div>
                    </div>
                    {rangeMode === 'custom' && (
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                        <div>
                          <label className="text-xs text-muted-foreground">{t('clmmCreation.minPrice')}</label>
                          <Input
                            type="number"
                            inputMode="decimal"
                            value={priceMin}
                            onChange={(e) => setPriceMin(e.target.value)}
                            placeholder="0.0001"
                          />
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">{t('clmmCreation.maxPrice')}</label>
                          <Input
                            type="number"
                            inputMode="decimal"
                            value={priceMax}
                            onChange={(e) => setPriceMax(e.target.value)}
                            placeholder="1000"
                          />
                        </div>
                      </div>
                    )}
                  </div>

                  <div className="flex justify-between">
                    <Button variant="outline" onClick={() => setStep(1)}>
                      {t('common.previous')}
                    </Button>
                    <Button disabled={!canStep2Next} onClick={() => setStep(3)}>
                      {t('clmmCreation.continue')}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )}
            {step === 3 && (
              <Card>
                <CardHeader className="space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <div className="w-8 h-8 rounded-full bg-primary/10 flex items-center justify-center">
                        <Zap className="w-4 h-4 text-primary" />
                      </div>
                      <div>
                        <CardTitle>{t('clmmCreation.step3Title')}</CardTitle>
                        <CardDescription>{t('clmmCreation.step3Desc')}</CardDescription>
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center justify-between p-3 rounded-xl border border-border/60 bg-muted/40">
                    <div className="flex items-center gap-2">
                      <div className="-space-x-2 flex items-center">
                        {renderTokenIcon(token0, 'w-9 h-9')}
                        {renderTokenIcon(token1, 'w-9 h-9')}
                      </div>
                      <div className="flex flex-col">
                        <span className="font-semibold">{token0?.symbol || 'Token0'}/{token1?.symbol || 'Token1'}</span>
                        <span className="text-xs text-muted-foreground">
                          {currentFeeInfo?.label || (feeTier ? `${(Number(feeTier) / 100).toFixed(2)}%` : '--')}
                        </span>
                      </div>
                    </div>
                    <Button variant="ghost" size="sm" onClick={() => switchToStep(1)}>
                      <Edit3 className="w-4 h-4 mr-1" />
                      {t('clmmCreation.edit')}
                    </Button>
                  </div>
                  <div className="flex items-center justify-between p-3 rounded-xl border border-border/60 bg-muted/40">
                    <div>
                      <p className="text-sm font-semibold">{t('clmmCreation.initialPrice')}</p>
                      <p className="text-xs text-muted-foreground">
                        {initialPrice || '--'} {token0?.symbol || 'Token0'}/{token1?.symbol || 'Token1'}
                      </p>
                      {rangeMode === 'custom' ? (
                        <p className="text-xs text-muted-foreground">
                          {t('clmmCreation.rangeLabel')}: {priceMin || '--'} - {priceMax || '--'}
                        </p>
                      ) : (
                        <p className="text-xs text-muted-foreground">{t('clmmCreation.fullRange')}</p>
                      )}
                    </div>
                    <Button variant="ghost" size="sm" onClick={() => switchToStep(2)}>
                      <Edit3 className="w-4 h-4 mr-1" />
                      {t('clmmCreation.edit')}
                    </Button>
                  </div>
                </CardHeader>
                <CardContent className="space-y-6">
                    {[{ token: token0, balance: balance0, amount: amount0, setter: setAmount0 }, { token: token1, balance: balance1, amount: amount1, setter: setAmount1 }].map((row, idx) => (
                    <div key={row.token?.mint || idx} className="space-y-2">
                      <div className="flex items-center justify-between text-xs text-muted-foreground">
                        <div className="flex items-center gap-2">
                          <Info className="w-3 h-3" />
                          <span>
                            {t('clmmCreation.balance')}:
                            <button
                              type="button"
                              className="ml-1 text-primary hover:underline"
                              onClick={loadBalances}
                              disabled={!connected}
                            >
                              {row.balance} {row.token?.symbol || `Token${idx}`}
                            </button>
                          </span>
                        </div>
                        <div className="flex items-center gap-2">
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="h-7 px-2"
                            disabled={!connected}
                            onClick={() => {
                              const half = row.balance === '--' ? '' : (Number(row.balance) / 2).toString();
                              syncAmounts(idx === 0 ? 'amount0' : 'amount1', half);
                            }}
                          >
                            50%
                          </Button>
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="h-7 px-2"
                            disabled={!connected}
                            onClick={() => {
                              const full = row.balance === '--' ? '' : row.balance;
                              syncAmounts(idx === 0 ? 'amount0' : 'amount1', full);
                            }}
                          >
                            MAX
                          </Button>
                        </div>
                      </div>
                      <div className={cn('flex items-center gap-4 border rounded-xl bg-card/70 px-4 py-3', !row.token ? 'opacity-70' : '')}>
                        <div className="flex items-center gap-3 min-w-[140px]">
                          {renderTokenIcon(row.token)}
                          <div className="flex flex-col">
                            <span className="text-sm text-muted-foreground">{t('clmmCreation.tokens')}</span>
                            <span className="text-lg font-semibold">{row.token?.symbol || `Token${idx}`}</span>
                          </div>
                        </div>
                        <Input
                          type="number"
                          inputMode="decimal"
                          placeholder="0.0"
                          value={row.amount}
                          onChange={(e) => {
                            syncAmounts(idx === 0 ? 'amount0' : 'amount1', e.target.value);
                          }}
                          className="h-12 text-right text-xl font-semibold flex-1"
                        />
                      </div>
                    </div>
                  ))}

                  <div className="rounded-2xl border border-dashed border-border/60 p-4 bg-muted/30 space-y-2 text-sm">
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t('clmmCreation.totalDeposit')}</span>
                      <span>{`${amount0 || '--'} ${token0?.symbol || ''} + ${amount1 || '--'} ${token1?.symbol || ''}`}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t('clmmCreation.depositRatio')}</span>
                      <span>{ratioText()}</span>
                    </div>
                  </div>

                  <div className="flex justify-between">
                    <Button variant="outline" onClick={() => setStep(2)}>
                      {t('common.previous')}
                    </Button>
                    <Button disabled={!canCreate || isSubmitting} onClick={handleCreate}>
                      {isSubmitting ? <Loader2 className="w-4 h-4 animate-spin" /> : t('clmmCreation.create')}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )}
          </div>
        </div>
      </motion.div>

      <TokenSelectDialog
        open={tokenModal.open}
        onOpenChange={(open) => setTokenModal((prev) => ({ ...prev, open }))}
        tokens={tokens}
        title={t('clmmCreation.selectTokenTitle')}
        onSelect={(tk) => {
          if (tokenModal.target === 'base') {
            setBaseToken(tk);
          } else {
            setQuoteToken(tk);
          }
        }}
      />
    </div>
  );
};

export default PoolCreationNew;
