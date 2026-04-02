import React, { useState } from 'react';
import { motion } from 'framer-motion';
import { useWallet, useConnection } from '@solana/wallet-adapter-react';
import { 
  PublicKey, 
  Transaction, 
  SystemProgram,
  Keypair,
  LAMPORTS_PER_SOL
} from '@solana/web3.js';
import {
  createInitializeMintInstruction,
  createAssociatedTokenAccountInstruction,
  createMintToInstruction,
  getAssociatedTokenAddress,
  TOKEN_PROGRAM_ID,
  TOKEN_2022_PROGRAM_ID
} from '@solana/spl-token';

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../UI/enhanced-card';
import { Button } from '../UI/Button';
import { EnhancedInput, FormField, NumberInput } from '../UI/enhanced-input';
import { Switch } from '../UI/switch';
import { useTranslation } from '../../i18n/LanguageContext';
import { useToast } from '../../hooks/use-toast';
import { 
  Coins, 
  Wallet, 
  Settings, 
  Info, 
  CheckCircle, 
  AlertCircle,
  Loader2,
  ExternalLink,
  ArrowLeft
} from 'lucide-react';
import { cn } from '../../lib/utils';

// Standard sizes for SPL Token accounts
const MINT_SIZE = 82; // Size of a mint account in bytes

const TokenCreationNew = ({ onNavigateBack }) => {
  const { publicKey, connected, signTransaction, sendTransaction } = useWallet();
  const { connection } = useConnection();
  const { t } = useTranslation();
  const { toast } = useToast();
  
  // Form state
  const [formData, setFormData] = useState({
    name: '',
    symbol: '',
    decimals: 9,
    supply: 1000000,
    description: '',
    image: '',
    useToken2022: false,
    freezeAuthority: true,
    updateAuthority: true
  });
  
  // UI state
  const [isLoading, setIsLoading] = useState(false);
  const [currentStep, setCurrentStep] = useState(1);
  const [errors, setErrors] = useState({});
  const [txSignature, setTxSignature] = useState('');
  const [tokenMint, setTokenMint] = useState('');

  const handleInputChange = (e) => {
    const { name, value, type, checked } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: type === 'checkbox' ? checked : value
    }));
    
    // Clear error when user starts typing
    if (errors[name]) {
      setErrors(prev => ({ ...prev, [name]: '' }));
    }
  };

  const handleSwitchChange = (name, checked) => {
    setFormData(prev => ({
      ...prev,
      [name]: checked
    }));
  };

  const validateForm = () => {
    const newErrors = {};
    
    if (!formData.name.trim()) {
      newErrors.name = t('tokenCreation.formValidation.nameRequired');
    }
    
    if (!formData.symbol.trim()) {
      newErrors.symbol = t('tokenCreation.formValidation.symbolRequired');
    } else if (formData.symbol.length > 10) {
      newErrors.symbol = t('tokenCreation.formValidation.symbolTooLong');
    }
    
    if (formData.decimals < 0 || formData.decimals > 9) {
      newErrors.decimals = t('tokenCreation.formValidation.decimalsRange');
    }
    
    if (formData.supply <= 0) {
      newErrors.supply = t('tokenCreation.formValidation.supplyRequired');
    }
    
    // Check if the final token amount would exceed JavaScript's safe integer limit
    const finalAmount = formData.supply * Math.pow(10, formData.decimals);
    if (finalAmount > Number.MAX_SAFE_INTEGER) {
      newErrors.supply = t('tokenCreation.formValidation.supplyTooLarge');
    }
    
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const createToken = async () => {
    if (!validateForm()) {
      toast({
        title: t('common.error'),
        description: t('tokenCreation.formValidation.nameRequired'),
        variant: "destructive",
      });
      return;
    }

    if (isLoading) return;
    setIsLoading(true);
    setTxSignature('');
    setTokenMint('');
    
    try {
      console.log('🚀 Starting token creation...');
      console.log('Form data:', formData);
      console.log('Connected wallet:', publicKey.toString());
      
      // Generate new mint keypair
      const mintKeypair = Keypair.generate();
      console.log('✅ Generated mint keypair:', mintKeypair.publicKey.toString());

      // For now, use Token Program (Token-2022 will be added later when packages support it)
      const tokenProgram = TOKEN_PROGRAM_ID;
      console.log('✅ Using token program:', tokenProgram.toString());
      
      // Calculate space needed for mint account  
      const mintSpace = MINT_SIZE;
      console.log('✅ Mint space calculated:', mintSpace);

      // Calculate rent
      const rentExemptBalance = await connection.getMinimumBalanceForRentExemption(mintSpace);
      console.log('✅ Rent exempt balance:', rentExemptBalance);

      // Get associated token account
      const associatedTokenAccount = await getAssociatedTokenAddress(
        mintKeypair.publicKey,
        publicKey,
        false,
        tokenProgram
      );
      console.log('✅ Associated token account:', associatedTokenAccount.toString());

      const transaction = new Transaction();
      console.log('✅ Transaction created');

      // Create mint account
      const createAccountIx = SystemProgram.createAccount({
        fromPubkey: publicKey,
        newAccountPubkey: mintKeypair.publicKey,
        space: mintSpace,
        lamports: rentExemptBalance,
        programId: tokenProgram,
      });
      transaction.add(createAccountIx);
      console.log('✅ Added create account instruction');

      // Initialize mint
      const freezeAuthority = formData.freezeAuthority ? publicKey : null;
      const initMintIx = createInitializeMintInstruction(
        mintKeypair.publicKey,    // mint
        formData.decimals,        // decimals
        publicKey,               // mintAuthority
        freezeAuthority          // freezeAuthority (can be null)
      );
      transaction.add(initMintIx);
      console.log('✅ Added initialize mint instruction with decimals:', formData.decimals);

      // Create associated token account
      const createATAIx = createAssociatedTokenAccountInstruction(
        publicKey,           // payer
        associatedTokenAccount, // associatedToken
        publicKey,           // owner
        mintKeypair.publicKey   // mint
      );
      transaction.add(createATAIx);
      console.log('✅ Added create ATA instruction');

      // Mint initial supply
      const mintAmount = formData.supply * Math.pow(10, formData.decimals);
      console.log('💰 Mint amount calculated:', mintAmount);
      
      const mintToIx = createMintToInstruction(
        mintKeypair.publicKey,    // mint
        associatedTokenAccount,   // destination
        publicKey,               // authority (mint authority)
        mintAmount              // amount
      );
      transaction.add(mintToIx);
      console.log('✅ Added mint to instruction');

      // Get latest blockhash
      const { blockhash } = await connection.getLatestBlockhash();
      transaction.recentBlockhash = blockhash;
      transaction.feePayer = publicKey;
      console.log('✅ Set blockhash and fee payer');

      // Sign and send transaction
      console.log('🖊️ Requesting wallet signature...');
      
      // Sign with mint keypair first
      transaction.partialSign(mintKeypair);
      console.log('✅ Partially signed with mint keypair');
      
      // Get wallet signature
      const signedTransaction = await signTransaction(transaction);
      console.log('✅ Transaction signed by wallet');
      
      // Send with sendRawTransaction
      console.log('📡 Sending transaction...');
      const txid = await connection.sendRawTransaction(signedTransaction.serialize());
      await connection.confirmTransaction(txid, 'confirmed');
      console.log('✅ Transaction confirmed!');

      setTxSignature(txid);
      setTokenMint(mintKeypair.publicKey.toString());
      setCurrentStep(3); // Success step
      
      toast({
        title: t('common.success'),
        description: t('tokenCreation.tokenCreated'),
        variant: "default",
      });

    } catch (err) {
      console.error('Token creation error:', err);
      toast({
        title: t('common.error'),
        description: `Failed to create token: ${err.message}`,
        variant: "destructive",
      });
    } finally {
      setIsLoading(false);
    }
  };

  const nextStep = () => {
    if (currentStep === 1 && validateForm()) {
      setCurrentStep(2);
    }
  };

  const prevStep = () => {
    if (currentStep > 1) {
      setCurrentStep(currentStep - 1);
    }
  };

  // Wallet not connected state
  if (!connected) {
    return (
      <div className="container mx-auto px-4 py-8">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="max-w-2xl mx-auto"
        >
          <Card variant="glass" className="text-center">
            <CardHeader>
              <div className="mx-auto w-16 h-16 bg-primary/10 rounded-full flex items-center justify-center mb-4">
                <Wallet className="w-8 h-8 text-primary" />
              </div>
              <CardTitle className="text-2xl">{t('tokenCreation.title')}</CardTitle>
              <CardDescription>
                {t('tokenCreation.connectWallet')}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Button 
                variant="solana" 
                size="lg" 
                className="w-full"
                onClick={() => {
                  // This will be handled by the wallet adapter
                  document.querySelector('[data-testid="wallet-adapter-button"]')?.click();
                }}
              >
                <Wallet className="w-5 h-5 mr-2" />
                {t('header.wallet.connect')}
              </Button>
            </CardContent>
          </Card>
        </motion.div>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-8 bg-background/50">
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="max-w-4xl mx-auto space-y-8"
      >
        {/* Header */}
        <div className="flex items-center space-x-4">
          {onNavigateBack && (
            <Button variant="ghost" size="icon" onClick={onNavigateBack}>
              <ArrowLeft className="w-5 h-5" />
            </Button>
          )}
          <div>
            <h1 className="text-3xl font-bold bg-gradient-to-r from-blue-600 to-purple-600 bg-clip-text text-transparent">
              {t('tokenCreation.title')}
            </h1>
            <p className="text-muted-foreground mt-1">
              {t('tokenCreation.deployToken')}
            </p>
          </div>
        </div>

        {/* Progress Steps */}
        <div className="flex items-center justify-center space-x-4 mb-8">
          {[1, 2, 3].map((step) => (
            <div key={step} className="flex items-center">
              <div className={cn(
                "w-10 h-10 rounded-full flex items-center justify-center text-sm font-semibold transition-all",
                currentStep >= step 
                  ? "bg-primary text-primary-foreground" 
                  : "bg-muted text-muted-foreground"
              )}>
                {step === 3 && currentStep >= 3 ? (
                  <CheckCircle className="w-5 h-5" />
                ) : (
                  step
                )}
              </div>
              {step < 3 && (
                <div className={cn(
                  "w-16 h-1 mx-2 transition-all",
                  currentStep > step ? "bg-primary" : "bg-muted"
                )} />
              )}
            </div>
          ))}
        </div>

        {/* Step 1: Basic Information */}
        {currentStep === 1 && (
          <motion.div
            initial={{ opacity: 0, x: 20 }}
            animate={{ opacity: 1, x: 0 }}
            className="grid grid-cols-1 lg:grid-cols-3 gap-8"
          >
            <div className="lg:col-span-2 space-y-6">
              <Card variant="glass">
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2">
                    <Info className="w-5 h-5" />
                    <span>{t('tokenCreation.basicInfo')}</span>
                  </CardTitle>
                  <CardDescription>
                    {t('tokenCreation.description')}
                  </CardDescription>
                </CardHeader>
                <CardContent className="space-y-6">
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <FormField
                      label={t('tokenCreation.tokenName')}
                      required
                      error={errors.name}
                    >
                      <EnhancedInput
                        name="name"
                        value={formData.name}
                        onChange={handleInputChange}
                        placeholder={t('tokenCreation.placeholders.tokenName')}
                        variant={errors.name ? "error" : "default"}
                        maxLength={32}
                      />
                    </FormField>

                    <FormField
                      label={t('tokenCreation.symbol')}
                      required
                      error={errors.symbol}
                    >
                      <EnhancedInput
                        name="symbol"
                        value={formData.symbol}
                        onChange={handleInputChange}
                        placeholder={t('tokenCreation.placeholders.symbol')}
                        variant={errors.symbol ? "error" : "default"}
                        maxLength={10}
                      />
                    </FormField>
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <FormField
                      label={t('tokenCreation.decimals')}
                      error={errors.decimals}
                      hint={t('tokenCreation.hints.decimals')}
                    >
                      <NumberInput
                        name="decimals"
                        value={formData.decimals}
                        onChange={handleInputChange}
                        min={0}
                        max={9}
                        variant={errors.decimals ? "error" : "default"}
                      />
                    </FormField>

                    <FormField
                      label={t('tokenCreation.initialSupply')}
                      error={errors.supply}
                      hint={t('tokenCreation.hints.supply')}
                    >
                      <NumberInput
                        name="supply"
                        value={formData.supply}
                        onChange={handleInputChange}
                        min={1}
                        variant={errors.supply ? "error" : "default"}
                      />
                    </FormField>
                  </div>
                </CardContent>
              </Card>

              <Card variant="glass">
                <CardHeader>
                  <CardTitle className="flex items-center space-x-2">
                    <Settings className="w-5 h-5" />
                    <span>{t('tokenCreation.advancedSettings')}</span>
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-6">
                  {/* Program Selector */}
                  <div className="space-y-3">
                    <label className="text-sm font-medium">{t('tokenCreation.programSelector')}</label>
                    <div className="flex items-center space-x-3 p-4 bg-muted/30 rounded-lg">
                      <Switch
                        checked={formData.useToken2022}
                        onCheckedChange={(checked) => handleSwitchChange('useToken2022', checked)}
                      />
                      <div className="flex-1">
                        <div className="font-medium">
                          {formData.useToken2022 ? t('tokenCreation.token2022') : t('tokenCreation.tokenProgram')}
                        </div>
                        <div className="text-sm text-muted-foreground">
                          {formData.useToken2022 
                            ? t('tokenCreation.token2022ComingSoon')
                            : t('tokenCreation.classicTokenProgram')
                          }
                        </div>
                      </div>
                    </div>
                  </div>

                  {/* Token Authorities */}
                  <div className="space-y-4">
                    <h3 className="text-lg font-semibold">{t('tokenCreation.tokenAuthorities')}</h3>
                    
                    <div className="space-y-3">
                      <div className="flex items-center justify-between p-4 bg-muted/30 rounded-lg">
                        <div>
                          <div className="font-medium flex items-center space-x-2">
                            <span>🧊</span>
                            <span>{t('tokenCreation.freezeAuthority')}</span>
                          </div>
                          <div className="text-sm text-muted-foreground">
                            {t('tokenCreation.freezeAuthorityDesc')}
                          </div>
                        </div>
                        <Switch
                          checked={formData.freezeAuthority}
                          onCheckedChange={(checked) => handleSwitchChange('freezeAuthority', checked)}
                        />
                      </div>
                    </div>
                  </div>
                </CardContent>
              </Card>
            </div>

            {/* Summary Card */}
            <div className="space-y-6">
              <Card variant="elevated">
                <CardHeader>
                  <CardTitle>{t('tokenCreation.preview.title')}</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="space-y-3">
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t('tokenCreation.preview.name')}</span>
                      <span className="font-medium">{formData.name || t('tokenCreation.preview.notSet')}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t('tokenCreation.preview.symbol')}</span>
                      <span className="font-medium">{formData.symbol || t('tokenCreation.preview.notSet')}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t('tokenCreation.preview.decimals')}</span>
                      <span className="font-medium">{formData.decimals}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t('tokenCreation.preview.supply')}</span>
                      <span className="font-medium">{formData.supply.toLocaleString()}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t('tokenCreation.preview.program')}</span>
                      <span className="font-medium">
                        {formData.useToken2022 ? t('tokenCreation.token2022') : t('tokenCreation.tokenProgram')}
                      </span>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Button 
                onClick={nextStep} 
                className="w-full" 
                size="lg"
                variant="solana"
              >
                {t('tokenCreation.steps.nextStep')}
              </Button>
            </div>
          </motion.div>
        )}

        {/* Step 2: Confirmation */}
        {currentStep === 2 && (
          <motion.div
            initial={{ opacity: 0, x: 20 }}
            animate={{ opacity: 1, x: 0 }}
            className="max-w-2xl mx-auto"
          >
            <Card variant="glass">
              <CardHeader>
                <CardTitle className="flex items-center space-x-2">
                  <AlertCircle className="w-5 h-5" />
                  <span>{t('tokenCreation.steps.confirmTitle')}</span>
                </CardTitle>
                <CardDescription>
                  {t('tokenCreation.steps.confirmDesc')}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="bg-muted/30 p-6 rounded-lg space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <div className="text-sm text-muted-foreground mb-1">{t('tokenCreation.steps.tokenName')}</div>
                      <div className="font-semibold">{formData.name}</div>
                    </div>
                    <div>
                      <div className="text-sm text-muted-foreground mb-1">{t('tokenCreation.steps.tokenSymbol')}</div>
                      <div className="font-semibold">{formData.symbol}</div>
                    </div>
                    <div>
                      <div className="text-sm text-muted-foreground mb-1">{t('tokenCreation.steps.decimals')}</div>
                      <div className="font-semibold">{formData.decimals}</div>
                    </div>
                    <div>
                      <div className="text-sm text-muted-foreground mb-1">{t('tokenCreation.steps.supply')}</div>
                      <div className="font-semibold">{formData.supply.toLocaleString()}</div>
                    </div>
                  </div>
                  
                  <div className="pt-4 border-t border-border">
                    <div className="text-sm text-muted-foreground mb-2">{t('tokenCreation.steps.authorities')}</div>
                    <div className="space-y-2">
                      <div className="flex items-center justify-between">
                        <span>{t('tokenCreation.steps.freezeAuth')}</span>
                        <span className={formData.freezeAuthority ? "text-green-500" : "text-muted-foreground"}>
                          {formData.freezeAuthority ? t('tokenCreation.steps.enabled') : t('tokenCreation.steps.disabled')}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="bg-yellow-500/10 border border-yellow-500/20 p-4 rounded-lg">
                  <div className="flex items-start space-x-3">
                    <AlertCircle className="w-5 h-5 text-yellow-500 mt-0.5" />
                    <div className="text-sm">
                      <div className="font-medium text-yellow-700 dark:text-yellow-400 mb-1">
                        {t('tokenCreation.steps.warning')}
                      </div>
                      <div className="text-yellow-600 dark:text-yellow-300">
                        {t('tokenCreation.steps.warningText')}
                      </div>
                    </div>
                  </div>
                </div>

                <div className="flex space-x-4">
                  <Button 
                    variant="outline" 
                    onClick={prevStep}
                    className="flex-1"
                  >
                    {t('tokenCreation.steps.backToEdit')}
                  </Button>
                  <Button 
                    onClick={createToken} 
                    disabled={isLoading}
                    className="flex-1"
                    variant="solana"
                  >
                    {isLoading ? (
                      <>
                        <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                        {t('tokenCreation.creatingToken')}
                      </>
                    ) : (
                      t('tokenCreation.createTokenBtn')
                    )}
                  </Button>
                </div>
              </CardContent>
            </Card>
          </motion.div>
        )}

        {/* Step 3: Success */}
        {currentStep === 3 && (
          <motion.div
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            className="max-w-2xl mx-auto"
          >
            <Card variant="glass" className="text-center">
              <CardHeader>
                <div className="mx-auto w-16 h-16 bg-green-500/10 rounded-full flex items-center justify-center mb-4">
                  <CheckCircle className="w-8 h-8 text-green-500" />
                </div>
                <CardTitle className="text-2xl text-green-600">
                  {t('tokenCreation.tokenCreated')}
                </CardTitle>
                <CardDescription>
                  {t('tokenCreation.steps.successTitle')}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                {txSignature && (
                  <div className="bg-muted/30 p-4 rounded-lg text-left space-y-3">
                    <div>
                      <div className="text-sm text-muted-foreground mb-1">{t('tokenCreation.steps.txHash')}</div>
                      <div className="flex items-center space-x-2">
                        <code className="text-sm bg-background px-2 py-1 rounded">
                          {txSignature.slice(0, 8)}...{txSignature.slice(-8)}
                        </code>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          asChild
                        >
                          <a
                            href={`https://explorer.solana.com/tx/${txSignature}?cluster=devnet`}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            <ExternalLink className="w-4 h-4" />
                          </a>
                        </Button>
                      </div>
                    </div>
                    
                    {tokenMint && (
                      <div>
                        <div className="text-sm text-muted-foreground mb-1">{t('tokenCreation.steps.tokenAddress')}</div>
                        <div className="flex items-center space-x-2">
                          <code className="text-sm bg-background px-2 py-1 rounded">
                            {tokenMint.slice(0, 8)}...{tokenMint.slice(-8)}
                          </code>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            asChild
                          >
                            <a
                              href={`https://explorer.solana.com/address/${tokenMint}?cluster=devnet`}
                              target="_blank"
                              rel="noopener noreferrer"
                            >
                              <ExternalLink className="w-4 h-4" />
                            </a>
                          </Button>
                        </div>
                      </div>
                    )}
                  </div>
                )}

                <div className="flex space-x-4">
                  <Button 
                    variant="outline" 
                    onClick={() => {
                      setCurrentStep(1);
                      setFormData({
                        name: '',
                        symbol: '',
                        decimals: 9,
                        supply: 1000000,
                        description: '',
                        image: '',
                        useToken2022: false,
                        freezeAuthority: true,
                        updateAuthority: true
                      });
                      setTxSignature('');
                      setTokenMint('');
                    }}
                    className="flex-1"
                  >
                    {t('tokenCreation.steps.createAnother')}
                  </Button>
                  {onNavigateBack && (
                    <Button 
                      onClick={onNavigateBack}
                      variant="solana"
                      className="flex-1"
                    >
                      {t('tokenCreation.steps.backToHome')}
                    </Button>
                  )}
                </div>
              </CardContent>
            </Card>
          </motion.div>
        )}
      </motion.div>
    </div>
  );
};

export default TokenCreationNew;
