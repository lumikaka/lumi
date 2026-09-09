import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createMCPGrant, decideMCPCall, listMCPCalls, listMCPGrants, revokeMCPGrant } from '../api/mcp.js'
import { useI18n } from '../i18n/useI18n.js'
import LocalizedErrorMessage from '../i18n/LocalizedErrorMessage.jsx'

const statusKeys = { pending_confirmation: 'mcp.status.pending_confirmation', executing: 'mcp.status.executing', succeeded: 'mcp.status.succeeded', failed: 'mcp.status.failed', rejected: 'mcp.status.rejected', expired: 'mcp.status.expired', interrupted: 'mcp.status.interrupted' }

export default function ProjectMCPAccessCard({ projectUuid }) {
  const { t, formatDateTime } = useI18n()
  const client = useQueryClient()
  const [name, setName] = useState('')
  const [permission, setPermission] = useState('read')
  const [created, setCreated] = useState(null)
  const [error, setError] = useState(null)
  const [copied, setCopied] = useState(false)
  const grants = useQuery({ queryKey: ['mcp-grants', projectUuid], queryFn: () => listMCPGrants(projectUuid) })
  const calls = useQuery({ queryKey: ['mcp-calls', projectUuid], queryFn: () => listMCPCalls(projectUuid) })
  const refresh = () => {
    void client.invalidateQueries({ queryKey: ['mcp-grants', projectUuid] })
    void client.invalidateQueries({ queryKey: ['mcp-calls', projectUuid] })
  }
  const create = useMutation({ mutationFn: () => createMCPGrant(projectUuid, { name: name.trim(), permission }), onSuccess: (value) => { setCreated(value); setCopied(false); setName(''); setError(null); refresh() }, onError: setError })
  const revoke = useMutation({ mutationFn: (uuid) => revokeMCPGrant(projectUuid, uuid), onSuccess: () => { setCreated(null); refresh() }, onError: setError })
  const decide = useMutation({ mutationFn: ({ call, decision }) => decideMCPCall(projectUuid, call, decision), onSuccess: () => { setError(null); refresh() }, onError: setError })
  const copy = async () => {
    try { await navigator.clipboard.writeText(JSON.stringify(created.client_config, null, 2)); setCopied(true) } catch (value) { setError(value) }
  }
  return <section className="overview-card" id="mcp-access">
    <header className="overview-card__header"><div><h2>{t('mcp.title')}</h2><p>{t('mcp.intro')}</p></div></header>
    <p>{t('mcp.scope')}</p>
    <h3>{t('mcp.stdio.title')}</h3>
    <form className="overview-edit-form" onSubmit={(event) => { event.preventDefault(); create.mutate() }}>
      <label>{t('mcp.name')}<input value={name} maxLength={120} onChange={(event) => setName(event.target.value)} placeholder={t('mcp.placeholder')} required /></label>
      <label>{t('mcp.permission')}<select value={permission} onChange={(event) => setPermission(event.target.value)}><option value="read">{t('mcp.read')}</option><option value="edit">{t('mcp.edit_generate')}</option></select></label>
      <div className="overview-form-actions"><button type="submit" disabled={!name.trim() || create.isPending}>{t('mcp.create')}</button></div>
    </form>
    {created ? <div role="status">
      <p>{t('mcp.secret')}</p>
      <label>{t('mcp.config')}<textarea readOnly rows={9} value={JSON.stringify(created.client_config, null, 2)} /></label>
      <div className="overview-form-actions"><button type="button" onClick={copy}>{copied ? t('mcp.copied') : t('mcp.copy')}</button><button type="button" className="button-secondary" onClick={() => { setCreated(null); create.reset() }}>{t('mcp.hide')}</button></div>
    </div> : null}
    <LocalizedErrorMessage error={error || grants.error || calls.error} />
    <h3>{t('mcp.grants')}</h3>
    {grants.isLoading ? <p>{t('mcp.loading')}</p> : null}
    {(grants.data?.items || []).map((grant) => <div className="overview-card__meta" key={grant.uuid}><span>{grant.name} · {t(grant.oauth_resource ? 'mcp.transport.http' : 'mcp.transport.stdio')} · {grant.permission === 'edit' ? t('mcp.edit') : t('mcp.read')} · {grant.token_prefix}… · {grant.revoked_at ? t('mcp.revoked') : t('mcp.active')}</span>{!grant.revoked_at ? <button type="button" className="button-secondary" disabled={revoke.isPending} onClick={() => revoke.mutate(grant.uuid)}>{t('mcp.revoke')}</button> : null}</div>)}
    <h3>{t('mcp.calls')}</h3>
    <p>{t('mcp.confirm_help')}</p>
    {(calls.data?.items || []).length === 0 ? <p>{t('mcp.empty')}</p> : null}
    {(calls.data?.items || []).map(({ call, request, result, client_name }) => <details key={call.uuid} open={call.status === 'pending_confirmation'}>
      <summary>{client_name} · {call.action} · {statusKeys[call.status] ? t(statusKeys[call.status]) : call.status}</summary>
      <p>{t('mcp.call')}{call.uuid} · {formatDateTime(call.created_at)}</p>
      <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: '20rem', overflow: 'auto' }}>{JSON.stringify(request, null, 2)}</pre>
      <p>{t('mcp.fingerprint')}<code style={{ overflowWrap: 'anywhere' }}>{call.fingerprint}</code></p>
      {result ? <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: '10rem', overflow: 'auto' }}>{JSON.stringify(result, null, 2)}</pre> : null}
      {call.status === 'pending_confirmation' ? <div className="overview-form-actions"><button type="button" className="button-secondary" disabled={decide.isPending} onClick={() => decide.mutate({ call, decision: 'reject' })}>{t('mcp.reject')}</button><button type="button" disabled={decide.isPending || Date.now() >= Date.parse(call.expires_at)} onClick={() => decide.mutate({ call, decision: 'approve' })}>{t('mcp.approve')}</button></div> : null}
    </details>)}
  </section>
}
