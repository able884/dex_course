import React, { useRef, useEffect } from 'react';
import { useLanguage, useTranslation } from '../../i18n/LanguageContext';
import { X, Globe, Check } from 'lucide-react';
import { Button } from '../UI/Button';
import { EnhancedCard } from '../UI/enhanced-card';

const LanguageSelector = ({ isOpen, onClose }) => {
  const { currentLanguage, changeLanguage, supportedLanguages } = useLanguage();
  const { t } = useTranslation();
  const dropdownRef = useRef(null);

  // 点击外部关闭下拉菜单
  useEffect(() => {
    const handleClickOutside = (event) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target)) {
        onClose();
      }
    };

    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
      return () => {
        document.removeEventListener('mousedown', handleClickOutside);
      };
    }
  }, [isOpen, onClose]);

  // ESC键关闭
  useEffect(() => {
    const handleEscKey = (event) => {
      if (event.key === 'Escape') {
        onClose();
      }
    };

    if (isOpen) {
      document.addEventListener('keydown', handleEscKey);
      return () => {
        document.removeEventListener('keydown', handleEscKey);
      };
    }
  }, [isOpen, onClose]);

  const handleLanguageChange = (languageCode) => {
    changeLanguage(languageCode);
    onClose();
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-60 flex items-center justify-center bg-black/50 backdrop-blur-sm">
      <EnhancedCard 
        className="w-full max-w-md mx-4 p-0 overflow-hidden shadow-2xl border-border/50"
        ref={dropdownRef}
      >
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <div className="flex items-center space-x-2">
            <Globe className="h-5 w-5 text-primary" />
            <h3 className="text-lg font-semibold text-foreground">
              语言 / Language
            </h3>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={onClose}
            className="h-8 w-8 rounded-full hover:bg-muted"
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
        
        {/* Content */}
        <div className="p-4 space-y-2">
          {supportedLanguages.map((language) => (
            <Button
              key={language.code}
              variant={currentLanguage === language.code ? 'default' : 'ghost'}
              className="w-full justify-start h-auto p-4 text-left"
              onClick={() => handleLanguageChange(language.code)}
            >
              <div className="flex items-center justify-between w-full">
                <div className="flex items-center space-x-3">
                  <span className="text-2xl">
                    {language.code === 'zh' ? '🇨🇳' : '🇺🇸'}
                  </span>
                  <div className="flex flex-col">
                    <span className="font-medium text-base">
                      {language.nativeName}
                    </span>
                    <span className="text-sm text-muted-foreground">
                      {language.name}
                    </span>
                  </div>
                </div>
                {currentLanguage === language.code && (
                  <Check className="h-4 w-4 text-primary" />
                )}
              </div>
            </Button>
          ))}
        </div>
        
        {/* Footer */}
        <div className="px-6 py-4 bg-muted/30 border-t border-border/50">
          <p className="text-sm text-muted-foreground text-center">
            {t('settings.language.autoSave')}
          </p>
        </div>
      </EnhancedCard>
    </div>
  );
};

export default LanguageSelector;
