import { Fragment, useContext, useLayoutEffect, useState } from 'react'
import { ArrowLeft, ArrowRight } from 'lucide-react'
import { createPath, UNSAFE_NavigationContext, useLocation, useNavigate, useNavigationType } from 'react-router-dom'
import { uuidV7 } from './contracts.js'

export default function AddressBar({ confirmLeave, children }) {
  const location = useLocation()
  const navigate = useNavigate()
  const action = useNavigationType()
  const { navigator } = useContext(UNSAFE_NavigationContext)
  const index = navigator.index
  const [lastIndex, setLastIndex] = useState(index)
  // A push discards the forward branch. Moving or replacing an entry preserves it.
  useLayoutEffect(() => {
    if (action === 'PUSH') setLastIndex(index)
  }, [action, index, location.key])
  const hasPrevious = index > 0
  const hasNext = index < lastIndex && action !== 'PUSH'
  const backLabel = location.state?.backLabel || '返回上一页'
  const address = 'ui://lumi' + createPath(location)
  const segments = ('ui://lumi' + location.pathname).split('/')
  function go(delta) {
    if (confirmLeave()) navigate(delta)
  }

  return <header className="mcp-address-bar" aria-label="插件内导航">
    <button className="mcp-address-bar__arrow" type="button" aria-label={hasPrevious ? backLabel : '返回'} title={hasPrevious ? backLabel : '没有上一页'} disabled={!hasPrevious}
      onClick={() => go(-1)}>
      <ArrowLeft size={16} aria-hidden="true" />
    </button>
    <button className="mcp-address-bar__arrow" type="button" aria-label="前进" title={hasNext ? '前进' : '没有可前进的页面'} disabled={!hasNext}
      onClick={() => go(1)}>
      <ArrowRight size={16} aria-hidden="true" />
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
