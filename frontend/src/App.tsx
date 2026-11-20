import { useState } from 'react'

type Order = { id: string; tenant: string; product: string; status: string; total_usd: number; version: number }
type Preview = { order: Order; can_execute: boolean; confirmation_required: boolean; reason: string }
type Receipt = { order: Order; duplicate: boolean; message: string }
const base = 'http://localhost:8090'

async function api<T>(path: string, tenant: string, opts?: RequestInit): Promise<T> {
  const response = await fetch(base + path, { ...opts, headers: { 'Content-Type': 'application/json', 'X-Demo-Tenant': tenant, ...opts?.headers } })
  const data = await response.json()
  if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`)
  return data as T
}

export default function App() {
  const [tenant, setTenant] = useState<'acme' | 'globex'>('acme')
  const [orderID, setOrderID] = useState('ORD-1002')
  const [order, setOrder] = useState<Order | null>(null)
  const [preview, setPreview] = useState<Preview | null>(null)
  const [receipt, setReceipt] = useState<Receipt | null>(null)
  const [key, setKey] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  function reset() { setOrder(null); setPreview(null); setReceipt(null); setConfirmed(false); setKey(''); setError('') }
  async function run(fn: () => Promise<void>) {
    setError(''); setPending(true)
    try { await fn() } catch (e) { setError(e instanceof Error ? e.message : 'Unknown error') } finally { setPending(false) }
  }
  function lookup() { reset(); void run(async () => setOrder(await api<Order>(`/api/orders/${encodeURIComponent(orderID)}`, tenant))) }
  function preflight() {
    setPreview(null); setReceipt(null); setConfirmed(false)
    void run(async () => {
      const p = await api<Preview>('/api/actions/preview', tenant, { method: 'POST', body: JSON.stringify({ order_id: orderID, action: 'cancel' }) })
      setPreview(p); setOrder(p.order); setKey(crypto.randomUUID())
    })
  }
  function execute() {
    void run(async () => {
      const r = await api<Receipt>('/api/actions/execute', tenant, { method: 'POST', body: JSON.stringify({ order_id: orderID, action: 'cancel', confirmed, idempotency_key: key }) })
      setReceipt(r); setOrder(r.order)
    })
  }
  return <main>
    <header><div><small>SIERRA-INSPIRED ENGINEERING LAB · PROJECT 01</small><h1>Enterprise Action Agent</h1><p>Milestone 1: deterministic authorization, confirmation, and idempotent actions.</p></div><span className="tag">Local demo · no LLM yet</span></header>
    <section className="layout">
      <article className="panel">
        <h2>1. Identify context</h2>
        <label>Demo tenant <select value={tenant} onChange={e => { setTenant(e.target.value as 'acme'|'globex'); reset(); setOrderID(e.target.value === 'acme' ? 'ORD-1002' : 'ORD-2001') }}><option value="acme">Acme</option><option value="globex">Globex</option></select></label>
        <label>Order ID <input value={orderID} onChange={e => { setOrderID(e.target.value); reset() }} placeholder="ORD-1002" /></label>
        <div className="actions"><button onClick={lookup} disabled={pending}>Look up order</button><button className="secondary" onClick={preflight} disabled={pending}>Preview cancellation</button></div>
        <p className="hint">Try ORD-1002 (processing), ORD-1001 (delivered), or ORD-2001 (other tenant).</p>
        {error && <p role="alert" className="error">{error}</p>}
      </article>
      <article className="panel">
        <h2>2. Verify proposed action</h2>
        {!order ? <p className="hint">Look up an order or preview a cancellation to begin.</p> : <div className="order"><strong>{order.id}</strong><span className="status">{order.status}</span><p>{order.product}</p><p>${order.total_usd.toFixed(2)} · Version {order.version}</p></div>}
        {preview && <div className={preview.can_execute ? 'notice' : 'notice blocked'}><strong>{preview.can_execute ? 'Action eligible' : 'Action blocked'}</strong><p>{preview.reason}</p></div>}
        {preview?.can_execute && <><label className="checkbox"><input type="checkbox" checked={confirmed} onChange={e => setConfirmed(e.target.checked)}/> I explicitly approve cancelling this order.</label><button onClick={execute} disabled={!confirmed || pending}>Confirm cancellation</button></>}
        {receipt && <div className="notice"><strong>{receipt.duplicate ? 'Replayed previous result — no duplicate mutation' : 'Cancellation completed'}</strong><p>{receipt.message}</p><p className="mono">Idempotency key: {key}</p><button className="secondary" onClick={execute} disabled={pending}>Replay same request</button></div>}
      </article>
    </section>
    <section className="panel"><h2>What this milestone proves</h2><div className="chips"><span>Tenant-aware access</span><span>Server-side business rules</span><span>Explicit human confirmation</span><span>Idempotency</span><span>Concurrency-safe in-process mutation</span></div><p className="hint">Security note: X-Demo-Tenant is a selectable demo context, NOT authentication. The in-memory store is for local learning only. Next milestones add identity verification, durable PostgreSQL, actual LLM reasoning, MCP tools, and tracing.</p></section>
  </main>
}
