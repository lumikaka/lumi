import GlobalTopbar from './GlobalTopbar.jsx'

export default function AppPageShell({ title, actions, children }) {
  return (
    <main className="app-route-shell">
      <GlobalTopbar title={title} actions={actions} />
      <div className="app-page-content">{children}</div>
    </main>
  )
}

