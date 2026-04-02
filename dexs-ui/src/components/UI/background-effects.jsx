import React from 'react';

// 专门为不同页面设计的背景效果组件
export const DashboardBackground = () => (
  <div className="absolute inset-0 overflow-hidden pointer-events-none">
    {/* 仪表板专用的径向渐变 */}
    <div className="absolute inset-0 bg-gradient-radial from-solana/5 via-transparent to-transparent" />
    
    {/* 脉冲圆环 */}
    <div className="absolute top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2">
      {[...Array(4)].map((_, i) => (
        <div
          key={i}
          className="absolute border border-solana/10 rounded-full animate-ping"
          style={{
            width: `${200 + i * 150}px`,
            height: `${200 + i * 150}px`,
            animationDuration: `${3 + i}s`,
            animationDelay: `${i * 0.5}s`,
            top: '50%',
            left: '50%',
            transform: 'translate(-50%, -50%)'
          }}
        />
      ))}
    </div>
  </div>
);

export const ChartBackground = () => (
  <div className="absolute inset-0 overflow-hidden pointer-events-none">
    {/* 图表页面的网格背景 */}
    <div 
      className="absolute inset-0 opacity-[0.02]"
      style={{
        backgroundImage: `
          linear-gradient(rgba(20, 251, 175, 0.1) 1px, transparent 1px),
          linear-gradient(90deg, rgba(20, 251, 175, 0.1) 1px, transparent 1px)
        `,
        backgroundSize: '40px 40px',
        animation: 'grid-drift 25s linear infinite'
      }}
    />
    
    {/* 数据流效果 */}
    <div className="absolute inset-0">
      {[...Array(6)].map((_, i) => (
        <div
          key={i}
          className="absolute w-px bg-gradient-to-b from-transparent via-solana/20 to-transparent animate-data-flow"
          style={{
            height: '100%',
            left: `${15 + i * 15}%`,
            animationDelay: `${i * 2}s`,
            animationDuration: '8s'
          }}
        />
      ))}
    </div>
  </div>
);

export const TrenchesBackground = () => (
  <div className="absolute inset-0 overflow-hidden pointer-events-none">
    {/* 高能量粒子效果 */}
    <div className="absolute inset-0">
      {[...Array(25)].map((_, i) => (
        <div
          key={i}
          className="absolute rounded-full animate-particle-burst"
          style={{
            width: `${2 + Math.random() * 4}px`,
            height: `${2 + Math.random() * 4}px`,
            background: `rgba(20, 251, 175, ${0.3 + Math.random() * 0.4})`,
            left: `${Math.random() * 100}%`,
            top: `${Math.random() * 100}%`,
            animationDelay: `${Math.random() * 10}s`,
            animationDuration: `${8 + Math.random() * 6}s`,
            boxShadow: `0 0 ${6 + Math.random() * 8}px rgba(20, 251, 175, 0.6)`
          }}
        />
      ))}
    </div>
    
    {/* 能量波纹 */}
    <div className="absolute inset-0">
      {[...Array(3)].map((_, i) => (
        <div
          key={i}
          className="absolute rounded-full border-2 border-solana/20 animate-energy-wave"
          style={{
            width: `${300 + i * 200}px`,
            height: `${300 + i * 200}px`,
            top: `${20 + i * 10}%`,
            left: `${10 + i * 20}%`,
            animationDelay: `${i * 2}s`,
            animationDuration: '6s'
          }}
        />
      ))}
    </div>
  </div>
);

// 通用背景效果选择器
export const BackgroundEffects = ({ activeTab }) => {
  switch (activeTab) {
    case 'dashboard':
      return <DashboardBackground />;
    case 'chart':
      return <ChartBackground />;
    case 'trenches':
      return <TrenchesBackground />;
    default:
      return null;
  }
};