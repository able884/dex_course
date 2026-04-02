import * as React from "react"
import { cva } from "class-variance-authority"
import { X, CheckCircle, AlertCircle, AlertTriangle, Info } from "lucide-react"
import { cn } from "../../lib/utils"

const toastVariants = cva(
  "group pointer-events-auto relative flex w-full items-center justify-between space-x-4 overflow-hidden rounded-md border p-6 pr-8 shadow-lg transition-all data-[swipe=cancel]:translate-x-0 data-[swipe=end]:translate-x-[var(--radix-toast-swipe-end-x)] data-[swipe=move]:translate-x-[var(--radix-toast-swipe-move-x)] data-[swipe=move]:transition-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[swipe=end]:animate-out data-[state=closed]:fade-out-80 data-[state=closed]:slide-out-to-right-full data-[state=open]:slide-in-from-top-full data-[state=open]:sm:slide-in-from-bottom-full",
  {
    variants: {
      variant: {
        default: "border bg-background text-foreground",
        success: "border-success/50 bg-success/10 text-success-foreground",
        error: "border-error/50 bg-error/10 text-error-foreground",
        warning: "border-warning/50 bg-warning/10 text-warning-foreground",
        info: "border-info/50 bg-info/10 text-info-foreground",
        solana: "border-solana/50 bg-gradient-to-r from-solana/10 to-solana-secondary/10 text-foreground",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
)

const ToastProvider = ({ children }) => {
  return children
}

const ToastViewport = React.forwardRef(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      "fixed top-0 z-[100] flex max-h-screen w-full flex-col-reverse p-4 sm:bottom-0 sm:right-0 sm:top-auto sm:flex-col md:max-w-[420px]",
      className
    )}
    {...props}
  />
))
ToastViewport.displayName = "ToastViewport"

const Toast = React.forwardRef(({ className, variant, ...props }, ref) => {
  return (
    <div
      ref={ref}
      className={cn(toastVariants({ variant }), className)}
      {...props}
    />
  )
})
Toast.displayName = "Toast"

const ToastAction = React.forwardRef(({ className, ...props }, ref) => (
  <button
    ref={ref}
    className={cn(
      "inline-flex h-8 shrink-0 items-center justify-center rounded-md border bg-transparent px-3 text-sm font-medium ring-offset-background transition-colors hover:bg-secondary focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 group-[.destructive]:border-muted/40 group-[.destructive]:hover:border-destructive/30 group-[.destructive]:hover:bg-destructive group-[.destructive]:hover:text-destructive-foreground group-[.destructive]:focus:ring-destructive",
      className
    )}
    {...props}
  />
))
ToastAction.displayName = "ToastAction"

const ToastClose = React.forwardRef(({ className, ...props }, ref) => (
  <button
    ref={ref}
    className={cn(
      "absolute right-2 top-2 rounded-md p-1 text-foreground/50 opacity-0 transition-opacity hover:text-foreground focus:opacity-100 focus:outline-none focus:ring-2 group-hover:opacity-100 group-[.destructive]:text-red-300 group-[.destructive]:hover:text-red-50 group-[.destructive]:focus:ring-red-400 group-[.destructive]:focus:ring-offset-red-600",
      className
    )}
    toast-close=""
    {...props}
  >
    <X className="h-4 w-4" />
  </button>
))
ToastClose.displayName = "ToastClose"

const ToastTitle = React.forwardRef(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn("text-sm font-semibold", className)}
    {...props}
  />
))
ToastTitle.displayName = "ToastTitle"

const ToastDescription = React.forwardRef(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn("text-sm opacity-90", className)}
    {...props}
  />
))
ToastDescription.displayName = "ToastDescription"

// 增强的 Toast 组件，带图标
const EnhancedToast = React.forwardRef(({ 
  className, 
  variant = "default", 
  title, 
  description, 
  action,
  onClose,
  showIcon = true,
  ...props 
}, ref) => {
  const getIcon = () => {
    switch (variant) {
      case "success":
        return <CheckCircle className="h-5 w-5 text-success" />
      case "error":
        return <AlertCircle className="h-5 w-5 text-error" />
      case "warning":
        return <AlertTriangle className="h-5 w-5 text-warning" />
      case "info":
        return <Info className="h-5 w-5 text-info" />
      case "solana":
        return (
          <div className="h-5 w-5 rounded-full bg-gradient-to-r from-solana to-solana-secondary" />
        )
      default:
        return <Info className="h-5 w-5 text-muted-foreground" />
    }
  }

  return (
    <Toast ref={ref} className={className} variant={variant} {...props}>
      <div className="flex items-start space-x-3">
        {showIcon && (
          <div className="flex-shrink-0 mt-0.5">
            {getIcon()}
          </div>
        )}
        <div className="flex-1 space-y-1">
          {title && <ToastTitle>{title}</ToastTitle>}
          {description && <ToastDescription>{description}</ToastDescription>}
        </div>
      </div>
      {action && <ToastAction>{action}</ToastAction>}
      {onClose && <ToastClose onClick={onClose} />}
    </Toast>
  )
})
EnhancedToast.displayName = "EnhancedToast"

// Toast Hook
const useToast = () => {
  const [toasts, setToasts] = React.useState([])

  const addToast = React.useCallback((toast) => {
    const id = Math.random().toString(36).substr(2, 9)
    const newToast = { ...toast, id }
    
    setToasts((prev) => [...prev, newToast])
    
    // 自动移除 toast
    if (toast.duration !== 0) {
      setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== id))
      }, toast.duration || 5000)
    }
    
    return id
  }, [])

  const removeToast = React.useCallback((id) => {
    setToasts((prev) => prev.filter((toast) => toast.id !== id))
  }, [])

  const toast = React.useCallback((props) => {
    return addToast(props)
  }, [addToast])

  // 便捷方法
  toast.success = (title, description, options = {}) => 
    addToast({ variant: "success", title, description, ...options })
  
  toast.error = (title, description, options = {}) => 
    addToast({ variant: "error", title, description, ...options })
  
  toast.warning = (title, description, options = {}) => 
    addToast({ variant: "warning", title, description, ...options })
  
  toast.info = (title, description, options = {}) => 
    addToast({ variant: "info", title, description, ...options })
  
  toast.solana = (title, description, options = {}) => 
    addToast({ variant: "solana", title, description, ...options })

  return {
    toasts,
    toast,
    addToast,
    removeToast,
  }
}

// Toast 容器组件
const ToastContainer = () => {
  const { toasts, removeToast } = useToast()

  return (
    <ToastViewport>
      {toasts.map((toast) => (
        <EnhancedToast
          key={toast.id}
          variant={toast.variant}
          title={toast.title}
          description={toast.description}
          action={toast.action}
          onClose={() => removeToast(toast.id)}
          showIcon={toast.showIcon}
        />
      ))}
    </ToastViewport>
  )
}

export {
  type ToastProps,
  ToastProvider,
  ToastViewport,
  Toast,
  ToastTitle,
  ToastDescription,
  ToastClose,
  ToastAction,
  EnhancedToast,
  ToastContainer,
  useToast,
  toastVariants,
}