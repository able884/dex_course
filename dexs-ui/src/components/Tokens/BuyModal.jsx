import React, { useState, useEffect } from 'react';
import { useWallet, useConnection } from '@solana/wallet-adapter-react';
import { SendTransactionError, Transaction, VersionedTransaction } from '@solana/web3.js';
import { Buffer } from 'buffer';
import { useTranslation } from '../../i18n/LanguageContext';
import { useToast } from '../../hooks/use-toast';

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../UI/dialog';
import { Button } from '../UI/Button';
import { Card, CardContent, CardHeader, CardTitle } from '../UI/enhanced-card';
import { Badge } from '../UI/badge';
import { LoadingSpinner } from '../UI/loading-spinner';
import { Input } from '../UI/input';
import { 
  ExternalLink, 
  Zap,
  Copy,
  TrendingUp
} from 'lucide-react';
import { cn } from '../../lib/utils';

// API URL configuration
const API_URL = process.env.NODE_ENV === 'development' 
  ? '' // Use proxy in development
  : '/api'; // Use Nginx proxy in production

const SLIPPAGE_OPTIONS = [0.1, 0.5, 1, 2];
const DEFAULT_SLIPPAGE = 1;
const SLIPPAGE_MIN = 0.1;
const SLIPPAGE_MAX = 5;
const QUOTE_DEBOUNCE_MS = 400;

const BuyModal = ({ isOpen, onClose, token }) => {
  const { t } = useTranslation();
  const { publicKey, connected, wallet, signTransaction } = useWallet();
  const { connection } = useConnection();
  const { toast } = useToast();
  const [amountIn, setAmountIn] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [txSignature, setTxSignature] = useState('');
  const [tradeMode, setTradeMode] = useState('buy');
  const [tokenInput, setTokenInput] = useState('');
  const [lastInputField, setLastInputField] = useState('sol'); // sol | token
  const [slippageChoice, setSlippageChoice] = useState(`${DEFAULT_SLIPPAGE}`);
  const [customSlippage, setCustomSlippage] = useState('');
  const [quoteState, setQuoteState] = useState({ loading: false, data: null, error: '' });
  const [quoteMint, setQuoteMint] = useState(token?.tokenAddress || '');

  useEffect(() => {
    if (isOpen) {
      setQuoteMint(token?.tokenAddress || '');
    }
  }, [token?.tokenAddress, isOpen]);

  const formatErrorMessage = (message) => {
    if (!message) return 'Unknown error';
    return message
      .replace(/\s*\|\s*/g, '\n')
      .replace(/Logs:\s*\[/, 'Logs:\n[')
      .trim();
  };

  const detectErrorType = (message) => {
    const messageLower = message.toLowerCase();
    
    // 滑点相关错误
    const slippageKeywords = [
      'slippage',
      'SlippageToleranceExceeded',
      'exceeds',
      'tolerance',
      'below',
      'minimum',
      'InsufficientOutput',
      'PriceImpactTooHigh',
      '0x1771', // Slippage tolerance exceeded error code
      '0x1772', // Price impact too high
    ];
    
    // 余额不足
    const insufficientFundsKeywords = [
      'insufficient',
      'balance',
      'funds',
      'not enough',
      '0x1'
    ];
    
    // 账户相关
    const accountKeywords = [
      'account',
      'not found',
      'does not exist',
      'AccountNotInitialized',
      '0x3'
    ];
    
    if (slippageKeywords.some(keyword => messageLower.includes(keyword.toLowerCase()))) {
      return 'slippage';
    }
    
    if (insufficientFundsKeywords.some(keyword => messageLower.includes(keyword.toLowerCase()))) {
      return 'insufficient';
    }
    
    if (accountKeywords.some(keyword => messageLower.includes(keyword.toLowerCase()))) {
      return 'account';
    }
    
    return 'unknown';
  };

  const summarizeLogs = (logs = []) => {
    if (!Array.isArray(logs) || !logs.length) return '';
    return (
      logs.find((line) => line.includes('AnchorError')) ||
      logs.find((line) => line.includes('custom program error')) ||
      logs.find((line) => line.toLowerCase().includes('error')) ||
      logs[logs.length - 1]
    );
  };

  const deriveFriendlySendError = (message, logs = []) => {
    const allText = [message, ...(logs || [])].filter(Boolean).join(' ').toLowerCase();
    if (allText.includes('computational budget exceeded')) {
      return '交易耗尽计算预算，请降低交易复杂度或稍后重试。';
    }
    return '';
  };

  const findSendTransactionError = (error) => {
    if (!error || typeof error !== 'object') return null;
    if (error instanceof SendTransactionError) return error;
    if (error.cause && error.cause !== error) {
      const nested = findSendTransactionError(error.cause);
      if (nested) return nested;
    }
    if (error.error && error.error !== error) {
      return findSendTransactionError(error.error);
    }
    return null;
  };

  const showError = (message) => {
    const formattedMessage = formatErrorMessage(message);
    const errorType = detectErrorType(formattedMessage);
    const fallbackMessages = {
      slippage: t('buyModal.slippageErrorDetail'),
      insufficient: t('buyModal.insufficientErrorDetail'),
      account: t('buyModal.accountErrorDetail'),
    };
    const fallback = fallbackMessages[errorType];
    const description = fallback ? `${fallback}\n\n${formattedMessage}` : formattedMessage;

    toast({
      title: t('common.error'),
      description,
      variant: 'destructive',
    });
  };

  const assetIsSol = (asset) => {
    if (asset === null || asset === undefined) return false;
    if (typeof asset === 'number') {
      return asset === 1;
    }
    if (typeof asset === 'string') {
      const normalized = asset.toLowerCase();
      return normalized.includes('sol');
    }
    return false;
  };

  const tokenDecimals =
    typeof token?.decimals === 'number'
      ? token.decimals
      : typeof token?.tokenDecimals === 'number'
      ? token.tokenDecimals
      : 6;

  const parsedSlippage = (() => {
    if (slippageChoice === 'custom') {
      const val = parseFloat(customSlippage);
      if (Number.isFinite(val)) {
        return Math.min(Math.max(val, SLIPPAGE_MIN), SLIPPAGE_MAX);
      }
      return DEFAULT_SLIPPAGE;
    }
    const numeric = parseFloat(slippageChoice);
    return Number.isFinite(numeric) ? numeric : DEFAULT_SLIPPAGE;
  })();

  const slippageBps = Math.round(parsedSlippage * 100);
  const manualValue = (lastInputField === 'sol' ? amountIn : tokenInput).trim();
  const manualValueKey = `${lastInputField}:${manualValue}`;
  const tradeAmount = tradeMode === 'buy' ? amountIn : tokenInput;

  const clearTradeResultState = () => {
    if (!txSignature) return;
    setTxSignature('');
    setAmountIn('');
    setTokenInput('');
    setQuoteState({ loading: false, data: null, error: '' });
    setLastInputField('sol');
  };

  const decimalRegex = /^\d*(\.\d{0,9})?$/;

  const handleSolInputChange = (value) => {
    if (value === '' || decimalRegex.test(value)) {
      clearTradeResultState();
      setAmountIn(value);
      setLastInputField('sol');
    }
  };

  const handleTokenInputChange = (value) => {
    if (value === '' || decimalRegex.test(value)) {
      clearTradeResultState();
      setTokenInput(value);
      setLastInputField('token');
    }
  };

  const handleSlippageSelect = (value) => {
    clearTradeResultState();
    if (value === 'custom') {
      setSlippageChoice('custom');
      return;
    }
    setSlippageChoice(value);
    setCustomSlippage('');
  };

  const handleCustomSlippageChange = (value) => {
    clearTradeResultState();
    setCustomSlippage(value);
  };

  const normalizeQuoteResult = (payload = {}) => {
    const pick = (snake, camel) => {
      if (payload[snake] !== undefined && payload[snake] !== null) return payload[snake];
      if (payload[camel] !== undefined && payload[camel] !== null) return payload[camel];
      return '';
    };
    return {
      payAsset: pick('pay_asset', 'payAsset'),
      receiveAsset: pick('receive_asset', 'receiveAsset'),
      payAmount: pick('pay_amount', 'payAmount'),
      payAmountWithSlippage: pick('pay_amount_with_slippage', 'payAmountWithSlippage'),
      receiveAmount: pick('receive_amount', 'receiveAmount'),
      minReceiveAmount: pick('min_receive_amount', 'minReceiveAmount'),
      priceImpactPct: pick('price_impact_pct', 'priceImpactPct'),
      tokenDecimal: pick('token_decimal', 'tokenDecimal'),
      solDecimal: pick('sol_decimal', 'solDecimal'),
    };
  };

  const syncCounterField = (payload) => {
    if (!payload) return;
    const receiveValue = payload.receiveAmount || '';
    const payValue = payload.payAmount || '';
    if (tradeMode === 'buy') {
      if (lastInputField === 'sol') {
        setTokenInput(receiveValue);
      } else {
        setAmountIn(payValue);
      }
    } else {
      if (lastInputField === 'token') {
        setAmountIn(receiveValue);
      } else {
        setTokenInput(payValue);
      }
    }
  };

  useEffect(() => {
    if (!isOpen) return;
    const mint = (quoteMint || '').trim();
    if (!mint || !manualValue || Number(manualValue) <= 0) {
      setQuoteState((prev) =>
        prev.loading || prev.data || prev.error ? { loading: false, data: null, error: '' } : prev,
      );
      return;
    }

    const controller = new AbortController();
    const timer = setTimeout(async () => {
      try {
        setQuoteState({ loading: true, data: null, error: '' });
        const payload = {
          chain_id: 100000,
          token_mint: mint,
          swap_type: tradeMode === 'buy' ? 1 : 2,
          input_asset: lastInputField === 'sol' ? 1 : 2,
          amount: manualValue,
          slippage_bps: slippageBps,
          token_decimal: tokenDecimals,
        };
        const resp = await fetch(`${API_URL}/v1/pump/quote`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
          signal: controller.signal,
        });
        const data = await resp.json();
        if (!resp.ok) {
          throw new Error(data?.message || 'Quote request failed');
        }
        const rawResult = data?.data || data;
        const normalized = normalizeQuoteResult(rawResult);
        setQuoteState({ loading: false, data: normalized, error: '' });
        syncCounterField(normalized);
      } catch (err) {
        if (err.name === 'AbortError') return;
        setQuoteState({ loading: false, data: null, error: err.message || 'Quote request failed' });
      }
    }, QUOTE_DEBOUNCE_MS);

    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [quoteMint, tradeMode, manualValueKey, slippageBps, tokenDecimals, isOpen]);

  // Helper function to convert VersionedTransaction to legacy Transaction
  const convertToLegacyTransaction = (versionedTx) => {
    try {
      console.log('🔄 Converting VersionedTransaction to legacy Transaction...');
      
      const message = versionedTx.message;
      console.log('Message type:', message.constructor.name);
      
      const hasV0Properties = message.header && 
                             message.staticAccountKeys && 
                             message.recentBlockhash && 
                             message.instructions &&
                             Array.isArray(message.staticAccountKeys) &&
                             Array.isArray(message.instructions);
      
      if (hasV0Properties) {
        console.log('Detected MessageV0-like structure, proceeding with conversion...');
        
        const {
          header,
          staticAccountKeys,
          recentBlockhash,
          instructions,
          addressTableLookups
        } = message;
        
        console.log('Message data:', {
          staticAccountKeys: staticAccountKeys?.length || 0,
          instructions: instructions?.length || 0,
          addressTableLookups: addressTableLookups?.length || 0,
          recentBlockhash: recentBlockhash?.toString().slice(0, 10) + '...',
          header: header ? Object.keys(header) : 'none'
        });
        
        if (addressTableLookups && addressTableLookups.length > 0) {
          throw new Error('Cannot convert transaction with address table lookups to legacy format');
        }
        
        const legacyTransaction = new Transaction();
        const recentBlockhashString =
          typeof recentBlockhash === 'string'
            ? recentBlockhash
            : recentBlockhash?.toBase58?.() || recentBlockhash?.toString();
        legacyTransaction.recentBlockhash = recentBlockhashString;
        legacyTransaction.feePayer = staticAccountKeys[0];
        
        for (let index = 0; index < instructions.length; index++) {
          const instruction = instructions[index];
          
          try {
            console.log(`Converting instruction ${index}:`);
            console.log(`  programIdIndex: ${instruction.programIdIndex}`);
            console.log(`  accountsLength: ${instruction.accounts?.length || 0}`);
            console.log(`  dataLength: ${instruction.data?.length || 0}`);
            
            let instructionData = instruction.data;
            
            if (!instructionData) {
              console.log(`Instruction ${index} has no data, creating empty buffer`);
              instructionData = Buffer.alloc(0);
            } else if (!Buffer.isBuffer(instructionData)) {
              if (Array.isArray(instructionData)) {
                instructionData = Buffer.from(instructionData);
              } else if (typeof instructionData === 'string') {
                instructionData = Buffer.from(instructionData, 'base64');
              } else if (instructionData instanceof Uint8Array) {
                instructionData = Buffer.from(instructionData);
              } else if (instructionData.constructor && (instructionData.constructor.name === 'Uint8Array' || instructionData.constructor.name === 'Er' || instructionData.constructor.name.includes('Array'))) {
                instructionData = Buffer.from(instructionData);
              } else if (typeof instructionData === 'object' && instructionData !== null) {
                if (instructionData.type === 'Buffer' && Array.isArray(instructionData.data)) {
                  instructionData = Buffer.from(instructionData.data);
                } else if (instructionData.length !== undefined && typeof instructionData.length === 'number') {
                  instructionData = Buffer.from(Object.values(instructionData));
                } else {
                  console.warn(`Unknown data type for instruction ${index}, attempting conversion:`, instructionData);
                  instructionData = Buffer.from(Object.values(instructionData));
                }
              } else {
                console.warn(`Unknown data type for instruction ${index}, attempting conversion:`, instructionData);
                instructionData = Buffer.from(Object.values(instructionData));
              }
            }
            
            const convertedInstruction = {
              programId: staticAccountKeys[instruction.programIdIndex],
              keys: instruction.accounts.map((accountIndex, keyIndex) => {
                const accountIndexValue = typeof accountIndex === 'object' ? accountIndex.accountIndex : accountIndex;
                const pubkey = staticAccountKeys[accountIndexValue];
                
                if (!pubkey) {
                  throw new Error(`Invalid account index ${accountIndexValue} for instruction ${index}, key ${keyIndex}`);
                }
                
                return {
                  pubkey: pubkey,
                  isSigner: accountIndexValue < header.numRequiredSignatures,
                  isWritable: accountIndexValue < header.numRequiredSignatures - header.numReadonlySignedAccounts || 
                             (accountIndexValue >= header.numRequiredSignatures && 
                              accountIndexValue < staticAccountKeys.length - header.numReadonlyUnsignedAccounts)
                };
              }),
              data: Buffer.isBuffer(instructionData) ? instructionData : Buffer.alloc(0)
            };
            
            legacyTransaction.add(convertedInstruction);
            
          } catch (instError) {
            console.error(`Failed to convert instruction ${index}:`, instError);
            throw new Error(`Instruction conversion failed: ${instError.message}`);
          }
        }
        
        console.log('�?Successfully converted to legacy Transaction');
        return legacyTransaction;
        
      } else {
        console.error('Message structure:', Object.keys(message));
        throw new Error(`Unsupported message structure. Expected MessageV0-like properties but got: ${Object.keys(message).join(', ')}`);
      }
      
    } catch (conversionError) {
      console.error('�?Failed to convert to legacy transaction:', conversionError);
      throw conversionError;
    }
  };

  const handleBuy = async () => {
    if (!connected || !publicKey) {
      showError('Please connect your wallet first');
      return;
    }

    if (!tradeAmount || isNaN(parseFloat(tradeAmount)) || parseFloat(tradeAmount) <= 0) {
      showError(tradeMode === 'buy' ? 'Please enter a valid SOL amount' : 'Please enter a valid token amount');
      return;
    }

    if (!signTransaction && !wallet?.adapter?.signTransaction) {
      showError('Current wallet cannot sign this transaction');
      return;
    }

    setIsLoading(true);
    setTxSignature('');

    const supportedVersions = wallet?.adapter?.supportedTransactionVersions;
    const walletSupportsVersioned =
      supportedVersions === 'all' ||
      (supportedVersions instanceof Set && supportedVersions.has(0));
    console.log('Wallet supports versioned transactions:', walletSupportsVersioned);

    try {
      
      const apiPayload = {
        chain_id: 100000, // Solana
        token_ca: token.tokenAddress,
        swap_type: tradeMode === 'buy' ? 1 : 2,
        amount_in: tradeAmount,
        user_wallet_address: publicKey.toString(),
        slippage_bps: slippageBps,
      };

      console.log('API request payload:', apiPayload);

      const response = await fetch(`${API_URL}/v1/trade/create_market_order`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(apiPayload),
      });

      const data = await response.json();
      console.log('📨 Full API response:', data);

      if (!response.ok) {
        console.error('API response not OK:', data);
        showError(data.message || `API Error ${response.status}: ${response.statusText}`);
        return;
      }
      
      const txHash = data.data?.txHash || data.txHash || data.tx_hash;
      
      if (!txHash) {
        console.error('Transaction data not found in response:', data);
        if (data.code && data.message) {
          console.error('Backend returned:', data.code, data.message);
          showError(`Backend Error ${data.code}: ${data.message}`);
        } else {
          showError('No transaction data received from server');
        }
        return;
      }

      console.log('Unsigned transaction (base64):', txHash);

      let transaction;
      let transactionType;
      let isVersionedTransaction = false;
      
      try {
        const transactionBuffer = Buffer.from(txHash, 'base64');
        console.log('Successfully decoded base64 to buffer, length:', transactionBuffer.length);

        let originalVersionedTransaction = null;
        
        try {
          transaction = Transaction.from(transactionBuffer);
          transactionType = 'Transaction (legacy)';
          isVersionedTransaction = false;
          console.log('�?Successfully created legacy Transaction object:', transaction);
          
        } catch (legacyError) {
          console.log('Failed to deserialize as legacy transaction, trying versioned format:', legacyError);
          
          try {
            originalVersionedTransaction = VersionedTransaction.deserialize(transactionBuffer);
            isVersionedTransaction = true;
            console.log('Successfully created VersionedTransaction object:', originalVersionedTransaction);
            
            if (!walletSupportsVersioned) {
              console.warn('⚠️ Wallet does not support versioned transactions, attempting conversion...');
              
              try {
                transaction = convertToLegacyTransaction(originalVersionedTransaction);
                transactionType = 'Transaction (converted from VersionedTransaction)';
                isVersionedTransaction = false;
                console.log('�?Successfully converted to legacy Transaction format');
              } catch (conversionError) {
                console.error('�?Failed to convert to legacy format:', conversionError);
                showError(`Cannot convert transaction for your wallet: ${conversionError.message}. Please try a different wallet like Phantom or Solflare.`);
                return;
              }
            } else {
              transaction = originalVersionedTransaction;
              transactionType = 'VersionedTransaction';
            }
            
          } catch (versionedError) {
            console.error('Failed to deserialize as both legacy and versioned transaction:', { legacyError, versionedError });
            showError(`Cannot deserialize transaction: ${legacyError.message}`);
            return;
          }
        }
        
        console.log('Final transaction type:', transactionType);
        
      } catch (decodeError) {
        console.error('Failed to decode transaction:', decodeError);
        showError(`Failed to decode transaction: ${decodeError.message}`);
        return;
      }

      
      let signature;
      try {
        console.log('Signing transaction with connected wallet...');

        if (transaction instanceof Transaction) {
          transaction.feePayer = publicKey;
        }

        let signedTx;
        if (isVersionedTransaction && walletSupportsVersioned && wallet?.adapter?.signTransaction) {
          signedTx = await wallet.adapter.signTransaction(transaction);
        } else {
          signedTx = await signTransaction(transaction);
        }

        const rawTx = signedTx.serialize();
        try {
          signature = await connection.sendRawTransaction(rawTx, {
            skipPreflight: false,
          });
        } catch (sendErr) {
          if (sendErr instanceof SendTransactionError) {
            let detailedMessage = sendErr.message || 'SendTransactionError';
            let enrichedLogs = [];
            let summaryLine = '';
            try {
              enrichedLogs = await sendErr.getLogs(connection);
              if (enrichedLogs?.length) {
                console.error('Transaction simulation logs:', enrichedLogs);
                summaryLine = summarizeLogs(enrichedLogs);
                const shortLogs = enrichedLogs.slice(-12).join('\n');
                detailedMessage = summaryLine ? `${summaryLine}\n\n${detailedMessage}` : detailedMessage;
                detailedMessage += `\n\nLogs:\n${shortLogs}`;
              }
            } catch (logErr) {
              console.warn('Failed to fetch transaction logs:', logErr);
            }
            const friendly = deriveFriendlySendError(detailedMessage, enrichedLogs);
            if (friendly) {
              detailedMessage = `${friendly}\n\n${detailedMessage}`;
            }
            sendErr.message = detailedMessage;
            sendErr.enrichedLogs = enrichedLogs;
            sendErr.summaryLine = summaryLine;
            throw sendErr;
          }
          throw sendErr;
        }
        console.log('�?Transaction submitted, signature:', signature);
        
        setTxSignature(signature);
        toast({
          title: t('common.success'),
          description: t('buyModal.transactionSubmittedToast'),
        });

        const confirmation = await connection.confirmTransaction(signature, 'confirmed');
        
        if (confirmation.value.err) {
          showError(`Transaction failed: ${confirmation.value.err.toString()}`);
        } else {
          toast({
            title: t('common.success'),
            description: t('buyModal.transactionConfirmedToast'),
          });
        }
        
      } catch (walletError) {
        console.error('Wallet error:', walletError);
        const sendTxError = findSendTransactionError(walletError);
        const walletErrorMessage =
          walletError?.message ||
          walletError?.cause?.message ||
          walletError?.cause?.toString?.() ||
          'Unknown error';

        if (sendTxError) {
          let logs = sendTxError.enrichedLogs || [];
          if ((!logs || !logs.length) && typeof sendTxError.getLogs === 'function') {
            try {
              logs = await sendTxError.getLogs(connection);
            } catch (logErr) {
              console.warn('Failed to fetch transaction logs (post-catch):', logErr);
            }
          }
          const shortLogs = logs?.length ? logs.slice(-12).join('\n') : '';
          const friendly = deriveFriendlySendError(sendTxError.message || walletErrorMessage, logs);
          const baseMessage = sendTxError.detailedMessage || sendTxError.message || walletErrorMessage;
          const primaryMessage = friendly || baseMessage;
          const extraDetail =
            friendly && baseMessage && !baseMessage.includes(friendly) ? baseMessage : '';
          const includeLogs = shortLogs && !primaryMessage.includes('Logs:') && !extraDetail.includes('Logs:');
          const description = [primaryMessage, extraDetail, includeLogs ? `Logs:\n${shortLogs}` : '']
            .filter(Boolean)
            .join('\n\n');
          showError(description);
          return;
        }
        
        if (walletErrorMessage.includes('User rejected')) {
          showError('Transaction was rejected by user');
        } else if (walletErrorMessage.includes('blockhash')) {
          showError('Transaction expired. Please try again.');
        } else if (walletErrorMessage.includes('insufficient')) {
          showError('Insufficient funds for transaction');
        } else if (walletErrorMessage.includes('network')) {
          showError('Network error. Please check your connection.');
        } else if (walletErrorMessage.includes('signTransaction')) {
          showError('Wallet failed to sign the transaction. Please reconnect your wallet and retry.');
        } else {
          showError('Failed to sign/send transaction: ' + walletErrorMessage);
        }
      }

    } catch (err) {
      console.error('General error:', err);
      showError('Network error: ' + err.message);
    } finally {
      setIsLoading(false);
    }
  };

  const resetForm = () => {
    setTradeMode('buy');
    setAmountIn('');
    setTokenInput('');
    setLastInputField('sol');
    setSlippageChoice(`${DEFAULT_SLIPPAGE}`);
    setCustomSlippage('');
    setQuoteState({ loading: false, data: null, error: '' });
    setTxSignature('');
  };

  const handleClose = () => {
    resetForm();
    onClose();
  };

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text);
  };

  // Quick amount buttons
  const quickAmounts = ['0.0001', '0.001', '0.01', '0.1', '1'];

  const quoteInfo = quoteState.data || {};
  const tokenSymbol = token?.tokenName || token?.tokenSymbol || token?.symbol || t('buyModal.tokenSymbolFallback');
  const payAssetLabel = assetIsSol(quoteInfo.payAsset) ? 'SOL' : tokenSymbol;
  const receiveAssetLabel = assetIsSol(quoteInfo.receiveAsset) ? 'SOL' : tokenSymbol;
  const tradeButtonLabel = tradeMode === 'buy' ? t('buyModal.buyToken') : t('buyModal.sellToken');
  const tokenAmountLabel = tradeMode === 'buy' ? t('buyModal.tokenAmountLabel') : t('buyModal.tokenAmountSellLabel');
  const solAmountLabel = tradeMode === 'buy' ? t('buyModal.solAmountLabel') : t('buyModal.solTargetLabel');

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-2xl w-full max-w-[95vw] max-h-[90vh] flex flex-col">
        <DialogHeader className="flex-shrink-0">
          <DialogTitle className="flex items-center gap-2">
            <Zap className="h-5 w-5 text-green-500" />
            {t('buyModal.title')} {token?.tokenName || 'Token'}
          </DialogTitle>
          <DialogDescription>
            {t('buyModal.description')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 overflow-y-auto overflow-x-hidden flex-1 pr-2 -mr-2" style={{ maxHeight: 'calc(90vh - 200px)' }}>
          {/* Token Info Card */}
          <Card variant="elevated" padding="none">
            <CardContent className="p-4">
              <div className="flex items-center gap-4 w-full">
                <div className="relative flex-shrink-0">
                  {token?.tokenIcon ? (
                    <img 
                      src={token.tokenIcon} 
                      alt={token.tokenName} 
                      className="w-16 h-16 rounded-lg border-2 border-border object-cover"
                    />
                  ) : (
                    <div className="w-16 h-16 bg-gradient-to-r from-blue-500 to-purple-500 rounded-lg flex items-center justify-center text-white font-bold text-xl border-2 border-border">
                      {(token?.tokenName || '?').charAt(0).toUpperCase()}
                    </div>
                  )}
                  <div className="absolute -top-4 -right-2">
                    <Badge variant="outline" className="text-xs px-1.5 py-0.5 bg-green-50 text-green-600 border-green-200">
                      <TrendingUp className="w-3 h-3 mr-1" />
{t('buyModal.new')}
                    </Badge>
                  </div>
                </div>
                <div className="flex-1 min-w-0">
                  <h3 className="font-semibold text-xl text-foreground mb-2">
                    {token?.tokenName || 'Unknown Token'}
                  </h3>
                  <div className="space-y-1">
                    <div className="flex items-center gap-2 text-sm text-muted-foreground">
                      <span className="text-xs text-muted-foreground/70 flex-shrink-0">{t('buyModal.address')}:</span>
                      <span className="font-mono text-sm truncate">
                        {token?.tokenAddress ? 
                          `${token.tokenAddress.slice(0, 12)}...${token.tokenAddress.slice(-8)}` : 
                          'N/A'
                        }
                      </span>
                      {token?.tokenAddress && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => copyToClipboard(token.tokenAddress)}
                          className="h-6 w-6 p-0 hover:bg-muted"
                          title={t('buyModal.copyAddress')}
                        >
                          <Copy className="h-3 w-3" />
                        </Button>
                      )}
                    </div>
                    {token?.mktCap && (
                      <div className="flex items-center gap-2 text-sm">
                        <span className="text-xs text-muted-foreground/70">{t('buyModal.marketCap')}:</span>
                        <span className="font-semibold text-blue-600">
                          ${token.mktCap >= 1000000 ? `${(token.mktCap / 1000000).toFixed(2)}M` : 
                            token.mktCap >= 1000 ? `${(token.mktCap / 1000).toFixed(1)}K` : 
                            token.mktCap.toFixed(2)}
                        </span>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card variant="outline" padding="none">
            <CardContent className="space-y-4 p-4">
              <div className="flex items-center justify-between">
                <div className="font-medium text-foreground">{t('buyModal.pumpControlsTitle')}</div>
                <div className="flex gap-2 rounded-md bg-muted p-1">
                  {['buy', 'sell'].map((mode) => (
                    <button
                      key={mode}
                      type="button"
                      onClick={() => {
                        if (tradeMode !== mode) {
                          clearTradeResultState();
                          setTradeMode(mode);
                          setAmountIn('');
                          setTokenInput('');
                          setLastInputField('sol');
                          setQuoteState({ loading: false, data: null, error: '' });
                        }
                      }}
                      className={cn(
                        'px-3 py-1 text-sm font-medium rounded-md transition-colors',
                        tradeMode === mode ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
                      )}
                    >
                      {mode === 'buy' ? t('buyModal.buyTab') : t('buyModal.sellTab')}
                    </button>
                  ))}
                </div>
              </div>

              <div className="grid gap-3">
                {tradeMode === 'buy' ? (
                  <>
                    <div className="space-y-2">
                      <div>
                        <div className="flex items-center justify-between text-xs text-muted-foreground mb-1">
                          <span>{solAmountLabel}</span>
                          {lastInputField === 'sol' && <span className="text-primary">{t('buyModal.inputActive')}</span>}
                        </div>
                        <Input
                          inputMode="decimal"
                          placeholder="0.0"
                          value={amountIn}
                          onChange={(e) => handleSolInputChange(e.target.value)}
                        />
                      </div>
                      <div className="grid grid-cols-5 gap-2">
                        {quickAmounts.map((amount) => (
                          <Button
                            key={amount}
                            variant="outline"
                            size="sm"
                            onClick={() => handleSolInputChange(amount)}
                            disabled={isLoading}
                            className="text-xs px-2 py-1 h-8 flex-1"
                          >
                            {amount} SOL
                          </Button>
                        ))}
                      </div>
                    </div>
                    <div>
                      <div className="flex items-center justify-between text-xs text-muted-foreground mb-1">
                        <span>{tokenAmountLabel}</span>
                        {lastInputField === 'token' && <span className="text-primary">{t('buyModal.inputActive')}</span>}
                      </div>
                      <Input
                        inputMode="decimal"
                        placeholder="0.0"
                        value={tokenInput}
                        onChange={(e) => handleTokenInputChange(e.target.value)}
                      />
                    </div>
                  </>
                ) : (
                  <>
                    <div>
                      <div className="flex items-center justify-between text-xs text-muted-foreground mb-1">
                        <span>{tokenAmountLabel}</span>
                        {lastInputField === 'token' && <span className="text-primary">{t('buyModal.inputActive')}</span>}
                      </div>
                      <Input
                        inputMode="decimal"
                        placeholder="0.0"
                        value={tokenInput}
                        onChange={(e) => handleTokenInputChange(e.target.value)}
                      />
                    </div>
                    <div>
                      <div className="flex items-center justify-between text-xs text-muted-foreground mb-1">
                        <span>{solAmountLabel}</span>
                        {lastInputField === 'sol' && <span className="text-primary">{t('buyModal.inputActive')}</span>}
                      </div>
                      <Input
                        inputMode="decimal"
                        placeholder="0.0"
                        value={amountIn}
                        onChange={(e) => handleSolInputChange(e.target.value)}
                      />
                    </div>
                  </>
                )}
              </div>

              <div className="space-y-2">
                <div className="text-xs text-muted-foreground">{t('buyModal.slippageLabel')}</div>
                <div className="flex flex-wrap gap-2">
                  {SLIPPAGE_OPTIONS.map((option) => (
                    <button
                      key={option}
                      type="button"
                      onClick={() => handleSlippageSelect(`${option}`)}
                      className={cn(
                        'px-2.5 py-1 rounded-full text-xs transition-colors',
                        slippageChoice === `${option}`
                          ? 'bg-primary text-primary-foreground'
                          : 'bg-muted text-muted-foreground hover:text-foreground',
                      )}
                    >
                      {option}%
                    </button>
                  ))}
                  <button
                    type="button"
                    onClick={() => handleSlippageSelect('custom')}
                    className={cn(
                      'px-2.5 py-1 rounded-full text-xs transition-colors',
                      slippageChoice === 'custom'
                        ? 'bg-primary text-primary-foreground'
                        : 'bg-muted text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {t('buyModal.customSlippage')}
                  </button>
                </div>
                {slippageChoice === 'custom' && (
                  <Input
                    inputMode="decimal"
                    placeholder="0.1 - 5"
                    value={customSlippage}
                    onChange={(e) => handleCustomSlippageChange(e.target.value)}
                    className="w-32"
                  />
                )}
                <div className="text-[10px] text-muted-foreground">{t('buyModal.slippageRangeNote')}</div>
              </div>

              <div className="rounded-md bg-muted/50 p-3 text-sm space-y-1 border border-muted">
                {quoteState.loading ? (
                  <div className="flex items-center gap-2 text-muted-foreground">
                    <LoadingSpinner className="w-4 h-4" />
                    {t('buyModal.quoteCalculating')}
                  </div>
                ) : quoteState.error ? (
                  <div className="text-red-500 text-xs">{quoteState.error}</div>
                ) : quoteState.data ? (
                  <>
                    <div className="flex justify-between">
                      <span>{t('buyModal.expectedReceive')}</span>
                      <span>
                        {quoteInfo.receiveAmount || '0'} {receiveAssetLabel}
                      </span>
                    </div>
                    <div className="flex justify-between text-xs text-muted-foreground">
                      <span>
                        {t('buyModal.minReceive')} ({parsedSlippage}% {t('buyModal.slippageLabel')})
                      </span>
                      <span>
                        {quoteInfo.minReceiveAmount || '0'} {receiveAssetLabel}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>{t('buyModal.maxPay')}</span>
                      <span>
                        {quoteInfo.payAmountWithSlippage || quoteInfo.payAmount || '0'} {payAssetLabel}
                      </span>
                    </div>
                    <div className="flex justify-between text-xs text-muted-foreground">
                      <span>{t('buyModal.priceImpact')}</span>
                      <span>{quoteInfo.priceImpactPct ? `${quoteInfo.priceImpactPct}%` : '0%'}</span>
                    </div>
                  </>
                ) : (
                  <div className="text-xs text-muted-foreground">{t('buyModal.quoteEmptyHint')}</div>
                )}
              </div>
            </CardContent>
          </Card>

          {txSignature && (
            <Card variant="elevated" padding="sm">
              <CardContent className="p-3 space-y-2 overflow-x-hidden">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium flex-shrink-0">{t('buyModal.transactionSignature')}:</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => copyToClipboard(txSignature)}
                    className="h-6 w-6 p-0 flex-shrink-0"
                  >
                    <Copy className="h-3 w-3" />
                  </Button>
                </div>
                <div className="font-mono text-xs text-muted-foreground break-all overflow-x-hidden max-w-full">
                  {txSignature}
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  className="w-full"
                  onClick={() => window.open(`https://solscan.io/tx/${txSignature}?cluster=devnet`, '_blank')}
                >
                  <ExternalLink className="h-4 w-4 mr-2" />
                  {t('buyModal.viewOnSolscan')}
                </Button>
              </CardContent>
            </Card>
          )}
        </div>

        <DialogFooter className="flex flex-row gap-3 pt-4 border-t flex-shrink-0">
          <Button 
            variant="outline" 
            onClick={handleClose}
            disabled={isLoading}
            className="flex-1 h-10"
          >
            {t('buyModal.cancel')}
          </Button>
          <Button 
            variant="success"
            onClick={handleBuy}
            disabled={isLoading || !tradeAmount}
            className="flex-1 h-10"
          >
            {isLoading ? (
              <>
                <LoadingSpinner className="mr-2 h-4 w-4" />
                {t('buyModal.creating')}
              </>
            ) : (
              <>
                <Zap className="mr-2 h-4 w-4" />
                {tradeButtonLabel}
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default BuyModal;
