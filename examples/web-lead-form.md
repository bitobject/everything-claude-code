# Lead form: reference implementation (web)

The complete code behind the "Server Actions and forms" section of the `nextjs-patterns` skill, with its tests.
Paths are relative to `web/`. Verified on 2026-11-13 in a scratch app with the `STACK.md` versions (Next.js 16.4.6,
React 19.3.1, TypeScript 6.0.3, zod 4.6.7, next-safe-action 8.7.3, openapi-fetch 0.17.0, Vitest 5.1.0, Ajv 8.20.0,
yaml 2.9.1) against the reference contract `skills/go-clean-architecture/reference/api/openapi.yaml`:
`tsc --noEmit`, ESLint (`strictTypeChecked`, boundaries, jsx-a11y-x), Vitest, `next build`, and form submits
without JavaScript against `next start` with a stand-in for the Go API.

What it shows:
- the action is a next-safe-action `stateAction` passed straight to React's `useActionState`, so the form works
  before hydration and without JavaScript;
- React 19 resets the form after every action, also one that returns an error: the schema's validation errors are
  shaped to carry the typed `values`, Go's answers come back as typed `data` with them, and inputs use them as
  `defaultValue`;
- the request body is an object literal of the parsed fields; the phone is normalized to the contract's pattern;
- `Idempotency-Key` is minted in the browser at the first submit (or by the action without JavaScript) and kept for
  retries; `idempotency.key_reused` is retried once with a new key;
- honeypot and fill time are checked in the action and never sent to Go; SmartCaptcha's `smart-token` will be
  passed through as the contract declares it (built with the first real form; without JavaScript there is no token
  and Go accepts the submission under stricter limits);
- deploy skew ("Failed to find Server Action") gets a reload that keeps the input.

Shared code it imports, shown in `nextjs-patterns`: `lib/api/server.ts` (`apiFor`), `lib/api/forwarded.ts`
(`requestHeaders`, `sessionHeaders`), `lib/api/schema.ts` (generated).

## `src/lib/api/action-client.ts`

The next-safe-action clients every Server Action is built with.

```ts
import 'server-only'
import { createSafeActionClient } from 'next-safe-action'
import { requestHeaders, sessionHeaders } from './forwarded'

/** Server Actions of public forms: the context carries the proxy's client IP and request id for Go. */
export const actionClient = createSafeActionClient({
  // Never send an exception's text to the browser; the log line has the error name and the request id has the rest.
  handleServerError: (error) => {
    console.error('Server Action failed', error.name)
    return 'Не удалось выполнить действие. Попробуйте ещё раз.'
  },
}).use(async ({ next }) => next({ ctx: { forwarded: await requestHeaders() } }))

/** Server Actions in the cabinets: also the session cookie, verbatim. Go authorizes every call. */
export const sessionActionClient = actionClient.use(async ({ next }) =>
  next({ ctx: { forwarded: await sessionHeaders() } }),
)
```

## `src/features/leads/form.ts`

State types and field names shared by the action and the form.

```ts
// Lead form state, shared by the Server Action and the form. Type-only imports: zod and next-safe-action's
// server code stay out of the browser bundle.
import type { SafeActionResult } from 'next-safe-action'
import type { LeadFormInput } from './schema'

/** Hidden anti-spam fields. The action checks them and never sends them to Go. */
export const HONEYPOT_FIELD = 'company_site' // a name browsers do not autofill
export const ELAPSED_FIELD = 'elapsedMs' // set by the browser at submit; empty without JS
export const MIN_FILL_MS = 3_000

/** What the visitor typed, sent back after a failed submit: React resets the form, inputs use it as defaultValue. */
export type LeadFormValues = { phone: string }
export type LeadFieldErrors = Partial<Record<keyof LeadFormValues, string>>

/** The input schema's errors, shaped to carry the typed values back. */
export type LeadValidationErrors = {
  fieldErrors: LeadFieldErrors
  pageOutdated: boolean // only the page's own hidden fields failed
  values: LeadFormValues
  idempotencyKey?: string
}

/** The action's answer once the input passed the schema: Go's answer or the anti-spam verdict. */
export type LeadFormData =
  | { status: 'sent' }
  | { status: 'invalid'; values: LeadFormValues; fieldErrors: LeadFieldErrors; idempotencyKey: string }
  | { status: 'rejected'; values: LeadFormValues; code: string; retryAfterSeconds?: number; idempotencyKey: string }
  | { status: 'failed'; values: LeadFormValues; idempotencyKey?: string } // retry with the same key

export type LeadFormState = SafeActionResult<string, typeof LeadFormInput, LeadValidationErrors, LeadFormData>
```

## `src/features/leads/schema.ts`

The input schema (FormData in), phone normalization, the raw values and the key.

```ts
// The lead form's input schema and helpers. Server side only (zod stays out of the browser bundle).
import * as z from 'zod'
import { PHONE_ERROR } from './copy'
import { ELAPSED_FIELD, HONEYPOT_FIELD, type LeadFormValues } from './form'

/** The contract's CreateLeadRequest.phone pattern. */
const PHONE_PATTERN = /^\+7[0-9]{10}$/
const KEY_PATTERN = /^[A-Za-z0-9_-]{16,64}$/

/** "+7 (999) 123-45-67", "8 999 123 45 67", "9991234567" → "+79991234567"; anything else → "" (fails the pattern). */
export function normalizePhone(input: string): string {
  const national = /^(?:\+7|7|8)?(\d{10})$/.exec(input.replace(/[\s().-]/g, ''))?.[1]
  return national === undefined ? '' : `+7${national}`
}

/**
 * FormData in, the fields the action needs out. z.object, never z.strictObject: FormData also carries React's
 * $ACTION_* fields. Bounds are the contract's; Go stays the authority.
 */
export const LeadFormInput = z
  .instanceof(FormData)
  .transform((data) => Object.fromEntries(data))
  .pipe(
    z.object({
      phone: z
        .string({ error: PHONE_ERROR })
        .transform(normalizePhone)
        .pipe(z.string().regex(PHONE_PATTERN, { error: PHONE_ERROR })),
      topic: z.string().trim().min(3).max(200),
      city: z.string().trim().min(2).max(100),
      idempotencyKey: z.string().optional(),
      [ELAPSED_FIELD]: z.string().optional(),
      [HONEYPOT_FIELD]: z.string().optional(),
    }),
  )
export type LeadSubmission = z.output<typeof LeadFormInput>

const text = (value: FormDataEntryValue | null): string => (typeof value === 'string' ? value : '')

/** What the visitor typed, from the raw FormData. */
export function leadFormValues(formData: FormData): LeadFormValues {
  return { phone: text(formData.get('phone')).slice(0, 40) }
}

/** A well-formed key, or undefined (absent, tampered with). */
export function validKey(key: string | null | undefined): string | undefined {
  return key !== null && key !== undefined && KEY_PATTERN.test(key) ? key : undefined
}

export function idempotencyKeyOf(formData: FormData): string | undefined {
  return validKey(text(formData.get('idempotencyKey')))
}
```

## `src/features/leads/submit.ts`

The submission flow without Next or HTTP; the action injects `send`.

```ts
// The lead submission flow without Next or HTTP: the Server Action injects `send`, tests inject fakes.
import { fieldErrorCodes, type Problem } from '@/lib/api/problem'
import type { components } from '@/lib/api/schema'
import { PHONE_ERROR } from './copy'
import { ELAPSED_FIELD, HONEYPOT_FIELD, MIN_FILL_MS, type LeadFormData, type LeadFormValues } from './form'
import { validKey, type LeadSubmission } from './schema'

type CreateLeadRequest = components['schemas']['CreateLeadRequest']

/** The outcome of one createLead call. */
export type CreateLeadResult =
  | { kind: 'created' }
  | { kind: 'problem'; status: number; problem: Problem | undefined; retryAfterSeconds: number | undefined }
  | { kind: 'unreachable' } // network error or timeout

export type SubmitDeps = {
  send: (body: CreateLeadRequest, idempotencyKey: string) => Promise<CreateLeadResult>
  newIdempotencyKey: () => string
}

export async function submitLeadForm(
  input: LeadSubmission,
  values: LeadFormValues,
  deps: SubmitDeps,
): Promise<LeadFormData> {
  if ((input[HONEYPOT_FIELD] ?? '') !== '') return { status: 'sent' } // a bot: pretend success, nothing reaches Go
  const submittedKey = validKey(input.idempotencyKey)
  const elapsed = input[ELAPSED_FIELD] ?? '' // empty without JS: nothing to measure, the honeypot still works
  if (elapsed !== '' && Number(elapsed) < MIN_FILL_MS) return { status: 'failed', values, idempotencyKey: submittedKey }

  const body: CreateLeadRequest = { phone: input.phone, topic: input.topic, city: input.city } // a literal: no extras
  let key = submittedKey ?? deps.newIdempotencyKey() // no JS: the action mints the key
  let result = await deps.send(body, key)
  if (result.kind === 'problem' && result.problem?.code === 'idempotency.key_reused') {
    key = deps.newIdempotencyKey() // the visitor edited the input after an error: this is a new submission
    result = await deps.send(body, key)
  }
  return toLeadFormData(result, values, key)
}

function toLeadFormData(result: CreateLeadResult, values: LeadFormValues, key: string): LeadFormData {
  switch (result.kind) {
    case 'created':
      return { status: 'sent' }
    case 'unreachable':
      return { status: 'failed', values, idempotencyKey: key }
    case 'problem': {
      const { status, problem } = result
      if (status >= 500 || problem === undefined) return { status: 'failed', values, idempotencyKey: key }
      const phoneCode = fieldErrorCodes(problem, ['phone'] as const).phone
      if (problem.code === 'lead.invalid_phone' || phoneCode !== undefined) {
        return { status: 'invalid', values, fieldErrors: { phone: PHONE_ERROR }, idempotencyKey: key }
      }
      return {
        status: 'rejected',
        values,
        code: problem.code,
        retryAfterSeconds: result.retryAfterSeconds,
        idempotencyKey: key,
      }
    }
  }
}
```

## `src/features/leads/actions.ts`

The stateful Server Action.

```ts
'use server'
import { flattenValidationErrors } from 'next-safe-action'
import { actionClient } from '@/lib/api/action-client'
import { asProblem, retryAfterSeconds } from '@/lib/api/problem'
import { apiFor } from '@/lib/api/server'
import { idempotencyKeyOf, LeadFormInput, leadFormValues } from './schema'
import { submitLeadForm, type CreateLeadResult } from './submit'
import type { LeadFormData, LeadValidationErrors } from './form'

// A stateful action: React's useActionState calls it with (previous result, FormData), which also works
// before hydration and without JavaScript.
export const submitLead = actionClient
  .inputSchema(LeadFormInput, {
    // React resets the form after every action: send what was typed back with the errors.
    handleValidationErrorsShape: (errors, { clientInput }): Promise<LeadValidationErrors> => {
      const { fieldErrors } = flattenValidationErrors(errors)
      const phone = fieldErrors.phone?.[0]
      return Promise.resolve({
        fieldErrors: phone === undefined ? {} : { phone },
        pageOutdated: phone === undefined, // only the page's hidden city/topic failed
        values: leadFormValues(clientInput),
        idempotencyKey: idempotencyKeyOf(clientInput),
      })
    },
  })
  .stateAction<LeadFormData>(async ({ parsedInput, clientInput, ctx }) => {
    const api = apiFor(ctx.forwarded) // client IP and request id; a public form sends no cookie
    return submitLeadForm(parsedInput, leadFormValues(clientInput), {
      newIdempotencyKey: () => crypto.randomUUID(),
      send: async (body, key): Promise<CreateLeadResult> => {
        try {
          const { error, response } = await api.POST('/v1/leads', {
            params: { header: { 'Idempotency-Key': `"${key}"` } }, // a Structured Field String: quoted
            body,
          })
          if (error === undefined) return { kind: 'created' }
          return {
            kind: 'problem',
            status: response.status,
            problem: asProblem(error),
            retryAfterSeconds: retryAfterSeconds(response),
          }
        } catch (error: unknown) {
          console.error('createLead: Go API unreachable', error instanceof Error ? error.name : 'unknown')
          return { kind: 'unreachable' } // timeout or network: the retry reuses the key, so no duplicate
        }
      },
    })
  })
```

## `src/features/leads/copy.ts`

Russian copy keyed by problem `code`, and `leadFormView()`: what the form renders for a result.

```ts
import type { LeadFieldErrors, LeadFormState, LeadFormValues } from './form'

export const PHONE_ERROR = 'Проверьте номер телефона'
const FAILED = 'Не удалось отправить заявку. Попробуйте ещё раз: повторная отправка не создаст дубль.'
const OUTDATED = 'Страница устарела. Обновите её и отправьте заявку ещё раз.'

/** Russian copy for a problem `code` from Go (codes are listed in the contract's x-problem-codes). */
function rejectionMessage(code: string, retryAfterSeconds?: number): string {
  switch (code) {
    case 'lead.rate_limited': {
      const minutes = Math.max(1, Math.ceil((retryAfterSeconds ?? 60) / 60))
      return `Слишком много заявок подряд. Попробуйте через ${String(minutes)} мин.`
    }
    case 'lead.invalid_topic':
    case 'lead.invalid_city':
    case 'request.invalid':
      return OUTDATED
    default:
      return 'Не удалось отправить заявку. Проверьте номер и попробуйте ещё раз.'
  }
}

export type LeadFormView = {
  sent: boolean
  values?: LeadFormValues
  fieldErrors: LeadFieldErrors
  idempotencyKey?: string
  message?: string
}

/** What the form renders for an action result. Pure: tested without React. */
export function leadFormView(state: LeadFormState): LeadFormView {
  if (state.validationErrors !== undefined) {
    const { fieldErrors, pageOutdated, values, idempotencyKey } = state.validationErrors
    return { sent: false, values, fieldErrors, idempotencyKey, message: pageOutdated ? OUTDATED : undefined }
  }
  if (state.serverError !== undefined) return { sent: false, fieldErrors: {}, message: FAILED }
  const data = state.data
  if (data === undefined) return { sent: false, fieldErrors: {} }
  switch (data.status) {
    case 'sent':
      return { sent: true, fieldErrors: {} }
    case 'invalid':
      return { sent: false, values: data.values, fieldErrors: data.fieldErrors, idempotencyKey: data.idempotencyKey }
    case 'rejected':
      return {
        sent: false,
        values: data.values,
        fieldErrors: {},
        idempotencyKey: data.idempotencyKey,
        message: rejectionMessage(data.code, data.retryAfterSeconds),
      }
    case 'failed':
      return { sent: false, values: data.values, fieldErrors: {}, idempotencyKey: data.idempotencyKey, message: FAILED }
  }
}
```

## `src/features/leads/components/lead-form.tsx`

The form.

```tsx
'use client'
import { useActionState, useEffect, useRef, type SubmitEvent } from 'react'
import { submitLead } from '../actions'
import { leadFormView } from '../copy'
import { ELAPSED_FIELD, HONEYPOT_FIELD, type LeadFormState } from '../form'
import { rememberSubmission, restoreDraft } from './draft'

const initialState: LeadFormState = {}

export function LeadForm({ city, topic }: { city: string; topic: string }) {
  const [state, formAction, pending] = useActionState(submitLead, initialState)
  const formRef = useRef<HTMLFormElement>(null)
  const mountedAt = useRef<number | null>(null)

  useEffect(() => {
    mountedAt.current = performance.now()
    if (formRef.current) restoreDraft(formRef.current) // after the reload for a new deployment
  }, [])

  const view = leadFormView(state)
  if (view.sent) return <p role="status">Заявка отправлена. Юрист свяжется с вами.</p>
  const phoneError = view.fieldErrors.phone

  // Runs after hydration only, so nothing random is baked into the prerendered HTML.
  function handleSubmit(event: SubmitEvent<HTMLFormElement>) {
    const form = event.currentTarget
    const keyInput = form.elements.namedItem('idempotencyKey')
    if (keyInput instanceof HTMLInputElement && keyInput.value === '') keyInput.value = crypto.randomUUID()
    const elapsedInput = form.elements.namedItem(ELAPSED_FIELD)
    if (elapsedInput instanceof HTMLInputElement && mountedAt.current !== null) {
      elapsedInput.value = String(Math.round(performance.now() - mountedAt.current))
    }
    rememberSubmission(form)
  }

  return (
    <form ref={formRef} action={formAction} onSubmit={handleSubmit}>
      <input type="hidden" name="city" value={city} />
      <input type="hidden" name="topic" value={topic} />
      <input type="hidden" name="idempotencyKey" defaultValue={view.idempotencyKey ?? ''} />
      <input type="hidden" name={ELAPSED_FIELD} defaultValue="" />
      {/* SmartCaptcha's widget adds <input type="hidden" name="smart-token"> here; the action passes it on. */}
      <div aria-hidden="true" className="absolute -left-[9999px] h-px w-px overflow-hidden">
        <label htmlFor="lead-company-site">Сайт компании</label>
        <input id="lead-company-site" name={HONEYPOT_FIELD} tabIndex={-1} autoComplete="off" />
      </div>

      <label htmlFor="lead-phone">Телефон</label>
      <input
        id="lead-phone"
        name="phone"
        type="tel"
        autoComplete="tel"
        required
        className="ym-disable-keys"
        defaultValue={view.values?.phone}
        aria-invalid={phoneError ? true : undefined}
        aria-describedby={phoneError ? 'lead-phone-error' : undefined}
      />
      {phoneError && <p id="lead-phone-error">{phoneError}</p>}

      <button type="submit" disabled={pending}>
        {pending ? 'Отправляем…' : 'Отправить заявку'}
      </button>
      {view.message && <p role="alert">{view.message}</p>}
    </form>
  )
}
```

## `src/features/leads/components/lead-form-boundary.tsx`

Deploy skew: a reload that keeps the input.

```tsx
'use client'
import { catchError, type ErrorInfo } from 'next/error'
import { unstable_isUnrecognizedActionError } from 'next/navigation'
import { keepDraftForReload } from './draft'

// After a deployment the page's Server Action id no longer exists: the action throws instead of
// returning state. Offer a reload that keeps the input; every other error goes to error.tsx.
function LeadFormFallback(_props: object, { error }: ErrorInfo) {
  if (!unstable_isUnrecognizedActionError(error)) throw error
  return (
    <div role="alert">
      <p>Сайт обновился, пока страница была открыта. Обновите страницу: введённый номер сохранится.</p>
      <button
        type="button"
        onClick={() => {
          keepDraftForReload()
          window.location.reload()
        }}
      >
        Обновить страницу
      </button>
    </div>
  )
}

export const LeadFormBoundary = catchError(LeadFormFallback)
```

## `src/features/leads/components/draft.ts`

The input kept across that reload.

```ts
// Keeps what the visitor typed across the reload that recovers from a new deployment
// ("Failed to find Server Action"). Browser only; storage may be unavailable, then the visitor retypes.
const DRAFT_KEY = 'lead-form-draft'

type Draft = { phone: string; idempotencyKey: string }
let lastSubmission: Draft | undefined

function field(form: HTMLFormElement, name: string): HTMLInputElement | undefined {
  const element = form.elements.namedItem(name)
  return element instanceof HTMLInputElement ? element : undefined
}

export function rememberSubmission(form: HTMLFormElement): void {
  lastSubmission = {
    phone: field(form, 'phone')?.value ?? '',
    idempotencyKey: field(form, 'idempotencyKey')?.value ?? '',
  }
}

export function keepDraftForReload(): void {
  if (lastSubmission === undefined) return
  try {
    sessionStorage.setItem(DRAFT_KEY, JSON.stringify(lastSubmission))
  } catch {
    // storage disabled: nothing to keep
  }
}

export function restoreDraft(form: HTMLFormElement): void {
  let raw: string | null
  try {
    raw = sessionStorage.getItem(DRAFT_KEY)
    sessionStorage.removeItem(DRAFT_KEY)
  } catch {
    return
  }
  if (raw === null) return
  const draft: unknown = JSON.parse(raw)
  if (typeof draft !== 'object' || draft === null) return
  const { phone, idempotencyKey } = draft as Record<string, unknown>
  const phoneInput = field(form, 'phone')
  const keyInput = field(form, 'idempotencyKey')
  if (typeof phone === 'string' && phoneInput && phoneInput.value === '') phoneInput.value = phone
  if (typeof idempotencyKey === 'string' && keyInput && keyInput.value === '') keyInput.value = idempotencyKey
}
```

## `src/lib/api/problem.ts`

RFC 9457 problems from Go, read defensively.

```ts
// RFC 9457 error bodies from Go. Pure functions: no Next imports, tested with Vitest.
import type { components } from './schema'

export type Problem = components['schemas']['Problem']
type FieldError = components['schemas']['FieldError']

const isFieldError = (value: unknown): value is FieldError =>
  typeof value === 'object' && value !== null && typeof (value as { code?: unknown }).code === 'string'

/**
 * The error body as a problem document, or undefined when it is not one: a proxy's 502 page, an empty body,
 * or text. openapi-fetch types `error` as the contract's Problem, but only Go writes problem documents.
 */
export function asProblem(body: unknown): Problem | undefined {
  if (typeof body !== 'object' || body === null) return undefined
  const { type, title, status, code, detail, requestId, errors } = body as Record<string, unknown>
  if (typeof type !== 'string' || typeof title !== 'string' || typeof status !== 'number' || typeof code !== 'string') {
    return undefined
  }
  return {
    type,
    title,
    status,
    code,
    detail: typeof detail === 'string' ? detail : undefined,
    requestId: typeof requestId === 'string' ? requestId : undefined,
    errors: Array.isArray(errors) ? errors.filter(isFieldError) : undefined,
  }
}

/** Codes of the field errors whose JSON Pointer names a top-level body field: "#/phone" → { phone: "pattern" }. */
export function fieldErrorCodes<F extends string>(problem: Problem, fields: readonly F[]): Partial<Record<F, string>> {
  const result: Partial<Record<F, string>> = {}
  for (const error of problem.errors ?? []) {
    const field = fields.find((f) => error.pointer === `#/${f}`)
    if (field !== undefined && result[field] === undefined) result[field] = error.code
  }
  return result
}

/** Seconds from a Retry-After header in delta-seconds form (what Go sends); undefined otherwise. */
export function retryAfterSeconds(response: Response): number | undefined {
  const value = response.headers.get('retry-after')
  return value !== null && /^\d+$/.test(value) ? Number(value) : undefined
}
```

## `src/features/leads/schema.test.ts`

Tests: the input schema.

```ts
import { describe, expect, it } from 'vitest'
import { PHONE_ERROR } from './copy'
import { HONEYPOT_FIELD } from './form'
import { idempotencyKeyOf, LeadFormInput, normalizePhone } from './schema'

const form = (fields: Record<string, string>): FormData => {
  const data = new FormData()
  for (const [k, v] of Object.entries(fields)) data.append(k, v)
  return data
}

describe('normalizePhone', () => {
  it.each([
    ['+7 (999) 123-45-67', '+79991234567'],
    ['8 (999) 123-45-67', '+79991234567'],
    ['8 999 123 45 67', '+79991234567'],
    ['79991234567', '+79991234567'],
    ['999.123.45.67', '+79991234567'],
    ['+7 999 12', ''],
    ['+1 202 555 0143', ''],
    ['телефон', ''],
  ])('%s → %s', (input, expected) => {
    expect(normalizePhone(input)).toBe(expected)
  })
})

describe('LeadFormInput', () => {
  it('parses FormData, normalizes the phone and ignores React and unknown fields', () => {
    const parsed = LeadFormInput.parse(
      form({
        phone: '8 (999) 123-45-67',
        city: 'moskva',
        topic: 'razvod',
        $ACTION_ID_abc: '',
        name: 'Анна',
        [HONEYPOT_FIELD]: '',
      }),
    )
    expect(parsed).toEqual({ phone: '+79991234567', city: 'moskva', topic: 'razvod', [HONEYPOT_FIELD]: '' })
  })

  it('reports a bad phone on the phone field', () => {
    const result = LeadFormInput.safeParse(form({ phone: '+7 999 12', city: 'moskva', topic: 'razvod' }))
    expect(result.success).toBe(false)
    expect(result.error?.issues.map((i) => [i.path.join('.'), i.message])).toEqual([['phone', PHONE_ERROR]])
  })
})

describe('idempotencyKeyOf', () => {
  it('accepts a UUID and rejects tampered values', () => {
    expect(idempotencyKeyOf(form({ idempotencyKey: '6f1c0e5a-8d0b-4a49-9b8e-6f2d1c3a4b5c' }))).toBe(
      '6f1c0e5a-8d0b-4a49-9b8e-6f2d1c3a4b5c',
    )
    expect(idempotencyKeyOf(form({ idempotencyKey: 'short' }))).toBeUndefined()
    expect(idempotencyKeyOf(form({ idempotencyKey: '"quoted-value-not-allowed"' }))).toBeUndefined()
  })
})
```

## `src/features/leads/submit.test.ts`

Tests: the flow, with a fake `send`.

```ts
import { describe, expect, it, vi } from 'vitest'
import type { Problem } from '@/lib/api/problem'
import { PHONE_ERROR } from './copy'
import { ELAPSED_FIELD, HONEYPOT_FIELD } from './form'
import type { LeadSubmission } from './schema'
import { submitLeadForm, type CreateLeadResult, type SubmitDeps } from './submit'

const KEY = '6f1c0e5a-8d0b-4a49-9b8e-6f2d1c3a4b5c'
const values = { phone: '8 999 123 45 67' }
const input = (extra: Partial<LeadSubmission> = {}): LeadSubmission => ({
  phone: '+79991234567',
  city: 'moskva',
  topic: 'razvod',
  idempotencyKey: KEY,
  ...extra,
})

type ProblemResult = Extract<CreateLeadResult, { kind: 'problem' }>
const problem = (status: number, code: string, extra: Partial<Problem> = {}): ProblemResult => ({
  kind: 'problem',
  status,
  problem: { type: `https://example.com/problems/${code}`, title: code, status, code, ...extra },
  retryAfterSeconds: undefined,
})

function deps(...results: CreateLeadResult[]) {
  const send = vi.fn<SubmitDeps['send']>()
  for (const r of results) send.mockResolvedValueOnce(r)
  let n = 0
  return { send, newIdempotencyKey: () => `minted-key-${String(++n).padStart(4, '0')}-xxxxxxxx` }
}

describe('submitLeadForm', () => {
  it('sends exactly the contract fields and reports success', async () => {
    const d = deps({ kind: 'created' })
    expect(await submitLeadForm(input(), values, d)).toEqual({ status: 'sent' })
    expect(d.send).toHaveBeenCalledWith({ phone: '+79991234567', city: 'moskva', topic: 'razvod' }, KEY)
  })

  it('maps Go 400 field errors and 422 lead.invalid_phone to the phone field, keeping input and key', async () => {
    for (const result of [
      problem(400, 'request.invalid', { errors: [{ pointer: '#/phone', code: 'pattern' }] }),
      problem(422, 'lead.invalid_phone'),
    ]) {
      expect(await submitLeadForm(input(), values, deps(result))).toEqual({
        status: 'invalid',
        values,
        fieldErrors: { phone: PHONE_ERROR },
        idempotencyKey: KEY,
      })
    }
  })

  it('mints the key when the form has none (no JavaScript) and returns it for the retry', async () => {
    const d = deps(problem(503, 'unavailable'))
    const state = await submitLeadForm(input({ idempotencyKey: undefined }), values, d)
    expect(state).toEqual({ status: 'failed', values, idempotencyKey: 'minted-key-0001-xxxxxxxx' })
    expect(d.send).toHaveBeenCalledWith(expect.anything(), 'minted-key-0001-xxxxxxxx')
  })

  it('retries once with a new key when Go says the key was used for another body', async () => {
    const d = deps(problem(422, 'idempotency.key_reused'), { kind: 'created' })
    expect(await submitLeadForm(input(), values, d)).toEqual({ status: 'sent' })
    expect(d.send.mock.calls.map(([, key]) => key)).toEqual([KEY, 'minted-key-0001-xxxxxxxx'])
  })

  it('treats a proxy error page, a timeout and 5xx as retryable with the same key', async () => {
    const notAProblem: CreateLeadResult = {
      kind: 'problem',
      status: 502,
      problem: undefined,
      retryAfterSeconds: undefined,
    }
    for (const result of [notAProblem, { kind: 'unreachable' } as const, problem(500, 'internal')]) {
      expect(await submitLeadForm(input(), values, deps(result))).toEqual({
        status: 'failed',
        values,
        idempotencyKey: KEY,
      })
    }
  })

  it('passes rate limiting through with Retry-After', async () => {
    const limited: CreateLeadResult = { ...problem(429, 'lead.rate_limited'), retryAfterSeconds: 120 }
    expect(await submitLeadForm(input(), values, deps(limited))).toMatchObject({
      status: 'rejected',
      code: 'lead.rate_limited',
      retryAfterSeconds: 120,
    })
  })

  it('answers bots without calling Go: a filled honeypot looks sent, a too fast submit looks transient', async () => {
    const d = deps()
    expect(await submitLeadForm(input({ [HONEYPOT_FIELD]: 'x' }), values, d)).toEqual({ status: 'sent' })
    expect(await submitLeadForm(input({ [ELAPSED_FIELD]: '500' }), values, d)).toMatchObject({ status: 'failed' })
    expect(d.send).not.toHaveBeenCalled()
  })
})
```

## `src/features/leads/contract.test.ts`

Tests: what the form sends against the contract (Ajv, JSON Schema 2020-12).

```ts
// What the form sends must pass the contract's own schemas: compiled from api/openapi.yaml
// (OpenAPI 3.1 = JSON Schema 2020-12), so a drift between form and contract fails here, not in production.
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import Ajv2020 from 'ajv/dist/2020'
import { parse } from 'yaml'
import { describe, expect, it } from 'vitest'
import { LeadFormInput, leadFormValues } from './schema'
import { submitLeadForm, type CreateLeadResult } from './submit'

type OpenApi = { components: { schemas: Record<string, object>; parameters: Record<string, { schema: object }> } }
const spec = parse(readFileSync(resolve(process.cwd(), '../api/openapi.yaml'), 'utf8')) as OpenApi

function required<T>(value: T | undefined, name: string): T {
  if (value === undefined) throw new Error(`${name} is missing from api/openapi.yaml`)
  return value
}

const ajv = new Ajv2020({ strict: true, allErrors: true })
const validateBody = ajv.compile(required(spec.components.schemas.CreateLeadRequest, 'CreateLeadRequest'))
const validateKey = ajv.compile(required(spec.components.parameters.IdempotencyKey, 'IdempotencyKey').schema)

async function sentBy(fields: Record<string, string>): Promise<{ body: unknown; key: string }[]> {
  const data = new FormData()
  for (const [k, v] of Object.entries(fields)) data.append(k, v)
  const calls: { body: unknown; key: string }[] = []
  await submitLeadForm(LeadFormInput.parse(data), leadFormValues(data), {
    newIdempotencyKey: () => crypto.randomUUID(),
    send: (body, key): Promise<CreateLeadResult> => {
      calls.push({ body, key: `"${key}"` }) // the header value the action builds
      return Promise.resolve({ kind: 'created' })
    },
  })
  return calls
}

describe('lead form output against the contract', () => {
  it.each(['+7 (999) 123-45-67', '8 (999) 123-45-67', '8 999 123 45 67', '9991234567'])(
    'phone %s: the body and the Idempotency-Key are valid',
    async (phone) => {
      const calls = await sentBy({ phone, city: 'moskva', topic: 'razvod', name: 'Анна' })
      expect(calls).toHaveLength(1)
      const [call] = calls
      expect(validateBody(call?.body), ajv.errorsText(validateBody.errors)).toBe(true)
      expect(validateKey(call?.key), ajv.errorsText(validateKey.errors)).toBe(true)
    },
  )

  it('the validator really enforces the contract: the old example body is rejected', () => {
    expect(validateBody({ name: 'Анна', phone: '+79991234567', city: 'moskva', topic: 'razvod' })).toBe(false) // closed body
    expect(validateBody({ phone: '8 (999) 123-45-67', city: 'moskva', topic: 'razvod' })).toBe(false) // pattern
    expect(validateKey(crypto.randomUUID())).toBe(false) // unquoted key
  })
})
```

## `src/features/leads/copy.test.ts`

Tests: what the form shows for each result.

```ts
import { describe, expect, it } from 'vitest'
import { leadFormView, PHONE_ERROR } from './copy'

const values = { phone: '+7 999 12' }

describe('leadFormView', () => {
  it('idle and sent', () => {
    expect(leadFormView({})).toEqual({ sent: false, fieldErrors: {} })
    expect(leadFormView({ data: { status: 'sent' } }).sent).toBe(true)
  })

  it('a schema error brings the typed values back', () => {
    const view = leadFormView({
      validationErrors: { fieldErrors: { phone: PHONE_ERROR }, pageOutdated: false, values, idempotencyKey: undefined },
    })
    expect(view).toEqual({
      sent: false,
      values,
      fieldErrors: { phone: PHONE_ERROR },
      idempotencyKey: undefined,
      message: undefined,
    })
  })

  it('maps Go problems to copy and keeps the key', () => {
    const view = leadFormView({
      data: { status: 'rejected', values, code: 'lead.rate_limited', retryAfterSeconds: 120, idempotencyKey: 'k' },
    })
    expect(view.message).toBe('Слишком много заявок подряд. Попробуйте через 2 мин.')
    expect(view.idempotencyKey).toBe('k')
  })

  it('an unexpected server error shows the generic message', () => {
    expect(leadFormView({ serverError: 'x' }).message).toMatch(/Не удалось отправить заявку/)
  })
})
```

## `src/features/leads/components/lead-form.test.tsx`

Tests: React 19 reset, kept input, the key, deploy skew.

```tsx
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useActionState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { LeadFormState } from '../form'
import { LeadForm } from './lead-form'
import { LeadFormBoundary } from './lead-form-boundary'

const { submitLead, SkewError } = vi.hoisted(() => ({
  submitLead: vi.fn<(prev: LeadFormState, formData: FormData) => Promise<LeadFormState>>(),
  SkewError: class SkewError extends Error {},
}))
vi.mock('../actions', () => ({ submitLead })) // the real module is 'use server' + server-only
vi.mock('next/navigation', () => ({ unstable_isUnrecognizedActionError: (e: unknown) => e instanceof SkewError }))

afterEach(() => {
  cleanup()
  submitLead.mockReset()
  sessionStorage.clear()
})

const text = (value: FormDataEntryValue | null): string => (typeof value === 'string' ? value : '')
const phoneInput = () => screen.getByLabelText('Телефон')
const type = (value: string) => fireEvent.change(phoneInput(), { target: { value } })
const submit = () => fireEvent.click(screen.getByRole('button', { name: 'Отправить заявку' }))

describe('React 19 form reset', () => {
  function PlainForm({ action }: { action: (prev: string, data: FormData) => Promise<string> }) {
    const [state, formAction] = useActionState(action, 'idle')
    return (
      <form action={formAction}>
        <label htmlFor="p">Телефон</label>
        <input id="p" name="phone" />
        <button type="submit">Отправить заявку</button>
        <output>{state}</output>
      </form>
    )
  }

  it('clears an uncontrolled input even when the action returns an error (why values come back)', async () => {
    render(<PlainForm action={() => Promise.resolve('invalid')} />)
    type('+7 999 12')
    submit()
    await screen.findByText('invalid')
    expect(phoneInput()).toHaveProperty('value', '')
  })
})

describe('LeadForm', () => {
  it('keeps what the visitor typed after an error and marks the field', async () => {
    submitLead.mockResolvedValueOnce({
      validationErrors: {
        fieldErrors: { phone: 'Проверьте номер телефона' },
        pageOutdated: false,
        values: { phone: '+7 999 12' },
      },
    })
    render(<LeadForm city="moskva" topic="razvod" />)
    type('+7 999 12')
    submit()
    await screen.findByText('Проверьте номер телефона')
    expect(phoneInput()).toHaveProperty('value', '+7 999 12')
    expect(phoneInput().getAttribute('aria-invalid')).toBe('true')
  })

  it('mints the Idempotency-Key at the first submit, not in the markup, and reuses it for the retry', async () => {
    submitLead.mockImplementation((_prev, data) =>
      Promise.resolve({
        data: {
          status: 'failed',
          values: { phone: text(data.get('phone')) },
          idempotencyKey: text(data.get('idempotencyKey')),
        },
      }),
    )
    const { container } = render(<LeadForm city="moskva" topic="razvod" />)
    expect(container.querySelector('input[name="idempotencyKey"]')).toHaveProperty('value', '')
    type('89991234567')
    submit()
    await screen.findByRole('alert')
    submit()
    await waitFor(() => {
      expect(submitLead).toHaveBeenCalledTimes(2)
    })
    const [first, second] = submitLead.mock.calls.map(([, data]) => data.get('idempotencyKey'))
    expect(first).toMatch(/^[0-9a-f-]{36}$/)
    expect(second).toBe(first)
    expect(phoneInput()).toHaveProperty('value', '89991234567')
  })

  it('after a deployment ("Failed to find Server Action") offers a reload that keeps the phone', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined) // boundary + jsdom navigation noise
    submitLead.mockRejectedValueOnce(new SkewError('Server Action was not found on the server'))
    const first = render(
      <LeadFormBoundary>
        <LeadForm city="moskva" topic="razvod" />
      </LeadFormBoundary>,
    )
    type('+7 999 123-45-67')
    submit()
    fireEvent.click(await screen.findByRole('button', { name: 'Обновить страницу' }))
    first.unmount()

    render(<LeadForm city="moskva" topic="razvod" />) // the page after the reload
    await waitFor(() => {
      expect(phoneInput()).toHaveProperty('value', '+7 999 123-45-67')
    })
    consoleError.mockRestore()
  })
})
```

## `src/lib/api/action-client.test.ts`

Tests: headers forwarded by the action clients; no exception text to the browser.

```ts
import { describe, expect, it, vi } from 'vitest'

vi.mock('server-only', () => ({})) // the real module throws outside the React server environment
vi.mock('next/headers', () => ({
  headers: () =>
    Promise.resolve(
      new Headers({
        cookie: '_ym_uid=1; __Host-session=ab+/c==; theme=dark',
        'x-forwarded-for': '203.0.113.7',
        'x-request-id': 'req-42',
        traceparent: '00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01',
      }),
    ),
}))

const { actionClient, sessionActionClient } = await import('./action-client')

describe('action clients', () => {
  it('a public action forwards client IP and request id, but no cookie (traceparent comes from OpenTelemetry)', async () => {
    const probe = actionClient.action(({ ctx }) => Promise.resolve(ctx.forwarded))
    expect((await probe()).data).toEqual({ 'x-forwarded-for': '203.0.113.7', 'x-request-id': 'req-42' })
  })

  it('a cabinet action also forwards the session cookie byte for byte', async () => {
    const probe = sessionActionClient.action(({ ctx }) => Promise.resolve(ctx.forwarded))
    expect((await probe()).data).toMatchObject({ cookie: '__Host-session=ab+/c==', 'x-forwarded-for': '203.0.113.7' })
  })

  it('never sends an exception text to the browser', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const failing = actionClient.action(() => Promise.reject(new Error('pg: connection refused at 10.0.0.5')))
    expect((await failing()).serverError).toBe('Не удалось выполнить действие. Попробуйте ещё раз.')
    consoleError.mockRestore()
  })
})
```

## `src/lib/api/problem.test.ts`

Tests: problem parsing.

```ts
import { describe, expect, it } from 'vitest'
import { asProblem, fieldErrorCodes, retryAfterSeconds } from './problem'

const problem = {
  type: 'https://example.com/problems/request.invalid',
  title: 'Request does not match the contract',
  status: 400,
  code: 'request.invalid',
  requestId: 'req-1',
  errors: [{ pointer: '#/phone', code: 'pattern' }, { parameter: 'Idempotency-Key', code: 'required' }, 'junk'],
}

describe('asProblem', () => {
  it('accepts a problem document and drops malformed field errors', () => {
    expect(asProblem(problem)).toEqual({ ...problem, detail: undefined, errors: problem.errors.slice(0, 2) })
  })

  it.each([
    ['a proxy error page', '<html><body>502 Bad Gateway</body></html>'],
    ['an empty body', null],
    ['JSON without a code', { status: 502, title: 'Bad Gateway', type: 'about:blank' }],
  ])('rejects %s', (_name, body) => {
    expect(asProblem(body)).toBeUndefined()
  })
})

describe('fieldErrorCodes', () => {
  it('maps JSON Pointers to top-level fields', () => {
    const parsed = asProblem(problem)
    expect(parsed && fieldErrorCodes(parsed, ['phone', 'city'] as const)).toEqual({ phone: 'pattern' })
  })
})

describe('retryAfterSeconds', () => {
  it('reads delta-seconds and ignores dates', () => {
    expect(retryAfterSeconds(new Response(null, { headers: { 'Retry-After': '120' } }))).toBe(120)
    expect(
      retryAfterSeconds(new Response(null, { headers: { 'Retry-After': 'Wed, 21 Oct 2026 07:28:00 GMT' } })),
    ).toBeUndefined()
  })
})
```

## `e2e/lead-form.spec.ts`

Playwright: with and without JavaScript.

```ts
import { expect, test } from './fixtures'

for (const javaScriptEnabled of [true, false]) {
  test.describe(javaScriptEnabled ? 'lead form with JavaScript' : 'lead form without JavaScript', () => {
    test.use({ javaScriptEnabled })

    test('marks an invalid phone without losing input', async ({ page }) => {
      await page.goto('/moskva/razvod')
      const phone = page.getByLabel('Телефон')
      await phone.fill('+7 999 12')
      await page.getByRole('button', { name: 'Отправить заявку' }).click()
      await expect(phone).toHaveAttribute('aria-invalid', 'true')
      await expect(phone).toHaveValue('+7 999 12') // React resets uncontrolled fields; the state brings it back
    })

    test('sends a lead', async ({ page, makeAxeBuilder }) => {
      await page.goto('/moskva/razvod')
      expect((await makeAxeBuilder().analyze()).violations).toEqual([])
      await page.getByLabel('Телефон').fill(`8 900 ${String(Date.now()).slice(-7)}`) // unique per run
      await page.getByRole('button', { name: 'Отправить заявку' }).click()
      await expect(page.getByRole('status')).toContainText('Заявка отправлена')
    })
  })
}
```
