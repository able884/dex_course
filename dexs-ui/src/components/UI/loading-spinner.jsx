import * as React from "react"
import { cva } from "class-variance-authority"
import { cn } from "../../lib/utils"

const spinnerVariants = cva(
  "animate-spin rounded-full border-solid border-current border-r-transparent",
  {
    variants: {
      size: {
        sm: "h-4 w-4 border-2",
        default: "h-6 w-6 border-2",
        lg: "h-8 w-8 border-2",
        xl: "h-12 w-12 border-4",
      },
      variant: {
        default: "text-primary",
        secondary: "text-secondary",
        muted: "text-muted-foreground",
        success: "text-success",
        warning: "text-warning",
        error: "text-error",
        solana: "text-solana",
      },
    },
    defaultVariants: {
      size: "default",
      variant: "default",
    },
  }
)

const LoadingSpinner = React.forwardRef(({ 
  className, 
  size, 
  variant,
  ...props 
}, ref) => {
  return (
    <div
      ref={ref}
      className={cn(spinnerVariants({ size, variant }), className)}
      {...props}
    />
  )
})
LoadingSpinner.displayName = "LoadingSpinner"

// 带文本的加载组件
const LoadingWithText = React.forwardRef(({ 
  className,
  size,
  variant,
  text = "加载中...",
  direction = "vertical",
  ...props 
}, ref) => {
  return (
    <div
      ref={ref}
      className={cn(
        "flex items-center gap-2",
        direction === "vertical" ? "flex-col" : "flex-row",
        className
      )}
      {...props}
    >
      <LoadingSpinner size={size} variant={variant} />
      <span className="text-sm text-muted-foreground">{text}</span>
    </div>
  )
})
LoadingWithText.displayName = "LoadingWithText"

// 页面级加载组件
const PageLoading = React.forwardRef(({ 
  className,
  text = "页面加载中...",
  ...props 
}, ref) => {
  return (
    <div
      ref={ref}
      className={cn(
        "flex flex-col items-center justify-center min-h-[200px] space-y-4",
        className
      )}
      {...props}
    >
      <LoadingSpinner size="xl" variant="solana" />
      <p className="text-lg text-muted-foreground">{text}</p>
    </div>
  )
})
PageLoading.displayName = "PageLoading"

// 按钮加载状态
const ButtonLoading = React.forwardRef(({ 
  className,
  size = "sm",
  ...props 
}, ref) => {
  return (
    <LoadingSpinner
      ref={ref}
      size={size}
      className={cn("mr-2", className)}
      {...props}
    />
  )
})
ButtonLoading.displayName = "ButtonLoading"

// 骨架屏加载组件
const SkeletonLoader = React.forwardRef(({ 
  className,
  ...props 
}, ref) => {
  return (
    <div
      ref={ref}
      className={cn(
        "animate-pulse rounded-md bg-muted",
        className
      )}
      {...props}
    />
  )
})
SkeletonLoader.displayName = "SkeletonLoader"

// 卡片骨架屏
const CardSkeleton = React.forwardRef(({ className, ...props }, ref) => {
  return (
    <div
      ref={ref}
      className={cn("space-y-3 p-4", className)}
      {...props}
    >
      <SkeletonLoader className="h-4 w-3/4" />
      <SkeletonLoader className="h-4 w-1/2" />
      <SkeletonLoader className="h-20 w-full" />
      <div className="flex space-x-2">
        <SkeletonLoader className="h-8 w-16" />
        <SkeletonLoader className="h-8 w-16" />
      </div>
    </div>
  )
})
CardSkeleton.displayName = "CardSkeleton"

// 表格骨架屏
const TableSkeleton = React.forwardRef(({ 
  className,
  rows = 5,
  columns = 4,
  ...props 
}, ref) => {
  return (
    <div
      ref={ref}
      className={cn("space-y-2", className)}
      {...props}
    >
      {/* 表头 */}
      <div className="flex space-x-4 p-2">
        {Array.from({ length: columns }).map((_, i) => (
          <SkeletonLoader key={i} className="h-4 flex-1" />
        ))}
      </div>
      {/* 表格行 */}
      {Array.from({ length: rows }).map((_, rowIndex) => (
        <div key={rowIndex} className="flex space-x-4 p-2">
          {Array.from({ length: columns }).map((_, colIndex) => (
            <SkeletonLoader key={colIndex} className="h-4 flex-1" />
          ))}
        </div>
      ))}
    </div>
  )
})
TableSkeleton.displayName = "TableSkeleton"

export {
  LoadingSpinner,
  LoadingWithText,
  PageLoading,
  ButtonLoading,
  SkeletonLoader,
  CardSkeleton,
  TableSkeleton,
  spinnerVariants,
}