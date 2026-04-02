import React, { useEffect, useState } from 'react';

const Watermark = ({ text = '仅供学习使用', opacity = 0.05 }) => {
  const [dimensions, setDimensions] = useState({
    width: typeof window !== 'undefined' ? window.innerWidth : 1920,
    height: typeof window !== 'undefined' ? window.innerHeight : 1080
  });

  useEffect(() => {
    const handleResize = () => {
      setDimensions({
        width: window.innerWidth,
        height: window.innerHeight
      });
    };

    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  // 生成水印样式
  const watermarkStyle = {
    position: 'fixed',
    top: 0,
    left: 0,
    width: '100vw',
    height: '100vh',
    pointerEvents: 'none',
    zIndex: 1000,
    overflow: 'hidden',
    opacity: opacity,
  };

  // 生成重复的水印文字
  const generateWatermarkPattern = () => {
    const watermarks = [];
    const spacing = 200; // 水印之间的间距
    const rows = Math.ceil(dimensions.height / spacing) + 4;
    const cols = Math.ceil(dimensions.width / spacing) + 4;

    for (let row = 0; row < rows; row++) {
      for (let col = 0; col < cols; col++) {
        const x = col * spacing - spacing;
        const y = row * spacing - spacing;
        
        watermarks.push(
          <div
            key={`${row}-${col}`}
            className="absolute select-none"
            style={{
              left: x,
              top: y,
              transform: 'rotate(-45deg)',
              transformOrigin: 'center',
              fontSize: '16px',
              fontWeight: '500',
              color: 'currentColor',
              whiteSpace: 'nowrap',
              userSelect: 'none',
            }}
          >
            {text}
          </div>
        );
      }
    }
    
    return watermarks;
  };

  return (
    <div style={watermarkStyle} className="text-gray-600 dark:text-gray-400">
      {generateWatermarkPattern()}
    </div>
  );
};

export default Watermark;
