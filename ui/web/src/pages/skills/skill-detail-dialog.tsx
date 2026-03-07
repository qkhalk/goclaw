import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { MarkdownRenderer } from "@/components/shared/markdown-renderer";
import type { SkillInfo } from "./hooks/use-skills";

interface SkillDetailDialogProps {
  skill: SkillInfo & { content: string };
  onClose: () => void;
}

export function SkillDetailDialog({ skill, onClose }: SkillDetailDialogProps) {
  return (
    <Dialog open onOpenChange={() => onClose()}>
      <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {skill.name}
            <Badge variant="outline">{skill.source || "file"}</Badge>
            {skill.visibility && (
              <Badge variant="secondary">{skill.visibility}</Badge>
            )}
            {skill.version ? (
              <span className="text-xs font-normal text-muted-foreground">v{skill.version}</span>
            ) : null}
          </DialogTitle>
          {skill.description && (
            <p className="text-sm text-muted-foreground">{skill.description}</p>
          )}
          {skill.tags && skill.tags.length > 0 && (
            <div className="flex flex-wrap gap-1 pt-1">
              {skill.tags.map((tag) => (
                <Badge key={tag} variant="outline" className="text-xs">{tag}</Badge>
              ))}
            </div>
          )}
        </DialogHeader>
        <div className="mt-2">
          {skill.content ? (
            <div className="overflow-hidden rounded-md border bg-muted/30 p-4">
              <MarkdownRenderer content={skill.content} />
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">No content available.</p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
