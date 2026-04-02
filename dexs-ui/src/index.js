import React from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';
import App from './App';
import { LanguageProvider } from './i18n/LanguageContext';

// 防止第三方/钱包脚本重复定义 window.ethereum 导致 "Cannot redefine property: ethereum"
if (typeof window !== 'undefined') {
  const desc = Object.getOwnPropertyDescriptor(window, 'ethereum');
  if (desc && desc.configurable === false) {
    // 已存在且不可配置，跳过任何重新定义尝试
    // no-op
  } else if (!('ethereum' in window)) {
    // 没有 ethereum，不做任何注入，由钱包或必要库自行注入
  } else {
    // 存在且可配置，但避免重复 defineProperty：保持现状
    // no-op
  }
}

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
  <LanguageProvider>
    <App />
  </LanguageProvider>
); 