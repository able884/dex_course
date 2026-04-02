import React, { useState } from 'react';

function Fallback({ symbol }) {
  const initials = (symbol || '?').slice(0, 2).toUpperCase();
  return (
    <div
      className="flex items-center justify-center rounded-full bg-card text-foreground/80 text-xs font-medium border-2"
      style={{ width: 24, height: 24 }}
    >
      {initials}
    </div>
  );
}

function TokenIcon({ src, symbol, className }) {
  const [error, setError] = useState(false);
  if (!src || error) return <Fallback symbol={symbol} />;
  return (
    <img
      src={src}
      alt={symbol || 'Token'}
      className={`rounded-full object-cover border-2 bg-card ${className || ''}`}
      style={{ width: 24, height: 24 }}
      onError={() => setError(true)}
    />
  );
}

const TokenPairAvatar = ({ tokenA, tokenB }) => {
  return (
    <div className="relative" style={{ width: 40, height: 24 }}>
      <div className="absolute left-0 top-0">
        <TokenIcon src={tokenA?.icon} symbol={tokenA?.symbol} />
      </div>
      <div className="absolute" style={{ left: 16, top: 0 }}>
        <TokenIcon src={tokenB?.icon} symbol={tokenB?.symbol} />
      </div>
    </div>
  );
};

export default TokenPairAvatar;
