import { useEffect, useRef, useState } from 'react'
import { Activity, Bot, Languages, Settings } from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'

import { useI18n } from '../i18n/useI18n.js'

export default function AccountMenu() {
  const { t } = useI18n()
  const location = useLocation()
  const menuRef = useRef(null)
  const triggerRef = useRef(null)
  const [open, setOpen] = useState(false)

  useEffect(() => setOpen(false), [location.hash, location.pathname])

  useEffect(() => {
    if (!open) return undefined
    const handlePointerDown = (event) => {
      if (!menuRef.current?.contains(event.target)) setOpen(false)
    }
    const handleKeyDown = (event) => {
      if (event.key !== 'Escape') return
      setOpen(false)
      triggerRef.current?.focus()
    }
    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [open])

  return (
    <div className="account-menu" ref={menuRef}>
      <button
        ref={triggerRef}
        className="account-menu__trigger"
        type="button"
        aria-label={t('settings.account_menu')}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
      >
        <span className="account-menu__icon" aria-hidden="true" />
      </button>

      {open ? (
        <div className="account-menu__dropdown" role="menu" aria-label={t('settings.account_menu')}>
          <div className="account-menu__identity">
            <strong>{t('settings.local_account')}</strong>
            <span>{t('settings.local_only')}</span>
          </div>
          <Link className="account-menu__item" role="menuitem" to="/settings/account">
            <Settings size={16} aria-hidden="true" />
            <span>{t('settings.account')}</span>
          </Link>
          <Link className="account-menu__item" role="menuitem" to="/settings/providers">
            <Bot size={16} aria-hidden="true" />
            <span>{t('settings.llm_management')}</span>
          </Link>
          <Link className="account-menu__item" role="menuitem" to="/settings/llm-logs">
            <Activity size={16} aria-hidden="true" />
            <span>{t('settings.llm_logs')}</span>
          </Link>
          <div className="account-menu__divider" />
          <Link className="account-menu__item" role="menuitem" to="/settings/account#language">
            <Languages size={16} aria-hidden="true" />
            <span>{t('settings.language')}</span>
          </Link>
        </div>
      ) : null}
    </div>
  )
}
