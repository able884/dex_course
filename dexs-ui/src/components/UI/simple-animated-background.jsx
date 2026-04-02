import React from 'react';

const SimpleAnimatedBackground = ({ activeTab = 'dashboard' }) => {
  // 根据页面调整背景强度
  const getIntensity = () => {
    switch (activeTab) {
      case 'dashboard':
        return 0.06;
      case 'chart':
        return 0.03;
      case 'trenches':
        return 0.08;
      default:
        return 0.04;
    }
  };

  const intensity = getIntensity();

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
            width: '500px',
            height: '500px',
            background: `radial-gradient(circle, rgba(20, 251, 175, ${intensity}) 0%, transparent 70%)`,
            filter: 'blur(100px)',
            top: '10%',
            left: '10%',
          }}
        />
        
        {/* 中型光球 - Solana 紫色 */}
        <div 
          className="absolute rounded-full animate-float-reverse"
          style={{
            width: '350px',
            height: '350px',
            background: `radial-gradient(circle, rgba(147, 51, 234, ${intensity}) 0%, transparent 70%)`,
            filter: 'blur(80px)',
            top: '60%',
            right: '15%',
          }}
        />
        
        {/* 小型光球 - 蓝色 */}
        <div 
          className="absolute rounded-full animate-float-diagonal"
          style={{
            width: '250px',
            height: '250px',
            background: `radial-gradient(circle, rgba(59, 130, 246, ${intensity * 0.8}) 0%, transparent 70%)`,
            filter: 'blur(60px)',
            bottom: '20%',
            left: '20%',
          }}
        />
        
        {/* 额外的装饰光点 */}
        <div 
          className="absolute rounded-full animate-pulse-slow"
          style={{
            width: '150px',
            height: '150px',
            background: `radial-gradient(circle, rgba(20, 251, 175, ${intensity * 0.6}) 0%, transparent 60%)`,
            filter: 'blur(40px)',
            top: '30%',
            right: '30%',
          }}
        />
        
        <div 
          className="absolute rounded-full animate-pulse-slow"
          style={{
            width: '120px',
            height: '120px',
            background: `radial-gradient(circle, rgba(147, 51, 234, ${intensity * 0.5}) 0%, transparent 60%)`,
            filter: 'blur(30px)',
            bottom: '40%',
            right: '40%',
            animationDelay: '3s'
          }}
        />
      </div>
      
      {/* Dashboard 页面的波纹效果 */}
      {activeTab === 'dashboard' && (
        <div className="absolute inset-0">
          {[...Array(3)].map((_, i) => (
            <div
              key={i}
              className="absolute rounded-full border animate-ping"
              style={{
                width: `${300 + i * 150}px`,
                height: `${300 + i * 150}px`,
                borderColor: `rgba(20, 251, 175, ${intensity * 0.3})`,
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

      {/* Chart 页面的网格效果 */}
      {activeTab === 'chart' && (
        <div 
          className="absolute inset-0"
          style={{
            opacity: intensity * 0.5,
            backgroundImage: `
              linear-gradient(rgba(20, 251, 175, 0.1) 1px, transparent 1px),
              linear-gradient(90deg, rgba(20, 251, 175, 0.1) 1px, transparent 1px)
            `,
            backgroundSize: '50px 50px',
            animation: 'grid-move 20s linear infinite'
          }}
        />
      )}

      {/* Trenches 页面的粒子效果 */}
      {activeTab === 'trenches' && (
        <div className="absolute inset-0">
          {[...Array(12)].map((_, i) => (
            <div
              key={i}
              className="absolute rounded-full animate-float-particle"
              style={{
                width: `${2 + Math.random() * 3}px`,
                height: `${2 + Math.random() * 3}px`,
                background: `rgba(20, 251, 175, ${intensity * 0.8})`,
                left: `${Math.random() * 100}%`,
                top: `${Math.random() * 100}%`,
                animationDelay: `${Math.random() * 20}s`,
                animationDuration: `${15 + Math.random() * 10}s`,
                boxShadow: `0 0 ${4 + Math.random() * 6}px rgba(20, 251, 175, 0.5)`
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
};

export default SimpleAnimatedBackground;