import * as React from "react"
import { cva } from "class-variance-authority"
import { cn } from "../../lib/utils"

const inputVariants = cva(
  "flex w-full rounded-md border bg-background text-sm ring-offset-background file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 transition-all duration-200",
  {
    variants: {
      variant: {
        default: "border-input hover:border-ring/50",
        success: "border-success focus-visible:ring-success",
        warning: "border-warning focus-visible:ring-warning",
        error: "border-error focus-visible:ring-error",
        ghost: "border-transparent bg-muted hover:bg-background hover:border-input",
      },
      size: {
        sm: "h-8 px-2 py-1 text-xs",
        default: "h-10 px-3 py-2",
        lg: "h-12 px-4 py-3 text-base",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

const EnhancedInput = React.forwardRef(({ className, variant, size, type, ...props }, ref) => {
  return (
    <input
      type={type}
      className={cn(inputVariants({ variant, size, className }))}
      ref={ref}
      {...props}
    />
  )
})
EnhancedInput.displayName = "EnhancedInput"

// 带图标的输入框组件
const InputWithIcon = React.forwardRef(({ 
  className, 
  variant, 
  size, 
  leftIcon, 
  rightIcon, 
  onRightIconClick,
  ...props 
}, ref) => {
  return (
    <div className="relative">
      {leftIcon && (
        <div className="absolute left-3 top-1/2 transform -translate-y-1/2 text-muted-foreground">
          {leftIcon}
        </div>
      )}
      <EnhancedInput
        ref={ref}
        className={cn(
          leftIcon && "pl-10",
          rightIcon && "pr-10",
          className
        )}
        variant={variant}
        size={size}
        {...props}
      />
      {rightIcon && (
        <div 
          className={cn(
            "absolute right-3 top-1/2 transform -translate-y-1/2 text-muted-foreground",
            onRightIconClick && "cursor-pointer hover:text-foreground transition-colors"
          )}
          onClick={onRightIconClick}
        >
          {rightIcon}
        </div>
      )}
    </div>
  )
})
InputWithIcon.displayName = "InputWithIcon"

// 搜索输入框组件
const SearchInput = React.forwardRef(({ 
  className, 
  placeholder = "搜索...", 
  onClear,
  value,
  onChange,
  ...props 
}, ref) => {
  const [searchValue, setSearchValue] = React.useState(value || "")

  React.useEffect(() => {
    setSearchValue(value || "")
  }, [value])

  const handleChange = (e) => {
    const newValue = e.target.value
    setSearchValue(newValue)
    onChange?.(e)
  }

  const handleClear = () => {
    setSearchValue("")
    onClear?.()
    // 创建一个模拟的事件对象
    const mockEvent = {
      target: { value: "" },
      currentTarget: { value: "" }
    }
    onChange?.(mockEvent)
  }

  return (
    <InputWithIcon
      ref={ref}
      className={className}
      placeholder={placeholder}
      value={searchValue}
      onChange={handleChange}
      leftIcon={
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
      }
      rightIcon={searchValue && (
        <svg 
          className="w-4 h-4" 
          fill="none" 
          stroke="currentColor" 
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
        </svg>
      )}
      onRightIconClick={searchValue ? handleClear : undefined}
      {...props}
    />
  )
})
SearchInput.displayName = "SearchInput"

// 数字输入框组件
const NumberInput = React.forwardRef(({ 
  className, 
  min, 
  max, 
  step = 1,
  onIncrement,
  onDecrement,
  value,
  onChange,
  ...props 
}, ref) => {
  const [inputValue, setInputValue] = React.useState(value || props.defaultValue || 0)

  React.useEffect(() => {
    setInputValue(value || 0)
  }, [value])

  const handleChange = (e) => {
    const newValue = e.target.value
    setInputValue(newValue)
    onChange?.(e)
  }

  const handleIncrement = () => {
    const newValue = Number(inputValue) + Number(step)
    if (max === undefined || newValue <= max) {
      setInputValue(newValue)
      onIncrement?.(newValue)
      // 创建模拟事件
      const mockEvent = {
        target: { value: newValue.toString() },
        currentTarget: { value: newValue.toString() }
      }
      onChange?.(mockEvent)
    }
  }

  const handleDecrement = () => {
    const newValue = Number(inputValue) - Number(step)
    if (min === undefined || newValue >= min) {
      setInputValue(newValue)
      onDecrement?.(newValue)
      // 创建模拟事件
      const mockEvent = {
        target: { value: newValue.toString() },
        currentTarget: { value: newValue.toString() }
      }
      onChange?.(mockEvent)
    }
  }

  return (
    <div className="relative">
      <EnhancedInput
        ref={ref}
        type="number"
        className={cn("pr-16", className)}
        min={min}
        max={max}
        step={step}
        value={inputValue}
        onChange={handleChange}
        {...props}
      />
      <div className="absolute right-1 top-1/2 transform -translate-y-1/2 flex flex-col">
        <button
          type="button"
          className="px-2 py-0.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
          onClick={handleIncrement}
        >
          ▲
        </button>
        <button
          type="button"
          className="px-2 py-0.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
          onClick={handleDecrement}
        >
          ▼
        </button>
      </div>
    </div>
  )
})
NumberInput.displayName = "NumberInput"

// 表单字段组件
const FormField = React.forwardRef(({ 
  className,
  label,
  error,
  hint,
  required,
  children,
  ...props 
}, ref) => {
  return (
    <div className={cn("space-y-2", className)} {...props}>
      {label && (
        <label className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
          {label}
          {required && <span className="text-error ml-1">*</span>}
        </label>
      )}
      <div ref={ref}>
        {children}
      </div>
      {error && (
        <p className="text-sm text-error">{error}</p>
      )}
      {hint && !error && (
        <p className="text-sm text-muted-foreground">{hint}</p>
      )}
    </div>
  )
})
FormField.displayName = "FormField"

export { 
  EnhancedInput, 
  InputWithIcon, 
  SearchInput, 
  NumberInput, 
  FormField,
  inputVariants 
}