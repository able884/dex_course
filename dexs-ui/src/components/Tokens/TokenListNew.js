import React, { useState, useEffect, useRef, useCallback } from 'react';
import useTokenListWebSocket from '../../hooks/useTokenListWebSocket';
import { useTranslation } from '../../i18n/LanguageContext';
import { Card, CardContent, CardHeader, CardTitle } from '../UI/enhanced-card';
import { Button } from '../UI/Button';
import { Badge } from '../UI/badge';
import { LoadingSpinner } from '../UI/loading-spinner';

import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../UI/table';
import BuyModal from './BuyModal.jsx';
import { 
  RefreshCw, 
  Wifi, 
  WifiOff,
  Clock,
  Users,
  BarChart3,
  AlertCircle,
  Zap,
  Filter,
  Search,
  TrendingUp,
  TrendingDown,
  Flame,
  CheckCircle,
  ArrowUpRight,
  ExternalLink
} from 'lucide-react';
import { cn } from '../../lib/utils';

const API_BASE_URL = process.env.NODE_ENV === 'development' 
  ? '' // Use proxy in development
  : '/api'; // Use Nginx proxy in production

// 添加样式来处理图片加载失败的情况
const tokenIconStyles = `
  .token-icon-container.show-fallback img {
    display: none !important;
  }
  .token-icon-container.show-fallback .fallback-text {
    display: flex !important;
  }
`;

const TokenList = ({ onTokenSelect, filterType = 'all' }) => {
  const { t } = useTranslation();
  // Tab state
  const [activeTab, setActiveTab] = useState('pumpfun'); // 'pumpfun' or 'pumpamm'
  
  // PumpFun state
  const [newTokens, setNewTokens] = useState([]);
  const [completingTokens, setCompletingTokens] = useState([]);
  const [completedTokens, setCompletedTokens] = useState([]);
  
  // (removed) CLMM state
  
  // Common state
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [lastUpdate, setLastUpdate] = useState(null);
  const [realtimeCount, setRealtimeCount] = useState(0);
  const [newTokenNotifications, setNewTokenNotifications] = useState([]);
  
  
  // BuyModal state
  const [showBuyModal, setShowBuyModal] = useState(false);
  const [selectedToken, setSelectedToken] = useState(null);
  // const intervalRef = useRef(null);
  // const setupCountRef = useRef(0);
  const componentIdRef = useRef(Math.random().toString(36).substr(2, 9));
  const hasInitializedRef = useRef(false);

  console.log(`🔄 TokenList render - Component ID: ${componentIdRef.current}`);
  console.log(`📊 TokenList current state:`, {
    activeTab,
    newTokens: newTokens.length,
    completingTokens: completingTokens.length, 
    completedTokens: completedTokens.length,
    // clmmV1Pools: clmmV1Pools.length,
    // clmmV2Pools: clmmV2Pools.length,
    loading,
    error: !!error,
    lastUpdate: lastUpdate?.toLocaleTimeString() || 'none',
    hasInitialized: hasInitializedRef.current,
    realtimeCount
  });

  // WebSocket handlers for PumpFun and PumpAMM real-time updates
  const handleNewToken = useCallback((tokenData) => {
    if (activeTab !== 'pumpfun' && activeTab !== 'pumpamm') return; // Only process for PumpFun/PumpAMM tabs
    
    console.log(`🆕 [${componentIdRef.current}] New token received via WebSocket:`, tokenData);
    
    const newToken = {
      id: tokenData.tokenAddress,
      tokenAddress: tokenData.tokenAddress,
      tokenName: tokenData.tokenName || tokenData.tokenSymbol,
      tokenIcon: tokenData.tokenIcon || '',
      launchTime: tokenData.launchTime,
      mktCap: tokenData.mktCap || 0,
      holdCount: tokenData.holdCount || 0,
      change24: 0,
      txs24h: 0,
      pairAddress: tokenData.pairAddress,
      _realtimeTimestamp: Date.now()
    };

    const pumpStatus = tokenData.pumpStatus || 1;
    
    if (pumpStatus === 1) {
      setNewTokens(prevTokens => {
        const exists = prevTokens.some(token => token.tokenAddress === newToken.tokenAddress);
        if (exists) return prevTokens;
        return [newToken, ...prevTokens];
      });
    } else if (pumpStatus === 2) {
      setCompletingTokens(prevTokens => {
        const exists = prevTokens.some(token => token.tokenAddress === newToken.tokenAddress);
        if (exists) return prevTokens;
        return [newToken, ...prevTokens];
      });
    } else if (pumpStatus === 4) {
      setCompletedTokens(prevTokens => {
        const exists = prevTokens.some(token => token.tokenAddress === newToken.tokenAddress);
        if (exists) return prevTokens;
        return [newToken, ...prevTokens];
      });
    }

    const notificationId = Date.now();
    setNewTokenNotifications(prev => [{
      id: notificationId,
      tokenName: newToken.tokenName,
      timestamp: Date.now()
    }, ...prev.slice(0, 4)]);

    setTimeout(() => {
      setNewTokenNotifications(prev => 
        prev.filter(notification => notification.id !== notificationId)
      );
    }, 5000);

    setRealtimeCount(prev => prev + 1);
  }, [activeTab]);

  const handleTokenUpdate = useCallback((tokenData) => {
    if (activeTab !== 'pumpfun' && activeTab !== 'pumpamm') return; // Only process for PumpFun/PumpAMM tabs
    
    const { tokenAddress, pumpStatus, oldPumpStatus } = tokenData;
    
    if (oldPumpStatus !== pumpStatus) {
      // Remove from old list
      if (oldPumpStatus === 1) {
        setNewTokens(prev => prev.filter(token => token.tokenAddress !== tokenAddress));
      } else if (oldPumpStatus === 2) {
        setCompletingTokens(prev => prev.filter(token => token.tokenAddress !== tokenAddress));
      } else if (oldPumpStatus === 4) {
        setCompletedTokens(prev => prev.filter(token => token.tokenAddress !== tokenAddress));
      }
      
      // Add to new list
      const updatedToken = { ...tokenData, id: tokenAddress, _realtimeTimestamp: Date.now() };
      
      if (pumpStatus === 1) {
        setNewTokens(prev => [updatedToken, ...prev]);
      } else if (pumpStatus === 2) {
        setCompletingTokens(prev => [updatedToken, ...prev]);
      } else if (pumpStatus === 4) {
        setCompletedTokens(prev => [updatedToken, ...prev]);
      }
    }
  }, [activeTab]);

  // Initialize WebSocket connection
  const { connectionStatus } = useTokenListWebSocket(handleNewToken, handleTokenUpdate);

  // Fetch PumpFun or PumpAMM tokens
  const fetchTokens = useCallback(async (pumpType = 'pumpamm') => {
    // 先清空所有 token 数据
    setNewTokens([]);
    setCompletingTokens([]);
    setCompletedTokens([]);
    
    setLoading(true);
    setError('');
    
    try {
      const [resNew, resCompleting, resCompleted] = await Promise.all([
        fetch(`${API_BASE_URL}/v1/market/index_pump?chain_id=100000&pump_status=1&page_no=1&page_size=50&pump_type=${pumpType}`),
        fetch(`${API_BASE_URL}/v1/market/index_pump?chain_id=100000&pump_status=2&page_no=1&page_size=50&pump_type=${pumpType}`),
        fetch(`${API_BASE_URL}/v1/market/index_pump?chain_id=100000&pump_status=4&page_no=1&page_size=50&pump_type=${pumpType}`)
      ]);

      const [dataNew, dataCompleting, dataCompleted] = await Promise.all([
        resNew.json(),
        resCompleting.json(),
        resCompleted.json()
      ]);

      const newTokens = (dataNew?.data?.list) || [];
      const completingTokens = (dataCompleting?.data?.list) || [];
      const completedTokens = (dataCompleted?.data?.list) || [];
      
      console.log(`✅ Fetched ${pumpType} tokens:`, {
        newTokens: newTokens.length,
        completingTokens: completingTokens.length,
        completedTokens: completedTokens.length
      });
      
      setNewTokens(newTokens);
      setCompletingTokens(completingTokens);
      setCompletedTokens(completedTokens);
      setLastUpdate(new Date());
    } catch (err) {
      console.error(`❌ Failed to fetch ${pumpType} tokens:`, err);
      setError(`Failed to fetch tokens: ${err.message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  // (removed) Fetch CLMM pools

  // Initialize data
  useEffect(() => {
    if (hasInitializedRef.current) return;
    
    hasInitializedRef.current = true;
    console.log(`🚀 [${componentIdRef.current}] TokenList initializing...`);
    
    if (activeTab === 'pumpfun') {
      fetchTokens('pumpfun');
    } else if (activeTab === 'pumpamm') {
      fetchTokens('pumpamm');
    }
  }, [activeTab, fetchTokens]);

  // Tab switching handler
  const handleTabSwitch = (tab) => {
    console.log(`🔀 [${componentIdRef.current}] Switching tab from ${activeTab} to ${tab}`);
    setActiveTab(tab);
    setError('');
    
    if (tab === 'pumpfun') {
      fetchTokens('pumpfun');
    } else if (tab === 'pumpamm') {
      fetchTokens('pumpamm');
    }
  };

  // Retry handler
  const handleRetry = () => {
    if (activeTab === 'pumpfun') {
      fetchTokens('pumpfun');
    } else if (activeTab === 'pumpamm') {
      fetchTokens('pumpamm');
    }
  };

  // Buy Modal handlers
  const handleBuyClick = (token) => {
    console.log('Buy button clicked for token:', token);
    setSelectedToken(token);
    setShowBuyModal(true);
  };

  const handleCloseBuyModal = () => {
    setShowBuyModal(false);
    setSelectedToken(null);
  };

  // 状态指示器组件
  const StatusIndicator = ({ status }) => (
    <div className="flex items-center space-x-2">
      {status === 'connected' ? (
        <>
          <Wifi className="h-4 w-4 text-green-500" />
          <span className="text-sm text-green-500 font-medium">已连接</span>
          <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
        </>
      ) : (
        <>
          <WifiOff className="h-4 w-4 text-red-500" />
          <span className="text-sm text-red-500 font-medium">已断开</span>
        </>
      )}
    </div>
  );

  // 错误状态组件
  const ErrorState = () => (
    <Card className="border-red-200 bg-red-50 dark:border-red-800 dark:bg-red-900/20 mb-6">
      <CardContent className="p-6">
        <div className="flex items-center space-x-3 text-red-600 dark:text-red-400 mb-4">
          <AlertCircle className="h-6 w-6" />
          <h3 className="font-semibold text-lg">加载失败</h3>
        </div>
        <p className="text-muted-foreground mb-4">{error}</p>
        <Button
          variant="outline"
          onClick={handleRetry}
          disabled={loading}
          className="flex items-center space-x-2"
        >
          <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
          <span>重试</span>
        </Button>
      </CardContent>
    </Card>
  );

  // 格式化价格
  const formatPrice = (price) => {
    if (!price || price === 0) return '$0.00';
    if (price < 0.000001) return `$${price.toFixed(8)}`;
    if (price < 0.001) return `$${price.toFixed(6)}`;
    if (price < 1) return `$${price.toFixed(4)}`;
    return `$${price.toFixed(2)}`;
  };

  // 格式化数字
  const formatNumber = (num) => {
    if (!num || num === 0) return '0';
    if (num >= 1e9) return `${(num / 1e9).toFixed(1)}B`;
    if (num >= 1e6) return `${(num / 1e6).toFixed(1)}M`;
    if (num >= 1e3) return `${(num / 1e3).toFixed(1)}K`;
    return num.toString();
  };

  // Token 卡片组件 - 使用真实的 index_pump 接口数据结构
  const TokenCard = ({ token, type }) => {
    const liveStatus = token._realtimeTimestamp && (Date.now() - token._realtimeTimestamp) < 10000;
    
    // 格式化启动时间 - 1小时内用1m，1天内用1h，1月内用1d
    const formatLaunchTime = (timestamp) => {
      if (!timestamp) return '';
      const now = Date.now();
      const launchTime = parseInt(timestamp) * 1000;
      const diff = now - launchTime;
      
      const minutes = Math.floor(diff / (1000 * 60));
      const hours = Math.floor(diff / (1000 * 60 * 60));
      const days = Math.floor(diff / (1000 * 60 * 60 * 24));
      
      if (days > 0) return `${days}d`;
      if (hours > 0) return `${hours}h`;
      return `${Math.max(1, minutes)}m`;
    };
    
    // 缩略显示Token地址
    const formatTokenAddress = (address) => {
      if (!address || address.length < 8) return address;
      return `${address.slice(0, 4)}...${address.slice(-4)}`;
    };
    
    // 格式化价格和交易量 - $1.1或$1.1k，保留1位小数
    const formatVolPrice = (value) => {
      if (!value || value === 0) return '$0.0';
      if (value >= 1000) {
        return `$${(value / 1000).toFixed(1)}k`;
      }
      return `$${value.toFixed(1)}`;
    };
    
    // 解析社交媒体用户名
    const getSocialUsername = () => {
      if (token.twitterUsername) {
        return `@${token.twitterUsername}`;
      }
      if (token.telegram) {
        // 从telegram URL中解析用户名
        const match = token.telegram.match(/t\.me\/([^/?]+)/);
        return match ? `@${match[1]}` : null;
      }
      return null;
    };
    
    return (
      <div 
        className="bg-card border-b border-border p-3 hover:bg-accent/50 cursor-pointer transition-all duration-200 h-[120px] flex"
        onClick={() => onTokenSelect(token)}
      >
        {/* 左侧：头像和地址 */}
        <div className="flex flex-col items-center justify-center w-18 flex-shrink-0">
          {/* 1行：头像 */}
          <div className="relative mb-1">
            <div className="w-16 h-16 bg-gradient-to-r from-blue-500 to-purple-500 rounded-md border-2 border-border/50 flex items-center justify-center text-white font-bold text-lg overflow-hidden token-icon-container">
              {token.tokenIcon ? (
                <img 
                  src={token.tokenIcon} 
                  alt={token.tokenName || 'Token'} 
                  className="w-full h-full rounded-sm object-cover"
                  onError={(e) => {
                    e.target.style.display = 'none';
                    e.target.parentElement.classList.add('show-fallback');
                  }}
                />
              ) : null}
              <div 
                className={`fallback-text w-full h-full flex items-center justify-center ${token.tokenIcon ? 'hidden' : 'flex'}`}
              >
                {(token.tokenName || '?').charAt(0).toUpperCase()}
              </div>
            </div>
            {liveStatus && (
              <div className="absolute -top-1 -right-1 w-3 h-3 bg-green-500 rounded-full flex items-center justify-center">
                <div className="w-1.5 h-1.5 bg-white rounded-full"></div>
              </div>
            )}
          </div>
          
          {/* 2行：Token地址缩略 */}
          <div className="text-xs text-muted-foreground text-center">
            {formatTokenAddress(token.tokenAddress)}
          </div>
        </div>

        {/* 中间：Token信息 */}
        <div className="flex-1 min-w-0 flex flex-col justify-center px-3">
          {/* 1行：Token Name，地址前4位 */}
          <div className="flex items-center gap-2 mb-1">
            {token.tokenName && (
              <span className="font-bold text-foreground text-sm truncate">
                {token.tokenName}
              </span>
            )}
            {token.tokenAddress && (
              <span className="text-muted-foreground text-xs">
                {token.tokenAddress.slice(0, 4)}
              </span>
            )}
            {liveStatus && (
              <span className="bg-green-500/20 text-green-500 dark:text-green-400 px-1.5 py-0.5 rounded text-xs font-medium">
                LIVE
              </span>
            )}
          </div>
          
          {/* 2行：时间，holdCount，change24%，txs24h */}
          <div className="flex items-center gap-3 text-xs text-muted-foreground mb-1">
            <span>⏰{formatLaunchTime(token.launchTime) || '0m'}</span>
            <span className="flex items-center gap-1">
              <Users className="w-3 h-3" />
              <span>{formatNumber(parseInt(token.holdCount) || 0)}</span>
            </span>
            <span className={`font-bold ${(token.change24 || 0) >= 0 ? 'text-green-500 dark:text-green-400' : 'text-red-500 dark:text-red-400'}`}>
              {(token.change24 || 0) >= 0 ? '+' : ''}{Number(token.change24 || 0).toFixed(1)}%
            </span>
            <span>📊{formatNumber(token.txs24h || 0)}</span>
          </div>

          {/* 3行：社交媒体（始终显示，保持高度一致） */}
          <div className="text-xs text-blue-500 mb-1 min-h-[16px]">
            {getSocialUsername() || ''}
          </div>

          {/* 4行：进度条（始终显示，保持高度一致） */}
          <div className="mb-1">
            <div className="flex items-center justify-between text-xs mb-1">
              <span className="text-muted-foreground">{t('tokenList.labels.progress')}</span>
              <span className="text-foreground font-medium">
                {((token.domesticProgress || 0) * 100).toFixed(1)}%
              </span>
            </div>
            <div className="w-full bg-muted rounded-full h-1.5">
              <div 
                className="bg-gradient-to-r from-blue-500 to-purple-500 h-1.5 rounded-full transition-all duration-300"
                style={{ width: `${Math.min(100, (token.domesticProgress || 0) * 100)}%` }}
              ></div>
            </div>
          </div>
        </div>

        {/* 右侧：价格和购买按钮 */}
        <div className="flex flex-col items-end justify-center w-18 flex-shrink-0">
          {/* 1行：V $vol24h，MC $mktCap */}
          <div className="text-right mb-2 space-y-1">
            <div className="flex items-center justify-end gap-1 text-xs">
              <span className="text-muted-foreground/70">V</span>
              <span className="text-foreground font-bold">{formatVolPrice(token.vol24h || 0)}</span>
            </div>
            <div className="flex items-center justify-end gap-1 text-xs">
              <span className="text-muted-foreground/70">MC</span>
              <span className="text-blue-500 font-bold">{formatVolPrice(token.mktCap || 0)}</span>
            </div>
          </div>
          
          {/* 2行：Buy按钮 */}
          <button 
            className="bg-green-500 hover:bg-green-600 text-white px-3 py-1.5 rounded text-xs font-bold transition-colors w-full"
            onClick={(e) => {
              e.stopPropagation();
              handleBuyClick(token);
            }}
          >
            ⚡ Buy
          </button>
        </div>
      </div>
    );
  };

  // CLMM Pool 卡片组件
  const PoolCard = ({ pool, version }) => {
    return (
      <div 
        className="bg-card border border-border rounded-lg p-4 hover:bg-accent/50 cursor-pointer transition-all duration-200 hover:shadow-md"
        onClick={() => onTokenSelect(pool)}
      >
        <div className="flex items-start justify-between mb-3">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 bg-gradient-to-r from-cyan-500 to-blue-500 rounded-full flex items-center justify-center text-white font-bold text-sm">
              🏊
            </div>
            <div>
              <h3 className="font-semibold text-sm text-foreground">{pool.name || pool.tokenName || 'Pool'}</h3>
              <p className="text-xs text-muted-foreground">{pool.symbol || 'POOL'}</p>
            </div>
          </div>
          <Badge variant="outline" className="text-xs">
            {version}
          </Badge>
        </div>

        <div className="grid grid-cols-2 gap-4 mb-3">
          <div>
            <p className="text-sm text-muted-foreground">{t('tokenList.labels.tvl')}</p>
            <p className="font-semibold">{formatNumber(pool.tvl || 0)}</p>
          </div>
          <div className="text-right">
            <p className="text-sm text-muted-foreground">{t('tokenList.labels.apr')}</p>
            <p className="font-semibold">{pool.apr || '0%'}</p>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-2 text-xs text-muted-foreground">
          <div>
            <span className="block">{t('tokenList.labels.volume24h')}</span>
            <span className="font-medium text-foreground">{formatNumber(pool.volume24h || 0)}</span>
          </div>
          <div>
            <span className="block">{t('tokenList.labels.fees')}</span>
            <span className="font-medium text-foreground">{formatNumber(pool.fees || 0)}</span>
          </div>
        </div>
      </div>
    );
  };

  return (
    <div className="flex flex-col min-h-[calc(100vh-64px)]">
      {/* 添加样式 */}
      <style>{tokenIconStyles}</style>
      <div className="container mx-auto p-6 flex-1 bg-background/80">
        {/* Header */}
        <div>
          <div className="flex items-center justify-between mb-6">
            <div>
              <h1 className="text-3xl font-bold text-foreground mb-2">{t('tokenList.title')}</h1>
              <p className="text-muted-foreground">发现 Solana 上的最新代币</p>
            </div>
            
              <div className="flex items-center space-x-4 text-sm text-muted-foreground">
                <StatusIndicator status={connectionStatus} />

                {lastUpdate && (
                  <div className="flex items-center space-x-2">
                    <Clock className="h-4 w-4" />
                    <span>最后更新: {lastUpdate.toLocaleTimeString()}</span>
                  </div>
                )}
                
                {realtimeCount > 0 && (
                  <Badge variant="outline" className="animate-pulse">
                    实时更新: {realtimeCount}
                  </Badge>
                )}
              </div>
          </div>

          {/* Toolbar */}
          <div className="flex items-center justify-between flex-col sm:flex-row mb-2">
            <div className="flex items-center space-x-4 text-sm text-muted-foreground">
              {/* Platform Buttons: Pump.fun / Pump.fun AMM */}
              <div className="inline-flex items-center gap-2">
                <Button
                  variant={activeTab === 'pumpfun' ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => handleTabSwitch('pumpfun')}
                >
                  Pump.fun
                </Button>
                <Button
                  variant={activeTab === 'pumpamm' ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => handleTabSwitch('pumpamm')}
                >
                  Pump.fun AMM
                </Button>
              </div>
            </div>
            
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={handleRetry}
                disabled={loading}
                className="flex items-center space-x-2"
              >
                <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
                <span>{t('tokenList.buttons.refresh')}</span>
              </Button>
            </div>
          </div>
        </div>

        {/* 错误状态 */}
        {error && <ErrorState />}

        {/* 新代币通知 */}
        {newTokenNotifications.length > 0 && (
          <div className="flex flex-wrap gap-2 mb-6">
            {newTokenNotifications.map((notification) => (
              <Badge
                key={notification.id}
                variant="default"
                className="animate-in slide-in-from-right-5 duration-300 bg-green-500/10 text-green-600 border-green-500/20"
              >
                🆕 {notification.tokenName}
              </Badge>
            ))}
          </div>
        )}

        {/* 主要内容 - 三列并排布局 */}
        {(activeTab === 'pumpfun' || activeTab === 'pumpamm') && !error && (
          <div className="flex border min-h-[calc(100vh-276px)] flex-1">
            {/* New Tokens 列 */}
            <div className="flex flex-col flex-1 h-full">
              <div className="flex items-center space-x-3 p-2 border-b border-border sticky top-0 bg-background z-10">
                <Flame className="h-5 w-5 text-orange-500" />
                <h2 className="text-xl font-bold text-foreground">{t('tokenList.sections.newTokens')}</h2>
                <Badge variant="outline">{newTokens.length}</Badge>
              </div>
              <div className="flex-1 overflow-y-scroll max-h-[calc(100vh-350px)] custom-scrollbar">
                {loading && newTokens.length === 0 ? (
                  <div className="flex items-center justify-center py-12">
                    <LoadingSpinner className="mr-2" />
                    <span>{t('tokenList.loading.loadingNewTokens')}</span>
                  </div>
                ) : newTokens.length === 0 ? (
                  <div className="text-center py-12 text-muted-foreground">
                    <Flame className="h-8 w-8 mx-auto mb-2 opacity-50" />
                    <p>暂无新发布的代币</p>
                  </div>
                ) : (
                  <div className="space-y-0">
                    {newTokens.map((token) => (
                      <TokenCard key={token.id || token.tokenAddress} token={token} type="new" />
                    ))}
                  </div>
                )}
              </div>
            </div>

            {/* 分割线 */}
            <div className="border-r border-border"></div>

            {/* Almost Bonded Tokens 列 */}
            <div className="flex flex-col flex-1 h-full">
              <div className="flex items-center space-x-3 p-2 border-b border-border sticky top-0 bg-background z-10">
                <Clock className="h-5 w-5 text-yellow-500" />
                <h2 className="text-xl font-bold text-foreground">{t('tokenList.sections.almostBonded')}</h2>
                <Badge variant="outline">{completingTokens.length}</Badge>
              </div>
              <div className="flex-1 overflow-y-scroll max-h-[calc(100vh-350px)] custom-scrollbar">
                {loading && completingTokens.length === 0 ? (
                  <div className="flex items-center justify-center py-12">
                    <LoadingSpinner className="mr-2" />
                    <span>{t('tokenList.loading.loadingCompletingTokens')}</span>
                  </div>
                ) : completingTokens.length === 0 ? (
                  <div className="text-center py-12 text-muted-foreground">
                    <Clock className="h-8 w-8 mx-auto mb-2 opacity-50" />
                    <p>暂无即将完成的代币</p>
                  </div>
                ) : (
                  <div className="space-y-0">
                    {completingTokens.map((token) => (
                      <TokenCard key={token.id || token.tokenAddress} token={token} type="completing" />
                    ))}
                  </div>
                )}
              </div>
            </div>

            {/* 分割线 */}
            <div className="border-r border-border"></div>

            {/* Migrated Tokens 列 */}
            <div className="flex flex-col flex-1 h-full">
              <div className="flex items-center space-x-3 p-2 border-b border-border sticky top-0 bg-background z-10">
                <CheckCircle className="h-5 w-5 text-green-500" />
                <h2 className="text-xl font-bold text-foreground">{t('tokenList.sections.migrated')}</h2>
                <Badge variant="outline">{completedTokens.length}</Badge>
              </div>
              <div className="flex-1 overflow-y-scroll max-h-[calc(100vh-350px)] custom-scrollbar">
                {loading && completedTokens.length === 0 ? (
                  <div className="flex items-center justify-center py-12">
                    <LoadingSpinner className="mr-2" />
                    <span>{t('tokenList.loading.loadingCompletedTokens')}</span>
                  </div>
                ) : completedTokens.length === 0 ? (
                  <div className="text-center py-12 text-muted-foreground">
                    <CheckCircle className="h-8 w-8 mx-auto mb-2 opacity-50" />
                    <p>暂无已完成的代币</p>
                  </div>
                ) : (
                  <div className="space-y-0">
                    {completedTokens.map((token) => (
                      <TokenCard key={token.id || token.tokenAddress} token={token} type="completed" />
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        )}
        
        {/* Removed CLMM sections from Trenches */}
        
        {/* Mock 模式指示器 removed */}
      </div>

      {/* BuyModal */}
      <BuyModal
        isOpen={showBuyModal}
        onClose={handleCloseBuyModal}
        token={selectedToken}
      />
    </div>
  );
};

export default TokenList;
