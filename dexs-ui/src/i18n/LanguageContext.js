import React, { createContext, useContext, useState } from 'react';
import { zh } from './locales/zh';
import { en } from './locales/en';

// 语言包映射
const locales = {
  zh,
  en
};

// 支持的语言列表
export const supportedLanguages = [
  { code: 'zh', name: '中文', nativeName: '中文' },
  { code: 'en', name: 'English', nativeName: 'English' }
];

// 创建语言上下文
const LanguageContext = createContext();

// 从本地存储获取语言设置
const getStoredLanguage = () => {
  try {
    const stored = localStorage.getItem('gmgn-language');
    if (stored && locales[stored]) {
      return stored;
    }
  } catch (error) {
    console.warn('Failed to read language from localStorage:', error);
  }
  
  // 默认根据浏览器语言判断
  const browserLang = navigator.language.toLowerCase();
  if (browserLang.startsWith('zh')) {
    return 'zh';
  }
  return 'en'; // 默认英文
};

// 语言提供者组件
export const LanguageProvider = ({ children }) => {
  const [currentLanguage, setCurrentLanguage] = useState(getStoredLanguage);

  // 切换语言
  const changeLanguage = (languageCode) => {
    if (locales[languageCode]) {
      setCurrentLanguage(languageCode);
      try {
        localStorage.setItem('gmgn-language', languageCode);
      } catch (error) {
        console.warn('Failed to save language to localStorage:', error);
      }
    }
  };

  // 获取翻译文本
  const t = (key, fallback = key) => {
    try {
      const keys = key.split('.');
      let value = locales[currentLanguage];
      
      for (const k of keys) {
        if (value && typeof value === 'object' && k in value) {
          value = value[k];
        } else {
          // 如果当前语言没有找到，尝试英文
          if (currentLanguage !== 'en') {
            let enValue = locales.en;
            for (const k of keys) {
              if (enValue && typeof enValue === 'object' && k in enValue) {
                enValue = enValue[k];
              } else {
                return fallback;
              }
            }
            return typeof enValue === 'string' ? enValue : fallback;
          }
          return fallback;
        }
      }
      
      return typeof value === 'string' ? value : fallback;
    } catch (error) {
      console.warn('Translation error:', error);
      return fallback;
    }
  };

  // 获取当前语言信息
  const getCurrentLanguageInfo = () => {
    return supportedLanguages.find(lang => lang.code === currentLanguage) || supportedLanguages[1];
  };

  const value = {
    currentLanguage,
    changeLanguage,
    t,
    getCurrentLanguageInfo,
    supportedLanguages,
    isRTL: false // 当前支持的语言都是从左到右
  };

  return (
    <LanguageContext.Provider value={value}>
      {children}
    </LanguageContext.Provider>
  );
};

// 使用语言的钩子
export const useLanguage = () => {
  const context = useContext(LanguageContext);
  if (!context) {
    throw new Error('useLanguage must be used within a LanguageProvider');
  }
  return context;
};

// 简化的翻译钩子
export const useTranslation = () => {
  const { t } = useLanguage();
  return { t };
};
