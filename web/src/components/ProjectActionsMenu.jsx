export default function ProjectActionsMenu({
  actions,
  className = '',
  menuRef,
  onEnter,
  onForget,
  onRelocate,
  onReveal,
  project,
  style,
  t,
}) {
  return (
    <div
      className={`project-index-menu${className ? ` ${className}` : ''}`}
      ref={menuRef}
      role="menu"
      style={style}
    >
      <div className="project-index-menu__path">
        <span>{t('projects.index.column.path')}</span>
        <code data-no-i18n>{project.root_path}</code>
      </div>
      {actions.includes('enter') ? <button className="project-index-menu__item" type="button" role="menuitem" onClick={onEnter}>{t('projects.action.enter')}</button> : null}
      {actions.includes('reveal') ? <button className="project-index-menu__item" type="button" role="menuitem" onClick={onReveal}>{t('projects.action.reveal')}</button> : null}
      {actions.includes('relocate') ? <button className="project-index-menu__item" type="button" role="menuitem" onClick={onRelocate}>{t('projects.action.relocate')}</button> : null}
      {actions.includes('forget') ? <><span className="project-index-menu__separator" role="separator" /><button className="project-index-menu__item project-index-menu__item--danger" type="button" role="menuitem" onClick={onForget}>{t('projects.action.forget')}</button></> : null}
    </div>
  )
}
