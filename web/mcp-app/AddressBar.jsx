import { Fragment, useContext } from 'react'
import { ArrowLeft } from 'lucide-react'
import { createPath, UNSAFE_NavigationContext, useLocation, useNavigate } from 'react-router-dom'
import { uuidV7 } from './contracts.js'

function parentPage(pathname, base) {
  const [area, detail] = pathname.slice(base.length + 1).split('/')
  if (!area) return null
  if (area === 'chapters' && detail) return { path: base + '/chapters', label: '返回章节列表' }
  if (area === 'premise' && detail) return { path: base + '/premise', label: '返回设定列表' }
  return { path: base + '/', label: '返回作品' }
}

export default function AddressBar({ base, confirmLeave, children }) {
  const location = useLocation()
  const navigate = useNavigate()
  // MemoryRouter owns the history of this workspace, including push, replace and pop.
  const { navigator } = useContext(UNSAFE_NavigationContext)
  const hasPrevious = navigator.index > 0
  const parent = parentPage(location.pathname, base)
  const label = hasPrevious ? '返回上一页' : parent?.label
  const address = 'ui://lumi' + createPath(location)
  const segments = ('ui://lumi' + location.pathname).split('/')
  function goBack() {
    if (!confirmLeave()) return
    if (hasPrevious) navigate(-1)
    else if (parent) navigate(parent.path, { replace: true })
  }

  return <header className="mcp-address-bar" aria-label="插件内导航">
    <button className="mcp-address-bar__back" type="button" aria-label={label || '返回'} title={label || '没有上一页'} disabled={!label}
      onClick={goBack}>
      <ArrowLeft size={16} aria-hidden="true" />
    </button>
    <div className="mcp-address-bar__field" title={address}>
      <input aria-label="当前插件内地址" readOnly value={address} onFocus={event => event.currentTarget.select()} />
      <span aria-hidden="true">{segments.map((part, index) =>
        <Fragment key={index}>{index > 0 && '/'}{uuidV7.test(part) ? <span className="mcp-address-bar__segment">{part}</span> : part}</Fragment>
      )}{(location.search || location.hash) && <span className="mcp-address-bar__segment">{location.search}{location.hash}</span>}</span>
    </div>
    {children}
  </header>
}
