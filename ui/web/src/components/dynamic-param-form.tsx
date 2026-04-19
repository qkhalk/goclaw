import type { ParamSchema } from "@/api/tts-capabilities"
import {
  fieldRenderers,
  DefaultField,
  type FieldProps,
  type ParamValue,
} from "./dynamic-param-form-fields"

export type { ParamValue, FieldProps }

export interface DynamicParamFormProps {
  /** Ordered list of param schemas from ProviderCapabilities.params */
  schema: ParamSchema[]
  /** Current values keyed by ParamSchema.key */
  value: Record<string, ParamValue>
  /** Called when a field changes. Not fired when readonly=true. */
  onChange?: (key: string, val: ParamValue) => void
  /** When true all inputs are rendered in read-only / display mode. */
  readonly?: boolean
}

/**
 * Returns true when all DependsOn constraints are satisfied by formState (AND semantics).
 * An empty DependsOn array means always visible.
 */
export function evaluateDependsOn(
  deps: ParamSchema["depends_on"],
  formState: Record<string, ParamValue>,
): boolean {
  if (!deps || deps.length === 0) return true
  return deps.every((d) => String(formState[d.field]) === String(d.value))
}

/**
 * Renders a list of TTS provider params from a ParamSchema array.
 * Each field type maps to a dedicated renderer. Visibility is gated by
 * evaluateDependsOn. When readonly=true, no onChange callbacks fire.
 *
 * NOT mounted in tts-page.tsx in Phase A — exported for Phase C wiring.
 */
export function DynamicParamForm({
  schema,
  value,
  onChange,
  readonly = false,
}: DynamicParamFormProps) {
  if (!schema || schema.length === 0) return null

  return (
    <div className="space-y-4">
      {schema.map((param) => {
        if (!evaluateDependsOn(param.depends_on, value)) return null

        const Renderer = fieldRenderers[param.type] ?? DefaultField
        const currentVal: ParamValue =
          value[param.key] !== undefined
            ? value[param.key]!
            : ((param.default as ParamValue) ?? "")

        const handleChange = (val: ParamValue) => {
          if (!readonly && onChange) {
            onChange(param.key, val)
          }
        }

        return (
          <div key={param.key} className="space-y-1">
            <label className="text-sm font-medium" htmlFor={`param-${param.key}`}>
              {param.label}
            </label>
            {param.description && (
              <p className="text-xs text-muted-foreground">{param.description}</p>
            )}
            <Renderer
              schema={param}
              value={currentVal}
              onChange={handleChange}
              readonly={readonly}
            />
          </div>
        )
      })}
    </div>
  )
}
