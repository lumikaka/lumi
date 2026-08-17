export default function GlobalTopbar({
  title,
  actions,
}) {
  return (
    <header className="project-topbar project-topbar--index">
      <div className="project-topbar__title"><h1 data-no-i18n>{title}</h1></div>
      <div className="project-topbar__actions">{actions}</div>
    </header>
  )
}
