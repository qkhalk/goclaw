import { useState, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

interface ConfirmDeleteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  confirmValue: string;
  confirmLabel?: string;
  onConfirm: () => void;
  loading?: boolean;
}

export function ConfirmDeleteDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmValue,
  confirmLabel = "Delete",
  onConfirm,
  loading,
}: ConfirmDeleteDialogProps) {
  const [inputValue, setInputValue] = useState("");

  useEffect(() => {
    if (!open) setInputValue("");
  }, [open]);

  const isMatch = inputValue.toLowerCase() === confirmValue.toLowerCase();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="py-2">
          <p className="mb-2 text-sm text-muted-foreground">
            Type <span className="font-semibold text-foreground">{confirmValue}</span> to confirm.
          </p>
          <Input
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            placeholder={confirmValue}
            autoFocus
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={loading}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={onConfirm}
            disabled={!isMatch || loading}
          >
            {loading ? "..." : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
