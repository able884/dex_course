import React, { createContext, useContext } from 'react';
import { useTheme } from '../../hooks/useTheme';

const ThemeProviderContext = createContext(undefined);

export function ThemeProvider({ children, ...props }) {
  const themeState = useTheme();

  return (
    <ThemeProviderContext.Provider value={themeState} {...props}>
      {children}
    </ThemeProviderContext.Provider>
  );
}

export const useThemeContext = () => {
  const context = useContext(ThemeProviderContext);

  if (context === undefined) {
    throw new Error('useThemeContext must be used within a ThemeProvider');
  }

  return context;
};