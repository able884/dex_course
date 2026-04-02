import React from 'react';
import { 
  Github, 
  Twitter, 
  MessageCircle, 
  Mail, 
  Globe, 
  Zap, 
  Users, 
  Eye, 
  BarChart3, 
  Briefcase,
  ExternalLink
} from 'lucide-react';
import { useThemeContext } from '../UI/theme-provider';
import { useTranslation } from '../../i18n/LanguageContext';

// Logo imports
import logoBlack from '../../assets/logo-black.png';
import logoWhite from '../../assets/logo-white.png';

const Footer = () => {
  const { t } = useTranslation();
  const { resolvedTheme, themes } = useThemeContext();
  
  // 根据主题选择Logo
  const currentLogo = resolvedTheme === themes.DARK ? logoWhite : logoBlack;

  const contactLinks = [
    { name: 'GitHub', href: 'https://github.com', icon: Github },
    { name: 'Twitter', href: 'https://twitter.com', icon: Twitter },
    { name: 'Discord', href: 'https://discord.com', icon: MessageCircle },
    { name: t('footer.emailContact'), href: 'mailto:contact@pumptokens.com', icon: Mail }
  ];

  const ecosystemLinks = [
    { name: t('footer.solanaOfficial'), href: 'https://solana.com', icon: Globe },
    { name: 'Raydium', href: 'https://raydium.io', icon: ExternalLink },
    { name: 'Jupiter', href: 'https://jup.ag', icon: ExternalLink },
    { name: 'Magic Eden', href: 'https://magiceden.io', icon: ExternalLink }
  ];

  return (
    <footer className="mt-auto bg-background/90 backdrop-blur supports-[backdrop-filter]:bg-background/80 border-t border-border">
      <div className="container mx-auto px-4 py-12">
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-8">
          {/* 品牌信息 */}
          <div className="space-y-4">
            <div className="flex items-center space-x-2">
              <img 
                src={currentLogo} 
                alt="RichCode DEX Logo" 
                className="h-8 w-auto"
              />
              <span className="text-xl font-bold">{t('footer.pumpTokens')}</span>
            </div>
            <p className="text-muted-foreground text-sm leading-relaxed text-left">
              {t('footer.description')}
            </p>
            <div className="flex space-x-3">
              {contactLinks.map((link) => {
                const IconComponent = link.icon;
                return (
                  <a
                    key={link.name}
                    href={link.href}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="p-2 rounded-lg bg-muted hover:bg-muted/80 transition-colors"
                    aria-label={link.name}
                  >
                    <IconComponent className="h-4 w-4" />
                  </a>
                );
              })}
            </div>
          </div>

          {/* 功能导航 */}
          <div className="space-y-4">
            <h3 className="text-lg font-semibold text-left">{t('footer.features')}</h3>
            <ul className="space-y-3 text-left">
              <li>
                <button className="text-muted-foreground hover:text-foreground transition-colors flex items-center space-x-2 bg-transparent border-none cursor-pointer justify-start w-full">
                  <Zap className="h-4 w-4" />
                  <span>{t('footer.tradingHall')}</span>
                </button>
              </li>
              <li>
                <button className="text-muted-foreground hover:text-foreground transition-colors flex items-center space-x-2 bg-transparent border-none cursor-pointer justify-start w-full">
                  <Users className="h-4 w-4" />
                  <span>{t('footer.copyTrade')}</span>
                </button>
              </li>
              <li>
                <button className="text-muted-foreground hover:text-foreground transition-colors flex items-center space-x-2 bg-transparent border-none cursor-pointer justify-start w-full">
                  <Eye className="h-4 w-4" />
                  <span>{t('footer.monitorPanel')}</span>
                </button>
              </li>
              <li>
                <button className="text-muted-foreground hover:text-foreground transition-colors flex items-center space-x-2 bg-transparent border-none cursor-pointer justify-start w-full">
                  <BarChart3 className="h-4 w-4" />
                  <span>{t('footer.trackingAnalysis')}</span>
                </button>
              </li>
              <li>
                <button className="text-muted-foreground hover:text-foreground transition-colors flex items-center space-x-2 bg-transparent border-none cursor-pointer justify-start w-full">
                  <Briefcase className="h-4 w-4" />
                  <span>{t('footer.positionManagement')}</span>
                </button>
              </li>
            </ul>
          </div>

          {/* 联系我们 */}
          <div className="space-y-4">
            <h3 className="text-lg font-semibold text-left">{t('footer.contactUs')}</h3>
            <ul className="space-y-3 text-left">
              <li>
                <a href="mailto:support@pumptokens.com" className="text-muted-foreground hover:text-foreground transition-colors flex items-center space-x-2 justify-start">
                  <Mail className="h-4 w-4" />
                  <span>{t('footer.technicalSupport')}</span>
                </a>
              </li>
              <li>
                <a href="mailto:business@pumptokens.com" className="text-muted-foreground hover:text-foreground transition-colors flex items-center space-x-2 justify-start">
                  <Mail className="h-4 w-4" />
                  <span>{t('footer.businessCooperation')}</span>
                </a>
              </li>
              <li>
                <a href="https://t.me/pumptokens" target="_blank" rel="noopener noreferrer" className="text-muted-foreground hover:text-foreground transition-colors flex items-center space-x-2 justify-start">
                  <MessageCircle className="h-4 w-4" />
                  <span>{t('footer.telegramGroup')}</span>
                </a>
              </li>
              <li>
                <a href="https://discord.gg/pumptokens" target="_blank" rel="noopener noreferrer" className="text-muted-foreground hover:text-foreground transition-colors flex items-center space-x-2 justify-start">
                  <MessageCircle className="h-4 w-4" />
                  <span>{t('footer.discordCommunity')}</span>
                </a>
              </li>
            </ul>
          </div>

          {/* 生态系统 */}
          <div className="space-y-4">
            <h3 className="text-lg font-semibold text-left">{t('footer.ecosystem')}</h3>
            <ul className="space-y-3 text-left">
              {ecosystemLinks.map((link) => {
                const IconComponent = link.icon;
                return (
                  <li key={link.name}>
                    <a 
                      href={link.href} 
                      target="_blank" 
                      rel="noopener noreferrer" 
                      className="text-muted-foreground hover:text-foreground transition-colors flex items-center space-x-2 justify-start"
                    >
                      <IconComponent className="h-4 w-4" />
                      <span>{link.name}</span>
                    </a>
                  </li>
                );
              })}
            </ul>
          </div>
        </div>

        {/* 版权信息 */}
        <div className="mt-12 pt-8 border-t border-border">
          <div className="flex flex-col md:flex-row justify-between items-start space-y-4 md:space-y-0">
            <div className="text-sm text-muted-foreground">
              {t('footer.copyright')}
            </div>
            <div className="flex space-x-6 text-sm">
              <button className="text-muted-foreground hover:text-foreground transition-colors bg-transparent border-none cursor-pointer">
                {t('footer.privacyPolicy')}
              </button>
              <button className="text-muted-foreground hover:text-foreground transition-colors bg-transparent border-none cursor-pointer">
                {t('footer.termsOfService')}
              </button>
              <button className="text-muted-foreground hover:text-foreground transition-colors bg-transparent border-none cursor-pointer">
                {t('footer.disclaimer')}
              </button>
            </div>
          </div>
        </div>
      </div>
    </footer>
  );
};

export default Footer;
