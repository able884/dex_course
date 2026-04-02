import * as React from "react"
import { cva } from "class-variance-authority"
import { cn } from "../../lib/utils"

const cardVariants = cva(
  "rounded-lg border text-card-foreground shadow-sm transition-all duration-200",
  {
    variants: {
      variant: {
        default: "bg-card border-border",
        elevated: "bg-card border-border shadow-md hover:shadow-lg",
        glass: "glass border-white/20",
        gradient: "bg-gradient-to-br from-card to-muted border-border",
        outline: "border-2 border-border bg-transparent",
        ghost: "border-transparent bg-transparent shadow-none",
      },
      padding: {
        none: "",
        sm: "p-4",
        default: "p-6",
        lg: "p-8",
      },
      interactive: {
        true: "cursor-pointer hover:shadow-lg",
        false: "",
      },
    },
    defaultVariants: {
      variant: "default",
      padding: "default",
      interactive: false,
    },
  }
)

const Card = React.forwardRef(({ className, variant, padding, interactive, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(cardVariants({ variant, padding, interactive, className }))}
    {...props}
  />
))
Card.displayName = "Card"

const CardHeader = React.forwardRef(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn("flex flex-col space-y-1.5 p-6", className)}
    {...props}
  />
))
CardHeader.displayName = "CardHeader"

const CardTitle = React.forwardRef(({ className, children, ...props }, ref) => (
  <h3
    ref={ref}
    className={cn(
      "text-2xl font-semibold leading-none tracking-tight",
      className
    )}
    {...props}
  >
    {children}
  </h3>
))
CardTitle.displayName = "CardTitle"

const CardDescription = React.forwardRef(({ className, ...props }, ref) => (
  <p
    ref={ref}
    className={cn("text-sm text-muted-foreground", className)}
    {...props}
  />
))
CardDescription.displayName = "CardDescription"

const CardContent = React.forwardRef(({ className, ...props }, ref) => (
  <div ref={ref} className={cn("p-6 pt-0", className)} {...props} />
))
CardContent.displayName = "CardContent"

const CardFooter = React.forwardRef(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn("flex items-center p-6 pt-0", className)}
    {...props}
  />
))
CardFooter.displayName = "CardFooter"

// 特殊用途的卡片组件
const TokenCard = React.forwardRef(({ className, token, onClick, ...props }, ref) => (
  <Card
    ref={ref}
    variant="elevated"
    interactive={!!onClick}
    className={cn("overflow-hidden", className)}
    onClick={onClick}
    {...props}
  >
    <CardContent className="p-4">
      <div className="flex items-center space-x-3">
        {token?.icon && (
          <img
            src={token.icon}
            alt={token.name}
            className="w-10 h-10 rounded-full"
            onError={(e) => {
              e.target.src = '/default-token.png';
            }}
          />
        )}
        <div className="flex-1 min-w-0">
          <h4 className="font-semibold text-sm truncate">{token?.name || 'Unknown Token'}</h4>
          <p className="text-xs text-muted-foreground truncate">{token?.symbol || 'N/A'}</p>
        </div>
        {token?.price && (
          <div className="text-right">
            <p className="font-semibold text-sm">${token.price}</p>
            {token?.change24h !== undefined && (
              <p className={cn(
                "text-xs",
                token.change24h >= 0 ? "text-success" : "text-error"
              )}>
                {token.change24h >= 0 ? '+' : ''}{token.change24h.toFixed(2)}%
              </p>
            )}
          </div>
        )}
      </div>
    </CardContent>
  </Card>
))
TokenCard.displayName = "TokenCard"

const StatsCard = React.forwardRef(({ className, title, value, change, icon, ...props }, ref) => (
  <Card
    ref={ref}
    variant="elevated"
    className={cn("overflow-hidden", className)}
    {...props}
  >
    <CardContent className="p-6">
      <div className="flex items-center justify-between">
        <div className="flex-1">
          <p className="text-sm font-medium text-muted-foreground">{title}</p>
          <p className="text-2xl font-bold">{value}</p>
          {change !== undefined && (
            <p className={cn(
              "text-xs flex items-center mt-1",
              change >= 0 ? "text-success" : "text-error"
            )}>
              {change >= 0 ? '↗' : '↘'} {Math.abs(change).toFixed(2)}%
            </p>
          )}
        </div>
        {icon && (
          <div className="text-muted-foreground">
            {icon}
          </div>
        )}
      </div>
    </CardContent>
  </Card>
))
StatsCard.displayName = "StatsCard"

// 为了向后兼容，添加别名导出
const EnhancedCard = Card;
const GlassCard = React.forwardRef(({ className, ...props }, ref) => (
  <Card 
    ref={ref}
    variant="glass" 
    className={cn("backdrop-blur-sm bg-card/80 border-border/50", className)} 
    {...props} 
  />
));
GlassCard.displayName = "GlassCard";

export { 
  Card, 
  CardHeader, 
  CardFooter, 
  CardTitle, 
  CardDescription, 
  CardContent,
  TokenCard,
  StatsCard,
  cardVariants,
  EnhancedCard,
  GlassCard
}