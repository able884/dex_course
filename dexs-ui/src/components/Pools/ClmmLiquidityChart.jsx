import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AreaChart, Area, XAxis, YAxis, ResponsiveContainer, ReferenceArea, ReferenceLine } from 'recharts';
import { RefreshCw, ZoomIn, ZoomOut, BarChart2 } from 'lucide-react';

const DEFAULT_HEIGHT = 260;
const FIXED_DOMAIN_MIN = -10000;
const FIXED_DOMAIN_MAX = 10000;
const MIN_PRICE_FLOOR = 1e-8;

const computeTickStep = (price) => {
  if (!Number.isFinite(price) || price <= 0) return 1;
  if (price < 1) return Math.pow(10, Math.floor(Math.log10(price)) - 1);
  if (price < 10) return 1;
  if (price < 100) return 5;
  if (price < 1000) return 50;
  const magnitude = Math.pow(10, Math.floor(Math.log10(price)) - 1);
  return 5 * magnitude;
};

const toNumber = (value) => {
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
};

const formatTick = (value) => {
  if (!Number.isFinite(value)) return '';
  if (Math.abs(value) >= 1) return value.toLocaleString(undefined, { maximumFractionDigits: 2 });
  return value.toPrecision(3);
};

const percentLabel = (price, current) => {
  if (!current || !price) return '';
  const delta = ((price - current) / current) * 100;
  const rounded = Math.abs(delta) >= 1 ? delta.toFixed(1) : delta.toFixed(2);
  return `${delta > 0 ? '+' : ''}${rounded}%`;
};

const buildInitialDomain = ({
  globalMin,
  globalMax,
}) => {
  return [globalMin, globalMax];
};

const clampSelection = (low, high, floor, cap) => {
  const nextMin = Math.max(floor, Math.min(low, high));
  const nextMax = Math.min(cap, Math.max(high, floor));
  return [nextMin, nextMax];
};

const ClmmLiquidityChart = ({
  currentPrice,
  minPrice,
  maxPrice,
  onMinPriceChange,
  onMaxPriceChange,
  priceRangeMin,
  priceRangeMax,
  timeRangePriceMin,
  timeRangePriceMax,
  depthDataPoints,
  token0Symbol,
  token1Symbol,
  inverted = false,
  feeTierBps,
  poolId,
  interactive = true,
  defaultRange = 0.2,
}) => {
  const chartContainerRef = useRef(null);
  const overlayRef = useRef(null);
  const [chartDimensions, setChartDimensions] = useState({ width: 0, height: DEFAULT_HEIGHT });
  const [dragState, setDragState] = useState(null);

  useEffect(() => {
    if (!chartContainerRef.current) return;
    const observer = new ResizeObserver(() => {
      const rect = chartContainerRef.current.getBoundingClientRect();
      setChartDimensions({ width: rect.width, height: DEFAULT_HEIGHT });
    });
    observer.observe(chartContainerRef.current);
    return () => observer.disconnect();
  }, []);

  const toDisplayPrice = useCallback(
    (value) => {
      const n = toNumber(value);
      if (!n || n <= 0) return null;
      const p = inverted ? 1 / n : n;
      return Number.isFinite(p) && p > 0 ? p : null;
    },
    [inverted]
  );

  const formattedData = useMemo(() => {
    if (!Array.isArray(depthDataPoints)) return [];
    return depthDataPoints
      .map(({ price, liquidity }) => {
        const p = toDisplayPrice(price);
        const d = toNumber(liquidity);
        if (!p || d === null) return null;
        return { price: p, depth: Math.max(0, d) };
      })
      .filter(Boolean)
      .sort((a, b) => a.price - b.price);
  }, [depthDataPoints, toDisplayPrice]);

  const current = toDisplayPrice(currentPrice);
  const poolRangeMinDisplay = toDisplayPrice(priceRangeMin);
  const poolRangeMaxDisplay = toDisplayPrice(priceRangeMax);
  const timeRangeMinDisplay = toDisplayPrice(timeRangePriceMin);
  const timeRangeMaxDisplay = toDisplayPrice(timeRangePriceMax);

  const selectionLowerInput = toNumber(minPrice);
  const selectionUpperInput = toNumber(maxPrice);

  // 浠锋牸鑼冨洿鍥哄畾涓?[-10000, 10000]锛岄伩鍏嶅洖閫€鍒版瀬绔祦鍔ㄦ€х偣
  const globalBounds = useMemo(
    () => ({
      min: FIXED_DOMAIN_MIN,
      max: FIXED_DOMAIN_MAX,
    }),
    []
  );

  const liquidityCenter = useMemo(() => {
    if (formattedData.length) {
      const mid = formattedData[Math.floor(formattedData.length / 2)]?.price;
      if (Number.isFinite(mid) && mid > 0) return mid;
    }
    return (globalBounds.min + globalBounds.max) / 2;
  }, [formattedData, globalBounds.min, globalBounds.max]);

  const anchorPrice = current || liquidityCenter;

  // 榛樿瑙嗗浘浠ラ敋鐐逛负涓績锛屽乏鍙冲悇 5 涓埢搴?
  const viewDomain = useMemo(() => {
    if (!anchorPrice) return [globalBounds.min, globalBounds.max];
    const step = computeTickStep(anchorPrice);
    let min = anchorPrice - step * 5;
    let max = anchorPrice + step * 5;
    const span = max - min;
    if (min < globalBounds.min) {
      min = globalBounds.min;
      max = min + span;
    }
    if (max > globalBounds.max) {
      max = globalBounds.max;
      min = max - span;
    }
    if (max <= min) {
      min = anchorPrice - step;
      max = anchorPrice + step;
    }
    return [min, max];
  }, [anchorPrice, globalBounds.min, globalBounds.max]);

const fallbackSelection = useMemo(() => {
    if (selectionLowerInput && selectionUpperInput) return [selectionLowerInput, selectionUpperInput];
    // 濡傛灉鍙粰浜嗗崟渚ц緭鍏ワ紝鐢ㄩ敋鐐规墿灞?
    if (selectionLowerInput && !selectionUpperInput && anchorPrice) {
      return [selectionLowerInput, anchorPrice * 1.2];
    }
    if (!selectionLowerInput && selectionUpperInput && anchorPrice) {
      return [anchorPrice * 0.8, selectionUpperInput];
    }
    // 榛樿鍥為€€鍒颁互閿氱偣涓轰腑蹇?卤10%
    if (anchorPrice) {
      return [anchorPrice * 0.8, anchorPrice * 1.2];
    }
    return [FIXED_DOMAIN_MIN, FIXED_DOMAIN_MAX];
  }, [selectionLowerInput, selectionUpperInput, anchorPrice]);

  const selection = useMemo(
    () => clampSelection(fallbackSelection[0], fallbackSelection[1], globalBounds.min, globalBounds.max),
    [fallbackSelection, globalBounds.min, globalBounds.max]
  );

  // 褰撶埗缁勪欢鏈彁渚涙渶灏?鏈€澶т环鏍兼椂锛岀敤閿氱偣浠锋牸鐢熸垚榛樿鑼冨洿骞跺洖鍐欙紝閬垮厤婊戝潡钀藉湪鏋佺鐐?
  useEffect(() => {
    const noMin = !selectionLowerInput && selectionLowerInput !== 0;
    const noMax = !selectionUpperInput && selectionUpperInput !== 0;
    if (noMin && noMax && anchorPrice) {
      // 根据当前价格设置 ±20% 范围
      const defMin = anchorPrice * 0.8;
      const defMax = anchorPrice * 1.2;
      onMinPriceChange?.(String(defMin));
      onMaxPriceChange?.(String(defMax));
    }
  }, [selectionLowerInput, selectionUpperInput, anchorPrice, onMinPriceChange, onMaxPriceChange]);

  const [domain, setDomain] = useState(() =>
    buildInitialDomain({
      globalMin: viewDomain[0],
      globalMax: viewDomain[1],
    })
  );

  // 浠呭湪閿氱偣鎴栬鍥惧熀鍑嗗彉鍔ㄦ椂閲嶇疆瑙嗗浘锛屾嫋鍔ㄦ粦鍧椾笉閲嶇疆
  useEffect(() => {
    setDomain(
      buildInitialDomain({
        globalMin: viewDomain[0],
        globalMax: viewDomain[1],
      })
    );
  }, [viewDomain, poolId]);

  const axisTicks = useMemo(() => {
    const [min, max] = domain;
    const step = (max - min) / 10;
    const base = Array.from({ length: 11 }, (_, i) => min + i * step);
    const extras = [current, selection[0], selection[1]].filter((v) => Number.isFinite(v));
    const merged = [...base, ...extras].sort((a, b) => a - b);
    return merged.filter((v, i) => i === 0 || Math.abs(v - merged[i - 1]) > (max - min) * 0.001);
  }, [domain, current, selection]);

  const scaleX = useCallback(
    (price) => {
      if (!chartDimensions.width) return 0;
      const clamped = Math.min(Math.max(price, domain[0]), domain[1]);
      const ratio = (clamped - domain[0]) / (domain[1] - domain[0] || 1);
      return ratio * chartDimensions.width;
    },
    [chartDimensions.width, domain]
  );

  const xToPrice = useCallback(
    (x) => {
      if (!chartDimensions.width) return domain[0];
      const ratio = Math.max(0, Math.min(1, x / chartDimensions.width));
      return domain[0] + ratio * (domain[1] - domain[0]);
    },
    [chartDimensions.width, domain]
  );

  const updateSelection = useCallback(
    (nextMin, nextMax) => {
      // 鎷栧姩鏃朵笉鍏佽浣庝簬0
      const floor = 0;
      const [low, high] = clampSelection(Math.max(floor, nextMin), Math.max(floor, nextMax), globalBounds.min, globalBounds.max);
      onMinPriceChange?.(String(low));
      onMaxPriceChange?.(String(high));
    },
    [globalBounds.min, globalBounds.max, onMinPriceChange, onMaxPriceChange]
  );

  // 鎷栧姩瓒呭嚭褰撳墠瑙嗗浘鏃讹紝鑷姩鎵╁睍瑙嗗浘浣嗕繚鎸佺幇鏈夊搴︽瘮渚?
  const handleDragFromClientX = useCallback(
    (clientX) => {
      if (!dragState || !overlayRef.current) return;
      const rect = overlayRef.current.getBoundingClientRect();
      const localX = Math.max(0, Math.min(rect.width, clientX
  - rect.left));
      const priceAtCursor = xToPrice(localX);
      const clampedPrice = Math.max(0, priceAtCursor);
      const floor = 0;
      const cap = FIXED_DOMAIN_MAX;

      if (dragState.type === 'min') {
        if (clampedPrice <= floor && selection[0] <= floor) return;
        updateSelection(clampedPrice, dragState.startMax);
      } else if (dragState.type === 'max') {
        if (clampedPrice >= cap && selection[1] >= cap) return;
        updateSelection(dragState.startMin, clampedPrice);
      } else if (dragState.type === 'range') {
        const delta = clampedPrice - dragState.startPrice;
        const newMin = dragState.startMin + delta;
        const newMax = dragState.startMax + delta;
        if (newMin < floor || newMax > cap) return;
        updateSelection(newMin, newMax);
      } else if (dragState.type === 'pan') {
        const span = dragState.startDomain[1] -
  dragState.startDomain[0];
        if (!chartDimensions.width || span <= 0) return;
        const deltaPrice = ((localX - dragState.startX) /
  chartDimensions.width) * span;
        let newMin = dragState.startDomain[0] - deltaPrice;
        let newMax = dragState.startDomain[1] - deltaPrice;
        if (newMin < globalBounds.min) {
          newMin = globalBounds.min;
          newMax = newMin + span;
        }
        if (newMax > globalBounds.max) {
          newMax = globalBounds.max;
          newMin = newMax - span;
        }
        setDomain([newMin, newMax]);
      }
    },
    [dragState, xToPrice, updateSelection, selection,
  chartDimensions.width, globalBounds.min, globalBounds.max]
  );

  useEffect(() => {
    if (!dragState) return undefined;
    const handleMove = (e) => handleDragFromClientX(e.clientX);
    const handleUp = () => setDragState(null);
    window.addEventListener('pointermove', handleMove);
    window.addEventListener('pointerup', handleUp);
    return () => {
      window.removeEventListener('pointermove', handleMove);
      window.removeEventListener('pointerup', handleUp);
    };
  }, [dragState, handleDragFromClientX]);

  const handlePointerDown = useCallback(
    (type, clientX) => {
      if (!overlayRef.current) return;
      const rect = overlayRef.current.getBoundingClientRect();
      const localX = Math.max(0, Math.min(rect.width, clientX
  - rect.left));
      setDragState({
        type,
        startX: localX,
        startMin: selection[0],
        startMax: selection[1],
        startPrice: xToPrice(localX),
        startDomain: domain,
      });
    },
    [selection, xToPrice, domain]
  );

  const handleZoom = useCallback(
    (direction) => {
      // 指数级缩放：每次缩放按照固定比率缩放跨度
      const span = domain[1] - domain[0];
      if (span <= 0) return;
      const center = current || (domain[0] + domain[1]) / 2;
      const scale = direction === 'in' ? 0.7 : 1 / 0.7; // 指数缩放比
      let newSpan = span * scale;
      // 限制最小跨度，避免无限放大
      if (newSpan < MIN_PRICE_FLOOR * 10) newSpan = MIN_PRICE_FLOOR * 10;

      let newMin = center - newSpan / 2;
      let newMax = center + newSpan / 2;

      if (newMin < globalBounds.min) {
        newMin = globalBounds.min;
        newMax = newMin + newSpan;
      }
      if (newMax > globalBounds.max) {
        newMax = globalBounds.max;
        newMin = newMax - newSpan;
      }

      if (newMax > newMin) {
        setDomain([newMin, newMax]);
      }
    },
    [current, domain, globalBounds.min, globalBounds.max]
  );

  const handleReset = useCallback(() => {
    if (!anchorPrice) return;
    // 根据当前价格设置 ±20% 范围
    const defMin = anchorPrice * 0.8;
    const defMax = anchorPrice * 1.2;
    onMinPriceChange?.(String(defMin));
    onMaxPriceChange?.(String(defMax));
    setDomain(
      buildInitialDomain({
        globalMin: viewDomain[0],
        globalMax: viewDomain[1],
      })
    );
  }, [anchorPrice, onMinPriceChange, onMaxPriceChange, viewDomain]);

  const chartData = useMemo(() => {
    if (formattedData.length) return formattedData;
    return [
      { price: domain[0], depth: 0 },
      { price: domain[1], depth: 0 },
    ];
  }, [formattedData, domain]);

  const maxDepth = useMemo(() => {
    if (!chartData.length) return 0;
    return Math.max(...chartData.map((d) => d.depth || 0));
  }, [chartData]);

  const minHandleX = scaleX(selection[0]);
  const maxHandleX = scaleX(selection[1]);

  // 浠呭湪瀹屽叏娌℃湁浼犲叆鏍锋湰鏃跺睍绀虹┖鎬侊紱濡傛灉鍏ュ弬鏈夋暟鎹絾琚繃婊わ紙濡備环鏍间负0锛夛紝浠嶄繚鎸佸彲瑙佷互渚挎帓鏌?
  const emptyState = !formattedData.length && !(depthDataPoints?.length > 0);

  return (
    <div className="w-full select-none">
      <div className="relative w-full" ref={chartContainerRef}>
        <div
          className="absolute top-2 right-2 z-10 flex items-center gap-2 bg-background/80 backdrop-blur-sm border border-border/60 rounded-lg px-2 py-1 shadow"
          aria-label="chart controls"
        >
          <button
            type="button"
            onClick={handleReset}
            className="p-1.5 rounded-md hover:bg-muted transition-colors"
            title="Reset view"
            disabled={!interactive}
          >
            <RefreshCw className="w-4 h-4 text-foreground" />
          </button>
          <button
            type="button"
            onClick={() => handleZoom('in')}
            className="p-1.5 rounded-md hover:bg-muted transition-colors"
            title="Zoom in"
            disabled={!interactive}
          >
            <ZoomIn className="w-4 h-4 text-foreground" />
          </button>
          <button
            type="button"
            onClick={() => handleZoom('out')}
            className="p-1.5 rounded-md hover:bg-muted transition-colors"
            title="Zoom out"
            disabled={!interactive}
          >
            <ZoomOut className="w-4 h-4 text-foreground" />
          </button>
        </div>

        <div className="h-[260px] w-full">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={chartData} margin={{ top: 20, right: 4, left: 4, bottom: 20 }}>
              <defs>
                <linearGradient id="liquidityGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#2B6AFF" stopOpacity={0.4} />
                  <stop offset="100%" stopColor="#2B6AFF" stopOpacity={0.05} />
                </linearGradient>
              </defs>

              <XAxis
                dataKey="price"
                type="number"
                domain={domain}
                allowDataOverflow
                axisLine={false}
                tickLine={false}
                tick={{ fontSize: 10, fill: 'hsl(var(--muted-foreground))' }}
                tickFormatter={formatTick}
                ticks={axisTicks}
              />
              <YAxis
                width={0}
                hide
                domain={[0, maxDepth ? maxDepth * 1.2 : 1]}
                axisLine={false}
                tickLine={false}
              />

              <Area
                type="monotone"
                dataKey="depth"
                stroke="#2B6AFF"
                strokeWidth={2}
                fill="url(#liquidityGradient)"
                isAnimationActive={false}
                activeDot={false}
                connectNulls
              />

              {timeRangeMinDisplay && (
                <ReferenceLine x={timeRangeMinDisplay} stroke="#8C6EEF" strokeDasharray="3 3" strokeWidth={1.25} />
              )}
              {timeRangeMaxDisplay && (
                <ReferenceLine x={timeRangeMaxDisplay} stroke="#8C6EEF" strokeDasharray="3 3" strokeWidth={1.25} />
              )}
              {current && <ReferenceLine x={current} stroke="#10b981" strokeDasharray="3 3" strokeWidth={1.5} />}
            </AreaChart>
          </ResponsiveContainer>
        </div>

        {emptyState && (
          <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-2 text-muted-foreground bg-background/70 backdrop-blur-sm rounded-lg border border-dashed border-border/60">
            <BarChart2 className="w-6 h-6" />
            <p className="text-xs">No liquidity samples yet.</p>
          </div>
        )}

        {interactive && (
          <div ref={overlayRef}
            className="absolute inset-0 pointer-events-none select-none"
            style={{ height: `${chartDimensions.height}px` }}
            >
            {/* 背景层：用于在滑块外侧区域触发拖动 */}
            <div
              className="absolute inset-0 pointer-events-auto cursor-grab active:cursor-grabbing"
              onPointerDown={(e) => {
                if (!overlayRef.current) return;
                const rect = overlayRef.current.getBoundingClientRect();
                const localX = Math.max(0, Math.min(rect.width, e.clientX - rect.left));
                
                // 检查点击位置是否在 min 滑块区域内（滑块宽度 24px，中心对齐）
                const minHandleLeft = minHandleX - 12;
                const minHandleRight = minHandleX + 12;
                const isInMinHandle = localX >= minHandleLeft && localX <= minHandleRight;
                
                // 检查点击位置是否在 max 滑块区域内
                const maxHandleLeft = maxHandleX - 12;
                const maxHandleRight = maxHandleX + 12;
                const isInMaxHandle = localX >= maxHandleLeft && localX <= maxHandleRight;
                
                // 检查点击位置是否在 range 区域内
                const rangeLeft = Math.min(minHandleX, maxHandleX);
                const rangeRight = Math.max(minHandleX, maxHandleX);
                const isInRange = localX >= rangeLeft && localX <= rangeRight;
                
                // 如果点击在滑块或 range 区域外，则触发 pan 拖动
                if (!isInMinHandle && !isInMaxHandle && !isInRange) {
                  e.stopPropagation();
                  handlePointerDown('pan', e.clientX);
                }
              }}
              style={{ zIndex: 0 }}
            />
            
            <div
              className="absolute top-0 bottom-0 pointer-events-auto cursor-ew-resize"
              style={{
                left: `${Math.max(0, Math.min(100, (minHandleX / Math.max(chartDimensions.width, 1)) * 100))}%`,
                width: '24px',
                transform: 'translateX(-50%)',
                zIndex: 10,
              }}
              onPointerDown={(e) => { e.stopPropagation();
                handlePointerDown('min', e.clientX); }}
            >
              <div
                className="absolute -top-6 left-1/2 -translate-x-1/2 text-[10px] px-2 py-0.5 rounded-full shadow text-white"
                style={{ backgroundColor: '#22D1F8' }}
              >
                {percentLabel(selection[0], current)}
              </div>
              <div
                className="absolute top-0 left-1/2 -translate-x-1/2 -translate-y-1/2 w-4 h-4 rounded-full border-2 border-white shadow"
                style={{ backgroundColor: '#22D1F8' }}
              />
            </div>

            <div
              className="absolute top-0 bottom-0 pointer-events-auto cursor-ew-resize"
              style={{
                left: `${Math.max(0, Math.min(100, (maxHandleX / Math.max(chartDimensions.width, 1)) * 100))}%`,
                width: '24px',
                transform: 'translateX(-50%)',
                zIndex: 10,
              }}
              onPointerDown={(e) => { e.stopPropagation();
                handlePointerDown('max', e.clientX); }}
            >
              <div
                className="absolute -top-6 left-1/2 -translate-x-1/2 text-[10px] px-2 py-0.5 rounded-full shadow text-white"
                style={{ backgroundColor: '#FF4EA3' }}
              >
                {percentLabel(selection[1], current)}
              </div>
              <div
                className="absolute top-0 left-1/2 -translate-x-1/2 -translate-y-1/2 w-4 h-4 rounded-full border-2 border-white shadow"
                style={{ backgroundColor: '#FF4EA3' }}
              />
            </div>

            <div
              className="absolute top-0 bottom-0 bg-blue-500/10 hover:bg-blue-500/15 transition-colors pointer-events-auto cursor-grab"
              style={{
                left: `${Math.max(0, Math.min(minHandleX, maxHandleX))}px`,
                width: `${Math.max(4, Math.abs(maxHandleX - minHandleX))}px`,
                zIndex: 5,
              }}
              onPointerDown={(e) => { e.stopPropagation();
                handlePointerDown('range', e.clientX); }}
            />
          </div>
        )}
      </div>
      <div className="mt-3 flex items-center justify-between text-xs text-muted-foreground">
        <span>
          Range: {token0Symbol || 'Token0'} / {token1Symbol || 'Token1'}
        </span>
        <span>
          View {formatTick(domain[0])} - {formatTick(domain[1])}
        </span>
      </div>
    </div>
  );
};

export default ClmmLiquidityChart;




