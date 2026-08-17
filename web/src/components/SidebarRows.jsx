import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'

import accountIcon from '../assets/figma/workspace/account.svg'
import moreIcon from '../assets/figma/workspace/more.svg'
import pinIcon from '../assets/figma/workspace/pin.svg'
import threadInProgressIcon from '../assets/figma/workspace/thread-in-progress.svg'
import threadUnreadIcon from '../assets/figma/workspace/thread-unread.svg'
import FigmaIcon from './FigmaIcon.jsx'

export function SidebarActionRow({ iconSrc, label, to, className = '', trailing = null, ...props }) {
  const content = <><FigmaIcon src={iconSrc} size={16} /><span>{label}</span>{trailing}</>
  if (to) return <Link className={`app-sidebar-action-row ${className}`.trim()} to={to} {...props}>{content}</Link>
  return <button className={`app-sidebar-action-row ${className}`.trim()} type="button" {...props}>{content}</button>
}

export function ConversationStatus({ status, label }) {
  if (status === 'read') return <span className="app-sidebar-thread__status-slot" aria-hidden="true" />
  const src = status === 'in_progress' ? threadInProgressIcon : threadUnreadIcon
  return (
    <span className="app-sidebar-thread__status-slot">
      <FigmaIcon className={`app-sidebar-thread__status app-sidebar-thread__status--${status}`} src={src} size={16} alt={label} />
    </span>
  )
}

export function ConversationRow({ active, href, pinned, status, title, statusLabel, archiveDisabled, onTogglePinned, onArchive, t }) {
  const menuRef = useRef(null)
  const [menuOpen, setMenuOpen] = useState(false)
  useEffect(() => {
    if (!menuOpen) return undefined
    const close = (event) => {
      if (event.key === 'Escape' || (event.type === 'pointerdown' && !menuRef.current?.contains(event.target))) setMenuOpen(false)
    }
    document.addEventListener('pointerdown', close)
    document.addEventListener('keydown', close)
    return () => {
      document.removeEventListener('pointerdown', close)
      document.removeEventListener('keydown', close)
    }
  }, [menuOpen])
  return (
    <div className={`app-sidebar-thread${active ? ' is-active' : ''}${menuOpen ? ' is-menu-open' : ''}`} ref={menuRef}>
      <Link className="app-sidebar-thread__main" to={href} title={title}>
        <span data-no-i18n>{title}</span>
      </Link>
      <ConversationStatus status={status} label={statusLabel} />
      <div className="app-sidebar-thread__actions">
        <button className={pinned ? 'is-active' : ''} type="button" aria-label={t(pinned ? 'sidebar.thread.unpin' : 'sidebar.thread.pin', { title })} title={t(pinned ? 'sidebar.thread.unpin' : 'sidebar.thread.pin', { title })} aria-pressed={pinned} onClick={onTogglePinned}><FigmaIcon src={pinIcon} size={16} /></button>
        <button type="button" aria-label={t('common.action.more')} title={t('common.action.more')} aria-haspopup="menu" aria-expanded={menuOpen} onClick={() => setMenuOpen((value) => !value)}><FigmaIcon src={moreIcon} size={16} /></button>
      </div>
      {menuOpen ? <div className="app-sidebar-thread__menu" role="menu"><button type="button" role="menuitem" disabled={archiveDisabled} onClick={() => { setMenuOpen(false); onArchive() }}>{t('sidebar.thread.archive', { title })}</button></div> : null}
    </div>
  )
}

export function SidebarProviderAction({ state, t }) {
  if (state === 'ready') return null
  const needsVerification = state === 'needs_verification'
  return (
    <Link className="app-sidebar-provider-action" to="/setup" aria-label={t(needsVerification ? 'sidebar.provider.verify' : 'sidebar.provider.bind')}>
      <span className="app-sidebar-provider-action__status" aria-hidden="true" />
      <span>{t(needsVerification ? 'sidebar.provider.verify' : 'sidebar.provider.bind')}</span>
      <span aria-hidden="true">›</span>
    </Link>
  )
}

export function SidebarAccountAction({ collapsed, open, onClick, t }) {
  return (
    <button className="app-sidebar__account" type="button" aria-haspopup="menu" aria-expanded={open} onClick={onClick} title={t('settings.account_and_settings')}>
      <span className="app-sidebar__avatar"><FigmaIcon src={accountIcon} size={20} leafWidth={12.91} leafHeight={14.85} /></span>
      <span className="app-sidebar__account-copy"><strong>{t('settings.local_account')}</strong></span>
    </button>
  )
}
