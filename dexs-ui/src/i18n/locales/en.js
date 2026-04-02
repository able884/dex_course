// English language pack
export const en = {
  // Header navigation
  header: {
    navigation: {
      trenches: 'Trenches',
      newPair: 'New pair',
      trending: 'Trending',
      copyTrade: 'CopyTrade',
      monitor: 'Monitor',
      track: 'Add Liquidity',
      holding: 'Holding',
      createToken: 'Create Token',
      createPool: 'Create Pool',
      addLiquidity: 'Add Liquidity',
      faucet: 'Faucet',
      tokenSecurity: 'Token Security'
    },
    faucet: {
      title: 'Solana Testnet Faucets'
    },
    search: {
      placeholder: 'Search token/contract/wallet'
    },
    wallet: {
      connect: 'Connect Wallet',
      disconnect: 'Disconnect',
      connecting: 'Connecting...'
    },
    buttons: {
      deposit: 'Deposit',
      settings: 'Settings'
    },
    network: {
      solana: 'SOL'
    }
  },

  // Token list
  tokenList: {
    title: 'Token Trenches',
    tabs: {
      pumpfun: 'PumpFun Tokens',
      pumpamm: 'PumpAMM Tokens',
      clmm: 'CLMM Tokens'
    },
    columns: {
      newCreations: 'New Creations',
      completing: 'Completing',
      completed: 'Completed'
    },
    status: {
      lastUpdated: 'Last updated',
      live: 'Live',
      offline: 'Offline',
      connecting: 'Connecting...'
    },
    buttons: {
      manualRefresh: 'Manual Refresh',
      mockMode: 'Mock ON',
      realMode: 'Real Mode',
      tryAgain: 'Try Again',
      refresh: 'Refresh',
      enableMock: 'Enable Mock',
      disableMock: 'Disable Mock'
    },
    loading: {
      loadingTokens: 'Loading pump tokens...',
      loadingPools: 'Loading CLMM tokens...',
      loadingNewTokens: 'Loading new tokens...',
      loadingCompletingTokens: 'Loading completing tokens...',
      loadingCompletedTokens: 'Loading completed tokens...',
      loadingV1Pools: 'Loading V1 pools...',
      loadingV2Pools: 'Loading V2 pools...'
    },
    empty: {
      noTokensFound: 'No Tokens Found',
      noTokensDescription: 'No tokens are currently available on devnet.',
      noClmmTokens: 'No CLMM Tokens Found',
      noClmmDescription: 'No concentrated liquidity tokens are currently available on devnet.'
    },
    notifications: {
      newToken: 'New Token'
    },
    sections: {
      newTokens: 'New Tokens',
      almostBonded: 'Almost Bonded',
      migrated: 'Migrated',
      clmmV1Pools: 'CLMM V1 Pools',
      clmmV2Pools: 'CLMM V2 Pools'
    },
    labels: {
      progress: 'Progress',
      tvl: 'TVL',
      apr: 'APR',
      volume24h: 'Volume 24h',
      fees: 'Fees',
      mockModeEnabled: 'Mock mode enabled'
    },
    selectOptions: {
      pumpfun: 'Pump.fun',
      pumpamm: 'Pump.fun AMM',
      clmm: 'CLMM'
    }
  },

  // Token card
  tokenCard: {
    buttons: {
      buy: 'Buy',
      chart: 'Chart'
    },
    metrics: {
      marketCap: 'Market Cap',
      holders: 'Holders',
      price: 'Price',
      volume: 'Volume'
    },
    status: {
      new: 'New',
      completing: 'Completing',
      completed: 'Completed'
    }
  },

  // Chart page
  chart: {
    title: 'Chart',
    noTokenSelected: 'No Token Selected',
    description: 'Please select a token from the token list to view its chart',
    backToTokens: 'Back to Token List'
  },

  // Feature placeholder pages
  features: {
    copyTrade: {
      title: 'Copy Trading',
      description: 'Copy trading feature coming soon...'
    },
    monitor: {
      title: 'Monitor',
      description: 'Monitor feature coming soon...'
    },
    track: {
      title: 'Track',
      description: 'Track feature coming soon...'
    },
    holding: {
      title: 'Holdings',
      description: 'Holdings feature coming soon...'
    }
  },

  // Token creation
  tokenCreation: {
    title: 'Create Token',
    description: 'Create your token on Solana network',
    subtitle: '🪙 Token Creation Tool'
  },

  // Pool creation
  poolCreation: {
    title: 'Create Pool',
    description: 'Create liquidity pool for your token',
    subtitle: '🏊 Pool Creation Tool'
  },

  // Add liquidity
  addLiquidity: {
    title: 'Add Liquidity',
    description: 'Add liquidity to existing pools',
    subtitle: '💧 Liquidity Management Tool'
  },

  // Faucet
  faucet: {
    title: 'Faucet',
    description: 'Get test tokens',
    subtitle: '🚰 Test Token Faucet Tool'
  },

  // Token security
  tokenSecurity: {
    title: 'Token Security',
    description: 'Check token security',
    subtitle: '🛡️ Token Security Check Tool'
  },

  // Dashboard
  dashboard: {
    totalTokens: 'Total Tokens',
    activeTraders: 'Active Traders',
    todayCompleted: 'Today Completed',
    totalVolume: 'Total Volume',
    fromLastMonth: 'from last month',
    welcome: 'Welcome to RichCode DEX',
    subtitle: 'Discover, trade and create the next hot token',
    quickActions: 'Quick Actions',
    startTrading: 'Start Trading',
    startTradingDesc: 'Browse trending tokens and start trading',
    createToken: 'Create Token',
    createTokenDesc: 'Create your own token project',
    createPool: 'Create Pool',
    createPoolDesc: 'Create liquidity pool for your token pair',
    getStarted: 'Get Started',
    trendingTokens: 'Trending Tokens',
    viewAll: 'View All',
    trending: 'Trending',
    marketCap: 'Market Cap',
    liquidity: 'Liquidity',
    recentTrades: 'Recent Trades',
    topPools: 'Top Pools',
    priceChart: 'Price Chart',
    recentTradesActivity: 'Real-time trading activity',
    live: 'Live',
    topPoolsTVL: 'Top TVL Pools'
  },

  // Token creation
  tokenCreation: {
    title: 'Create Token',
    description: 'Create your token on Solana network',
    subtitle: '🪙 Token Creation Tool',
    connectWallet: 'Please connect your wallet to create tokens',
    deployToken: 'Deploy your own SPL token with advanced features',
    tokenName: 'Token Name',
    symbol: 'Symbol',
    decimals: 'Decimals',
    initialSupply: 'Initial Supply',
    description: 'Description',
    imageUrl: 'Image URL',
    freezeAuthority: 'Freeze Authority',
    freezeAuthorityDesc: 'Ability to freeze token accounts',
    updateAuthority: 'Update Authority',
    updateAuthorityDesc: 'Ability to update metadata',
    transaction: 'Transaction',
    tokenMint: 'Token Mint',
    creatingToken: 'Creating Token...',
    createTokenBtn: '🚀 Create Token',
    tokenCreated: 'Token created successfully!',
    programSelector: 'Program Selector',
    tokenProgram: 'Token Program',
    token2022: 'Token-2022',
    token2022ComingSoon: '🚧 Token-2022 coming soon! Currently using Token Program.',
    classicTokenProgram: '🔒 Classic token program (widely supported)',
    tokenAuthorities: '🔐 Token Authorities',
    basicInfo: 'Basic Information',
    advancedSettings: 'Advanced Settings',
    formValidation: {
      nameRequired: 'Token name is required',
      symbolRequired: 'Token symbol is required',
      symbolTooLong: 'Symbol must be 10 characters or less',
      decimalsRange: 'Decimals must be between 0-9',
      supplyRequired: 'Supply must be greater than 0',
      supplyTooLarge: 'Token supply is too large. Please reduce supply or decimals.'
    },
    placeholders: {
      tokenName: 'My Awesome Token',
      symbol: 'MAT',
      description: 'Describe your token...',
      imageUrl: 'https://example.com/image.png'
    },
    preview: {
      title: 'Token Preview',
      name: 'Name',
      symbol: 'Symbol',
      decimals: 'Decimals',
      supply: 'Initial Supply',
      program: 'Program',
      notSet: 'Not set'
    },
    steps: {
      nextStep: 'Next: Confirm Creation',
      confirmTitle: 'Confirm Token Creation',
      confirmDesc: 'Please carefully check the following information, it cannot be modified after creation',
      tokenName: 'Token Name',
      tokenSymbol: 'Token Symbol',
      decimals: 'Decimals',
      supply: 'Initial Supply',
      authorities: 'Authority Settings',
      freezeAuth: 'Freeze Authority',
      enabled: 'Enabled',
      disabled: 'Disabled',
      warning: 'Important Notice',
      warningText: 'Token basic information cannot be modified after creation. Please ensure all information is correct.',
      backToEdit: 'Back to Edit',
      successTitle: 'Your token has been successfully created and deployed to the Solana network',
      txHash: 'Transaction Hash',
      tokenAddress: 'Token Address',
      createAnother: 'Create Another Token',
      backToHome: 'Back to Home'
    },
    hints: {
      decimals: 'Decimals determine the smallest unit of the token',
      supply: 'Initial amount of tokens to be minted'
    }
  },

  // Pool creation
  poolCreation: {
    title: 'Create Pool',
    description: 'Create liquidity pool for your token',
    subtitle: '🏊 Pool Creation Tool',
    connectWallet: 'Please connect your wallet to create pools',
    createPool: 'Create a concentrated liquidity pool for your token pair',
    token0Address: 'Token 0 Address',
    token0Desc: 'Base token (usually the project token)',
    token1Address: 'Token 1 Address',
    token1Desc: 'Quote token (usually SOL or USDC)',
    price: 'Price (Token1 per Token0)',
    priceDesc: 'Initial exchange rate for the pool',
    openTime: 'Open Time',
    currentTimeNotice: 'Using current time for pool opening',
    poolOpenDesc: 'Pool will open immediately for trading',
    poolInfo: 'Pool Information',
    type: 'Type',
    program: 'Program',
    network: 'Network',
    transactionId: 'Transaction ID',
    debugInfo: 'Transaction Debug Info',
    creatingPool: 'Creating Pool...',
    createPoolBtn: 'Create Pool',
    placeholders: {
      token0: 'e.g. So11111111111111111111111111111111111111112',
      token1: 'e.g. EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v',
      price: '0.001'
    },
    newTitle: 'Create CLMM Pool',
    newDescription: 'Create a concentrated liquidity market maker pool for your token pair and enjoy more efficient capital utilization',
    backToDashboard: 'Back to Dashboard',
    tokenPair: 'Token Pair',
    selectTokens: 'Select two tokens to create a liquidity pool',
    token0Required: 'Token 0 Address *',
    token1Required: 'Token 1 Address *',
    initialPrice: 'Initial Price',
    setPriceDesc: 'Set the initial exchange rate for the pool',
    priceRequired: 'Price (Token1 per Token0) *',
    feeTier: 'Fee Tier',
    selectFeeTier: 'Select the appropriate fee tier for your token pair',
    selectFeeTierPlaceholder: 'Select fee tier',
    feeTiers: {
      tier1: '0.01% - Stablecoin',
      tier1Desc: 'Best for stablecoin pairs',
      tier5: '0.05% - Low volatility',
      tier5Desc: 'Suitable for correlated assets',
      tier30: '0.3% - Standard',
      tier30Desc: 'Most trading pairs',
      tier100: '1% - High volatility',
      tier100Desc: 'High-risk assets'
    },
    tickSpacing: 'Tick Spacing',
    launchSettings: 'Launch Settings',
    poolOpenSettings: 'Pool opening time settings',
    useCurrentTime: 'Use current time to open pool',
    immediateOpen: 'Pool will open immediately for trading',
    poolInfoTitle: 'Pool Information',
    wallet: 'Wallet',
    transactionSuccess: 'Transaction Successful',
    copied: 'Copied',
    txIdCopied: 'Transaction ID copied to clipboard',
    viewOnExplorer: 'View on Solscan',
    debugInfoTitle: 'Debug Information',
    errors: {
      walletNotConnected: 'Wallet not connected',
      connectWalletFirst: 'Please connect your wallet first',
      formValidationFailed: 'Form validation failed',
      token0Required: 'Token 0 address is required',
      token1Required: 'Token 1 address is required',
      tokensMustBeDifferent: 'Token addresses must be different',
      priceRequired: 'Initial price must be greater than 0',
      createPoolFailed: 'Create pool failed',
      transactionSent: 'Transaction sent',
      waitingConfirmation: 'Waiting for confirmation...',
      poolCreated: 'Pool created successfully!',
      transactionConfirmed: 'Transaction confirmed',
      transactionFailed: 'Transaction failed',
      missingTxHash: 'Missing transaction hash in API response'
    }
  },

  // Token security
  tokenSecurity: {
    title: 'Token Security',
    description: 'Check token security',
    subtitle: '🛡️ Token Security Check Tool',
    enterAddress: 'Enter a Solana SPL Token Mint address to check its security status.',
    mintAddress: 'Mint Address',
    decimals: 'Decimals',
    initialized: 'Initialized',
    mintAuthority: 'Mint Authority',
    freezeAuthority: 'Freeze Authority',
    mintAuthoritySafe: 'Mint Authority Safe',
    freezeAuthoritySafe: 'Freeze Authority Safe',
    securitySummary: 'Security Summary',
    yes: 'Yes',
    no: 'No',
    none: 'None',
    safe: 'Safe',
    risk: 'Risk',
    placeholder: 'Enter token mint address...'
  },

  // Buy modal
  buyModal: {
    new: 'NEW',
    copyAddress: 'Copy Address',
    placeholder: {
      amount: '0.0001'
    },
    title: 'Buy',
    description: 'Enter the amount of SOL you want to spend to swap for this token',
    address: 'Address',
    marketCap: 'Market Cap',
    amount: 'Buy Amount (SOL)',
    transactionSignature: 'Transaction Signature',
    viewOnSolscan: 'View on Solscan',
    walletConnected: 'Wallet Connected',
    connectWallet: 'Please connect your wallet first',
    cancel: 'Cancel',
    creating: 'Creating transaction...',
    buyToken: 'Buy Token',
    sellToken: 'Sell Token',
    pumpControlsTitle: 'Pump Trade Controls',
    buyTab: 'Buy',
    sellTab: 'Sell',
    tokenAmountLabel: 'Token Amount',
    tokenAmountSellLabel: 'Tokens to Sell',
    solAmountLabel: 'SOL to Spend',
    solTargetLabel: 'SOL Target',
    inputActive: 'Active',
    slippageLabel: 'Slippage',
    customSlippage: 'Custom',
    slippageRangeNote: 'Allowed range 0.1% - 5%',
    quoteCalculating: 'Calculating quote...',
    quoteEmptyHint: 'Enter SOL or token amount to get a live quote.',
    expectedReceive: 'Expected Receive',
    minReceive: 'Min Receive',
    maxPay: 'Max Pay',
    priceImpact: 'Price Impact',
    tokenSymbolFallback: 'Token',
    transactionSubmittedToast: 'Transaction submitted. Waiting for confirmation...',
    transactionConfirmedToast: 'Transaction confirmed successfully! You can view it on Solscan.',
    slippageErrorDetail: '⚠️ Slippage protection triggered.\n\nThe trade failed because the expected price moved too far.\n\nPossible causes:\n• Rapid market swings\n• Pool liquidity is too shallow\n• Order size is large relative to the pool\n• Other traders are moving the price at the same time\n\nTry again after a short wait, reduce the trade size, or increase the tolerance if you can accept the risk.',
    insufficientErrorDetail: '💰 Insufficient balance.\n\nPlease ensure you have enough SOL to cover the trade amount plus fees, and that your account is active.',
    accountErrorDetail: '🔐 Related token accounts may be missing.\n\nKeep at least 0.01 SOL in your wallet so the system can create the required accounts, then retry the trade.',
    sellToken: 'Sell Token',
    pumpControlsTitle: 'Pump Trade Controls',
    buyTab: 'Buy',
    sellTab: 'Sell',
    tokenAmountLabel: 'Token Amount',
    tokenAmountSellLabel: 'Tokens to Sell',
    solAmountLabel: 'SOL to Spend',
    solTargetLabel: 'SOL Target',
    inputActive: 'Active',
    slippageLabel: 'Slippage',
    customSlippage: 'Custom',
    slippageRangeNote: 'Allowed range 0.1% - 5%',
    quoteCalculating: 'Calculating quote...',
    quoteEmptyHint: 'Enter SOL or token amount to get a live quote.',
    expectedReceive: 'Expected Receive',
    minReceive: 'Min Receive',
    maxPay: 'Max Pay',
    priceImpact: 'Price Impact',
    tokenSymbolFallback: 'Token',
    transactionSubmittedToast: 'Transaction submitted. Waiting for confirmation...',
    transactionConfirmedToast: 'Transaction confirmed successfully! You can view it on Solscan.',
    slippageErrorDetail: '⚠️ Slippage protection triggered.\n\nThe trade failed because the expected price moved too far.\n\nPossible causes:\n• Rapid market swings\n• Pool liquidity is too shallow\n• Order size is large relative to the pool\n• Other traders are moving the price at the same time\n\nTry again after a short wait, reduce the trade size, or raise the tolerance if you can accept the risk.',
    insufficientErrorDetail: '💰 Insufficient balance.\n\nPlease ensure you have enough SOL to cover the trade amount plus fees, and that your account is active.',
    accountErrorDetail: '🔐 Related token accounts may be missing.\n\nKeep at least 0.01 SOL in your wallet so the system can create the required accounts, then retry the trade.'
  },

  // Add liquidity
  addLiquidity: {
    title: 'Add Liquidity',
    description: 'Add liquidity to existing pools',
    subtitle: '💧 Liquidity Management Tool',
    enterPoolAddress: 'Enter pool address (e.g., Bevpu2aknCe7ZotQDRy2LgbG1gtU8S1BFwcpLPziy8af)',
    tokenAMint: 'Enter token A mint address',
    tokenASymbol: 'E.g., SOL',
    tokenBMint: 'Enter token B mint address',
    tokenBSymbol: 'E.g., USDC',
    priceRange: 'E.g., 1.5',
    modalTitle: 'Add Liquidity',
    modalDescription: 'Add liquidity to CLMM pools and earn fees',
    transactionSuccess: 'Transaction Successful!',
    transactionSignature: 'Transaction Signature',
    selectPool: 'Select Pool',
    selectFromList: 'Select from List',
    manualInput: 'Manual Input Address',
    selectPoolPlaceholder: 'Please select a pool',
    noPoolsAvailable: 'No pools available, please use manual input',
    poolAddress: 'Pool Address',
    poolAddressPlaceholder: 'Enter pool address',
    autoFetching: 'Fetching...',
    autoFetchToken: 'Auto Fetch Token Info',
    poolDetails: 'Pool Details',
    showDetails: 'Show Details',
    hideDetails: 'Hide Details',
    errors: {
      fetchPoolsFailed: 'Failed to fetch pools list',
      fetchPoolDetailsFailed: 'Failed to fetch pool details',
      poolNotFound: 'Pool account not found or data is empty',
      autoFetchSuccess: 'Auto fetch token info successful',
      fetchTokenInfoFailed: 'Failed to fetch pool token info',
      fillAllFields: 'Please fill in all required pool details',
      invalidPrice: 'Please enter a valid price',
      createPoolInfoFailed: 'Failed to create pool info, please check input',
      connectWalletFirst: 'Please connect your wallet first',
      walletNotSupported: 'Wallet does not support transaction signing',
      fillAllRequired: 'Please fill in all required fields',
      creatingTransaction: 'Creating transaction...',
      apiError: 'API Error',
      backendError: 'Error 515: Backend service error. Please check if wallet has sufficient tokens and has approved transactions.',
      unknownError: 'Unknown error',
      noTransactionData: 'Server did not return transaction data',
      cannotDeserialize: 'Cannot deserialize transaction',
      decodeTransactionFailed: 'Failed to decode transaction',
      signTransactionFailed: 'Failed to sign transaction',
      sendingTransaction: 'Sending transaction...',
      sendTransactionFailed: 'Failed to send transaction',
      liquidityAddedSuccess: 'Liquidity added successfully!',
      transactionProcessed: 'Liquidity added successfully! Transaction processed.',
      addLiquidityFailed: 'Failed to add liquidity'
    }
  },

  // Liquidity page
  liquidityPage: {
    title: 'Liquidity',
    subtitle: 'Create Raydium CPMM or CLMM pools',
    description: 'Launch a constant-product pool or concentrated liquidity market maker without leaving RC DEX.',
    back: 'Back',
    tabs: {
      cpmm: 'CPMM Pools',
      clmm: 'CLMM Pools'
    },
    form: {
      tokenPairTitle: 'Token Pair',
      tokenPairDesc: 'Select the base and quote tokens you want to seed in the pool',
      baseToken: 'Base Token',
      quoteToken: 'Quote Token',
      baseTokenShort: 'Base Token',
      quoteTokenShort: 'Quote Token',
      selectToken: 'Select token',
      selectTokenDesc: 'Choose from list or paste mint address',
      tokensMustDiffer: 'Base and quote tokens must be different.',
      initialPrice: 'Initial Price',
      initialPricePlaceholder: 'Automatically calculated after entering both amounts',
      currentPrice: '1 {{base}} ≈ {{value}} {{quote}}',
      feeTitle: 'Fee Tier & Schedule',
      feeSubtitle: 'Pick a trade fee and pool opening time',
      feeDescription: 'Choose the fee tier that best matches your token pair volatility and liquidity depth.',
      feeTier: 'Fee Tier',
      loadingFees: 'Loading fee tiers...',
      selectFeeTier: 'Select a fee tier',
      startTime: 'Start Time',
      startNow: 'Start Now',
      custom: 'Custom',
      startImmediate: 'Starts immediately after transaction is confirmed'
    },
    actions: {
      retry: 'Retry',
      swapTokens: 'Swap pair',
      initialize: 'Initialize Liquidity Pool',
      initializing: 'Initializing...',
      clear: 'Clear'
    },
    summary: {
      title: 'Pool Summary',
      subtitle: 'Review configuration before initializing the pool',
      network: 'Network',
      wallet: 'Wallet',
      notConnected: 'Not connected',
      base: 'Base',
      quote: 'Quote',
      fee: 'Fee tier',
      start: 'Start time',
      startImmediate: 'Immediately after confirmation',
      successTitle: 'Transaction Sent',
      successDesc: 'Unsigned transaction signed and submitted successfully.',
      viewOnExplorer: 'View on Solscan'
    },
    tokenModal: {
      selectBase: 'Select Base Token',
      selectQuote: 'Select Quote Token',
      description: 'Search popular tokens or paste any SPL mint address.',
      searchPlaceholder: 'Search by symbol, name, or address',
      loading: 'Loading tokens...',
      empty: 'No tokens matched your search.',
      manualTitle: 'Add custom token',
      manualMint: 'Token mint address',
      manualSymbol: 'Symbol or nickname',
      addCustom: 'Use this token'
    },
    toast: {
      connectWalletTitle: 'Connect wallet',
      connectWalletDesc: 'Please connect a wallet before creating a pool.',
      incompleteTitle: 'Missing pool details',
      incompleteDesc: 'Select tokens, enter amounts, and choose a fee tier to continue.',
      transactionSent: 'Transaction sent',
      waitingConfirmation: 'Waiting for confirmation...',
      poolCreated: 'Liquidity pool initialized',
      transactionConfirmed: 'Confirmed on-chain ({{signature}})',
      createFailed: 'Failed to initialize pool'
    },
    errors: {
      missingTransaction: 'Backend did not return an unsigned transaction.'
    }
  },

  // Footer
  footer: {
    pumpTokens: 'RichCode DEX',
    telegramGroup: 'Telegram Group',
    discordCommunity: 'Discord Community',
    description: 'Discover, trade and create the next hot token. A decentralized token trading platform based on Solana blockchain, providing users with secure, fast, and low-cost trading experience.',
    features: 'Features',
    tradingHall: 'Trading Hall',
    copyTrade: 'Copy Trade',
    monitorPanel: 'Monitor Panel',
    trackingAnalysis: 'Tracking Analysis',
    positionManagement: 'Position Management',
    contactUs: 'Contact Us',
    technicalSupport: 'Technical Support',
    businessCooperation: 'Business Cooperation',
    ecosystem: 'Ecosystem',
    solanaOfficial: 'Solana Official',
    copyright: '© 2024 RichCode DEX. All rights reserved.',
    privacyPolicy: 'Privacy Policy',
    termsOfService: 'Terms of Service',
    disclaimer: 'Disclaimer',
    emailContact: 'Email Contact'
  },

  // Wallet debugger
  walletDebugger: {
    connectionStatus: 'Connection Status',
    connected: '✅ Connected',
    disconnected: '❌ Disconnected',
    wallet: 'Wallet',
    publicKey: 'Public Key',
    balance: 'Balance',
    rpcEndpoint: 'RPC Endpoint',
    readyState: 'Ready State',
    legacyTransactions: 'Legacy Transactions: ✅ Supported',
    warning: '⚠️ Warning: Your wallet may not support some transactions. Consider using Phantom or Solflare.',
    walletObjectInfo: 'Wallet Object Info',
    capabilities: 'Capabilities',
    supportedTxVersions: 'Supported TX Versions',
    solanaVersion: 'Solana Version',
    availableWallets: 'Available Wallets',
    consoleDetails: 'Press F12 to open console for more details'
  },

  // Chart
  tradingChart: {
    retryBtn: 'Retry',
    loadingData: 'Loading price data...',
    noTokenSelected: 'Please select a token from the token list to view its price chart'
  },

  // WebSocket token list
  tokenListWebSocket: {
    disconnect: 'Disconnect',
    tokensListed: 'Tokens Listed',
    messagesReceived: 'Messages Received',
    newTokens: 'New Tokens',
    updates: 'Updates',
    marketCap: 'Market Cap',
    holders: 'Holders',
    launched: 'Launched',
    viewOnPumpfun: 'View on Pump.fun'
  },

  // Settings
  settings: {
    title: 'Settings',
    language: {
      title: 'Language',
      chinese: '中文',
      english: 'English',
      autoSave: 'Language preference will be saved automatically'
    }
  },

  // Common
  common: {
    loading: 'Loading...',
    error: 'Error',
    success: 'Success',
    warning: 'Warning',
    info: 'Info',
    close: 'Close',
    cancel: 'Cancel',
    confirm: 'Confirm',
    retry: 'Retry',
    all: 'All',
    filter: 'Filter',
    export: 'Export',
    help: 'Help',
    never: 'Never',
    previous: 'Previous',
    next: 'Next',
    page: 'Page',
    pageUnit: ''
  }
};
