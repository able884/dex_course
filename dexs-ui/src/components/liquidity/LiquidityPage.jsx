import React, { useEffect, useState } from 'react';
import { motion } from 'framer-motion';
import { Waves, Droplets, ArrowLeft } from 'lucide-react';
import { Button } from '../UI/Button';
import { useTranslation } from '../../i18n/LanguageContext';
import { cn } from '../../lib/utils';
import PoolCreationNew from '../Pools/PoolCreationNew';
import CpmmCreatePoolForm from './CpmmCreatePoolForm';

const LiquidityPage = ({ onNavigateBack, initialType = 'cpmm' }) => {
  const [activeType, setActiveType] = useState(initialType);
  const { t } = useTranslation();

  useEffect(() => {
    setActiveType(initialType);
  }, [initialType]);

  const tabs = [
    {
      id: 'cpmm',
      label: 'CPMM',
      description: t('liquidityPage.tabs.cpmm'),
      icon: Droplets
    },
    {
      id: 'clmm',
      label: 'CLMM',
      description: t('liquidityPage.tabs.clmm'),
      icon: Waves
    }
  ];

  const renderActiveTab = () => {
    if (activeType === 'clmm') {
      return <PoolCreationNew showHeader={false} />;
    }

    return <CpmmCreatePoolForm />;
  };

  return (
    <div className="container mx-auto px-4 py-8">
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="max-w-6xl mx-auto space-y-6"
      >
        <div className="flex justify-end">
          {onNavigateBack && (
            <Button variant="outline" onClick={onNavigateBack}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              {t('liquidityPage.back')}
            </Button>
          )}
        </div>

        <div className="rounded-2xl border border-border/60 bg-card/60 backdrop-blur-sm overflow-hidden">
          <div className="grid grid-cols-1 md:grid-cols-2">
            {tabs.map((tab) => {
              const Icon = tab.icon;
              const isActive = tab.id === activeType;
              return (
                <button
                  key={tab.id}
                  type="button"
                  onClick={() => setActiveType(tab.id)}
                  className={cn(
                    'flex flex-col md:flex-row items-start md:items-center gap-3 px-5 py-3 text-left transition-all border-b md:border-b-0 md:border-r last:border-0',
                    tab.id === 'clmm' && 'md:border-r-0',
                    isActive
                      ? 'bg-primary/5 text-primary'
                      : 'hover:bg-muted/50 text-muted-foreground'
                  )}
                >
                  <div
                    className={cn(
                      'p-2.5 rounded-xl bg-muted transition-colors',
                      isActive && 'bg-primary/10 text-primary'
                    )}
                  >
                    <Icon className="w-5 h-5" />
                  </div>
                  <div>
                    <p className="text-lg font-semibold">{tab.label}</p>
                    <p className="text-sm">{tab.description}</p>
                  </div>
                </button>
              );
            })}
          </div>
        </div>

        {renderActiveTab()}
      </motion.div>
    </div>
  );
};

export default LiquidityPage;
