// 性能优化工具函数

/**
 * 防抖函数 - 延迟执行函数，在指定时间内重复调用会重置计时器
 * @param {Function} func - 要防抖的函数
 * @param {number} wait - 延迟时间（毫秒）
 * @param {boolean} immediate - 是否立即执行
 * @returns {Function} 防抖后的函数
 */
export const debounce = (func, wait, immediate = false) => {
  let timeout;
  return function executedFunction(...args) {
    const later = () => {
      timeout = null;
      if (!immediate) func(...args);
    };
    const callNow = immediate && !timeout;
    clearTimeout(timeout);
    timeout = setTimeout(later, wait);
    if (callNow) func(...args);
  };
};

/**
 * 节流函数 - 限制函数在指定时间内最多执行一次
 * @param {Function} func - 要节流的函数
 * @param {number} limit - 时间限制（毫秒）
 * @returns {Function} 节流后的函数
 */
export const throttle = (func, limit) => {
  let inThrottle;
  return function executedFunction(...args) {
    if (!inThrottle) {
      func.apply(this, args);
      inThrottle = true;
      setTimeout(() => (inThrottle = false), limit);
    }
  };
};

/**
 * 懒加载图片
 * @param {HTMLImageElement} img - 图片元素
 * @param {string} src - 图片源地址
 * @param {string} placeholder - 占位图片
 */
export const lazyLoadImage = (img, src, placeholder = '') => {
  const observer = new IntersectionObserver(
    (entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          const image = entry.target;
          image.src = src;
          image.classList.remove('lazy-loading');
          image.classList.add('lazy-loaded');
          observer.unobserve(image);
        }
      });
    },
    {
      threshold: 0.1,
      rootMargin: '50px',
    }
  );

  img.src = placeholder;
  img.classList.add('lazy-loading');
  observer.observe(img);
};

/**
 * 虚拟滚动优化 - 只渲染可见区域的项目
 * @param {Array} items - 所有项目
 * @param {number} containerHeight - 容器高度
 * @param {number} itemHeight - 单个项目高度
 * @param {number} scrollTop - 滚动位置
 * @returns {Object} 包含可见项目和偏移量的对象
 */
export const getVisibleItems = (items, containerHeight, itemHeight, scrollTop) => {
  const startIndex = Math.floor(scrollTop / itemHeight);
  const endIndex = Math.min(
    startIndex + Math.ceil(containerHeight / itemHeight) + 1,
    items.length
  );
  
  return {
    visibleItems: items.slice(startIndex, endIndex),
    offsetY: startIndex * itemHeight,
    totalHeight: items.length * itemHeight,
  };
};

/**
 * 预加载关键资源
 * @param {Array<string>} urls - 资源URL数组
 * @param {string} type - 资源类型 ('image', 'script', 'style')
 */
export const preloadResources = (urls, type = 'image') => {
  urls.forEach((url) => {
    const link = document.createElement('link');
    link.rel = 'preload';
    link.href = url;
    
    switch (type) {
      case 'image':
        link.as = 'image';
        break;
      case 'script':
        link.as = 'script';
        break;
      case 'style':
        link.as = 'style';
        break;
      default:
        link.as = 'fetch';
        link.crossOrigin = 'anonymous';
    }
    
    document.head.appendChild(link);
  });
};

/**
 * 内存优化 - 清理不需要的定时器和事件监听器
 */
export class MemoryManager {
  constructor() {
    this.timers = new Set();
    this.listeners = new Map();
  }

  // 添加定时器
  setTimeout(callback, delay) {
    const timer = setTimeout(() => {
      callback();
      this.timers.delete(timer);
    }, delay);
    this.timers.add(timer);
    return timer;
  }

  setInterval(callback, interval) {
    const timer = setInterval(callback, interval);
    this.timers.add(timer);
    return timer;
  }

  // 添加事件监听器
  addEventListener(element, event, handler, options) {
    element.addEventListener(event, handler, options);
    
    if (!this.listeners.has(element)) {
      this.listeners.set(element, []);
    }
    this.listeners.get(element).push({ event, handler, options });
  }

  // 清理所有资源
  cleanup() {
    // 清理定时器
    this.timers.forEach((timer) => {
      clearTimeout(timer);
      clearInterval(timer);
    });
    this.timers.clear();

    // 清理事件监听器
    this.listeners.forEach((eventList, element) => {
      eventList.forEach(({ event, handler, options }) => {
        element.removeEventListener(event, handler, options);
      });
    });
    this.listeners.clear();
  }
}

/**
 * 检测设备性能
 * @returns {Object} 设备性能信息
 */
export const getDevicePerformance = () => {
  const connection = navigator.connection || navigator.mozConnection || navigator.webkitConnection;
  
  return {
    // 网络信息
    effectiveType: connection?.effectiveType || 'unknown',
    downlink: connection?.downlink || 0,
    rtt: connection?.rtt || 0,
    saveData: connection?.saveData || false,
    
    // 硬件信息
    hardwareConcurrency: navigator.hardwareConcurrency || 1,
    deviceMemory: navigator.deviceMemory || 0,
    
    // 性能评分 (1-5, 5为最高性能)
    score: (() => {
      let score = 3; // 默认中等性能
      
      if (connection?.effectiveType === '4g') score += 1;
      if (connection?.effectiveType === 'slow-2g') score -= 2;
      if (navigator.hardwareConcurrency >= 8) score += 1;
      if (navigator.deviceMemory >= 8) score += 1;
      
      return Math.max(1, Math.min(5, score));
    })(),
  };
};

/**
 * 根据设备性能调整渲染策略
 * @param {number} performanceScore - 性能评分
 * @returns {Object} 渲染配置
 */
export const getRenderConfig = (performanceScore) => {
  const configs = {
    1: { // 低性能设备
      maxItems: 20,
      animationsEnabled: false,
      lazyLoadThreshold: 0.3,
      debounceDelay: 300,
    },
    2: {
      maxItems: 40,
      animationsEnabled: false,
      lazyLoadThreshold: 0.2,
      debounceDelay: 200,
    },
    3: { // 中等性能设备
      maxItems: 60,
      animationsEnabled: true,
      lazyLoadThreshold: 0.1,
      debounceDelay: 150,
    },
    4: {
      maxItems: 100,
      animationsEnabled: true,
      lazyLoadThreshold: 0.1,
      debounceDelay: 100,
    },
    5: { // 高性能设备
      maxItems: 200,
      animationsEnabled: true,
      lazyLoadThreshold: 0.05,
      debounceDelay: 50,
    },
  };

  return configs[performanceScore] || configs[3];
};

/**
 * Web Worker 工厂
 * @param {Function} workerFunction - 要在 Worker 中执行的函数
 * @returns {Worker} Web Worker 实例
 */
export const createWebWorker = (workerFunction) => {
  const blob = new Blob([`(${workerFunction.toString()})()`], {
    type: 'application/javascript',
  });
  return new Worker(URL.createObjectURL(blob));
};

/**
 * 批处理更新 - 将多个 DOM 更新合并为一次重排
 * @param {Function} callback - 包含 DOM 更新的回调函数
 */
export const batchDOMUpdates = (callback) => {
  requestAnimationFrame(() => {
    callback();
  });
};

/**
 * 监控性能指标
 */
export const performanceMonitor = {
  // 监控 FCP (First Contentful Paint)
  measureFCP() {
    return new Promise((resolve) => {
      new PerformanceObserver((list) => {
        const entries = list.getEntries();
        const fcpEntry = entries.find(entry => entry.name === 'first-contentful-paint');
        if (fcpEntry) {
          resolve(fcpEntry.startTime);
        }
      }).observe({ entryTypes: ['paint'] });
    });
  },

  // 监控 LCP (Largest Contentful Paint)
  measureLCP() {
    return new Promise((resolve) => {
      new PerformanceObserver((list) => {
        const entries = list.getEntries();
        const lastEntry = entries[entries.length - 1];
        resolve(lastEntry.startTime);
      }).observe({ entryTypes: ['largest-contentful-paint'] });
    });
  },

  // 监控 CLS (Cumulative Layout Shift)
  measureCLS() {
    return new Promise((resolve) => {
      let clsValue = 0;
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries()) {
          if (!entry.hadRecentInput) {
            clsValue += entry.value;
          }
        }
        resolve(clsValue);
      }).observe({ entryTypes: ['layout-shift'] });
    });
  },
};
