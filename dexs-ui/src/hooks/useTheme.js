import { useState, useEffect } from 'react';

const THEME_KEY = 'ui-theme';
const THEMES = {
  LIGHT: 'light',
  DARK: 'dark',
  SYSTEM: 'system'
};

export function useTheme() {
  const [theme, setTheme] = useState(() => {
    // 从 localStorage 获取保存的主题，默认为 system
    if (typeof window !== 'undefined') {
      return localStorage.getItem(THEME_KEY) || THEMES.SYSTEM;
    }
    return THEMES.SYSTEM;
  });

  const [systemTheme, setSystemTheme] = useState(() => {
    if (typeof window !== 'undefined') {
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? THEMES.DARK : THEMES.LIGHT;
    }
    return THEMES.LIGHT;
  });

  // 监听系统主题变化
  useEffect(() => {
    if (typeof window === 'undefined') return;

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    const handleChange = (e) => {
      setSystemTheme(e.matches ? THEMES.DARK : THEMES.LIGHT);
    };

    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, []);

  // 计算实际应用的主题
  const resolvedTheme = theme === THEMES.SYSTEM ? systemTheme : theme;

  // 应用主题到 DOM
  useEffect(() => {
    if (typeof window === 'undefined') return;

    const root = window.document.documentElement;
    
    // 移除所有主题类
    root.classList.remove(THEMES.LIGHT, THEMES.DARK);
    
    // 添加当前主题类
    if (resolvedTheme === THEMES.DARK) {
      root.classList.add(THEMES.DARK);
    }
    
    // 设置 color-scheme 属性以优化浏览器默认样式
    root.style.colorScheme = resolvedTheme;
  }, [resolvedTheme]);

  // 保存主题到 localStorage
  useEffect(() => {
    if (typeof window !== 'undefined') {
      localStorage.setItem(THEME_KEY, theme);
    }
  }, [theme]);

  const setThemeWithTransition = (newTheme) => {
    // 添加过渡效果
    if (typeof window !== 'undefined') {
      const root = window.document.documentElement;
      
      // 添加过渡类
      root.classList.add('theme-transitioning');
      
      // 设置新主题
      setTheme(newTheme);
      
      // 移除过渡类
      setTimeout(() => {
        root.classList.remove('theme-transitioning');
      }, 300);
    } else {
      setTheme(newTheme);
    }
  };

  return {
    theme,
    resolvedTheme,
    systemTheme,
    setTheme: setThemeWithTransition,
    themes: THEMES,
    isDark: resolvedTheme === THEMES.DARK,
    isLight: resolvedTheme === THEMES.LIGHT,
    isSystem: theme === THEMES.SYSTEM
  };
}