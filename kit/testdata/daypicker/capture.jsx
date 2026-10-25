import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { writeFileSync, mkdirSync } from "node:fs";
import { Calendar } from "@/registry/new-york-v4/ui/calendar";

// Renders shadcn/ui's Calendar (new-york-v4 ui/calendar.tsx at the pinned
// registry version, unmodified) with react-day-picker 9.14.0 to static
// markup, one file per scenario in the directory given as the first
// argument. "Today" is fixed at 2026-10-25.
//
//	npm i react@19.2.3 react-dom@19.2.3 react-day-picker@9.14.0 lucide-react@0.474.0 \
//	    esbuild clsx tailwind-merge class-variance-authority @radix-ui/react-slot
//	esbuild capture.jsx --bundle --platform=node --format=esm --outfile=capture.mjs --jsx=automatic \
//	    --alias:@/registry/new-york-v4/ui/button=./button.tsx \
//	    --alias:@/registry/new-york-v4/ui/calendar=./calendar.tsx \
//	    --alias:cn=./cn.js --alias:radix-ui=./radix-shim.js
//	node capture.mjs out
const d = (y, m, day) => new Date(y, m - 1, day);
const today = d(2026, 10, 25);
const scenarios = {
  single: { month: d(2026, 10, 1), mode: "single", selected: d(2026, 10, 14) },
  range: { month: d(2026, 10, 1), mode: "range", selected: { from: d(2026, 10, 12), to: d(2026, 10, 18) } },
  two: { month: d(2026, 10, 1), mode: "range", numberOfMonths: 2, selected: { from: d(2026, 10, 20), to: d(2026, 11, 5) } },
  limits: {
    month: d(2026, 10, 1),
    mode: "single",
    selected: d(2026, 10, 20),
    startMonth: d(2026, 10, 1),
    endMonth: d(2026, 10, 1),
    disabled: [{ before: d(2026, 10, 8) }, { after: d(2026, 10, 28) }, d(2026, 10, 15)],
  },
  fixed: { month: d(2026, 2, 1), mode: "single", fixedWeeks: true },
  nooutside: { month: d(2026, 10, 1), mode: "single", showOutsideDays: false, selected: d(2026, 10, 14) },
  monday: { month: d(2026, 10, 1), mode: "single", weekStartsOn: 1, selected: d(2026, 10, 25) },
};
const out = process.argv[2];
mkdirSync(out, { recursive: true });
for (const [name, props] of Object.entries(scenarios)) {
  const html = renderToStaticMarkup(<Calendar today={today} {...props} />);
  writeFileSync(`${out}/${name}.html`, html + "\n");
}
