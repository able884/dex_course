import React from 'react';

const AnimatedBackground = ({ variant = 'default', intensity = 'medium' }) => {
  // 根据强度调整透明度和动画速度
  const intensityConfig = {
    low: { opacity: 0.03, speed: '60s', blur: '120px' },
    medium: { opacity: 0.05, speed: '45s', blur: '100px' },
    high: { opacity: 0.08, speed: '30s', blur: '80px' }
  };

  const config = intensityConfig[intensity] || intensityConfig.medium;

  return (
    <div className="fixed inset-0 -z-10 overflow-hidden pointer-events-none">
      {/* 主要渐变背景 */}
      <div className="absolute inset-0 bg-gradient-to-br from-background via-background to-background/95" />
      
      {/* 动态光球效果 */}
      <div className="absolute inset-0">
        {/* 大型光球 - Solana 绿色 */}
        <div 
          className="absolute rounded-full animate-float-slow"
          style={{
            width: '600px',
            height: '600px',
            background: `radial-gradient(circle, rgba(20, 251, 175, ${config.opacity}) 0%, transparent 70%)`,
            filter: `blur(${config.blur})`,
            top: '10%',
            left: '10%',
            animationDuration: config.speed,
            animationDelay: '0s'
          }}
        />
        
        {/* 中型光球 - Solana 紫色 */}
        <div 
          className="absolute rounded-full animate-float-reverse"
          style={{
            width: '400px',
            height: '400px',
            background: `radial-gradient(circle, rgba(147, 51, 234, ${config.opacity}) 0%, transparent 70%)`,
            filter: `blur(${config.blur})`,
            top: '60%',
            right: '15%',
            animationDuration: config.speed,
            animationDelay: '-15s'
          }}
        />
        
        {/* 小型光球 - 蓝色 */}
        <div 
          className="absolute rounded-full animate-float-diagonal"
          style={{
            width: '300px',
            height: '300px',
            background: `radial-gradient(circle, rgba(59, 130, 246, ${config.opacity}) 0%, transparent 70%)`,
            filter: `blur(${config.blur})`,
            bottom: '20%',
            left: '20%',
            animationDuration: config.speed,
            animationDelay: '-30s'
          }}
        />
        
        {/* 额外的小光点 */}
        <div 
          className="absolute rounded-full animate-pulse-slow"
          style={{
            width: '200px',
            height: '200px',
            background: `radial-gradient(circle, rgba(20, 251, 175, ${config.opacity * 0.8}) 0%, transparent 60%)`,
            filter: `blur(60px)`,
            top: '30%',
            right: '30%',
            animationDuration: '8s'
          }}
        />
        
        <div 
          className="absolute rounded-full animate-pulse-slow"
          style={{
            width: '150px',
            height: '150px',
            background: `radial-gradient(circle, rgba(147, 51, 234, ${config.opacity * 0.6}) 0%, transparent 60%)`,
            filter: `blur(40px)`,
            bottom: '40%',
            right: '40%',
            animationDuration: '12s',
            animationDelay: '-6s'
          }}
        />
      </div>
      
      {/* 网格效果 - 可选 */}
      {variant === 'grid' && (
        <div 
          className="absolute inset-0 opacity-[0.02]"
          style={{
            backgroundImage: `
              linear-gradient(rgba(20, 251, 175, 0.1) 1px, transparent 1px),
              linear-gradient(90deg, rgba(20, 251, 175, 0.1) 1px, transparent 1px)
            `,
            backgroundSize: '50px 50px',
            animation: 'grid-move 20s linear infinite'
          }}
        />
      )}
      
      {/* 粒子效果 - 可选 */}
      {variant === 'particles' && (
        <div className="absolute inset-0">
          {[...Array(20)].map((_, i) => (
            <div
              key={i}
              className="absolute w-1 h-1 bg-solana/20 rounded-full animate-float-particle"
              style={{
                left: `${Math.random() * 100}%`,
                top: `${Math.random() * 100}%`,
                animationDelay: `${Math.random() * 20}s`,
                animationDuration: `${15 + Math.random() * 10}s`
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
};

export default AnimatedBackground;