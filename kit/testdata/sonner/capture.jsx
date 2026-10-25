import React from "react";
import { createRoot } from "react-dom/client";
import { toast } from "sonner";
import { Toaster as Sonner } from "sonner";
import { CircleCheckIcon, InfoIcon, Loader2Icon, OctagonXIcon, TriangleAlertIcon } from "lucide-react";

// shadcn/ui's components/ui/sonner.tsx (new-york-v4 at the pinned commit),
// with the theme given as a prop instead of next-themes' useTheme.
const Toaster = (props) => (
  <Sonner
    className="toaster group"
    icons={{
      success: <CircleCheckIcon className="size-4" />,
      info: <InfoIcon className="size-4" />,
      warning: <TriangleAlertIcon className="size-4" />,
      error: <OctagonXIcon className="size-4" />,
      loading: <Loader2Icon className="size-4 animate-spin" />,
    }}
    style={{
      "--normal-bg": "var(--popover)",
      "--normal-text": "var(--popover-foreground)",
      "--normal-border": "var(--border)",
      "--border-radius": "var(--radius)",
    }}
    {...props}
  />
);

const scenarios = {
  stack: {
    props: {},
    toasts: [
      ["", "Event has been created", { description: "Sunday, December 03, 2023 at 9:00 AM" }],
      ["success", "Saved", { description: "Your changes were saved." }],
      ["error", "Something went wrong", {}],
      ["", "Undo available", { action: { label: "Undo", onClick: () => {} } }],
    ],
  },
  expanded: {
    props: { expand: true },
    toasts: [
      ["", "Event has been created", { description: "Sunday, December 03, 2023 at 9:00 AM" }],
      ["success", "Saved", { description: "Your changes were saved." }],
      ["error", "Something went wrong", {}],
      ["", "Undo available", { action: { label: "Undo", onClick: () => {} } }],
    ],
  },
  kinds: {
    props: { expand: true, visibleToasts: 4 },
    toasts: [
      ["loading", "Uploading file", {}],
      ["warning", "Low disk space", { description: "Only 2 GB are left." }],
      ["info", "A new version is available", {}],
      ["success", "Profile updated", {}],
    ],
  },
  single: {
    props: {},
    toasts: [["", "Event has been created", { description: "Sunday, December 03, 2023 at 9:00 AM" }]],
  },
  top: {
    props: { position: "top-center" },
    toasts: [
      ["success", "Saved", { description: "Your changes were saved." }],
      ["", "Undo available", { action: { label: "Undo", onClick: () => {} } }],
    ],
  },
  close: {
    props: { closeButton: true, expand: true },
    toasts: [
      ["", "Event has been created", { description: "Sunday, December 03, 2023 at 9:00 AM" }],
      ["error", "Something went wrong", {}],
    ],
  },
};

const q = new URLSearchParams(location.search);
const sc = scenarios[q.get("s") || "stack"];
createRoot(document.getElementById("root")).render(
  <Toaster theme={q.get("theme") || "light"} duration={Infinity} {...sc.props} />
);
setTimeout(() => {
  for (const [kind, title, opts] of sc.toasts) {
    if (kind) toast[kind](title, opts);
    else toast(title, opts);
  }
}, 50);
setTimeout(() => {
  document.getElementById("out").textContent = "BEGIN" + document.querySelector("section").outerHTML + "END";
}, 1500);
