const USD = new Intl.NumberFormat('en-US', {
  style: 'currency',
  currency: 'USD',
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

export function formatCurrencyUSD(n) {
  if (n === null || n === undefined) return '—';
  const val = Number(n);
  if (!Number.isFinite(val)) return '—';
  if (val > 0 && val < 0.01) return '<$0.01';
  return USD.format(val);
}

export function formatPercent(n) {
  if (n === null || n === undefined) return '—';
  const val = Number(n);
  if (!Number.isFinite(val)) return '—';
  if (val > 0 && val < 0.01) return '<0.01%';
  return `${val.toFixed(2)}%`;
}

