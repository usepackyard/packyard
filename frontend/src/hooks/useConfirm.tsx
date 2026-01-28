import { useCallback, useRef, useState, type ReactNode } from "react";
import { ConfirmDialog, type ConfirmDialogProps } from "@/components/ConfirmDialog";

type ConfirmOptions = Omit<ConfirmDialogProps, "open" | "onOpenChange" | "onConfirm">;

type State = { opts: ConfirmOptions } | null;

// useConfirm gives destructive call sites an imperative `await confirm(...)`
// that resolves true/false, while rendering a single styled modal per page.
// Usage:
//   const { confirm, dialog } = useConfirm();
//   if (!(await confirm({ title: "Delete?", description: "..." }))) return;
//   ...render {dialog} once in JSX.
export function useConfirm(): {
  confirm: (opts: ConfirmOptions) => Promise<boolean>;
  dialog: ReactNode;
} {
  const [state, setState] = useState<State>(null);
  const resolverRef = useRef<((value: boolean) => void) | null>(null);

  const settle = useCallback((value: boolean) => {
    const resolve = resolverRef.current;
    resolverRef.current = null;
    setState(null);
    if (resolve) resolve(value);
  }, []);

  const confirm = useCallback((opts: ConfirmOptions) => {
    // If a previous prompt is still open (shouldn't happen in normal flow),
    // resolve it as cancelled so we don't leak its Promise.
    if (resolverRef.current) resolverRef.current(false);
    setState({ opts });
    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
    });
  }, []);

  const dialog = state ? (
    <ConfirmDialog
      {...state.opts}
      open
      onOpenChange={(open) => { if (!open) settle(false); }}
      onConfirm={() => settle(true)}
    />
  ) : null;

  return { confirm, dialog };
}
