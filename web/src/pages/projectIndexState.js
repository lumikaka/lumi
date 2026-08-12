export function projectRowActions(project) {
	if (project.open) return ['enter', 'forget']
  if (project.available) return ['enter', 'relocate', 'forget']
  return ['relocate', 'forget']
}

export function projectRowPrimaryAction(project) {
	if (project.open || project.available) return 'enter'
  return null
}
