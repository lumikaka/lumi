// Without scripts, each column is a continuous strip in the original reading order.
const mode = document.querySelector('[data-strip-continuous]')
const hint = document.querySelector('[data-strip-hint]')
const tracks = document.querySelectorAll('[data-strip-track]')
tracks.forEach((track) => {
  const controls = track.querySelector('[data-strip-controls]')
  const figures = [...track.querySelectorAll('figure')]
  const buttons = [...controls.querySelectorAll('button')]

  if (buttons.length) {
    const show = (id) => {
      figures.forEach((figure) => { figure.hidden = figure.id !== id })
      buttons.forEach((button) => {
        button.setAttribute('aria-pressed', String(button.getAttribute('aria-controls') === id))
      })
    }
    show(figures[0].id)
    controls.addEventListener('click', (event) => {
      const button = event.target.closest('button')
      if (!button || button.getAttribute('aria-pressed') === 'true') return
      show(button.getAttribute('aria-controls'))
      // A sticky page selector may be used halfway down a strip. Start the new page at its top.
      if (track.getBoundingClientRect().top < track.firstElementChild.getBoundingClientRect().top - 1) {
        track.scrollIntoView({ block: 'start', behavior: 'instant' })
      }
    })
  }
  const applyMode = () => {
    const selected = controls.querySelector('[aria-pressed="true"]')?.getAttribute('aria-controls')
    figures.forEach((figure) => { figure.hidden = !mode.checked && !!selected && figure.id !== selected })
    controls.hidden = mode.checked
  }
  mode.addEventListener('change', applyMode)
  applyMode()
})
document.querySelector('[data-strip-mode]').hidden = false
hint.hidden = mode.checked
mode.addEventListener('change', () => { hint.hidden = mode.checked })

const brief = document.getElementById('brief')
const openBrief = () => { if (location.hash === '#brief') brief.open = true }
document.querySelector('a[href="#brief"]').addEventListener('click', () => { brief.open = true })
window.addEventListener('hashchange', openBrief)
openBrief()
