import { cn } from "@/lib/utils";

// Skeleton is a visual placeholder rendered while data is loading. Keeps
// page layout stable so we don't snap-shift content when the fetch lands.
function Skeleton({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      data-slot="skeleton"
      className={cn("animate-pulse rounded-md bg-muted", className)}
      {...props}
    />
  );
}

export { Skeleton };
