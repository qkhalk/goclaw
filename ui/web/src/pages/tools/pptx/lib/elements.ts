import type { DeckTheme, Slide, SlideElement } from "../types";
import { newElementId } from "../types";
import { buildSlidePrims } from "./slide-spec";

/**
 * Free-form element model helpers. A v1 (layout-driven) slide compiles to
 * prims through lib/slide-spec.ts; "freeing" a slide materializes those
 * prims into an explicit element list that the interactive stage can edit.
 * After freeing, the slide no longer follows theme re-coloring — the colors
 * were baked in at materialize time (the stage shows a hint about that).
 */

/** Hard cap per slide — keeps the DOM/pointer machinery and the .pptx export
 * bounded. */
export const MAX_ELEMENTS = 60;

/** Explicit element list for a slide: the slide's own elements when it has
 * them, otherwise the compiled layout prims (read-only semantics). */
export function elementsOf(slide: Slide, theme: DeckTheme): SlideElement[] {
  if (slide.elements) return slide.elements;
  return compileElements(slide, theme);
}

/** Does the slide carry its own editable element list? */
export function isFreeForm(slide: Slide): boolean {
  return Array.isArray(slide.elements);
}

/** Materialize the compiled layout prims into an editable element list. */
export function compileElements(slide: Slide, theme: DeckTheme): SlideElement[] {
  return buildSlidePrims(slide, theme).map((prim) => ({
    id: newElementId(),
    ...prim,
  }) as SlideElement);
}

/**
 * Attach an explicit element list to a layout-driven slide WITHOUT
 * regenerating ids. The list is normally the compiled prims currently on
 * screen, so in-flight interactions (a drag or an inline text edit that
 * started on a v1 slide) keep pointing at live ids and land on the promoted
 * slide. Returns the slide unchanged when it already carries elements.
 */
export function materialize(slide: Slide, elements: SlideElement[]): Slide {
  if (slide.elements) return slide;
  return { ...slide, elements };
}

/** Patch one element of a slide's explicit list (no-op on compiled slides —
 * callers must materialize first; the stage enforces that). */
export function patchElement(
  slide: Slide,
  id: string,
  patch: Partial<SlideElement>,
): Slide | undefined {
  if (!slide.elements) return undefined;
  return {
    ...slide,
    elements: slide.elements.map((el) => (el.id === id ? ({ ...el, ...patch } as SlideElement) : el)),
  };
}
