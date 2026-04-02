import React, { useState } from 'react';
import { useWallet, useConnection } from '@solana/wallet-adapter-react';
import { Transaction, VersionedTransaction, Message } from '@solana/web3.js';
import { Buffer } from 'buffer';
import { motion } from 'framer-motion';
import { 
  Card, 
  CardContent, 
  CardDescription, 
  CardHeader, 
  CardTitle 
} from '../UI/card';
import { Button } from '../UI/Button';
import { Input } from '../UI/input';
import { Badge } from '../UI/badge';
import { 
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../UI/select';
import { useToast } from '../../hooks/use-toast';
import { 
  Loader2, 
  Wallet, 
  ArrowLeftRight, 
  DollarSign, 
  Zap, 
  Clock, 
  Info,
  ExternalLink,
  CheckCircle2,
  AlertCircle,
  Copy,
  Waves
} from 'lucide-react';
import { cn } from '../../lib/utils';
import { useTranslation } from '../../i18n/LanguageContext';

const PoolCreationNew = ({ onNavigateBack, showHeader = true }) => {
  const { publicKey, connected, sendTransaction, signTransaction } = useWallet();
  const { connection } = useConnection();
  const { toast } = useToast();
  const { t } = useTranslation();
  
  // Form state
  const [formData, setFormData] = useState({
    tokenMint0: '',
    tokenMint1: '',
    initialPrice: '',
    feeTier: '30', // Default to 0.3% (30 basis points)
    openTime: Math.floor(Date.now() / 1000), // Current timestamp
  });
  
  // UI state
  const [isLoading, setIsLoading] = useState(false);
  const [txSignature, setTxSignature] = useState('');
  const [debugInfo, setDebugInfo] = useState(null);

  // API URL
  const API_URL = '/api/v1';

  // Fee tier options based on common CLMM standards (in basis points)
  const feeTiers = [
    { value: '1', label: t('poolCreation.feeTiers.tier1'), tickSpacing: 1, description: t('poolCreation.feeTiers.tier1Desc') },
    { value: '5', label: t('poolCreation.feeTiers.tier5'), tickSpacing: 10, description: t('poolCreation.feeTiers.tier5Desc') },
    { value: '30', label: t('poolCreation.feeTiers.tier30'), tickSpacing: 60, description: t('poolCreation.feeTiers.tier30Desc') },
    { value: '100', label: t('poolCreation.feeTiers.tier100'), tickSpacing: 200, description: t('poolCreation.feeTiers.tier100Desc') }
  ];
  const activeFeeTier = feeTiers.find(tier => tier.value === formData.feeTier);

  const handleInputChange = (name, value) => {
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));

    // If openTime is updated through the date picker, convert it to Unix timestamp
    if (name === 'openTime') {
      try {
        const timestamp = Math.floor(new Date(value).getTime() / 1000);
        setFormData(prev => ({ ...prev, openTime: timestamp }));
      } catch (err) {
        console.error("Date parsing error:", err);
      }
    }
  };

  const validateForm = () => {
    if (!formData.tokenMint0.trim()) return t('poolCreation.errors.token0Required');
    if (!formData.tokenMint1.trim()) return t('poolCreation.errors.token1Required');
    if (formData.tokenMint0 === formData.tokenMint1) return t('poolCreation.errors.tokensMustBeDifferent');
    if (!formData.initialPrice || parseFloat(formData.initialPrice) <= 0) return t('poolCreation.errors.priceRequired');
    return null;
  };

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text);
    toast({
      title: t('poolCreation.copied'),
      description: t('poolCreation.txIdCopied'),
    });
  };

  const createPool = async () => {
    if (!connected || !publicKey) {
      toast({
        title: t('poolCreation.errors.walletNotConnected'),
        description: t('poolCreation.errors.connectWalletFirst'),
        variant: "destructive",
      });
      return;
    }

    const validationError = validateForm();
    if (validationError) {
      toast({
        title: t('poolCreation.errors.formValidationFailed'),
        description: validationError,
        variant: "destructive",
      });
      return;
    }

    setIsLoading(true);
    setTxSignature('');
    setDebugInfo(null);

    try {
      // Set current time as the open time
      const currentTime = Math.floor(Date.now() / 1000);

      // Prepare the API request
      const requestData = {
        chain_id: 100000, // Solana devnet ID
        token_mint_0: formData.tokenMint0,
        token_mint_1: formData.tokenMint1,
        initial_price: formData.initialPrice.toString(),
        fee_tier: parseInt(formData.feeTier),
        open_time: currentTime, // Use current time
        user_wallet_address: publicKey.toString()
      };

      // Call the backend API
      const response = await fetch(`${API_URL}/trade/create_pool`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(requestData)
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`API error: ${response.status} ${errorText}`);
      }

      const result = await response.json();
      const txHash = result.data?.txHash;
      
      if (txHash) {
        // Decode and process transaction
        const transactionBuffer = Buffer.from(txHash, 'base64');
        let transaction;
        
        try {
          transaction = Transaction.from(transactionBuffer);
        } catch (legacyError) {
          try {
            transaction = VersionedTransaction.deserialize(transactionBuffer);
          } catch (versionedError) {
            transaction = new Transaction();
            transaction.feePayer = publicKey;
            const { blockhash } = await connection.getLatestBlockhash();
            transaction.recentBlockhash = blockhash;
          }
        }

        const sendOptions = {
          skipPreflight: true,
          maxRetries: 5
        };
        
        let signature;
        try {
          if (signTransaction) {
            const signedTx = await signTransaction(transaction);
            signature = await connection.sendRawTransaction(
              signedTx.serialize(),
              sendOptions
            );
          } else {
            signature = await sendTransaction(transaction, connection, sendOptions);
          }
          
          setTxSignature(signature);
          
          toast({
            title: t('poolCreation.errors.transactionSent'),
            description: t('poolCreation.errors.waitingConfirmation'),
          });
          
          const confirmation = await connection.confirmTransaction(signature);
          
          toast({
            title: t('poolCreation.errors.poolCreated'),
            description: `${t('poolCreation.errors.transactionConfirmed')}: ${signature.slice(0, 8)}...${signature.slice(-8)}`,
          });
          
        } catch (signError) {
          const errorMessage = `${t('poolCreation.errors.transactionFailed')}: ${signError.message}`;
          
          toast({
            title: t('poolCreation.errors.transactionFailed'),
            description: errorMessage,
            variant: "destructive",
          });
          
          throw signError;
        }
      } else {
        throw new Error(t('poolCreation.errors.missingTxHash'));
      }
      
    } catch (err) {
      toast({
        title: t('poolCreation.errors.createPoolFailed'),
        description: err.message,
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  };

  if (!connected) {
    return (
      <div className="container mx-auto px-4 py-8">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="max-w-md mx-auto"
        >
          <Card className="text-center">
            <CardHeader>
              <div className="w-16 h-16 mx-auto bg-primary/10 rounded-full flex items-center justify-center mb-4">
                <Wallet className="w-8 h-8 text-primary" />
              </div>
              <CardTitle className="text-2xl">{t('header.wallet.connect')}</CardTitle>
              <CardDescription>
                {t('poolCreation.connectWallet')}
              </CardDescription>
            </CardHeader>
          </Card>
        </motion.div>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-8">
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="max-w-4xl mx-auto space-y-8"
      >
        {/* Header */}
        {showHeader && (
          <div className="text-center space-y-4">
            <div className="flex items-center justify-center space-x-2">
              <div className="w-12 h-12 bg-gradient-to-br from-blue-500 to-purple-600 rounded-xl flex items-center justify-center">
                <Waves className="w-6 h-6 text-white" />
              </div>
              <h1 className="text-3xl font-bold bg-gradient-to-r from-blue-600 to-purple-600 bg-clip-text text-transparent">
                {t('poolCreation.newTitle')}
              </h1>
            </div>
            <p className="text-muted-foreground max-w-2xl mx-auto">
              {t('poolCreation.newDescription')}
            </p>
            {onNavigateBack && (
              <Button 
                variant="outline" 
                onClick={onNavigateBack}
                className="mb-4"
              >
                ← {t('poolCreation.backToDashboard')}
              </Button>
            )}
          </div>
        )}

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          {/* Main Form */}
          <div className="lg:col-span-2">
            <Card className="border border-border/60 shadow-sm">
              <CardContent className="p-6 space-y-8">
                <div className="space-y-4">
                  <div className="flex items-center gap-3">
                    <div className="p-2 rounded-xl bg-primary/10 text-primary">
                      <ArrowLeftRight className="w-5 h-5" />
                    </div>
                    <div>
                      <p className="font-semibold">{t('poolCreation.tokenPair')}</p>
                      <p className="text-sm text-muted-foreground">
                        {t('poolCreation.selectTokens')}
                      </p>
                    </div>
                  </div>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
                        {t('poolCreation.token0Required')}
                      </label>
                      <Input
                        placeholder="例如: So11111111111111111111111111111111111111112"
                        value={formData.tokenMint0}
                        onChange={(e) => handleInputChange('tokenMint0', e.target.value)}
                        disabled={isLoading}
                      />
                      <p className="text-xs text-muted-foreground">
                        {t('poolCreation.token0Desc')}
                      </p>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
                        {t('poolCreation.token1Required')}
                      </label>
                      <Input
                        placeholder="例如: EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
                        value={formData.tokenMint1}
                        onChange={(e) => handleInputChange('tokenMint1', e.target.value)}
                        disabled={isLoading}
                      />
                      <p className="text-xs text-muted-foreground">
                        {t('poolCreation.token1Desc')}
                      </p>
                    </div>
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="rounded-2xl border border-border/60 bg-card/40 p-4 space-y-3">
                    <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
                      <DollarSign className="w-4 h-4 text-primary" />
                      <span>{t('poolCreation.initialPrice')}</span>
                    </div>
                    <Input
                      type="number"
                      placeholder="0.001"
                      step="any"
                      min="0"
                      value={formData.initialPrice}
                      onChange={(e) => handleInputChange('initialPrice', e.target.value)}
                      disabled={isLoading}
                    />
                    <p className="text-xs text-muted-foreground">
                      {t('poolCreation.priceDesc')}
                    </p>
                  </div>
                  <div className="rounded-2xl border border-border/60 bg-card/40 p-4 space-y-3">
                    <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
                      <Zap className="w-4 h-4 text-primary" />
                      <span>{t('poolCreation.feeTier')}</span>
                    </div>
                    <Select
                      value={formData.feeTier}
                      onValueChange={(value) => handleInputChange('feeTier', value)}
                      disabled={isLoading}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder={t('poolCreation.selectFeeTierPlaceholder')} />
                      </SelectTrigger>
                      <SelectContent>
                        {feeTiers.map(tier => (
                          <SelectItem key={tier.value} value={tier.value}>
                            <div className="flex flex-col">
                              <span className="font-medium">{tier.label}</span>
                              <span className="text-xs text-muted-foreground">{tier.description}</span>
                            </div>
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    {activeFeeTier && (
                      <Badge variant="outline" className="text-xs w-fit">
                        {t('poolCreation.tickSpacing')}: {activeFeeTier.tickSpacing}
                      </Badge>
                    )}
                  </div>
                </div>

                <div className="rounded-2xl border border-dashed border-border/60 bg-muted/20 p-4 flex items-start gap-4">
                  <div className="p-2 rounded-full bg-background text-primary shadow">
                    <Clock className="w-4 h-4" />
                  </div>
                  <div>
                    <p className="font-semibold">{t('poolCreation.launchSettings')}</p>
                    <p className="text-sm text-muted-foreground">
                      {t('poolCreation.poolOpenSettings')}
                    </p>
                    <div className="flex items-center gap-2 text-sm text-foreground mt-3">
                      <CheckCircle2 className="w-4 h-4 text-green-500" />
                      <span>{t('poolCreation.useCurrentTime')}</span>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {t('poolCreation.immediateOpen')}
                    </p>
                  </div>
                </div>

                <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="text-sm text-muted-foreground flex items-center gap-2">
                    <Info className="w-4 h-4" />
                    <span>{t('poolCreation.setPriceDesc')}</span>
                  </div>
                  <Button
                    onClick={createPool}
                    disabled={isLoading}
                    className="w-full sm:w-auto h-11 px-6 text-base"
                  >
                    {isLoading ? (
                      <>
                        <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                        {t('poolCreation.creatingPool')}
                      </>
                    ) : (
                      <>
                        <Waves className="w-4 h-4 mr-2" />
                        {t('poolCreation.createPoolBtn')}
                      </>
                    )}
                  </Button>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Sidebar */}
          <div className="space-y-6">
            {/* Pool Information */}
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center space-x-2">
                  <Info className="w-5 h-5" />
                  <span>{t('poolCreation.poolInfoTitle')}</span>
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="flex justify-between text-sm">
                  <span className="text-muted-foreground">{t('poolCreation.type')}:</span>
                  <Badge>Raydium CLMM</Badge>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-muted-foreground">{t('poolCreation.program')}:</span>
                  <span className="font-mono text-xs">675kPX...1Mp8</span>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-muted-foreground">{t('poolCreation.network')}:</span>
                  <Badge variant="outline">Devnet</Badge>
                </div>
                <div className="flex justify-between text-sm">
                  <span className="text-muted-foreground">{t('poolCreation.wallet')}:</span>
                  <span className="font-mono text-xs">
                    {publicKey?.toString().slice(0, 4)}...{publicKey?.toString().slice(-4)}
                  </span>
                </div>
              </CardContent>
            </Card>

            {/* Transaction Result */}
            {txSignature && (
              <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
              >
                <Card className="border-green-200 bg-green-50 dark:bg-green-950 dark:border-green-800">
                  <CardHeader>
                    <CardTitle className="flex items-center space-x-2 text-green-700 dark:text-green-300">
                      <CheckCircle2 className="w-5 h-5" />
                      <span>{t('poolCreation.transactionSuccess')}</span>
                    </CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div>
                      <label className="text-xs text-muted-foreground">{t('poolCreation.transactionId')}:</label>
                      <div className="flex items-center space-x-2 mt-1">
                        <code className="text-xs bg-background px-2 py-1 rounded flex-1 break-all">
                          {txSignature}
                        </code>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => copyToClipboard(txSignature)}
                        >
                          <Copy className="w-3 h-3" />
                        </Button>
                      </div>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      className="w-full"
                      asChild
                    >
                      <a 
                        href={`https://explorer.solana.com/tx/${txSignature}?cluster=devnet`} 
                        target="_blank" 
                        rel="noopener noreferrer"
                        className="flex items-center justify-center space-x-2"
                      >
                        <ExternalLink className="w-3 h-3" />
                        <span>{t('poolCreation.viewOnExplorer')}</span>
                      </a>
                    </Button>
                  </CardContent>
                </Card>
              </motion.div>
            )}

            {/* Debug Information */}
            {debugInfo && (
              <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
              >
                <Card>
                  <CardHeader>
                    <CardTitle className="flex items-center space-x-2">
                      <AlertCircle className="w-5 h-5" />
                      <span>{t('poolCreation.debugInfoTitle')}</span>
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <pre className="text-xs bg-muted p-3 rounded-lg overflow-x-auto">
                      {JSON.stringify(debugInfo, null, 2)}
                    </pre>
                  </CardContent>
                </Card>
              </motion.div>
            )}
          </div>
        </div>
      </motion.div>
    </div>
  );
};

export default PoolCreationNew;
