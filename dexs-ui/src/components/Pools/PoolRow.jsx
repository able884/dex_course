import React from 'react';
import TokenPairAvatar from './TokenPairAvatar';
import { Button } from '../UI/Button';
import { formatCurrencyUSD, formatPercent } from '../../utils/format';

// Left-aligned fee badge
const FeeChip = ({ value }) => (
  <span className="inline-flex items-center justify-start self-start text-left px-2 py-0.5 rounded bg-muted text-xs text-foreground/80">
    {typeof value === 'number' ? `${value.toFixed(2)}%` : '—'}
  </span>
);

function normalizeFeePercent(tradeFeeRate) {
  const n = Number(tradeFeeRate);
  if (!Number.isFinite(n) || n <= 0) return 0;
  // Raydium fee rate is in hundredths of a bip (1e-6)
  return n / 10000;
}

const PoolRow = ({ pool, onCharts, onSwap, onDeposit }) => {
  const tokenA = { symbol: pool?.inputTokenSymbol, icon: pool?.inputTokenIcon };
  const tokenB = { symbol: pool?.outputTokenSymbol, icon: pool?.outputTokenIcon };
  const feePercent = normalizeFeePercent(pool?.tradeFeeRate);
  const fees24hUSD = (Number(pool?.vol24h) || 0) * (feePercent / 100);
  return (
    <tr className="border-b border-border/40">
      <td className="px-3 py-2">
        <div className="flex items-center gap-3">
          <TokenPairAvatar tokenA={tokenA} tokenB={tokenB} />
          <div className="flex flex-col items-start">
            <div className="text-sm font-medium">{tokenA?.symbol || '?'}-{tokenB?.symbol || '?'}</div>
            <div>
              <FeeChip value={feePercent} />
            </div>
          </div>
        </div>
      </td>
      <td className="px-3 py-2 text-right text-sm">{formatCurrencyUSD(pool?.liquidityUsd)}</td>
      <td className="px-3 py-2 text-right text-sm">{formatCurrencyUSD(pool?.vol24h)}</td>
      <td className="px-3 py-2 text-left text-sm">{formatCurrencyUSD(fees24hUSD)}</td>
      <td className="px-3 py-2 text-right text-sm">{formatPercent(pool?.apr)}</td>
      <td className="px-3 py-2 text-right text-sm">
        <div className="flex items-center justify-end gap-2">
          <Button size="sm" variant="secondary" onClick={() => onCharts && onCharts(pool)}>Charts</Button>
          <Button size="sm" variant="secondary" onClick={() => onSwap && onSwap(tokenA, tokenB)}>Swap</Button>
          <Button size="sm" onClick={() => onDeposit && onDeposit(pool)}>Deposit</Button>
        </div>
      </td>
    </tr>
  );
};

export default PoolRow;

