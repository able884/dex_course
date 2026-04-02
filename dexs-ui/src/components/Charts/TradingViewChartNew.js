import React, { useEffect, useRef, useState } from 'react';
import { createChart } from 'lightweight-charts';
import { Card, CardContent, CardHeader, CardTitle } from '../UI/enhanced-card';
import { Button } from '../UI/Button';
import { Badge } from '../UI/badge';
import { LoadingSpinner } from '../UI/loading-spinner';
import { 
  RefreshCw, 
  Wifi, 
  WifiOff, 
  TrendingUp, 
  TrendingDown,
  Activity,
  AlertCircle,
  BarChart3
} from 'lucide-react';
import { cn } from '../../lib/utils';
import { useTranslation } from '../../i18n/LanguageContext';

// API URL configuration - using the same pattern as other components
const API_URL = process.env.NODE_ENV === 'development' 
  ? '' // Use proxy in development
  : '/api'; // Use Nginx proxy in production

const TradingViewChart = ({ token, visible = true, mockMode = false }) => {
  const { t } = useTranslation();
  const chartContainerRef = useRef();
  const chartRef = useRef();
  const candlestickSeriesRef = useRef();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const [interval, setInterval] = useState('1h');
  const [wsConnection] = useState(null);
  // const mockIntervalRef = useRef(null);
  const [chartData, setChartData] = useState([]);
  const [lastPrice, setLastPrice] = useState(null);
  const [priceChange, setPriceChange] = useState(0);

  // 时间间隔选项
  const intervals = [
    { value: '1m', label: '1分钟' },
    { value: '5m', label: '5分钟' },
    { value: '15m', label: '15分钟' },
    { value: '1h', label: '1小时' },
    { value: '4h', label: '4小时' },
    { value: '1d', label: '1天' }
  ];

  // Initialize chart
  useEffect(() => {
    if (!chartContainerRef.current || !visible) {
      console.log('Chart initialization skipped:', { 
        hasContainer: !!chartContainerRef.current, 
        visible 
      });
      return;
    }

    console.log('Creating TradingView chart...');
    
    try {
      const chart = createChart(chartContainerRef.current, {
        layout: {
          background: { color: 'transparent' },
          textColor: 'hsl(var(--foreground))',
        },
        grid: {
          vertLines: { color: 'hsl(var(--border))' },
          horzLines: { color: 'hsl(var(--border))' },
        },
        crosshair: {
          mode: 1, // CrosshairMode.Normal
        },
        rightPriceScale: {
          borderColor: 'hsl(var(--border))',
        },
        timeScale: {
          borderColor: 'hsl(var(--border))',
          timeVisible: true,
          secondsVisible: false,
        },
        watermark: {
          visible: true,
          fontSize: 24,
          horzAlign: 'center',
          vertAlign: 'center',
          color: 'hsl(var(--muted-foreground))',
          text: token?.tokenName || 'Token Chart',
        },
        handleScroll: {
          mouseWheel: true,
          pressedMouseMove: true,
        },
        handleScale: {
          axisPressedMouseMove: true,
          mouseWheel: true,
          pinch: true,
        },
      });

      chartRef.current = chart;

      // Create candlestick series
      const candlestickSeries = chart.addCandlestickSeries({
        upColor: 'hsl(var(--success))',
        downColor: 'hsl(var(--error))',
        borderDownColor: 'hsl(var(--error))',
        borderUpColor: 'hsl(var(--success))',
        wickDownColor: 'hsl(var(--error))',
        wickUpColor: 'hsl(var(--success))',
      });

      candlestickSeriesRef.current = candlestickSeries;

      // Handle resize
      const handleResize = () => {
        if (chartRef.current && chartContainerRef.current) {
          chartRef.current.applyOptions({
            width: chartContainerRef.current.clientWidth,
            height: chartContainerRef.current.clientHeight,
          });
        }
      };

      window.addEventListener('resize', handleResize);

      return () => {
        window.removeEventListener('resize', handleResize);
        if (chartRef.current) {
          chartRef.current.remove();
          chartRef.current = null;
        }
      };
    } catch (error) {
      console.error('Error creating chart:', error);
      setError('Failed to initialize chart');
    }
  }, [visible, token]);

  // Generate mock data
  const generateMockData = () => {
    const data = [];
    const now = Math.floor(Date.now() / 1000);
    let price = 0.00001 + Math.random() * 0.00009; // Random price between 0.00001 and 0.0001

    for (let i = 100; i >= 0; i--) {
      const time = now - i * 3600; // 1 hour intervals
      const change = (Math.random() - 0.5) * 0.00001;
      price = Math.max(0.000001, price + change);
      
      const high = price + Math.random() * 0.000005;
      const low = price - Math.random() * 0.000005;
      const open = price + (Math.random() - 0.5) * 0.000002;
      const close = price + (Math.random() - 0.5) * 0.000002;

      data.push({
        time,
        open: Math.max(0.000001, open),
        high: Math.max(0.000001, high),
        low: Math.max(0.000001, low),
        close: Math.max(0.000001, close),
      });
    }

    return data;
  };

  // Load chart data
  const loadChartData = async () => {
    if (!token) return;

    setIsLoading(true);
    setError('');

    try {
      if (mockMode) {
        // Use mock data
        const mockData = generateMockData();
        setChartData(mockData);
        
        if (candlestickSeriesRef.current) {
          candlestickSeriesRef.current.setData(mockData);
        }
        
        // Set last price and change
        if (mockData.length > 0) {
          const lastCandle = mockData[mockData.length - 1];
          const prevCandle = mockData[mockData.length - 2];
          setLastPrice(lastCandle.close);
          if (prevCandle) {
            const change = ((lastCandle.close - prevCandle.close) / prevCandle.close) * 100;
            setPriceChange(change);
          }
        }
      } else {
        // Try to fetch real data (this would need actual API endpoint)
        const response = await fetch(`${API_URL}/v1/chart/${token.tokenAddress}?interval=${interval}`);
        
        if (!response.ok) {
          throw new Error('Failed to fetch chart data');
        }
        
        const data = await response.json();
        
        if (data.success && data.data) {
          setChartData(data.data);
          if (candlestickSeriesRef.current) {
            candlestickSeriesRef.current.setData(data.data);
          }
        } else {
          throw new Error('Invalid chart data format');
        }
      }
    } catch (err) {
      console.error('Error loading chart data:', err);
      setError(err.message);
      
      // Fallback to mock data on error
      const mockData = generateMockData();
      setChartData(mockData);
      
      if (candlestickSeriesRef.current) {
        candlestickSeriesRef.current.setData(mockData);
      }
    } finally {
      setIsLoading(false);
    }
  };

  // Load data when token or interval changes
  useEffect(() => {
    if (token && visible) {
      loadChartData();
    }
  }, [token, interval, visible, mockMode]);

  // Mock real-time updates
  useEffect(() => {
    if (!mockMode || !visible || !token) return;

    const updateInterval = setInterval(() => {
      if (candlestickSeriesRef.current && chartData.length > 0) {
        const lastCandle = chartData[chartData.length - 1];
        const newPrice = lastCandle.close + (Math.random() - 0.5) * 0.000001;
        
        const updatedCandle = {
          ...lastCandle,
          close: Math.max(0.000001, newPrice),
          high: Math.max(lastCandle.high, newPrice),
          low: Math.min(lastCandle.low, newPrice),
        };

        candlestickSeriesRef.current.update(updatedCandle);
        setLastPrice(updatedCandle.close);
        
        // Update price change
        if (chartData.length > 1) {
          const prevCandle = chartData[chartData.length - 2];
          const change = ((updatedCandle.close - prevCandle.close) / prevCandle.close) * 100;
          setPriceChange(change);
        }
      }
    }, 2000);

    return () => clearInterval(updateInterval);
  }, [mockMode, visible, token, chartData]);

  // Format price for display
  const formatPrice = (price) => {
    if (!price) return '$0.000000';
    if (price < 0.000001) return price.toExponential(2);
    return `$${price.toFixed(8)}`;
  };

  // Format percentage
  const formatPercentage = (percent) => {
    if (!percent) return '0.00%';
    const sign = percent >= 0 ? '+' : '';
    return `${sign}${percent.toFixed(2)}%`;
  };

  if (!token) {
    return (
      <Card className="h-[600px] flex items-center justify-center">
        <CardContent className="text-center">
          <BarChart3 className="h-16 w-16 mx-auto mb-4 text-muted-foreground" />
          <h3 className="text-lg font-semibold mb-2">选择代币查看图表</h3>
          <p className="text-muted-foreground">{t('tradingChart.noTokenSelected')}</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="h-[600px] flex flex-col">
      <CardHeader className="pb-4">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div className="flex items-center space-x-4">
            <div>
              <CardTitle className="flex items-center space-x-2">
                <span>{token.tokenName || 'Unknown Token'}</span>
                {mockMode && (
                  <Badge variant="warning" className="text-xs">
                    模拟数据
                  </Badge>
                )}
              </CardTitle>
              <p className="text-sm text-muted-foreground font-mono mt-1">
                {token.tokenAddress}
              </p>
            </div>
            
            {lastPrice && (
              <div className="text-right">
                <div className="text-2xl font-bold">
                  {formatPrice(lastPrice)}
                </div>
                <div className={cn(
                  "text-sm font-medium flex items-center",
                  priceChange >= 0 ? "text-success" : "text-error"
                )}>
                  {priceChange >= 0 ? (
                    <TrendingUp className="h-4 w-4 mr-1" />
                  ) : (
                    <TrendingDown className="h-4 w-4 mr-1" />
                  )}
                  {formatPercentage(priceChange)}
                </div>
              </div>
            )}
          </div>

          <div className="flex items-center space-x-2">
            {/* 时间间隔选择器 */}
            <div className="flex bg-muted rounded-lg p-1">
              {intervals.map((int) => (
                <Button
                  key={int.value}
                  variant={interval === int.value ? "default" : "ghost"}
                  size="sm"
                  onClick={() => setInterval(int.value)}
                  className={cn(
                    "px-3 py-1 text-xs",
                    interval === int.value && "bg-background shadow-sm"
                  )}
                >
                  {int.label}
                </Button>
              ))}
            </div>

            {/* 刷新按钮 */}
            <Button
              variant="outline"
              size="sm"
              onClick={loadChartData}
              disabled={isLoading}
              className="flex items-center space-x-2"
            >
              <RefreshCw className={cn("h-4 w-4", isLoading && "animate-spin")} />
            </Button>

            {/* 连接状态 */}
            <div className="flex items-center space-x-2 px-3 py-1 bg-muted rounded-full">
              {mockMode ? (
                <>
                  <Activity className="h-4 w-4 text-warning" />
                  <span className="text-xs text-warning font-medium">模拟模式</span>
                </>
              ) : wsConnection === 'connected' ? (
                <>
                  <Wifi className="h-4 w-4 text-success" />
                  <span className="text-xs text-success font-medium">实时</span>
                </>
              ) : (
                <>
                  <WifiOff className="h-4 w-4 text-muted-foreground" />
                  <span className="text-xs text-muted-foreground font-medium">离线</span>
                </>
              )}
            </div>
          </div>
        </div>
      </CardHeader>

      <CardContent className="flex-1 p-4 pt-0 relative">
        {error && (
          <div className="absolute top-4 left-4 right-4 z-10">
            <div className="bg-error/10 border border-error/20 rounded-lg p-3 flex items-center space-x-2">
              <AlertCircle className="h-4 w-4 text-error" />
              <span className="text-sm text-error">{error}</span>
              <Button
                variant="outline"
                size="sm"
                onClick={loadChartData}
                className="ml-auto"
              >
                重试
              </Button>
            </div>
          </div>
        )}

        {isLoading && (
          <div className="absolute inset-0 flex items-center justify-center bg-background/80 backdrop-blur-sm z-20">
            <div className="flex flex-col items-center space-y-2">
              <LoadingSpinner />
              <p className="text-sm text-muted-foreground">加载图表数据...</p>
            </div>
          </div>
        )}

        <div
          ref={chartContainerRef}
          className="w-full h-full bg-card rounded-lg border"
          style={{ minHeight: '400px' }}
        />
      </CardContent>
    </Card>
  );
};

export default TradingViewChart;
