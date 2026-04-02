import React, { useEffect, useRef, useState } from 'react';
import { motion } from 'framer-motion';

const OptimizedDynamicBackground = ({
  particleCount = 30,
  enableParticles = true,
  enableOrbs = true,
  className = ""
}) => {
  const canvasRef = useRef(null);
  const animationRef = useRef(null);
  const particlesRef = useRef([]);
  const [isDark, setIsDark] = useState(false);

  console.log('OptimizedDynamicBackground is rendering, isDark:', isDark);

  // Solana colors
  const colors = {
    primary: '#14F195', // Solana green
    secondary: '#9945FF', // Solana purple
    accent: '#00D4AA',
    particles: ['#14F195', '#9945FF', '#00D4AA']
  };

  // Initialize particles
  const initParticles = () => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    particlesRef.current = Array.from({ length: particleCount }, (_, i) => ({
      id: i,
      x: Math.random() * canvas.width,
      y: Math.random() * canvas.height,
      vx: (Math.random() - 0.5) * 0.5,
      vy: (Math.random() - 0.5) * 0.5,
      size: Math.random() * 3 + 1,
      opacity: Math.random() * 0.3 + 0.1, // 降低粒子透明度
      color: colors.particles[Math.floor(Math.random() * colors.particles.length)]
    }));
  };

  // Update particles
  const updateParticles = () => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    particlesRef.current.forEach(particle => {
      particle.x += particle.vx;
      particle.y += particle.vy;

      // Bounce off edges
      if (particle.x <= 0 || particle.x >= canvas.width) particle.vx *= -1;
      if (particle.y <= 0 || particle.y >= canvas.height) particle.vy *= -1;

      // Keep particles in bounds
      particle.x = Math.max(0, Math.min(canvas.width, particle.x));
      particle.y = Math.max(0, Math.min(canvas.height, particle.y));

      // Subtle opacity animation
      particle.opacity += (Math.random() - 0.5) * 0.005;
      particle.opacity = Math.max(0.05, Math.min(0.3, particle.opacity));
    });
  };

  // Draw particles
  const drawParticles = (ctx) => {
    particlesRef.current.forEach(particle => {
      ctx.save();
      ctx.globalAlpha = particle.opacity;
      ctx.fillStyle = particle.color;
      ctx.beginPath();
      ctx.arc(particle.x, particle.y, particle.size, 0, Math.PI * 2);
      ctx.fill();
      
      // Add subtle glow effect
      ctx.shadowBlur = 8;
      ctx.shadowColor = particle.color;
      ctx.fill();
      ctx.restore();
    });
  };

  // Animation loop
  const animate = () => {
    const canvas = canvasRef.current;
    const ctx = canvas?.getContext('2d');
    if (!canvas || !ctx) return;

    // Clear canvas
    ctx.clearRect(0, 0, canvas.width, canvas.height);

    if (enableParticles) {
      updateParticles();
      drawParticles(ctx);
    }

    animationRef.current = requestAnimationFrame(animate);
  };

  // Resize canvas
  const resizeCanvas = () => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;
  };

  // Theme detection
  useEffect(() => {
    const checkTheme = () => {
      const isDarkMode = document.documentElement.classList.contains('dark') ||
                        window.matchMedia('(prefers-color-scheme: dark)').matches;
      setIsDark(isDarkMode);
      console.log('Theme detected:', isDarkMode ? 'dark' : 'light');
    };

    checkTheme();
    
    const observer = new MutationObserver(checkTheme);
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
    
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    mediaQuery.addEventListener('change', checkTheme);

    return () => {
      observer.disconnect();
      mediaQuery.removeEventListener('change', checkTheme);
    };
  }, []);

  // Initialize and start animation
  useEffect(() => {
    resizeCanvas();
    initParticles();
    animate();

    const handleResize = () => {
      resizeCanvas();
      initParticles();
    };

    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [particleCount, enableParticles]);

  return (
    <div 
      className={`fixed inset-0 pointer-events-none overflow-hidden ${className}`} 
      style={{ 
        zIndex: 5,
        // 使用更低透明度的背景渐变
        background: isDark 
          ? 'linear-gradient(135deg, rgba(15, 15, 35, 0.4) 0%, rgba(26, 26, 46, 0.4) 50%, rgba(22, 33, 62, 0.4) 100%)'
          : 'linear-gradient(135deg, rgba(226, 232, 240, 0.3) 0%, rgba(203, 213, 225, 0.3) 50%, rgba(148, 163, 184, 0.3) 100%)'
      }}
    >
      {/* 更低透明度的径向渐变背景 */}
      <div 
        className="absolute inset-0"
        style={{
          background: isDark 
            ? `radial-gradient(circle at 20% 80%, rgba(20, 241, 149, 0.02) 0%, transparent 50%), 
               radial-gradient(circle at 80% 20%, rgba(153, 69, 255, 0.015) 0%, transparent 50%), 
               radial-gradient(circle at 40% 40%, rgba(0, 212, 170, 0.01) 0%, transparent 50%)`
            : `radial-gradient(circle at 20% 80%, rgba(20, 241, 149, 0.1) 0%, transparent 50%), 
               radial-gradient(circle at 80% 20%, rgba(153, 69, 255, 0.08) 0%, transparent 50%), 
               radial-gradient(circle at 40% 40%, rgba(0, 212, 170, 0.06) 0%, transparent 50%)`
        }}
      />

      {/* 更低透明度的光球 */}
      {enableOrbs && (
        <>
          <motion.div
            className="absolute w-80 h-80 rounded-full blur-3xl pointer-events-none"
            style={{ 
              backgroundColor: colors.primary,
              opacity: isDark ? 0.03 : 0.08,
              top: '10%',
              left: '10%'
            }}
            animate={{
              x: [0, 100, 0],
              y: [0, -50, 0],
              scale: [1, 1.2, 1],
            }}
            transition={{
              duration: 20,
              repeat: Infinity,
              ease: "easeInOut"
            }}
          />
          
          <motion.div
            className="absolute w-72 h-72 rounded-full blur-3xl pointer-events-none"
            style={{ 
              backgroundColor: colors.secondary,
              opacity: isDark ? 0.025 : 0.06,
              top: '60%',
              right: '10%'
            }}
            animate={{
              x: [0, -80, 0],
              y: [0, 60, 0],
              scale: [1, 0.8, 1],
            }}
            transition={{
              duration: 25,
              repeat: Infinity,
              ease: "easeInOut"
            }}
          />
          
          <motion.div
            className="absolute w-64 h-64 rounded-full blur-2xl pointer-events-none"
            style={{ 
              backgroundColor: colors.accent,
              opacity: isDark ? 0.02 : 0.05,
              bottom: '20%',
              left: '50%'
            }}
            animate={{
              x: [0, 60, 0],
              y: [0, -40, 0],
              scale: [1, 1.1, 1],
            }}
            transition={{
              duration: 18,
              repeat: Infinity,
              ease: "easeInOut"
            }}
          />
        </>
      )}

      {/* Particle Canvas */}
      {enableParticles && (
        <canvas
          ref={canvasRef}
          className="absolute inset-0 pointer-events-none"
          style={{ 
            mixBlendMode: 'normal',
            opacity: 0.4 // 降低粒子画布透明度
          }}
        />
      )}
    </div>
  );
};

export default OptimizedDynamicBackground;