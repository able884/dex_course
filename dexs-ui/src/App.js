import React, { useMemo, useState } from 'react';
import { WalletAdapterNetwork } from '@solana/wallet-adapter-base';
import { ConnectionProvider, WalletProvider } from '@solana/wallet-adapter-react';
import { WalletModalProvider } from '@solana/wallet-adapter-react-ui';
import { PhantomWalletAdapter, SolflareWalletAdapter } from '@solana/wallet-adapter-wallets';
import { clusterApiUrl } from '@solana/web3.js';
import HeaderNew from './components/Layout/HeaderNew';
import CreateTokenModal from './components/Pump/CreateTokenModal';
import DashboardNew from './components/Dashboard/DashboardNew';
import TokenListNew from './components/Tokens/TokenListNew';
import TokenCreationNew from './components/Tokens/TokenCreationNew';
import LiquidityPage from './components/liquidity/LiquidityPage';
import PoolsPage from './components/Pools/PoolsPage';
import PoolDeposit from './components/Pools/PoolDeposit';
import ClmmPoolDeposit from './components/Pools/ClmmPoolDeposit';
import SwapPage from './components/Swap/SwapPage';
import Footer from './components/Layout/Footer';
import LanguageSelector from './components/Layout/LanguageSelector';
import { LanguageProvider } from './i18n/LanguageContext';
import { ThemeProvider } from './components/UI/theme-provider';
import DynamicBackground from './components/UI/DynamicBackground';
import Watermark from './components/UI/Watermark';
import { Toaster } from './components/UI/toaster';

import '@solana/wallet-adapter-react-ui/styles.css';
import './App.css';

function App() {
  const [currentPage, setCurrentPage] = useState('tokens');
  const [tokenFilter, setTokenFilter] = useState('tokens');
  const [activeTab, setActiveTab] = useState('trenches');
  const [isLanguageSelectorOpen, setIsLanguageSelectorOpen] = useState(false);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [poolCreationType, setPoolCreationType] = useState('cpmm');
  const [selectedPool, setSelectedPool] = useState(null);
  const [selectedSwapPool, setSelectedSwapPool] = useState(null);

  // Solana network configuration
  const network = WalletAdapterNetwork.Devnet;
  const endpoint = useMemo(() => clusterApiUrl(network), [network]);
  
  const wallets = useMemo(
    () => [
      new PhantomWalletAdapter(),
      new SolflareWalletAdapter(),
    ],
    []
  );

  const navigateToTokens = (filterType) => {
    setTokenFilter(filterType);
    setCurrentPage('tokens');
  };

  const navigateToHome = () => {
    setCurrentPage('home');
  };

  const navigateToTokenCreation = () => {
    setCurrentPage('create-token');
  };

  const navigateToPoolsPage = () => {
    setSelectedPool(null);
    setSelectedSwapPool(null);
    setCurrentPage('pools');
    setActiveTab('pools');
  };

  const navigateToSwap = (pool) => {
    setSelectedSwapPool(pool || null);
    setSelectedPool(null);
    setCurrentPage('swap');
    setActiveTab('pools');
  };

  const navigateToPoolCreation = (type = 'cpmm') => {
    setPoolCreationType(type);
    setCurrentPage('create-pool');
  };

  // Placeholder navigation handlers for Pools operations
  const navigateToCharts = (pool) => {
    console.log('Navigate to charts for pool:', pool?.poolState);
  };
  const navigateToAddLiquidity = (pool) => {
    setSelectedPool(pool || null);
    setCurrentPage('deposit');
    setActiveTab('pools');
  };

  const handleTabChange = (tabId) => {
    setActiveTab(tabId);
    // 根据不同的tab处理导航逻辑
    switch (tabId) {
      case 'dashboard':
        setCurrentPage('home');
        break;
      case 'pools':
        setSelectedPool(null);
        setCurrentPage('pools');
        break;
      case 'tokens':
        setCurrentPage('tokens');
        break;
      case 'trenches':
        navigateToTokens('tokens');
        break;
      case 'track':
      case 'create-token':
        // 打开基于后端构建未签名交易的创建弹窗
        setShowCreateModal(true);
        break;
      case 'create-pool':
        setCurrentPage('create-pool');
        break;
      case 'add-liquidity':
      case 'token-security':
        // 这些功能可以根据需要实现页面切换
        console.log(`Navigating to ${tabId}`);
        setCurrentPage('home');
        break;
      default:
        setCurrentPage('home');
        break;
    }
  };

  const handleSettingsClick = () => {
    setIsLanguageSelectorOpen(true);
  };

  const handleLanguageSelectorClose = () => {
    setIsLanguageSelectorOpen(false);
  };

  const renderCurrentPage = () => {
    switch (currentPage) {
      case 'pools':
        return (
          <PoolsPage
            onNavigateToPoolCreation={navigateToPoolCreation}
            onNavigateCharts={navigateToCharts}
            onNavigateSwap={navigateToSwap}
            onNavigateAddLiquidity={navigateToAddLiquidity}
          />
        );
      case 'tokens':
        return <TokenListNew filterType={tokenFilter} onNavigateHome={navigateToHome} />;
      case 'create-token':
        return <TokenCreationNew onNavigateBack={navigateToHome} />;
      case 'create-pool':
        return (
          <LiquidityPage 
            onNavigateBack={navigateToPoolsPage} 
            initialType={poolCreationType}
          />
        );
      case 'deposit':
        // Check pool version to determine which deposit component to use
        // CLMM pools have poolVersion 1 or 2, CPMM pools have poolVersion 3
        const poolVersion = selectedPool?.poolVersion;
        const isClmm = poolVersion === 1 || poolVersion === 2;
        
        if (isClmm) {
          return (
            <ClmmPoolDeposit
              pool={selectedPool}
              onNavigateBack={navigateToPoolsPage}
            />
          );
        }
        
        return (
          <PoolDeposit
            pool={selectedPool}
            onNavigateBack={navigateToPoolsPage}
          />
        );
      case 'swap':
        return (
          <SwapPage
            pool={selectedSwapPool}
            onNavigateBack={navigateToPoolsPage}
          />
        );
      case 'home':
      default:
        return <DashboardNew 
          onNavigateToTokens={navigateToTokens} 
          onNavigateToTokenCreation={navigateToTokenCreation}
          onNavigateToPoolCreation={navigateToPoolCreation}
        />;
    }
  };

  return (
    <LanguageProvider>
      <ThemeProvider>
        <ConnectionProvider endpoint={endpoint}>
          <WalletProvider wallets={wallets} autoConnect>
            <WalletModalProvider>
              <DynamicBackground 
                particleCount={30}
                enableParticles={true}
                enableOrbs={true}
              >
                <div className="App min-h-screen text-foreground relative z-10 flex flex-col">
                  <HeaderNew 
                    activeTab={activeTab}
                    onTabChange={handleTabChange}
                    onSettingsClick={handleSettingsClick}
                  />
                  <main className="flex-1 bg-background/70 dark:bg-background/90">
                    {renderCurrentPage()}
                  </main>
                  {/* 创建 Token 弹窗（PumpFun create 指令，后端返回未签名交易）*/}
                  <CreateTokenModal isOpen={showCreateModal} onClose={() => setShowCreateModal(false)} />
                  <Footer />
                  <LanguageSelector
                    isOpen={isLanguageSelectorOpen}
                    onClose={handleLanguageSelectorClose}
                  />
                  <Watermark text="仅供学习使用" opacity={0.05} />
                </div>
              </DynamicBackground>
              <Toaster />
            </WalletModalProvider>
          </WalletProvider>
        </ConnectionProvider>
      </ThemeProvider>
    </LanguageProvider>
  );
}

export default App;
