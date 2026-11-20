import { useRef, useState, type FormEvent } from 'react'

type Tenant = 'acme' | 'globex'
type Order = { id: string; tenant: string; product: string; status: string; total_cents: number; version: number }
type Preview = { order: Order; can_execute: boolean; confirmation_required: boolean; reason: string }
type Receipt = { order: Order; duplicate: boolean; message: string }
// The exact request the user approved. A replay resends it unchanged.
type Submitted = { order_id: string; action: 'cancel'; confirmed: true; expected_version: number; idempotency_key: string }

const base = 'http://localhost:8090'
const hints: Record<Tenant, string> = {
  acme: 'Try ORD-1002 (processing), ORD-1001 (delivered), or ORD-2001 (other tenant).',
  globex: 'Try ORD-2001 (processing) or ORD-1002 (other tenant).',
}
const usd = (cents: number) => `$${(cents / 100).toFixed(2)}`

async function api<T>(path: string, tenant: string, opts?: RequestInit): Promise<T> {
  let response: Response
  try {
    response = await fetch(base + path, { ...opts, headers: { 'Content-Type': 'application/json', 'X-Demo-Tenant': tenant, ...opts?.headers } })
  } catch {
    throw new Error(`Cannot reach the API at ${base}. Is the backend running?`)
  }
  const data = await response.json().catch(() => null)
  if (!response.ok) throw new Error(data?.error || `Request failed (HTTP ${response.status})`)
  if (data === null) throw new Error('The API returned an unreadable response.')
  return data as T
}

export default function App() {
  const [tenant, setTenant] = useState<Tenant>('acme')
  const [orderID, setOrderID] = useState('ORD-1002')
  const [order, setOrder] = useState<Order | null>(null)
  const [preview, setPreview] = useState<Preview | null>(null)
  const [receipt, setReceipt] = useState<Receipt | null>(null)
  const [submitted, setSubmitted] = useState<Submitted | null>(null)
  const [key, setKey] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  // Bumped whenever the tenant or order ID changes, so a response requested for
  // an earlier selection is dropped instead of being shown against the new one.
  const selection = useRef(0)

  function reset() { selection.current++; setOrder(null); setPreview(null); setReceipt(null); setSubmitted(null); setConfirmed(false); setKey(''); setError('') }
  async function run<T>(request: () => Promise<T>, apply: (result: T) => void) {
    const started = selection.current
    setError(''); setPending(true)
    try {
      const result = await request()
      if (selection.current === started) apply(result)
    } catch (e) {
      if (selection.current === started) setError(e instanceof Error ? e.message : 'Unknown error')
    } finally { setPending(false) }
  }
  function lookup() {
    reset()
    const id = orderID.trim()
    if (!id) { setError('Enter an order ID.'); return }
    void run(() => api<Order>(`/api/orders/${encodeURIComponent(id)}`, tenant), setOrder)
  }
  function preflight() {
    setPreview(null); setReceipt(null); setSubmitted(null); setConfirmed(false); setError('')
    const id = orderID.trim()
    if (!id) { setError('Enter an order ID.'); return }
    void run(
      () => api<Preview>('/api/actions/preview', tenant, { method: 'POST', body: JSON.stringify({ order_id: id, action: 'cancel' }) }),
      p => { setPreview(p); setOrder(p.order); setKey(crypto.randomUUID()) },
    )
  }
  function send(request: Submitted) {
    void run(
      () => api<Receipt>('/api/actions/execute', tenant, { method: 'POST', body: JSON.stringify(request) }),
      r => { setReceipt(r); setOrder(r.order); setSubmitted(request); setPreview(null); setConfirmed(false) },
    )
  }
  function execute() {
    if (!preview || !confirmed) return
    // Act on the order and version that were previewed, never on the text field.
    send({ order_id: preview.order.id, action: 'cancel', confirmed: true, expected_version: preview.order.version, idempotency_key: key })
  }
  function onSubmit(e: FormEvent) { e.preventDefault(); if (!pending) lookup() }

  return <main>
    <header><div><small>SIERRA-INSPIRED ENGINEERING LAB · PROJECT 01</small><h1>Enterprise Action Agent</h1><p>Milestone 1: deterministic authorization, confirmation, and idempotent actions.</p></div><span className="tag">Local demo · no LLM yet</span></header>
    <section className="layout">
      <article className="panel">
        <h2>1. Identify context</h2>
        <form onSubmit={onSubmit}>
          <label>Demo tenant <select value={tenant} onChange={e => { const next = e.target.value as Tenant; setTenant(next); reset(); setOrderID(next === 'acme' ? 'ORD-1002' : 'ORD-2001') }}><option value="acme">Acme</option><option value="globex">Globex</option></select></label>
          <label>Order ID <input value={orderID} onChange={e => { setOrderID(e.target.value); reset() }} placeholder="ORD-1002" /></label>
          <div className="actions"><button type="submit" disabled={pending}>Look up order</button><button type="button" className="secondary" onClick={preflight} disabled={pending}>Preview cancellation</button></div>
        </form>
        <p className="hint">{hints[tenant]}</p>
        {error && <p role="alert" className="error">{error}</p>}
      </article>
      <article className="panel">
        <h2>2. Verify proposed action</h2>
        {!order ? <p className="hint">Look up an order or preview a cancellation to begin.</p> : <div className="order"><strong>{order.id}</strong><span className="status">{order.status}</span><p>{order.product}</p><p>{usd(order.total_cents)} · Version {order.version}</p></div>}
        {preview && <div className={preview.can_execute ? 'notice' : 'notice blocked'}><strong>{preview.can_execute ? 'Action eligible' : 'Action blocked'}</strong><p>{preview.reason}</p></div>}
        {preview?.can_execute && <><label className="checkbox"><input type="checkbox" checked={confirmed} onChange={e => setConfirmed(e.target.checked)}/> I explicitly approve cancelling this order.</label><button onClick={execute} disabled={!confirmed || pending}>Confirm cancellation</button></>}
        {receipt && submitted && <div className="notice"><strong>{receipt.duplicate ? 'Replayed previous result — no duplicate mutation' : 'Cancellation completed'}</strong><p>{receipt.message}</p><p className="mono">Idempotency key: {submitted.idempotency_key}</p><button className="secondary" onClick={() => send(submitted)} disabled={pending}>Replay same request</button></div>}
      </article>
    </section>
    <section className="panel"><h2>What this milestone proves</h2><div className="chips"><span>Tenant-aware access</span><span>Server-side business rules</span><span>Explicit human confirmation</span><span>Idempotency</span><span>Concurrency-safe in-process mutation</span></div><p className="hint">Security note: X-Demo-Tenant is a selectable demo context, NOT authentication. The in-memory store is for local learning only. Next milestones add identity verification, durable PostgreSQL, actual LLM reasoning, MCP tools, and tracing.</p></section>
  </main>
}
