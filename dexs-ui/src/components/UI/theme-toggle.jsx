import React from 'react';
import { Moon, Sun } from 'lucide-react';
import { Button } from './Button.jsx';
import { useThemeContext } from './theme-provider';
import { cn } from '../../lib/utils';

export function ThemeToggle({ className, ...props }) {
  const { theme, setTheme, themes } = useThemeContext();

  const toggleTheme = () => {
    if (theme === themes.LIGHT) {
      setTheme(themes.DARK);
    } else {
      setTheme(themes.LIGHT);
    }
  };

  const getIcon = () => {
    switch (theme) {
      case themes.LIGHT:
        return <Sun className="h-4 w-4" />;
      case themes.DARK:
        return <Moon className="h-4 w-4" />;
      default:
        return <Sun className="h-4 w-4" />;
    }
  };

  const getLabel = () => {
    switch (theme) {
      case themes.LIGHT:
        return '明亮模式';
      case themes.DARK:
        return '暗色模式';
      default:
        return '切换主题';
    }
  };

  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={toggleTheme}
      className={cn('h-9 w-9', className)}
      title={getLabel()}
      {...props}
    >
      {getIcon()}
      <span className="sr-only">{getLabel()}</span>
    </Button>
  );
}

export function ThemeSelector({ className, ...props }) {
  const { theme, setTheme, themes } = useThemeContext();

  const themeOptions = [
    { value: themes.LIGHT, label: '明亮', icon: Sun },
    { value: themes.DARK, label: '暗色', icon: Moon },
  ];

  return (
    <div className={cn('flex items-center space-x-1 rounded-md border p-1', className)} {...props}>
      {themeOptions.map(({ value, label, icon: Icon }) => (
        <Button
          key={value}
          variant={theme === value ? 'default' : 'ghost'}
          size="sm"
          onClick={() => setTheme(value)}
          className="h-8 px-2 text-xs"
        >
          <Icon className="h-3 w-3 mr-1" />
          {label}
        </Button>
      ))}
    </div>
  );
}