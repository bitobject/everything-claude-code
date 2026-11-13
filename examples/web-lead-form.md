# Lead form: reference implementation (web)

The complete code behind the "Server Actions and forms" section of the `nextjs-patterns` skill, with its tests.
Paths are relative to `web/`. Verified on 2026-11-13 in a scratch app with the `STACK.md` versions (Next.js 16.4.6,
React 19.3.1, TypeScript 6.0.3, zod 4.6.7, openapi-fetch 0.17.0, Vitest 5.1.0, Ajv 8.20.0, yaml 2.9.1) against the
reference contract `skills/go-clean-architecture/reference/api/openapi.yaml`: `tsc --noEmit`, ESLint
(`strictTypeChecked`, boundaries, jsx-a11y-x), Vitest, `next build`, and a no-JavaScript form submit against
`next start` with a stand-in for the Go API.

What it shows:
- every error state returns the typed `values`; inputs use them as `defaultValue` (React 19 resets the form after
  every action, also one that returns an error);
- the request body is an object literal of the parsed fields; the phone is normalized to the contract's pattern;
- `Idempotency-Key` is minted in the browser at the first submit (or by the action without JavaScript) and kept for
  retries; `idempotency.key_reused` is retried once with a new key;
- honeypot and fill time are checked in the action and never sent to Go; SmartCaptcha's `smart-token` would be
  passed through as the contract declares it (not shown: it needs the contract field and the widget script);
- deploy skew ("Failed to find Server Action") gets a reload that keeps the input.

Shared code it imports, shown in `nextjs-patterns`: `lib/api/server.ts` (`apiFor`), `lib/api/forwarded.ts`
(`requestHeaders`), `lib/api/schema.ts` (generated).

## `src/features/leads/form.ts`

State and field names shared by the action and the form.

```ts
// Lead form state and field names, shared by the Server Action and the form. No zod here: it would
// ship to the browser with the form.

/** Hidden anti-spam fields. The action checks them and never sends them to Go. */
export const HONEYPOT_FIELD = 'company_site' // a name browsers do not autofill
export const ELAPSED_FIELD = 'elapsedMs' // set by the browser at submit; empty without JS
export const MIN_FILL_MS = 3_000

/** What the visitor typed, sent back after a failed submit: React resets the form, inputs use it as defaultValue. */
export type LeadFormValues = { phone: string }
export type LeadFieldErrors = Partial<Record<keyof LeadFormValues, string>>

export type LeadFormState =
  | { status: 'idle' }
  | { status: 'invalid'; values: LeadFormValues; fieldErrors: LeadFieldErrors; idempotencyKey?: string }
  | { status: 'rejected'; values: LeadFormValues; code: string; retryAfterSeconds?: number; idempotencyKey?: string }
  | { status: 'failed'; values: LeadFormValues; idempotencyKey?: string } // retry with the same key
  | { status: 'sent' }
```

## `src/features/leads/copy.ts`

Russian copy keyed by problem `code`.

```ts
import type { LeadFormState } from './form'

export const PHONE_ERROR = 'Проверьте номер телефона'

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
      return 'Страница устарела. Обновите её и отправьте заявку ещё раз.'
    default:
      return 'Не удалось отправить заявку. Проверьте номер и попробуйте ещё раз.'
  }
}

export function formMessage(state: LeadFormState): string | undefined {
  switch (state.status) {
    case 'idle':
    case 'invalid':
    case 'sent':
      return undefined
    case 'failed':
      return 'Не удалось отправить заявку. Попробуйте ещё раз: повторная отправка не создаст дубль.'
    case 'rejected':
      return rejectionMessage(state.code, state.retryAfterSeconds)
  }
}
```

## `src/features/leads/schema.ts`

Parsing: phone normalization, the contract-shaped body, anti-spam verdict, the key.

```ts
// Parsing of the lead form. Server side only (zod stays out of the browser bundle).
import * as z from 'zod'
import type { components } from '@/lib/api/schema'
import { PHONE_ERROR } from './copy'
import { ELAPSED_FIELD, HONEYPOT_FIELD, MIN_FILL_MS, type LeadFieldErrors, type LeadFormValues } from './form'

type CreateLeadRequest = components['schemas']['CreateLeadRequest']

/** The contract's CreateLeadRequest.phone pattern. */
const PHONE_PATTERN = /^\+7[0-9]{10}$/

/** "+7 (999) 123-45-67", "8 999 123 45 67", "9991234567" → "+79991234567"; anything else → "" (fails the pattern). */
export function normalizePhone(input: string): string {
  const national = /^(?:\+7|7|8)?(\d{10})$/.exec(input.replace(/[\s().-]/g, ''))?.[1]
  return national === undefined ? '' : `+7${national}`
}

// z.object, never z.strictObject: FormData also carries React's $ACTION_* keys and the anti-spam fields.
// Bounds are the contract's; Go stays the authority.
const LeadFormInput = z.object({
  phone: z
    .string({ error: PHONE_ERROR })
    .transform(normalizePhone)
    .pipe(z.string().regex(PHONE_PATTERN, { error: PHONE_ERROR })),
  topic: z.string().trim().min(3).max(200),
  city: z.string().trim().min(2).max(100),
})

export type LeadFormParse = { ok: true; body: CreateLeadRequest } | { ok: false; fieldErrors: LeadFieldErrors } // empty when only the page's hidden fields failed

export function parseLeadForm(formData: FormData): LeadFormParse {
  const parsed = LeadFormInput.safeParse(Object.fromEntries(formData))
  if (!parsed.success) {
    const phone = z.flattenError(parsed.error).fieldErrors.phone?.[0]
    return { ok: false, fieldErrors: phone === undefined ? {} : { phone } }
  }
  const { phone, topic, city } = parsed.data
  return { ok: true, body: { phone, topic, city } } // a literal: a field the contract lacks fails tsc
}

const text = (value: FormDataEntryValue | null): string => (typeof value === 'string' ? value : '')

export function leadFormValues(formData: FormData): LeadFormValues {
  return { phone: text(formData.get('phone')).slice(0, 40) }
}

/** The key the browser minted (or the action returned last time); undefined when absent or malformed. */
export function idempotencyKeyOf(formData: FormData): string | undefined {
  const key = text(formData.get('idempotencyKey'))
  return /^[A-Za-z0-9_-]{16,64}$/.test(key) ? key : undefined
}

export type SpamVerdict = 'ok' | 'honeypot' | 'too_fast'

export function spamVerdict(formData: FormData): SpamVerdict {
  if (text(formData.get(HONEYPOT_FIELD)) !== '') return 'honeypot'
  const elapsed = text(formData.get(ELAPSED_FIELD)) // empty without JS: nothing to measure, the honeypot still works
  return elapsed !== '' && Number(elapsed) < MIN_FILL_MS ? 'too_fast' : 'ok'
}
```

## `src/features/leads/submit.ts`

The submission flow without Next or HTTP; the action injects `send`.

```ts
// The lead submission flow without Next or HTTP: the Server Action injects `send`, tests inject fakes.
import { fieldErrorCodes, type Problem } from '@/lib/api/problem'
import type { components } from '@/lib/api/schema'
import { PHONE_ERROR } from './copy'
import type { LeadFormState, LeadFormValues } from './form'
import { idempotencyKeyOf, leadFormValues, parseLeadForm, spamVerdict } from './schema'

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

export async function submitLeadForm(formData: FormData, deps: SubmitDeps): Promise<LeadFormState> {
  const values = leadFormValues(formData)
  const submittedKey = idempotencyKeyOf(formData)
  const verdict = spamVerdict(formData)
  if (verdict === 'honeypot') return { status: 'sent' } // a bot: pretend success, nothing reaches Go

  const parsed = parseLeadForm(formData)
  if (!parsed.ok) {
    return parsed.fieldErrors.phone === undefined
      ? { status: 'rejected', values, code: 'request.invalid', idempotencyKey: submittedKey }
      : { status: 'invalid', values, fieldErrors: parsed.fieldErrors, idempotencyKey: submittedKey }
  }
  if (verdict === 'too_fast') return { status: 'failed', values, idempotencyKey: submittedKey } // a person retries

  let key = submittedKey ?? deps.newIdempotencyKey() // no JS: the action mints the key
  let result = await deps.send(parsed.body, key)
  if (result.kind === 'problem' && result.problem?.code === 'idempotency.key_reused') {
    key = deps.newIdempotencyKey() // the visitor edited the input after an error: this is a new submission
    result = await deps.send(parsed.body, key)
  }
  return toLeadFormState(result, values, key)
}

function toLeadFormState(result: CreateLeadResult, values: LeadFormValues, key: string): LeadFormState {
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

The Server Action: wiring only.

```ts
'use server'
import { requestHeaders } from '@/lib/api/forwarded'
import { asProblem, retryAfterSeconds } from '@/lib/api/problem'
import { apiFor } from '@/lib/api/server'
import type { LeadFormState } from './form'
import { submitLeadForm, type CreateLeadResult } from './submit'

export async function submitLead(_prev: LeadFormState, formData: FormData): Promise<LeadFormState> {
  const api = apiFor(await requestHeaders()) // client IP, request id, traceparent; a public form sends no cookie
  return submitLeadForm(formData, {
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
}
```

## `src/features/leads/components/lead-form.tsx`

The form.

```tsx
'use client'
import { useActionState, useEffect, useRef, type SubmitEvent } from 'react'
import { submitLead } from '../actions'
import { formMessage } from '../copy'
import { ELAPSED_FIELD, HONEYPOT_FIELD, type LeadFormState } from '../form'
import { rememberSubmission, restoreDraft } from './draft'

const initialState: LeadFormState = { status: 'idle' }

export function LeadForm({ city, topic }: { city: string; topic: string }) {
  const [state, formAction, pending] = useActionState(submitLead, initialState)
  const formRef = useRef<HTMLFormElement>(null)
  const mountedAt = useRef<number | null>(null)

  useEffect(() => {
    mountedAt.current = performance.now()
    if (formRef.current) restoreDraft(formRef.current) // after the reload for a new deployment
  }, [])

  if (state.status === 'sent') return <p role="status">Заявка отправлена. Юрист свяжется с вами.</p>

  const values = state.status === 'idle' ? undefined : state.values
  const key = state.status === 'idle' ? undefined : state.idempotencyKey
  const phoneError = state.status === 'invalid' ? state.fieldErrors.phone : undefined
  const message = formMessage(state)

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
      <input type="hidden" name="idempotencyKey" defaultValue={key ?? ''} />
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
        defaultValue={values?.phone}
        aria-invalid={phoneError ? true : undefined}
        aria-describedby={phoneError ? 'lead-phone-error' : undefined}
      />
      {phoneError && <p id="lead-phone-error">{phoneError}</p>}

      <button type="submit" disabled={pending}>
        {pending ? 'Отправляем…' : 'Отправить заявку'}
      </button>
      {message && <p role="alert">{message}</p>}
    </form>
  )
}
```

## `src/features/leads/components/lead-form-boundary.tsx`

Deploy skew: reload that keeps the input.

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

Tests: parsing.

```ts
import { describe, expect, it } from 'vitest'
import { PHONE_ERROR } from './copy'
import { ELAPSED_FIELD, HONEYPOT_FIELD } from './form'
import { idempotencyKeyOf, normalizePhone, parseLeadForm, spamVerdict } from './schema'

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

describe('parseLeadForm', () => {
  it('builds the request from the parsed fields and ignores React and anti-spam fields', () => {
    const parsed = parseLeadForm(
      form({
        phone: '8 (999) 123-45-67',
        city: 'moskva',
        topic: 'razvod',
        $ACTION_ID_abc: '',
        [HONEYPOT_FIELD]: '',
        name: 'Анна',
      }),
    )
    expect(parsed).toEqual({ ok: true, body: { phone: '+79991234567', city: 'moskva', topic: 'razvod' } })
  })

  it('reports a bad phone on the phone field', () => {
    expect(parseLeadForm(form({ phone: '+7 999 12', city: 'moskva', topic: 'razvod' }))).toEqual({
      ok: false,
      fieldErrors: { phone: PHONE_ERROR },
    })
  })

  it('reports no visible field when only the hidden city is invalid', () => {
    expect(parseLeadForm(form({ phone: '89991234567', city: 'm', topic: 'razvod' }))).toEqual({
      ok: false,
      fieldErrors: {},
    })
  })
})

describe('spamVerdict', () => {
  it('flags a filled honeypot and a too fast submit; without JS there is no timing', () => {
    expect(spamVerdict(form({ [HONEYPOT_FIELD]: 'http://spam' }))).toBe('honeypot')
    expect(spamVerdict(form({ [ELAPSED_FIELD]: '800' }))).toBe('too_fast')
    expect(spamVerdict(form({ [ELAPSED_FIELD]: '15000' }))).toBe('ok')
    expect(spamVerdict(form({}))).toBe('ok')
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
import { submitLeadForm, type CreateLeadResult, type SubmitDeps } from './submit'

const KEY = '6f1c0e5a-8d0b-4a49-9b8e-6f2d1c3a4b5c'

const form = (fields: Record<string, string>): FormData => {
  const data = new FormData()
  for (const [k, v] of Object.entries({ city: 'moskva', topic: 'razvod', ...fields })) data.append(k, v)
  return data
}

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
  it('sends the normalized phone and reports success', async () => {
    const d = deps({ kind: 'created' })
    expect(await submitLeadForm(form({ phone: '8 999 123 45 67', idempotencyKey: KEY }), d)).toEqual({ status: 'sent' })
    expect(d.send).toHaveBeenCalledWith({ phone: '+79991234567', city: 'moskva', topic: 'razvod' }, KEY)
  })

  it('keeps the typed phone after a local validation error and does not call Go', async () => {
    const d = deps()
    expect(await submitLeadForm(form({ phone: '+7 999 12', idempotencyKey: KEY }), d)).toEqual({
      status: 'invalid',
      values: { phone: '+7 999 12' },
      fieldErrors: { phone: PHONE_ERROR },
      idempotencyKey: KEY,
    })
    expect(d.send).not.toHaveBeenCalled()
  })

  it('maps Go 400 field errors and 422 lead.invalid_phone to the phone field, keeping input and key', async () => {
    for (const result of [
      problem(400, 'request.invalid', { errors: [{ pointer: '#/phone', code: 'pattern' }] }),
      problem(422, 'lead.invalid_phone'),
    ]) {
      expect(await submitLeadForm(form({ phone: '89991234567', idempotencyKey: KEY }), deps(result))).toEqual({
        status: 'invalid',
        values: { phone: '89991234567' },
        fieldErrors: { phone: PHONE_ERROR },
        idempotencyKey: KEY,
      })
    }
  })

  it('mints the key when the form has none (no JavaScript) and returns it for the retry', async () => {
    const d = deps(problem(503, 'unavailable'))
    const state = await submitLeadForm(form({ phone: '89991234567' }), d)
    expect(state).toEqual({
      status: 'failed',
      values: { phone: '89991234567' },
      idempotencyKey: 'minted-key-0001-xxxxxxxx',
    })
    expect(d.send).toHaveBeenCalledWith(expect.anything(), 'minted-key-0001-xxxxxxxx')
  })

  it('retries once with a new key when Go says the key was used for another body', async () => {
    const d = deps(problem(422, 'idempotency.key_reused'), { kind: 'created' })
    expect(await submitLeadForm(form({ phone: '89991234567', idempotencyKey: KEY }), d)).toEqual({ status: 'sent' })
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
      expect(await submitLeadForm(form({ phone: '89991234567', idempotencyKey: KEY }), deps(result))).toEqual({
        status: 'failed',
        values: { phone: '89991234567' },
        idempotencyKey: KEY,
      })
    }
  })

  it('passes rate limiting through with Retry-After', async () => {
    const limited: CreateLeadResult = { ...problem(429, 'lead.rate_limited'), retryAfterSeconds: 120 }
    expect(await submitLeadForm(form({ phone: '89991234567', idempotencyKey: KEY }), deps(limited))).toMatchObject({
      status: 'rejected',
      code: 'lead.rate_limited',
      retryAfterSeconds: 120,
    })
  })

  it('answers bots without calling Go: a filled honeypot looks sent, a too fast submit looks transient', async () => {
    const d = deps()
    expect(await submitLeadForm(form({ phone: '89991234567', [HONEYPOT_FIELD]: 'x' }), d)).toEqual({ status: 'sent' })
    expect(await submitLeadForm(form({ phone: '89991234567', [ELAPSED_FIELD]: '500' }), d)).toMatchObject({
      status: 'failed',
    })
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
  await submitLeadForm(data, {
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
      status: 'invalid',
      values: { phone: '+7 999 12' },
      fieldErrors: { phone: 'Проверьте номер телефона' },
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
        status: 'failed',
        values: { phone: text(data.get('phone')) },
        idempotencyKey: text(data.get('idempotencyKey')),
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
