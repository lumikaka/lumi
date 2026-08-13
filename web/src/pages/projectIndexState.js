export function projectRowActions(project) {
	if (project.open) return ['enter', 'reveal', 'forget']
  if (project.available) return ['enter', 'reveal', 'relocate', 'forget']
  return ['relocate', 'forget']
}

export function projectRowPrimaryAction(project) {
	if (project.open || project.available) return 'enter'
  return null
}
