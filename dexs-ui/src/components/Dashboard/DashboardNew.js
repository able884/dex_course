import React, { useState, useMemo } from 'react';
import { motion } from 'framer-motion';
import { Card, CardContent, CardHeader, CardTitle } from '../UI/card';
import { Button } from '../UI/Button';
import { 
  TrendingUp, 
  TrendingDown, 
  DollarSign, 
  Users, 
  Activity, 
  Zap, 
  Shield, 
  Plus,
  BarChart3,
  RefreshCw,
  Flame,
} from 'lucide-react';
import { LineChart, Line, AreaChart, Area, XAxis, YAxis, ResponsiveContainer, Tooltip } from 'recharts';
import { cn } from '../../lib/utils';
import { useTranslation } from '../../i18n/LanguageContext';
function DashboardNew({ onNavigateToTokens, onNavigateToTokenCreation, onNavigateToPoolCreation }) {
  const [searchTerm, setSearchTerm] = useState("");
  const { t } = useTranslation();

  // Mock token data with more realistic DEX information
  const mockTokens = [
    {
      id: "solana",
      symbol: "SOL",
      name: "Solana",
      price: 98.45,
      change24h: 5.75,
      volume24h: 2100000000,
      marketCap: 43500000000,
      liquidity: 850000000,
      holders: 12800000,
      sparkline: [95, 97, 99, 96, 98, 100, 102, 98],
      isHot: true,
      isTrending: true
    },
    {
      id: "ethereum",
      symbol: "ETH",
      name: "Ethereum",
      price: 2650.75,
      change24h: 2.45,
      volume24h: 15420000000,
      marketCap: 318500000000,
      liquidity: 2500000000,
      holders: 98500000,
      sparkline: [2600, 2620, 2580, 2640, 2670, 2650, 2680, 2650],
      isTrending: true
    },
    {
      id: "pump",
      symbol: "PUMP",
      name: "Pump Token",
      price: 0.0045,
      change24h: 125.5,
      volume24h: 45000000,
      marketCap: 45000000,
      liquidity: 12000000,
      holders: 25000,
      sparkline: [0.002, 0.0025, 0.003, 0.0035, 0.004, 0.0042, 0.0048, 0.0045],
      isHot: true,
      isTrending: true
    },
    {
      id: "raydium",
      symbol: "RAY",
      name: "Raydium",
      price: 1.85,
      change24h: -2.15,
      volume24h: 85000000,
      marketCap: 850000000,
      liquidity: 180000000,
      holders: 180000,
      sparkline: [1.9, 1.88, 1.82, 1.87, 1.83, 1.85, 1.84, 1.85]
    }
  ];

  // Mock recent trades
  const mockTrades = [
    { id: "1", type: "buy", token: "SOL", amount: 50, price: 98.45, timestamp: Date.now() - 30000, user: "0x1234...5678" },
    { id: "2", type: "sell", token: "PUMP", amount: 10000, price: 0.0045, timestamp: Date.now() - 45000, user: "0x9876...5432" },
    { id: "3", type: "buy", token: "ETH", amount: 2.5, price: 2650.75, timestamp: Date.now() - 60000, user: "0xabcd...efgh" },
    { id: "4", type: "buy", token: "RAY", amount: 100, price: 1.85, timestamp: Date.now() - 90000, user: "0x1111...2222" }
  ];

  // Mock liquidity pools
  const mockPools = [
    { token0: "SOL", token1: "USDC", tvl: 125000000, volume24h: 8500000, fees24h: 25500, apr: 12.5 },
    { token0: "PUMP", token1: "SOL", tvl: 12000000, volume24h: 2200000, fees24h: 6600, apr: 45.2 },
    { token0: "ETH", token1: "USDC", tvl: 89000000, volume24h: 6200000, fees24h: 18600, apr: 8.9 },
    { token0: "RAY", token1: "SOL", tvl: 28000000, volume24h: 1800000, fees24h: 5400, apr: 18.7 }
  ];

  // Price chart data
  const chartData = useMemo(() => {
    return Array.from({ length: 24 }, (_, i) => ({
      time: `${i}:00`,
      price: 98 + Math.random() * 10 + Math.sin(i * 0.5) * 5,
      volume: Math.random() * 1000000 + 500000
    }))
  }, []);

  // Utility functions
  const formatPrice = (price) => {
    if (price < 0.01) return `$${price.toFixed(6)}`
    if (price < 1) return `$${price.toFixed(4)}`
    if (price < 100) return `$${price.toFixed(2)}`
    return `$${price.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
  }

  const formatVolume = (volume) => {
    if (volume >= 1e9) return `$${(volume / 1e9).toFixed(2)}B`
    if (volume >= 1e6) return `$${(volume / 1e6).toFixed(2)}M`
    if (volume >= 1e3) return `$${(volume / 1e3).toFixed(2)}K`
    return `$${volume.toFixed(2)}`
  }

  const formatPercentage = (percentage) => {
    const sign = percentage >= 0 ? "+" : ""
    return `${sign}${percentage.toFixed(2)}%`
  }

  const formatTimeAgo = (timestamp) => {
    const seconds = Math.floor((Date.now() - timestamp) / 1000)
    if (seconds < 60) return `${seconds}s ago`
    const minutes = Math.floor(seconds / 60)
    if (minutes < 60) return `${minutes}m ago`
    const hours = Math.floor(minutes / 60)
    return `${hours}h ago`
  }

  const totalMarketCap = useMemo(() => {
    return mockTokens.reduce((sum, token) => sum + token.marketCap, 0)
  }, []);

  const total24hVolume = useMemo(() => {
    return mockTokens.reduce((sum, token) => sum + token.volume24h, 0)
  }, []);

  const stats = [
    {
      title: t('dashboard.marketCap'),
      value: formatVolume(totalMarketCap),
      change: '+2.45%',
      trend: 'up',
      icon: DollarSign
    },
    {
      title: t('dashboard.totalVolume'),
      value: formatVolume(total24hVolume),
      change: '+8.92%',
      trend: 'up',
      icon: BarChart3
    },
    {
      title: t('dashboard.activeTraders'),
      value: '12,847',
      change: '+15.3%',
      trend: 'up',
      icon: Users
    },
    {
      title: t('dashboard.liquidity'),
      value: '$8.2B',
      change: '-1.2%',
      trend: 'down',
      icon: Activity
    }
  ];

  const quickActions = [
    {
      title: t('dashboard.startTrading'),
      description: t('dashboard.startTradingDesc'),
      icon: Zap,
      color: 'bg-blue-500',
      action: () => onNavigateToTokens('trending')
    },
    {
      title: t('dashboard.createToken'),
      description: t('dashboard.createTokenDesc'),
      icon: Plus,
      color: 'bg-green-500',
      action: () => onNavigateToTokenCreation?.()
    },
    {
      title: t('dashboard.createPool'),
      description: t('dashboard.createPoolDesc'),
      icon: Shield,
      color: 'bg-purple-500',
      action: () => onNavigateToPoolCreation?.()
    }
  ];

  // Component for enhanced stat cards
  const StatCard = ({ title, value, change, icon, trend }) => {
    return (
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="bg-card/90 dark:bg-card/60 backdrop-blur-sm border border-border/50 rounded-xl p-6 hover:shadow-lg transition-all duration-300"
      >
        <div className="flex items-center justify-between mb-4">
          <div className="p-2 bg-primary/10 rounded-lg">
            {React.createElement(icon, { className: "w-5 h-5 text-primary" })}
          </div>
          {trend && (
            <div className={cn(
              "flex items-center text-sm font-medium",
              trend === 'up' ? "text-green-500" : "text-red-500"
            )}>
              {trend === 'up' ? <TrendingUp className="w-4 h-4 mr-1" /> : <TrendingDown className="w-4 h-4 mr-1" />}
              {change}
            </div>
          )}
        </div>
        <div>
          <p className="text-sm text-muted-foreground mb-1">{title}</p>
          <p className="text-2xl font-bold text-foreground">{value}</p>
        </div>
      </motion.div>
    )
  }

  // Component for token rows
  const TokenRow = ({ token, index }) => {
    const isPositive = token.change24h >= 0

    return (
      <motion.div
        initial={{ opacity: 0, x: -20 }}
        animate={{ opacity: 1, x: 0 }}
        transition={{ delay: index * 0.1 }}
        className="flex items-center justify-between p-4 bg-muted/30 rounded-lg hover:bg-muted/50 transition-colors cursor-pointer"
        onClick={() => onNavigateToTokens('trending')}
      >
        <div className="flex items-center space-x-3">
          <div className="w-10 h-10 bg-gradient-to-br from-primary/20 to-primary/10 rounded-full flex items-center justify-center">
            <span className="text-sm font-bold text-primary">{token.symbol[0]}</span>
          </div>
          <div>
            <div className="flex items-center space-x-2">
              <span className="font-semibold text-sm">{token.symbol}</span>
              {token.isHot && <Flame className="w-3 h-3 text-orange-500" />}
              {token.isTrending && <TrendingUp className="w-3 h-3 text-green-500" />}
            </div>
            <span className="text-xs text-muted-foreground">{token.name}</span>
          </div>
        </div>
        <div className="flex items-center space-x-4">
          <div className="text-right">
            <div className="text-sm font-semibold">{formatPrice(token.price)}</div>
            <div className={cn(
              "text-xs font-medium",
              isPositive ? "text-green-500" : "text-red-500"
            )}>
              {formatPercentage(token.change24h)}
            </div>
          </div>
          <div className="w-16 h-8">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={token.sparkline.map((price, i) => ({ price, index: i }))}>
                <Line
                  type="monotone"
                  dataKey="price"
                  stroke={isPositive ? "#10b981" : "#ef4444"}
                  strokeWidth={2}
                  dot={false}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>
      </motion.div>
    )
  }

  // Component for trade rows
  const TradeRow = ({ trade, index }) => {
    const isBuy = trade.type === 'buy'

    return (
      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: index * 0.05 }}
        className="flex items-center justify-between py-3 px-4 border-b border-border last:border-b-0"
      >
        <div className="flex items-center space-x-3">
          <div className={cn(
            "w-2 h-2 rounded-full",
            isBuy ? "bg-green-500" : "bg-red-500"
          )} />
          <div>
            <div className="flex items-center space-x-2">
              <span className="font-semibold text-sm">{trade.type.toUpperCase()}</span>
              <span className="text-sm text-muted-foreground">{trade.token}</span>
            </div>
            <span className="text-xs text-muted-foreground">{trade.user}</span>
          </div>
        </div>
        <div className="text-right">
          <div className="text-sm font-semibold">{trade.amount} {trade.token}</div>
          <div className="text-xs text-muted-foreground">{formatTimeAgo(trade.timestamp)}</div>
        </div>
      </motion.div>
    )
  }

  return (
    <div className="container mx-auto px-4 py-8 space-y-8">
      {/* Hero Section */}
      <div className="text-center space-y-4">
        <motion.h1 
          initial={{ opacity: 0, y: -20 }}
          animate={{ opacity: 1, y: 0 }}
          className="text-4xl font-bold bg-gradient-to-r from-blue-600 to-purple-600 bg-clip-text text-transparent"
        >
          {t('dashboard.welcome')}
        </motion.h1>
        <motion.p 
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.1 }}
          className="text-xl text-muted-foreground max-w-2xl mx-auto"
        >
          {t('dashboard.subtitle')}
        </motion.p>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        {stats.map((stat, index) => (
          <StatCard
            key={index}
            title={stat.title}
            value={stat.value}
            change={stat.change}
            icon={stat.icon}
            trend={stat.trend}
          />
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
        {/* Price Chart */}
        <div className="lg:col-span-2">
          <motion.div
            initial={{ opacity: 0, x: -20 }}
            animate={{ opacity: 1, x: 0 }}
            className="bg-card/90 dark:bg-card/60 backdrop-blur-sm border border-border/50 rounded-xl p-6"
          >
            <div className="flex items-center justify-between mb-6">
              <div>
                <h3 className="text-lg font-semibold text-foreground">SOL/USDC</h3>
                <p className="text-sm text-muted-foreground">{t('dashboard.priceChart')}</p>
              </div>
              <div className="flex items-center space-x-2">
                <button className="p-2 hover:bg-muted rounded-lg transition-colors">
                  <RefreshCw className="w-4 h-4" />
                </button>
              </div>
            </div>
            <div className="h-64">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={chartData}>
                  <defs>
                    <linearGradient id="priceGradient" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="#3b82f6" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <XAxis 
                    dataKey="time" 
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 12, fill: '#6b7280' }}
                  />
                  <YAxis 
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 12, fill: '#6b7280' }}
                    domain={['dataMin - 5', 'dataMax + 5']}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: 'hsl(var(--card))',
                      border: '1px solid hsl(var(--border))',
                      borderRadius: '8px',
                      fontSize: '12px'
                    }}
                  />
                  <Area
                    type="monotone"
                    dataKey="price"
                    stroke="#3b82f6"
                    strokeWidth={2}
                    fill="url(#priceGradient)"
                  />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </motion.div>
        </div>

        {/* Recent Trades */}
        <motion.div
          initial={{ opacity: 0, x: 20 }}
          animate={{ opacity: 1, x: 0 }}
          className="bg-card/90 dark:bg-card/60 backdrop-blur-sm border border-border/50 rounded-xl p-6"
        >
          <div className="flex items-center justify-between mb-6">
            <div>
              <h3 className="text-lg font-semibold text-foreground">{t('dashboard.recentTrades')}</h3>
              <p className="text-sm text-muted-foreground">{t('dashboard.recentTradesActivity')}</p>
            </div>
            <div className="flex items-center space-x-1">
              <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
              <span className="text-xs text-muted-foreground">{t('dashboard.live')}</span>
            </div>
          </div>
          <div className="space-y-1 max-h-64 overflow-y-auto">
            {mockTrades.map((trade, index) => (
              <TradeRow key={trade.id} trade={trade} index={index} />
            ))}
          </div>
        </motion.div>
      </div>

      {/* Quick Actions */}
      <div className="space-y-6">
        <motion.h2 
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          className="text-2xl font-bold text-center"
        >
          {t('dashboard.quickActions')}
        </motion.h2>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {quickActions.map((action, index) => {
            const IconComponent = action.icon;
            return (
              <motion.div
                key={index}
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: index * 0.1 }}
              >
                <Card className="group hover:shadow-lg transition-all duration-300 cursor-pointer bg-card/90 dark:bg-card/60 backdrop-blur-sm border-border/50">
                  <CardHeader>
                    <div className={`w-12 h-12 rounded-lg ${action.color} flex items-center justify-center mb-4`}>
                      <IconComponent className="h-6 w-6 text-white" />
                    </div>
                    <CardTitle className="text-lg">{action.title}</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <p className="text-muted-foreground mb-4">{action.description}</p>
                    <Button 
                      className="w-full" 
                      variant="outline"
                      onClick={action.action}
                    >
                      {t('dashboard.getStarted')}
                    </Button>
                  </CardContent>
                </Card>
              </motion.div>
            );
          })}
        </div>
      </div>

      {/* Trending Tokens */}
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <motion.h2 
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            className="text-2xl font-bold"
          >
            {t('dashboard.trendingTokens')}
          </motion.h2>
          <Button variant="outline" onClick={() => onNavigateToTokens('trending')}>
            {t('dashboard.viewAll')}
          </Button>
        </div>
        
        <div className="space-y-4">
          {mockTokens.slice(0, 4).map((token, index) => (
            <TokenRow key={token.id} token={token} index={index} />
          ))}
        </div>
      </div>

      {/* Liquidity Pools */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="bg-card/90 dark:bg-card/60 backdrop-blur-sm border border-border/50 rounded-xl p-6"
      >
        <div className="flex items-center justify-between mb-6">
          <div>
            <h3 className="text-lg font-semibold text-foreground">{t('dashboard.topPools')}</h3>
            <p className="text-sm text-muted-foreground">{t('dashboard.topPoolsTVL')}</p>
          </div>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {mockPools.map((pool, index) => (
            <motion.div
              key={`${pool.token0}-${pool.token1}`}
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ delay: index * 0.1 }}
              className="flex items-center justify-between p-4 bg-muted/30 rounded-lg hover:bg-muted/50 transition-colors cursor-pointer"
              onClick={() => onNavigateToTokens('trending')}
            >
              <div className="flex items-center space-x-3">
                <div className="flex -space-x-2">
                  <div className="w-8 h-8 bg-gradient-to-br from-blue-500 to-blue-600 rounded-full flex items-center justify-center border-2 border-background">
                    <span className="text-xs font-bold text-white">{pool.token0[0]}</span>
                  </div>
                  <div className="w-8 h-8 bg-gradient-to-br from-green-500 to-green-600 rounded-full flex items-center justify-center border-2 border-background">
                    <span className="text-xs font-bold text-white">{pool.token1[0]}</span>
                  </div>
                </div>
                <div>
                  <div className="font-semibold text-sm">{pool.token0}/{pool.token1}</div>
                  <div className="text-xs text-muted-foreground">TVL: {formatVolume(pool.tvl)}</div>
                </div>
              </div>
              <div className="text-right">
                <div className="text-sm font-semibold text-green-500">{pool.apr.toFixed(1)}% APR</div>
                <div className="text-xs text-muted-foreground">24h Vol: {formatVolume(pool.volume24h)}</div>
              </div>
            </motion.div>
          ))}
        </div>
      </motion.div>
    </div>
  );
}

export default DashboardNew;
