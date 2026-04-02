import React from 'react';
import { Button } from '../UI/Button';

// CPMM = V1, CLMM = V2
const VersionSelector = ({ value, onChange }) => {
  const isV2 = value === 'V2';
  return (
    <div className="inline-flex items-center gap-2" role="group" aria-label="Pool Version">
      <Button
        variant={value === 'V1' ? 'default' : 'outline'}
        size="sm"
        onClick={() => onChange('V1')}
      >
        CPMM
      </Button>
      <Button
        variant={isV2 ? 'default' : 'outline'}
        size="sm"
        onClick={() => onChange('V2')}
      >
        CLMM
      </Button>
    </div>
  );
};

export default VersionSelector;
