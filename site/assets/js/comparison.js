(() => {
  'use strict'

  const dialog = document.querySelector('[data-comparison-lightbox]')
  if (!dialog || typeof dialog.showModal !== 'function') return
  const image = dialog.querySelector('[data-comparison-image]')
  const title = dialog.querySelector('[data-comparison-image-title]')
  const original = dialog.querySelector('[data-comparison-original]')
  const zoom = dialog.querySelector('[data-comparison-zoom]')
  const viewport = dialog.querySelector('.comparison-lightbox__viewport')
  let opener = null

  const setZoom = (actualSize) => {
    zoom.setAttribute('aria-pressed', String(actualSize))
    viewport.classList.toggle('is-enlarged', actualSize)
    viewport.scrollTo(0, 0)
  }
  zoom.addEventListener('click', () => setZoom(zoom.getAttribute('aria-pressed') !== 'true'))

  document.querySelectorAll('[data-comparison-image-link]').forEach((link) => {
    link.addEventListener('click', (event) => {
      if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
      event.preventDefault()
      opener = link
      image.src = link.href
      image.alt = link.querySelector('img').alt
      title.textContent = link.dataset.imageLabel
      original.href = link.href
      dialog.showModal()
      setZoom(false)
    })
  })

  dialog.addEventListener('click', (event) => {
    if (event.target !== dialog) return
    const bounds = dialog.getBoundingClientRect()
    if (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom) dialog.close()
  })
  dialog.addEventListener('close', () => {
    image.removeAttribute('src')
    opener?.focus({ preventScroll: true })
    opener = null
  })

  // In-page links to the evidence must reveal the closed details element.
  const revealNotes = () => {
    if (window.location.hash !== '#case-notes') return
    const notes = document.getElementById('case-notes')
    if (notes) notes.open = true
  }
  revealNotes()
  window.addEventListener('hashchange', revealNotes)
  document.querySelectorAll('a[href="#case-notes"]').forEach((link) => {
    link.addEventListener('click', () => {
      document.getElementById('case-notes').open = true
    })
  })
})()
