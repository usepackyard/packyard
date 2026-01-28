import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/utils";

interface CopyButtonProps {
  value: string;
  className?: string;
  "aria-label"?: string;
}

// CopyButton writes a string to the clipboard and briefly shows a check
// icon as feedback. Styled as an icon-only ghost button so it drops into
// any inline context (next to a URL, inside a code block, …).
export function CopyButton({ value, className, ...props }: CopyButtonProps) {
  const [copied, setCopied] = useState(false);

  const handleClick = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Some browsers refuse clipboard writes from insecure contexts or
      // when the document isn't focused. Nothing useful we can do — the
      // user can still select the text manually.
    }
  };

  return (
    <button
      type="button"
      onClick={handleClick}
      aria-label={props["aria-label"] ?? "Copy to clipboard"}
      className={cn(
        "inline-flex h-6 w-6 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground transition-colors",
        className
      )}
    >
      {copied ? <Check className="h-3.5 w-3.5 text-green-600" /> : <Copy className="h-3.5 w-3.5" />}
    </button>
  );
}
