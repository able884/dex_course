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

  // 捕获并静默处理 MetaMask 连接错误（因为这是 Solana 应用，不需要 MetaMask）
  const isMetaMaskError = (error) => {
    if (!error) return false;
    const errorMessage = error.message || error.toString() || '';
    const errorStack = error.stack || '';
    const errorSource = error.filename || error.source || '';
    
    // 检查错误消息、堆栈或来源是否包含 MetaMask 相关的内容
    const metaMaskKeywords = [
      'MetaMask',
      'metamask',
      'Failed to connect to MetaMask',
      'chrome-extension://nkbihfbeogaeaoehlefnkodbefgpgknn',
      'ethereum',
      'eth_requestAccounts',
      'wallet_requestPermissions'
    ];
    
    const combinedText = `${errorMessage} ${errorStack} ${errorSource}`.toLowerCase();
    return metaMaskKeywords.some(keyword => combinedText.includes(keyword.toLowerCase()));
  };

  // 处理未捕获的全局错误
  window.addEventListener('error', (event) => {
    if (isMetaMaskError(event.error || event)) {
      // 静默处理 MetaMask 相关错误，不显示在控制台
      event.preventDefault();
      return true;
    }
    // 其他错误正常处理
    return false;
  }, true); // 使用捕获阶段

  // 处理未处理的 Promise rejection
  window.addEventListener('unhandledrejection', (event) => {
    if (isMetaMaskError(event.reason)) {
      // 静默处理 MetaMask 相关的 Promise rejection
      event.preventDefault();
      return;
    }
    // 其他 rejection 正常处理
  });

  // 防止 MetaMask 扩展自动尝试连接
  if (window.ethereum) {
    try {
      // 拦截 MetaMask 的连接请求，静默处理
      const originalRequest = window.ethereum.request;
      if (originalRequest && typeof originalRequest === 'function') {
        window.ethereum.request = function(args) {
          // 如果是连接请求，返回一个被拒绝的 Promise，但静默处理
          if (args && typeof args === 'object' && (args.method === 'eth_requestAccounts' || args.method === 'wallet_requestPermissions')) {
            // 返回一个被拒绝的 Promise，但立即 catch 以静默处理
            const rejectedPromise = Promise.reject(new Error('MetaMask connection not needed for Solana app'));
            rejectedPromise.catch(() => {
              // 静默忽略错误，不输出到控制台
            });
            return rejectedPromise;
          }
          // 其他请求正常处理
          try {
            return originalRequest.apply(this, arguments);
          } catch (e) {
            // 如果原始请求失败，检查是否是 MetaMask 相关错误
            if (isMetaMaskError(e)) {
              const silentPromise = Promise.reject(e);
              silentPromise.catch(() => {});
              return silentPromise;
            }
            throw e;
          }
        };
      }
    } catch (e) {
      // 如果拦截失败，静默忽略
      console.debug('MetaMask interception setup failed:', e);
    }
  }
}

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
  <LanguageProvider>
    <App />
  </LanguageProvider>
); 