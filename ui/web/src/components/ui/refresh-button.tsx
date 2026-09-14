import { useState, useCallback } from "react";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface RefreshButtonProps {
  /** Callback when the refresh button is clicked. */
  onRefresh: () => void;
  /** Optional externally-controlled refreshing state (e.g. a query's
   * isFetching). When provided, the icon spins while true (in addition to the
   * local 600ms spin) and clicks are ignored while true. Omit for the
   * default fixed-600ms behavior. */
  refreshing?: boolean;
  /** Accessible label text (should be i18n'd by caller). */
  label?: string;
  /** Additional className for the button. */
  className?: string;
  /** Button variant (default: ghost). */
  variant?: "ghost" | "outline" | "default" | "secondary" | "destructive";
  /** Button size (default: icon). */
  size?: "default" | "sm" | "lg" | "icon" | "icon-sm";
  /** Whether the button is disabled. */
  disabled?: boolean;
}

/** Universal refresh button with a 600ms spin animation on click.
 *  Used site-wide for any refresh/reload action. */
export function RefreshButton({
  onRefresh,
  refreshing,
  label,
  className,
  variant = "ghost",
  size = "icon",
  disabled,
}: RefreshButtonProps) {
  const [spinning, setSpinning] = useState(false);

  const handleClick = useCallback(() => {
    if (spinning || refreshing || disabled) return;
    setSpinning(true);
    onRefresh();
    // Spin for 600ms then stop (covers typical network round-trip)
    setTimeout(() => setSpinning(false), 600);
  }, [onRefresh, spinning, refreshing, disabled]);

  return (
    <Button
      variant={variant}
      size={size}
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={handleClick}
      className={cn(
        "[&_svg]:transition-transform [&_svg]:duration-600 [&_svg]:ease-in-out",
        (spinning || refreshing) && "[&_svg]:animate-spin",
        className,
      )}
    >
      <RefreshCw className="h-4 w-4" />
    </Button>
  );
}
