/**
 * Pure-logic tests for the bidirectional adapter in tts-override-block.tsx.
 *
 * The adapter converts between:
 *   - Storage (generic keys: speed, emotion, style) — what agents.other_config.tts_params stores
 *   - Form state (capability-native keys: voice_settings.speed, etc.) — what DynamicParamForm uses
 *
 * Tests cover all 5 providers and the round-trip invariant.
 */
import { describe, it, expect } from "vitest";
import {
  genericToNativeFormState,
  nativeFormStateToGeneric,
} from "../tts-override-block";

// ---- genericToNativeFormState (load direction: storage → form) ----

describe("genericToNativeFormState", () => {
  it("openai: speed stays flat", () => {
    expect(genericToNativeFormState({ speed: 1.5 }, "openai")).toEqual({ speed: 1.5 });
  });

  it("openai: emotion dropped (not supported)", () => {
    const out = genericToNativeFormState({ speed: 1.5, emotion: "happy" }, "openai");
    expect(out).toEqual({ speed: 1.5 });
  });

  it("elevenlabs: speed → voice_settings.speed", () => {
    expect(genericToNativeFormState({ speed: 1.1 }, "elevenlabs")).toEqual({
      "voice_settings.speed": 1.1,
    });
  });

  it("elevenlabs: style → voice_settings.style", () => {
    expect(genericToNativeFormState({ style: 0.5 }, "elevenlabs")).toEqual({
      "voice_settings.style": 0.5,
    });
  });

  it("elevenlabs: emotion dropped", () => {
    const out = genericToNativeFormState({ speed: 1.0, style: 0.3, emotion: "happy" }, "elevenlabs");
    expect(out).toEqual({ "voice_settings.speed": 1.0, "voice_settings.style": 0.3 });
  });

  it("minimax: speed stays flat", () => {
    expect(genericToNativeFormState({ speed: 0.9 }, "minimax")).toEqual({ speed: 0.9 });
  });

  it("minimax: emotion stays flat", () => {
    expect(genericToNativeFormState({ emotion: "neutral" }, "minimax")).toEqual({ emotion: "neutral" });
  });

  it("minimax: style dropped", () => {
    const out = genericToNativeFormState({ speed: 1.0, emotion: "happy", style: 0.5 }, "minimax");
    expect(out).toEqual({ speed: 1.0, emotion: "happy" });
  });

  it("edge: all keys dropped", () => {
    expect(genericToNativeFormState({ speed: 1.0, emotion: "happy", style: 0.5 }, "edge")).toEqual({});
  });

  it("gemini: all keys dropped", () => {
    expect(genericToNativeFormState({ speed: 1.0, emotion: "happy", style: 0.5 }, "gemini")).toEqual({});
  });

  it("empty input → empty output", () => {
    expect(genericToNativeFormState({}, "openai")).toEqual({});
  });

  it("unknown provider → all keys dropped", () => {
    expect(genericToNativeFormState({ speed: 1.0 }, "azure")).toEqual({});
  });
});

// ---- nativeFormStateToGeneric (save direction: form → storage) ----

describe("nativeFormStateToGeneric", () => {
  it("openai: flat speed → generic speed", () => {
    expect(nativeFormStateToGeneric({ speed: 1.5 }, "openai")).toEqual({ speed: 1.5 });
  });

  it("elevenlabs: voice_settings.speed → generic speed", () => {
    expect(nativeFormStateToGeneric({ "voice_settings.speed": 1.1 }, "elevenlabs")).toEqual({
      speed: 1.1,
    });
  });

  it("elevenlabs: voice_settings.style → generic style", () => {
    expect(nativeFormStateToGeneric({ "voice_settings.style": 0.4 }, "elevenlabs")).toEqual({
      style: 0.4,
    });
  });

  it("elevenlabs: both native keys → both generic keys", () => {
    const out = nativeFormStateToGeneric(
      { "voice_settings.speed": 1.0, "voice_settings.style": 0.3 },
      "elevenlabs",
    );
    expect(out).toEqual({ speed: 1.0, style: 0.3 });
  });

  it("minimax: flat speed+emotion → generic", () => {
    expect(nativeFormStateToGeneric({ speed: 0.9, emotion: "neutral" }, "minimax")).toEqual({
      speed: 0.9,
      emotion: "neutral",
    });
  });

  it("edge: no mappings → empty", () => {
    expect(nativeFormStateToGeneric({ speed: 1.0 }, "edge")).toEqual({});
  });

  it("gemini: no mappings → empty", () => {
    expect(nativeFormStateToGeneric({ speed: 1.0 }, "gemini")).toEqual({});
  });
});

// ---- Round-trip invariant ----
// For any generic params + provider, load then save must return the same generic map.

describe("round-trip: genericToNative → nativeToGeneric", () => {
  const cases: Array<{ provider: string; generic: Record<string, unknown> }> = [
    { provider: "openai", generic: { speed: 1.5 } },
    { provider: "elevenlabs", generic: { speed: 1.1, style: 0.4 } },
    { provider: "minimax", generic: { speed: 0.9, emotion: "happy" } },
    { provider: "edge", generic: {} },
    { provider: "gemini", generic: {} },
    // Keys unsupported by provider get dropped on load, so round-trip correctly excludes them.
    { provider: "openai", generic: {} }, // emotion/style stripped on load
  ];

  for (const { provider, generic } of cases) {
    it(`${provider}: round-trip preserves supported keys`, () => {
      const nativeState = genericToNativeFormState(generic as Record<string, import("@/components/dynamic-param-form").ParamValue>, provider);
      const backToGeneric = nativeFormStateToGeneric(nativeState, provider);
      expect(backToGeneric).toEqual(generic);
    });
  }
});
