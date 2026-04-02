import React, { useEffect, useState } from 'react';
import PoolRow from './PoolRow';
import { getPools } from '../../api/pools';

const headers = [
  'Pool', 'Liquidity', 'Volume 24H', 'Fees 24H', 'APR 24H', 'Operation'
];

const PoolsTable = ({ version, refreshKey = 0, onNavigateCharts, onNavigateSwap, onNavigateAddLiquidity, onLoadingChange, onDataUpdate }) => {
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    let alive = true;
    async function load() {
      setLoading(true);
      setError(null);
      try {
        const data = await getPools(version);
        if (!alive) return;
        setItems(Array.isArray(data?.items) ? data.items : []);
        if (typeof onDataUpdate === 'function') {
          onDataUpdate(new Date());
        }
      } catch (e) {
        if (!alive) return;
        setError(e);
        setItems([]);
      } finally {
        if (alive) setLoading(false);
      }
    }
    load();
    return () => { alive = false; };
  }, [version, refreshKey]);

  useEffect(() => {
    if (typeof onLoadingChange === 'function') {
      onLoadingChange(loading);
    }
  }, [loading, onLoadingChange]);

  return (
    <div className="w-full overflow-x-auto border border-border/40 rounded-md">
      <table className="w-full text-sm">
        <thead className="bg-muted/40">
          <tr>
            {headers.map((h) => (
              <th
                key={h}
                className={
                  h === 'Pool' || h === 'Fees 24H'
                    ? 'text-left px-3 py-2 font-medium text-foreground/80'
                    : 'text-right px-3 py-2 font-medium text-foreground/80'
                }
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {loading && (
            Array.from({ length: 5 }).map((_, i) => (
              <tr key={`skeleton-${i}`} className="animate-pulse">
                {headers.map((_, j) => (
                  <td key={j} className="px-3 py-3">
                    <div className="h-3 w-full bg-muted rounded" />
                  </td>
                ))}
              </tr>
            ))
          )}
          {error && !loading && (
            <tr>
              <td colSpan={headers.length} className="px-3 py-6 text-center text-red-500">Failed to load pools. Please try again.</td>
            </tr>
          )}
          {!loading && !error && items.length === 0 && (
            <tr>
              <td colSpan={headers.length} className="px-3 py-6 text-center text-muted-foreground">No pools to display.</td>
            </tr>
          )}
          {!loading && !error && items.map((pool) => (
            <PoolRow key={pool.poolState}
                     pool={pool}
                     onCharts={onNavigateCharts}
                     onSwap={onNavigateSwap}
                     onDeposit={onNavigateAddLiquidity}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default PoolsTable;
