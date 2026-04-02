import React, { useState } from 'react';
import { WalletMultiButton } from '@solana/wallet-adapter-react-ui';
import logoWhite from '../../assets/logo-white.png';
import logoBlack from '../../assets/logo-black.png';
import { useTranslation } from '../../i18n/LanguageContext';
import { useThemeContext } from '../UI/theme-provider';
import { ThemeToggle } from '../UI/theme-toggle';
import { Search, ChevronDown, Settings, Zap, Coins, Waves, BarChart3, Droplets, ExternalLink } from 'lucide-react';
import { Button } from '../UI/Button';
import { SearchInput } from '../UI/enhanced-input';

const Header = ({ activeTab, onTabChange, onSettingsClick }) => {
  const { t } = useTranslation();
  const { resolvedTheme, themes } = useThemeContext();
  const [searchQuery, setSearchQuery] = useState('');

  // 根据主题选择Logo
  const currentLogo = resolvedTheme === themes.DARK ? logoWhite : logoBlack;

  // 水龙头数据
  const faucets = [
    {
      name: 'Solana Official Faucet',
      url: 'https://faucet.solana.com/',
      desc: '每8小时可以领取两次 (Official, 2x every 8h)'
    },
    {
      name: 'solfaucet.com',
      url: 'https://solfaucet.com/',
      desc: 'Simple devnet faucet'
    },
    {
      name: 'QuickNode Faucet',
      url: 'https://faucet.quicknode.com/solana/devnet',
      desc: 'QuickNode devnet faucet'
    },
    {
      name: 'DevnetFaucet.org',
      url: 'https://www.devnetfaucet.org/',
      desc: 'Another devnet faucet'
    },
    {
      name: 'solfate.com Faucet',
      url: 'https://solfate.com/faucet',
      desc: 'Solfate devnet faucet'
    },
    {
      name: 'Ashwin Narayan Faucet',
      url: 'https://www.ashwinnarayan.com/dapps/solana-faucet/',
      desc: 'Ashwin Narayan devnet faucet'
    },
    {
      name: 'SPL Token Faucet',
      url: 'https://spl-token-faucet.com/',
      desc: 'SPL Token faucet'
    },
    {
      name: 'Diadata Faucet List',
      url: 'https://www.diadata.org/web3-builder-hub/faucets/solana-faucets/',
      desc: 'Diadata faucet aggregator'
    }
  ];

  // 主导航标签 - 更新为Token和Pool
  const mainNavTabs = [
    { id: 'pools', label: 'Liquidity', icon: Waves },
    { id: 'create-token', label: t('header.navigation.createToken'), icon: Coins },
  ];

  const handleSearchChange = (e) => {
    setSearchQuery(e.target.value);
  };

  // 检查当前是否有工具功能被激活（不包括主导航的create-token和create-pool）
  const toolTabs = ['add-liquidity', 'token-security'];
  const isToolTabActive = toolTabs.includes(activeTab);

  return (
    <header className="sticky top-0 z-40 w-full border-b border-border/40 bg-background/80 backdrop-blur supports-[backdrop-filter]:bg-background/50">
      <div className="container flex h-16 max-w-screen-2xl items-center justify-between px-4 md:px-6">
        {/* Logo 区域 - 修复语言切换时消失的问题 */}
        <div 
          className="flex items-center cursor-pointer"
          onClick={() => onTabChange('dashboard')}
        >
          <img 
            src={currentLogo} 
            alt="RC DEX Logo" 
            className="h-8 w-auto transition-opacity duration-300" 
            onError={(e) => {
              console.error('Logo failed to load:', e);
              e.target.style.display = 'none';
            }}
            onLoad={() => {
              console.log('Logo loaded successfully');
            }}
          />
        </div>

        {/* 导航区域 */}
        <nav className="hidden md:flex items-center space-x-1">
          {/* Trenches 按钮 - 简化版本，移除下拉菜单 */}
          <Button
            variant={activeTab === 'trenches' || isToolTabActive ? 'default' : 'ghost'}
            className="flex items-center space-x-2 px-3 py-2"
            onClick={() => onTabChange('trenches')}
          >
            <Zap className="h-4 w-4" />
            <span>Pump.fun</span>
          </Button>

          {/* 主导航标签 */}
          {mainNavTabs.map((tab) => {
            const IconComponent = tab.icon;
            return (
              <Button
                key={tab.id}
                variant={activeTab === tab.id ? 'default' : 'ghost'}
                className="flex items-center space-x-2 px-3 py-2"
                onClick={() => {
                  if (tab.onClick) {
                    tab.onClick(); // 如果有自定义点击事件，执行它
                  } else {
                    onTabChange(tab.id); // 否则执行默认的标签切换
                  }
                }}
              >
                <IconComponent className="h-4 w-4" />
                <span className="hidden lg:inline">{tab.label}</span>
              </Button>
            );
          })}

          {/* 水龙头下拉菜单 */}
          <div className="relative group">
            <Button
              variant="ghost"
              className="flex items-center space-x-2 px-3 py-2 cursor-default"
              onClick={(e) => e.preventDefault()}
            >
              <Droplets className="h-4 w-4" />
              <span className="hidden lg:inline">{t('header.navigation.faucet')}</span>
              <ChevronDown className="h-3 w-3 opacity-50" />
            </Button>
            
            {/* 下拉菜单 - 添加连接区域避免鼠标移动时丢失hover */}
            <div className="absolute top-full left-0 pt-1 w-80 opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all duration-200 z-50">
              {/* 连接区域 - 透明但可以接收鼠标事件 */}
              <div className="h-1 w-full"></div>
              
              {/* 实际的下拉菜单 */}
              <div className="bg-background/95 backdrop-blur-sm border border-border/50 rounded-lg shadow-lg">
                <div className="p-2 space-y-1">
                  <div className="px-3 py-2 text-sm font-medium text-muted-foreground border-b border-border/50">
                    {t('header.faucet.title')}
                  </div>
                  {faucets.map((faucet, index) => (
                    <a
                      key={index}
                      href={faucet.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="flex items-center justify-between px-3 py-2 text-sm rounded-md hover:bg-muted/50 transition-colors group/item"
                    >
                      <div className="flex-1">
                        <div className="font-medium text-foreground">{faucet.name}</div>
                        <div className="text-xs text-muted-foreground truncate">{faucet.desc}</div>
                      </div>
                      <ExternalLink className="h-3 w-3 text-muted-foreground opacity-0 group-hover/item:opacity-100 transition-opacity ml-2 flex-shrink-0" />
                    </a>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </nav>

        {/* 右侧控制区域 */}
        <div className="flex items-center space-x-2">
          {/* 搜索栏 - 桌面端 - 缩短宽度 */}
          <div className="hidden lg:block">
            <SearchInput
              placeholder={t('header.search.placeholder')}
              value={searchQuery}
              onChange={handleSearchChange}
              onClear={() => setSearchQuery('')}
              className="w-48 xl:w-56 h-10"
            />
          </div>

          {/* 搜索按钮 - 平板端 */}
          <Button 
            variant="outline" 
            size="icon" 
            className="hidden sm:flex lg:hidden"
            title={t('header.search.placeholder')}
          >
            <Search className="h-4 w-4" />
          </Button>

          {/* 网络选择器 */}
          <Button variant="outline" className="hidden xl:flex items-center space-x-2 h-10">
            <div className="h-2 w-2 rounded-full bg-green-500" />
            <span>{t('header.network.solana')}</span>
            <ChevronDown className="h-4 w-4" />
          </Button>

          {/* 钱包连接 */}
          <div className="wallet-container">
            <WalletMultiButton 
              className="!bg-gradient-to-r !from-solana !to-solana-secondary !border-none !rounded-md !text-white !font-medium !text-sm !px-4 hover:!opacity-90 !transition-opacity !min-w-[120px]"
              style={{
                height: '40px',
                minHeight: '40px',
                maxHeight: '40px'
              }}
            />
          </div>

          {/* 主题切换按钮 - 桌面端显示 */}
          <div className="hidden sm:block">
            <ThemeToggle />
          </div>
          
          {/* 设置按钮 - 桌面端显示 */}
          <Button
            variant="ghost"
            size="icon"
            onClick={onSettingsClick}
            title={t('header.buttons.settings')}
            className="hidden md:flex"
          >
            <Settings className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* 移动端导航 */}
      <div className="md:hidden border-t border-border/40 bg-background/80 backdrop-blur">
        <div className="container px-2 py-2">
          <div className="flex items-center justify-between">
            {/* 移动端导航滚动区域 */}
            <div className="flex items-center space-x-1 overflow-x-auto scrollbar-none flex-1 mr-2">
              {/* 移动端 Trenches 按钮 */}
              <Button
                variant={activeTab === 'trenches' || isToolTabActive ? 'default' : 'ghost'}
                size="sm"
                onClick={() => onTabChange('trenches')}
                className="flex-shrink-0 px-3 py-2"
              >
                <Zap className="h-4 w-4 mr-1" />
                <span className="text-xs whitespace-nowrap">Pump.fun</span>
              </Button>

              {/* 移动端主导航 - 显示更多项目 */}
              {mainNavTabs.map((tab) => {
                const IconComponent = tab.icon;
                return (
                  <Button
                    key={tab.id}
                    variant={activeTab === tab.id ? 'default' : 'ghost'}
                    size="sm"
                    onClick={() => {
                      if (tab.onClick) {
                        tab.onClick(); // 如果有自定义点击事件，执行它
                      } else {
                        onTabChange(tab.id); // 否则执行默认的标签切换
                      }
                    }}
                    className="flex-shrink-0 px-2 py-2 min-w-[44px]"
                    title={tab.label}
                  >
                    <IconComponent className="h-4 w-4" />
                  </Button>
                );
              })}
            </div>

            {/* 移动端操作按钮 */}
            <div className="flex items-center space-x-1">
              {/* 移动端搜索按钮 */}
              <Button 
                variant="ghost" 
                size="sm" 
                className="sm:hidden p-2 min-w-[36px]"
                title={t('header.search.placeholder')}
              >
                <Search className="h-4 w-4" />
              </Button>
              
              {/* 移动端设置按钮 */}
              <Button
                variant="ghost"
                size="sm"
                onClick={onSettingsClick}
                className="p-2 min-w-[36px]"
                title={t('header.buttons.settings')}
              >
                <Settings className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
      </div>

      {/* Removed AddLiquidityModal from header */}
    </header>
  );
};

export default Header;
