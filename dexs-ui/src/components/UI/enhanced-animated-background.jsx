import React, { useEffect, useState } from 'react';

const EnhancedAnimatedBackground = ({ 
  variant = 'default', 
  intensity = 'medium',
  theme = 'solana',
  activeTab = 'dashboard'
}) => {
  const [mousePosition, setMousePosition] = useState({ x: 0, y: 0 });

  // 鼠标跟踪效果
  useEffect(() => {
    const handleMouseMove = (e) => {
      setMousePosition({
        x: (e.clientX / window.innerWidth) * 100,
        y: (e.clientY / window.innerHeight) * 100
      });
    };

    window.addEventListener('mousemove', handleMouseMove);
    return () => window.removeEventListener('mousemove', handleMouseMove);
  }, []);

  // 根据强度调整透明度和动画速度
  const intensityConfig = {
    low: { opacity: 0.02, speed: '60s', blur: '120px', scale: 0.8 },
    medium: { opacity: 0.04, speed: '45s', blur: '100px', scale: 1 },
    high: { opacity: 0.06, speed: '30s', blur: '80px', scale: 1.2 }
  };

  // 主题颜色配置
  const themeColors = {
    solana: {
      primary: 'rgba(20, 251, 175, {opacity})',
      secondary: 'rgba(147, 51, 234, {opacity})',
      accent: 'rgba(59, 130, 246, {opacity})',
      highlight: 'rgba(16, 185, 129, {opacity})'
    },
    dark: {
      primary: 'rgba(99, 102, 241, {opacity})',
      secondary: 'rgba(139, 92, 246, {opacity})',
      accent: 'rgba(59, 130, 246, {opacity})',
      highlight: 'rgba(168, 85, 247, {opacity})'
    }
  };

  const config = intensityConfig[intensity] || intensityConfig.medium;
  const colors = themeColors[theme] || themeColors.solana;

  // 根据页面调整背景样式
  const getPageSpecificStyle = () => {
    switch (activeTab) {
      case 'dashboard':
        return { intensity: 'medium', variant: 'default' };
      case 'chart':
        return { intensity: 'low', variant: 'grid' };
      case 'trenches':
        return { intensity: 'high', variant: 'particles' };
      default:
        return { intensity: 'low', variant: 'default' };
    }
  };

  const pageStyle = getPageSpecificStyle();
  const finalConfig = intensityConfig[pageStyle.intensity];

  return (
    <div className="fixed inset-0 -z-10 overflow-hidden pointer-events-none">
      {/* 主要渐变背景 */}
      <div className="absolute inset-0 bg-gradient-to-br from-background via-background to-background/95" />
      
      {/* 鼠标跟随光效 */}
      <div 
        className="absolute w-96 h-96 rounded-full transition-all duration-1000 ease-out"
        style={{
          background: `radial-gradient(circle, ${colors.primary.replace('{opacity}', finalConfig.opacity * 0.5)} 0%, transparent 70%)`,
          filter: `blur(60px)`,
          left: `${mousePosition.x}%`,
          top: `${mousePosition.y}%`,
          transform: 'translate(-50%, -50%)',
        }}
      />
      
      {/* 动态光球效果 */}
      <div className="absolute inset-0">
        {/* 大型光球 - 主色 */}
        <div 
          className="absolute rounded-full animate-float-slow"
          style={{
            width: `${600 * finalConfig.scale}px`,
            height: `${600 * finalConfig.scale}px`,
            background: `radial-gradient(circle, ${colors.primary.replace('{opacity}', finalConfig.opacity)} 0%, transparent 70%)`,
            filter: `blur(${finalConfig.blur})`,
            top: '10%',
            left: '10%',
            animationDuration: finalConfig.speed,
            animationDelay: '0s'
          }}
        />
        
        {/* 中型光球 - 次色 */}
        <div 
          className="absolute rounded-full animate-float-reverse"
          style={{
            width: `${400 * finalConfig.scale}px`,
            height: `${400 * finalConfig.scale}px`,
            background: `radial-gradient(circle, ${colors.secondary.replace('{opacity}', finalConfig.opacity)} 0%, transparent 70%)`,
            filter: `blur(${finalConfig.blur})`,
            top: '60%',
            right: '15%',
            animationDuration: finalConfig.speed,
            animationDelay: '-15s'
          }}
        />
        
        {/* 小型光球 - 强调色 */}
        <div 
          className="absolute rounded-full animate-float-diagonal"
          style={{
            width: `${300 * finalConfig.scale}px`,
            height: `${300 * finalConfig.scale}px`,
            background: `radial-gradient(circle, ${colors.accent.replace('{opacity}', finalConfig.opacity)} 0%, transparent 70%)`,
            filter: `blur(${finalConfig.blur})`,
            bottom: '20%',
            left: '20%',
            animationDuration: finalConfig.speed,
            animationDelay: '-30s'
          }}
        />
        
        {/* 额外的装饰光点 */}
        {[...Array(3)].map((_, i) => (
          <div 
            key={i}
            className="absolute rounded-full animate-pulse-slow"
            style={{
              width: `${(200 - i * 50) * finalConfig.scale}px`,
              height: `${(200 - i * 50) * finalConfig.scale}px`,
              background: `radial-gradient(circle, ${colors.highlight.replace('{opacity}', finalConfig.opacity * (0.8 - i * 0.2))} 0%, transparent 60%)`,
              filter: `blur(${60 - i * 20}px)`,
              top: `${30 + i * 20}%`,
              right: `${30 + i * 10}%`,
              animationDuration: `${8 + i * 4}s`,
              animationDelay: `${-i * 3}s`
            }}
          />
        ))}
      </div>
      
      {/* 网格效果 - 图表页面 */}
      {(pageStyle.variant === 'grid' || variant === 'grid') && (
        <div 
          className="absolute inset-0"
          style={{
            opacity: finalConfig.opacity * 0.5,
            backgroundImage: `
              linear-gradient(${colors.primary.replace('{opacity}', '0.1')} 1px, transparent 1px),
              linear-gradient(90deg, ${colors.primary.replace('{opacity}', '0.1')} 1px, transparent 1px)
            `,
            backgroundSize: '50px 50px',
            animation: 'grid-move 20s linear infinite'
          }}
        />
      )}
      
      {/* 粒子效果 - Trenches 页面 */}
      {(pageStyle.variant === 'particles' || variant === 'particles') && (
        <div className="absolute inset-0">
          {[...Array(15)].map((_, i) => (
            <div
              key={i}
              className="absolute rounded-full animate-float-particle"
              style={{
                width: `${2 + Math.random() * 3}px`,
                height: `${2 + Math.random() * 3}px`,
                background: colors.primary.replace('{opacity}', '0.3'),
                left: `${Math.random() * 100}%`,
                top: `${Math.random() * 100}%`,
                animationDelay: `${Math.random() * 20}s`,
                animationDuration: `${15 + Math.random() * 10}s`,
                boxShadow: `0 0 ${4 + Math.random() * 6}px ${colors.primary.replace('{opacity}', '0.5')}`
              }}
            />
          ))}
        </div>
      )}

      {/* 波纹效果 - Dashboard 页面 */}
      {activeTab === 'dashboard' && (
        <div className="absolute inset-0">
          {[...Array(3)].map((_, i) => (
            <div
              key={i}
              className="absolute rounded-full border animate-ping"
              style={{
                width: `${300 + i * 200}px`,
                height: `${300 + i * 200}px`,
                borderColor: colors.primary.replace('{opacity}', finalConfig.opacity * 0.3),
                borderWidth: '1px',
                top: '50%',
                left: '50%',
                transform: 'translate(-50%, -50%)',
                animationDuration: `${4 + i * 2}s`,
                animationDelay: `${i * 1}s`
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
};

export default EnhancedAnimatedBackground;