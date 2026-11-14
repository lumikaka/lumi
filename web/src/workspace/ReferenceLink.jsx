import { Link } from 'react-router-dom'
import { useWorkspaceRuntime } from './runtime.jsx'

// Preserve the Web link, route and styling; the App explicitly overrides clicks.
export default function ReferenceLink({ reference, ...props }) {
  const { referenceToAgent } = useWorkspaceRuntime()
  return <Link {...props} onClick={referenceToAgent ? event => { event.preventDefault(); referenceToAgent(reference) } : props.onClick} />
}
