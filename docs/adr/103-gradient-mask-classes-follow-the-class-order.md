# ADR-103: Gradient-mask classes follow the class order

- **Date:** 2026-10-21
- **Status:** Accepted

## Decision

Gradient-mask classes follow the class order (phase 8, T4b; the owner's decision; amends ADR-098). Mask classes that set the same Tailwind variable follow §10: the class that comes last in the element's class order wins, variants included (a matching `hover:` class later in the string beats an earlier one and loses to a later plain class), not the one Tailwind emits later in its stylesheet. Each family variable takes the form of its last setter: `--tw-mask-linear` is the four edges (`mask-t/r/b/l/x/y-from/to-*`), the angle form `linear-gradient(var(--tw-mask-linear-stops, <angle>))` (`mask-linear-<angle>`, `mask-linear-[…]`) or the stops form (`mask-linear-from/to-*`); `--tw-mask-radial` the size form (`mask-radial-[size]`) or the stops form; `--tw-mask-conic` the angle form or the stops form. The stops variables are the same whichever from/to class sets them, so an angle or size form is valid when any from/to class of its family is present anywhere in the string, and the angle, size, shape, position and every stop color and position are plain variables where the last class wins. So `mask-linear-from-20% mask-b-from-50%` is the edge mask and `mask-b-from-50% mask-linear-from-20%` the linear one; `mask-b-from-50% mask-linear-45` is invalid (an angle without stops: no mask) and `mask-linear-45 mask-b-from-50%` the edge mask. Unchanged from ADR-098: a family that is invalid at computed-value time leaves the element unmasked, the families are intersected, and an explicit `mask-add`/`subtract`/`intersect`/`exclude` wins over the gradients' intersect in either order. No Chromium cell sets one variable twice, so every cell and golden is unchanged

## Rationale

One rule for every class is easier to predict and to explain than Tailwind's stylesheet order, which no one can see from the class string. In practice the change is where the edges and a linear gradient are mixed (ADR-098 let the linear one win in any order); every other variable was already applied in class order. The explicit composite keeps winning because the gradient classes set `mask-composite` as a side effect, not as a choice the author wrote
