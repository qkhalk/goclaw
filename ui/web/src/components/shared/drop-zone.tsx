import { useRef, useState } from "react";
import { Upload } from "lucide-react";

interface DropZoneProps {
  onDrop: (files: File[]) => void;
  children: React.ReactNode;
  /** Overlay text shown while dragging (i18n belongs to the caller). */
  title?: string;
}

/** Drag-and-drop overlay for file uploads. Uses a counter to handle child
 * boundary events. Shared: used by chat and the cloud drive file area. */
export function DropZone({ onDrop, children, title }: DropZoneProps) {
  const [isDragging, setIsDragging] = useState(false);
  const dragCounterRef = useRef(0);

  return (
    <div
      className="relative flex h-full min-h-0 flex-1 flex-col"
      onDragOver={(e) => e.preventDefault()}
      onDragEnter={(e) => {
        e.preventDefault();
        dragCounterRef.current++;
        if (dragCounterRef.current === 1) setIsDragging(true);
      }}
      onDragLeave={() => {
        dragCounterRef.current--;
        if (dragCounterRef.current === 0) setIsDragging(false);
      }}
      onDrop={(e) => {
        e.preventDefault();
        dragCounterRef.current = 0;
        setIsDragging(false);
        const files = Array.from(e.dataTransfer.files);
        if (files.length > 0) onDrop(files);
      }}
    >
      {children}

      {isDragging && (
        <div className="absolute inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm">
          <div className="flex flex-col items-center gap-2 text-muted-foreground">
            <Upload className="h-10 w-10" />
            {title && <span className="text-lg font-medium">{title}</span>}
          </div>
        </div>
      )}
    </div>
  );
}
