"use client";

// Tiny accessible dialog primitive. Uses the native `<dialog>` element
// with the `showModal()` method so we get focus-trapping and ESC-to-close
// for free, no headless-ui dependency.
//
// Caveat: native `<dialog>` requires a click-on-backdrop polyfill, which
// we implement by checking if the click target IS the dialog itself.

import { useEffect, useRef, type ReactNode } from "react";

interface DialogProps {
  open: boolean;
  onClose: () => void;
  title?: string;
  description?: string;
  children: ReactNode;
}

export function Dialog({ open, onClose, title, description, children }: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null);

  // Sync `open` prop with the imperative <dialog> API. We use `showModal`
  // (not `show`) so the browser handles focus trap + ESC + backdrop layer.
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  }, [open]);

  // Bridge the native `close` event back to React state so closing via
  // ESC stays in sync with the parent.
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const handler = () => onClose();
    el.addEventListener("close", handler);
    return () => el.removeEventListener("close", handler);
  }, [onClose]);

  return (
    <dialog
      ref={ref}
      // Click on the dialog *background* (not its content) closes it.
      onClick={(e) => {
        if (e.target === ref.current) onClose();
      }}
      className="m-auto w-full max-w-md rounded-2xl border border-zinc-200 bg-white p-0 backdrop:bg-black/40 backdrop:backdrop-blur-sm dark:border-zinc-800 dark:bg-zinc-950 open:animate-in open:fade-in"
    >
      <div className="p-6">
        {title && (
          <h2 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">
            {title}
          </h2>
        )}
        {description && (
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-400">{description}</p>
        )}
        <div className={title || description ? "mt-5" : ""}>{children}</div>
      </div>
    </dialog>
  );
}
