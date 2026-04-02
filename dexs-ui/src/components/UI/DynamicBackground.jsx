"use client"

import React, { useEffect, useRef, useState } from 'react'
import { motion } from 'framer-motion'

const DynamicBackground = ({
  children,
  particleCount = 50,
  enableParticles = true,
  enableOrbs = true,
  className = ""
}) => {
  const canvasRef = useRef(null)
  const animationRef = useRef(null)
  const particlesRef = useRef([])
  const [isDark, setIsDark] = useState(false)
  const lastFrameTimeRef = useRef(0)
  const isScrollingRef = useRef(false)
  const scrollTimeoutRef = useRef(null)

  // Solana colors - 增加透明度和可见性
  const colors = {
    light: {
      primary: '#14F195', // Solana green
      secondary: '#9945FF', // Solana purple
      accent: '#00D4AA',
      background: '#ffffff',
      particles: ['#14F195', '#9945FF', '#00D4AA']
    },
    dark: {
      primary: '#14F195',
      secondary: '#9945FF', 
      accent: '#00D4AA',
      background: '#0a0a0a',
      particles: ['#14F195', '#9945FF', '#00D4AA']
    }
  }

  const currentTheme = isDark ? colors.dark : colors.light

  // Initialize particles
  const initParticles = () => {
    const canvas = canvasRef.current
    if (!canvas) return

    particlesRef.current = Array.from({ length: particleCount }, (_, i) => ({
      id: i,
      x: Math.random() * canvas.width,
      y: Math.random() * canvas.height,
      vx: (Math.random() - 0.5) * 0.3,
      vy: (Math.random() - 0.5) * 0.3,
      size: Math.random() * 3 + 1,
      opacity: Math.random() * 0.6 + 0.4,
      color: currentTheme.particles[Math.floor(Math.random() * currentTheme.particles.length)]
    }))
  }

  // Update particles
  const updateParticles = () => {
    const canvas = canvasRef.current
    if (!canvas) return

    particlesRef.current.forEach(particle => {
      particle.x += particle.vx
      particle.y += particle.vy

      // Bounce off edges
      if (particle.x <= 0 || particle.x >= canvas.width) particle.vx *= -1
      if (particle.y <= 0 || particle.y >= canvas.height) particle.vy *= -1

      // Keep particles in bounds
      particle.x = Math.max(0, Math.min(canvas.width, particle.x))
      particle.y = Math.max(0, Math.min(canvas.height, particle.y))

      // Subtle opacity animation
      particle.opacity += (Math.random() - 0.5) * 0.01
      particle.opacity = Math.max(0.4, Math.min(1.0, particle.opacity))
    })
  }

  // Draw particles
  const drawParticles = (ctx) => {
    particlesRef.current.forEach(particle => {
      ctx.save()
      ctx.globalAlpha = particle.opacity
      ctx.fillStyle = particle.color
      ctx.beginPath()
      ctx.arc(particle.x, particle.y, particle.size, 0, Math.PI * 2)
      ctx.fill()
      
      // Add glow effect
      ctx.shadowBlur = 10
      ctx.shadowColor = particle.color
      ctx.fill()
      ctx.restore()
    })
  }

  // 性能优化的动画循环
  const animate = (currentTime) => {
    const canvas = canvasRef.current
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return

    // 限制帧率到30fps以提高性能
    if (currentTime - lastFrameTimeRef.current < 33) {
      animationRef.current = requestAnimationFrame(animate)
      return
    }
    lastFrameTimeRef.current = currentTime

    // 滚动时降低动画频率
    if (isScrollingRef.current) {
      if (currentTime - lastFrameTimeRef.current < 100) {
        animationRef.current = requestAnimationFrame(animate)
        return
      }
    }

    // Clear canvas
    ctx.clearRect(0, 0, canvas.width, canvas.height)

    if (enableParticles && !isScrollingRef.current) {
      updateParticles()
      drawParticles(ctx)
    }

    animationRef.current = requestAnimationFrame(animate)
  }

  // Resize canvas
  const resizeCanvas = () => {
    const canvas = canvasRef.current
    if (!canvas) return

    canvas.width = window.innerWidth
    canvas.height = window.innerHeight
  }

  // Theme detection
  useEffect(() => {
    const checkTheme = () => {
      const isDarkMode = document.documentElement.classList.contains('dark') ||
                        window.matchMedia('(prefers-color-scheme: dark)').matches
      setIsDark(isDarkMode)
    }

    checkTheme()
    
    const observer = new MutationObserver(checkTheme)
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
    
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    mediaQuery.addEventListener('change', checkTheme)

    return () => {
      observer.disconnect()
      mediaQuery.removeEventListener('change', checkTheme)
    }
  }, [])

  // 滚动事件优化
  useEffect(() => {
    const handleScroll = () => {
      isScrollingRef.current = true
      
      if (scrollTimeoutRef.current) {
        clearTimeout(scrollTimeoutRef.current)
      }
      
      scrollTimeoutRef.current = setTimeout(() => {
        isScrollingRef.current = false
      }, 150)
    }

    const handleWheel = (e) => {
      // 防止滚轮事件与动画冲突
      if (e.deltaY !== 0) {
        isScrollingRef.current = true
        
        if (scrollTimeoutRef.current) {
          clearTimeout(scrollTimeoutRef.current)
        }
        
        scrollTimeoutRef.current = setTimeout(() => {
          isScrollingRef.current = false
        }, 200)
      }
    }

    window.addEventListener('scroll', handleScroll, { passive: true })
    window.addEventListener('wheel', handleWheel, { passive: true })

    return () => {
      window.removeEventListener('scroll', handleScroll)
      window.removeEventListener('wheel', handleWheel)
      if (scrollTimeoutRef.current) {
        clearTimeout(scrollTimeoutRef.current)
      }
    }
  }, [])

  // Initialize and start animation
  useEffect(() => {
    resizeCanvas()
    initParticles()
    animate()

    const handleResize = () => {
      resizeCanvas()
      initParticles()
    }

    window.addEventListener('resize', handleResize)

    return () => {
      window.removeEventListener('resize', handleResize)
      if (animationRef.current) {
        cancelAnimationFrame(animationRef.current)
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [particleCount, enableParticles])

  // Update particle colors when theme changes
  useEffect(() => {
    particlesRef.current.forEach(particle => {
      particle.color = currentTheme.particles[Math.floor(Math.random() * currentTheme.particles.length)]
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isDark])

  return (
    <>
      {/* 背景层 - 使用更高的 z-index 和强制可见性 */}
      <div 
        className={`fixed inset-0 pointer-events-none overflow-hidden ${className}`} 
        style={{ 
          zIndex: -10,
          background: isDark 
            ? 'linear-gradient(135deg, #0f0f23 0%, #1a1a2e 50%, #16213e 100%)'
            : 'linear-gradient(135deg, #e2e8f0 0%, #cbd5e1 50%, #94a3b8 100%)'
        }}
      >
        {/* 强化渐变背景 */}
        <div 
          className="absolute inset-0"
          style={{
            background: isDark 
              ? `radial-gradient(circle at 20% 80%, #14F19580 0%, transparent 50%), 
                 radial-gradient(circle at 80% 20%, #9945FF70 0%, transparent 50%), 
                 radial-gradient(circle at 40% 40%, #00D4AA60 0%, transparent 50%)`
              : `radial-gradient(circle at 20% 80%, #14F19560 0%, transparent 50%), 
                 radial-gradient(circle at 80% 20%, #9945FF50 0%, transparent 50%), 
                 radial-gradient(circle at 40% 40%, #00D4AA40 0%, transparent 50%)`
          }}
        />

        {/* 简化但更可见的光球 */}
        {enableOrbs && (
          <>
            <motion.div
              className="absolute w-80 h-80 rounded-full blur-2xl pointer-events-none"
              style={{ 
                backgroundColor: '#14F195',
                opacity: 0.8,
                top: '10%',
                left: '10%',
                willChange: 'transform'
              }}
              animate={{
                x: [0, 100, 0],
                y: [0, -50, 0],
                scale: [1, 1.2, 1],
              }}
              transition={{
                duration: 25,
                repeat: Infinity,
                ease: "easeInOut"
              }}
            />
            
            <motion.div
              className="absolute w-72 h-72 rounded-full blur-2xl pointer-events-none"
              style={{ 
                backgroundColor: '#9945FF',
                opacity: 0.7,
                top: '60%',
                right: '10%',
                willChange: 'transform'
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
              className="absolute w-64 h-64 rounded-full blur-xl pointer-events-none"
              style={{ 
                backgroundColor: '#00D4AA',
                opacity: 0.6,
                bottom: '20%',
                left: '50%',
                willChange: 'transform'
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

        {/* Particle Canvas - 修复混合模式 */}
        {enableParticles && (
          <canvas
            ref={canvasRef}
            className="absolute inset-0 pointer-events-none"
            style={{ 
              mixBlendMode: isDark ? 'screen' : 'normal',
              opacity: isDark ? 0.8 : 0.6
            }}
          />
        )}
      </div>

      {/* 内容层 - 直接返回子元素，不额外包裹 */}
      {children}
    </>
  )
}

export default DynamicBackground