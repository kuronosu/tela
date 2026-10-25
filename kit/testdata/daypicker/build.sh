#!/bin/sh
set -e
cd "$(dirname "$0")"
cp ../calendar.tsx ../button.tsx .
./node_modules/.bin/esbuild capture.jsx --bundle --platform=node --format=cjs --outfile=capture.cjs --jsx=automatic \
  --alias:@/registry/new-york-v4/ui/button=./button.tsx \
  --alias:@/registry/new-york-v4/ui/calendar=./calendar.tsx \
  --alias:cn=./cn.js --alias:radix-ui=./radix-shim.js
node capture.cjs out
ls out
