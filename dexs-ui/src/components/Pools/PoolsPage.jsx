import React, { useState } from 'react';
import VersionSelector from './VersionSelector';
import PoolsTable from './PoolsTable';
import { Button } from '../UI/Button';
import { RefreshCw } from 'lucide-react';
import { cn } from '../../lib/utils';
import { Clock } from 'lucide-react';
import { useTranslation } from '../../i18n/LanguageContext';

const PoolsPage = ({ onNavigateToPoolCreation, onNavigateCharts, onNavigateSwap, onNavigateAddLiquidity }) => {
  const [version, setVersion] = useState('V1');
  const [refreshKey, setRefreshKey] = useState(0);
  const [loading, setLoading] = useState(false);
  const [lastUpdate, setLastUpdate] = useState(null);
  const { t } = useTranslation();

  return (
    <div className="container mx-auto px-4 py-6 space-y-4">
      {/* 第一行（与战壕页面风格一致）：左侧标题+描述，右侧最近更新时间 */}
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-3xl font-bold text-foreground mb-2">Liquidity</h1>
          <p className="text-muted-foreground">发现链上流动性池</p>
        </div>
        <div className="flex items-center space-x-4 text-sm text-muted-foreground">
          {lastUpdate && (
            <div className="flex items-center space-x-2">
              <Clock className="h-4 w-4" />
              <span>最后更新: {lastUpdate.toLocaleTimeString()}</span>
            </div>
          )}
        </div>
      </div>
      {/* 第二行：左侧 CPMM/CLMM，右侧 Refresh + Create */}
      <div className="flex items-center justify-between gap-3">
        <VersionSelector value={version} onChange={setVersion} />
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRefreshKey((k) => k + 1)}
            disabled={loading}
            className="flex items-center space-x-2"
          >
            <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
            <span>{t('tokenList.buttons.refresh')}</span>
          </Button>
          <Button onClick={() => onNavigateToPoolCreation?.(version === 'V2' ? 'clmm' : 'cpmm')}>
            Create
          </Button>
        </div>
      </div>
      <PoolsTable version={version} refreshKey={refreshKey}
        onNavigateCharts={onNavigateCharts}
        onNavigateSwap={onNavigateSwap}
        onNavigateAddLiquidity={onNavigateAddLiquidity}
        onLoadingChange={setLoading}
        onDataUpdate={setLastUpdate}
      />
    </div>
  );
};

export default PoolsPage;
