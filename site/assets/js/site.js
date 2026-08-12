(() => {
  'use strict'

  const body = document.body
  const header = document.querySelector('[data-site-header]')
  const menuToggle = document.querySelector('[data-menu-toggle]')
  const siteNav = document.querySelector('[data-site-nav]')

  const setMenu = (open) => {
    if (!menuToggle || !siteNav) return
    menuToggle.setAttribute('aria-expanded', String(open))
    siteNav.classList.toggle('is-open', open)
  }

  menuToggle?.addEventListener('click', () => {
    setMenu(menuToggle.getAttribute('aria-expanded') !== 'true')
  })
  siteNav?.addEventListener('click', (event) => {
    if (event.target.closest('a')) setMenu(false)
  })
  document.addEventListener('click', (event) => {
    if (!siteNav?.classList.contains('is-open')) return
    if (!siteNav.contains(event.target) && !menuToggle?.contains(event.target)) setMenu(false)
  })
  window.addEventListener('resize', () => {
    if (window.innerWidth > 960) setMenu(false)
  }, { passive: true })

  if (header) {
    const updateHeader = () => header.classList.toggle('is-scrolled', window.scrollY > 10)
    updateHeader()
    window.addEventListener('scroll', updateHeader, { passive: true })
  }

  const docsSidebar = document.querySelector('[data-docs-sidebar]')
  const docsOpen = document.querySelector('[data-docs-open]')
  const docsClose = document.querySelector('[data-docs-close]')
  const docsBackdrop = document.querySelector('[data-docs-backdrop]')

  const syncDocsDrawerAccessibility = (open = docsSidebar?.classList.contains('is-open')) => {
    if (!docsSidebar) return
    const mobile = window.matchMedia('(max-width: 767px)').matches
    const sidebarHidden = mobile && !open
    docsSidebar.inert = sidebarHidden
    if (sidebarHidden) docsSidebar.setAttribute('aria-hidden', 'true')
    else docsSidebar.removeAttribute('aria-hidden')
    if (docsBackdrop) {
      const backdropHidden = !mobile || !open
      docsBackdrop.inert = backdropHidden
      docsBackdrop.tabIndex = backdropHidden ? -1 : 0
      if (backdropHidden) docsBackdrop.setAttribute('aria-hidden', 'true')
      else docsBackdrop.removeAttribute('aria-hidden')
    }
  }

  const setDocsDrawer = (open) => {
    if (!docsSidebar) return
    if (!open && docsSidebar.contains(document.activeElement)) docsOpen?.focus({ preventScroll: true })
    docsSidebar.classList.toggle('is-open', open)
    docsBackdrop?.classList.toggle('is-open', open)
    docsOpen?.setAttribute('aria-expanded', String(open))
    body.classList.toggle('has-docs-drawer', open)
    syncDocsDrawerAccessibility(open)
    if (open) docsClose?.focus()
    else docsOpen?.focus({ preventScroll: true })
  }

  syncDocsDrawerAccessibility(false)

  docsOpen?.addEventListener('click', () => setDocsDrawer(true))
  docsClose?.addEventListener('click', () => setDocsDrawer(false))
  docsBackdrop?.addEventListener('click', () => setDocsDrawer(false))
  window.addEventListener('resize', () => {
    if (window.innerWidth >= 768 && docsSidebar?.classList.contains('is-open')) setDocsDrawer(false)
    else syncDocsDrawerAccessibility()
  }, { passive: true })

  const dialog = document.querySelector('[data-search-dialog]')
  const searchOpeners = [...document.querySelectorAll('[data-search-open]')]
  const searchClose = dialog?.querySelector('[data-search-close]')
  const searchInput = dialog?.querySelector('[data-search-input]')
  const searchStatus = dialog?.querySelector('[data-search-status]')
  const searchResults = dialog?.querySelector('[data-search-results]')
  const script = document.currentScript || document.querySelector('script[data-search-url]')
  const searchURL = script?.dataset.searchUrl
  let searchIndex = null
  let searchPromise = null
  let activeResult = -1
  let lastSearchOpener = null

  const normalize = (value) => String(value || '')
    .normalize('NFKC')
    .toLocaleLowerCase(document.documentElement.lang)
    .replace(/\s+/g, ' ')
    .trim()

  const loadSearchIndex = async () => {
    if (searchIndex) return searchIndex
    if (!searchPromise) {
      searchStatus.textContent = dialog.dataset.loading
      searchPromise = fetch(searchURL, { credentials: 'same-origin' })
        .then((response) => {
          if (!response.ok) throw new Error(`Search index returned ${response.status}`)
          return response.json()
        })
        .then((items) => {
          searchIndex = items.map((item) => ({
            ...item,
            haystack: normalize([item.title, item.description, ...(item.headings || []), item.content].join(' ')),
          }))
          searchStatus.textContent = searchInput.value.trim() ? '' : searchStatus.dataset.initial || searchStatus.textContent
          return searchIndex
        })
        .catch((error) => {
          searchStatus.textContent = dialog.dataset.failed
          throw error
        })
    }
    return searchPromise
  }

  const excerpt = (item, query) => {
    const text = String(item.description || item.content || '').replace(/\s+/g, ' ').trim()
    const index = normalize(text).indexOf(query)
    if (index < 0) return text.slice(0, 138) + (text.length > 138 ? '…' : '')
    const start = Math.max(0, index - 46)
    const segment = text.slice(start, start + 150)
    return `${start > 0 ? '…' : ''}${segment}${start + 150 < text.length ? '…' : ''}`
  }

  const scoreItem = (item, terms) => {
    const title = normalize(item.title)
    const headings = normalize((item.headings || []).join(' '))
    const description = normalize(item.description)
    let score = 0
    for (const term of terms) {
      if (!item.haystack.includes(term)) return -1
      if (title === term) score += 120
      else if (title.includes(term)) score += 70
      if (headings.includes(term)) score += 35
      if (description.includes(term)) score += 20
      score += Math.max(1, 12 - item.haystack.indexOf(term) / 120)
    }
    return score
  }

  const setActiveResult = (next) => {
    const links = [...searchResults.querySelectorAll('a')]
    links.forEach((link) => link.classList.remove('is-selected'))
    if (!links.length) {
      activeResult = -1
      return
    }
    activeResult = (next + links.length) % links.length
    links[activeResult].classList.add('is-selected')
    links[activeResult].scrollIntoView({ block: 'nearest' })
  }

  const renderResults = async () => {
    const query = normalize(searchInput.value)
    searchResults.replaceChildren()
    activeResult = -1
    if (!query) {
      searchStatus.textContent = searchStatus.dataset.initial || ''
      return
    }
    let items
    try {
      items = await loadSearchIndex()
    } catch {
      return
    }
    const terms = query.split(' ').filter(Boolean)
    const matches = items
      .map((item) => ({ item, score: scoreItem(item, terms) }))
      .filter(({ score }) => score >= 0)
      .sort((a, b) => b.score - a.score || a.item.title.localeCompare(b.item.title))
      .slice(0, 8)

    if (!matches.length) {
      searchStatus.textContent = dialog.dataset.empty
      return
    }
    searchStatus.textContent = `${matches.length} ${dialog.dataset.resultLabel}`
    const fragment = document.createDocumentFragment()
    matches.forEach(({ item }) => {
      const li = document.createElement('li')
      const link = document.createElement('a')
      const group = document.createElement('span')
      const title = document.createElement('strong')
      const description = document.createElement('p')
      link.href = new URL(item.url, new URL(searchURL, document.baseURI)).href
      group.textContent = item.group || 'Lumi Docs'
      title.textContent = item.title
      description.textContent = excerpt(item, terms[0])
      link.append(group, title, description)
      li.append(link)
      fragment.append(li)
    })
    searchResults.append(fragment)
  }

  if (searchStatus) searchStatus.dataset.initial = searchStatus.textContent

  const openSearch = (opener) => {
    if (!dialog) return
    lastSearchOpener = opener || document.activeElement
    if (docsSidebar?.classList.contains('is-open')) setDocsDrawer(false)
    dialog.showModal()
    searchInput.focus()
    loadSearchIndex().catch(() => {})
  }

  const closeSearchDialog = () => {
    if (!dialog?.open) return
    dialog.close()
  }

  searchOpeners.forEach((opener) => opener.addEventListener('click', () => openSearch(opener)))
  searchClose?.addEventListener('click', closeSearchDialog)
  dialog?.addEventListener('click', (event) => {
    if (event.target === dialog) closeSearchDialog()
  })
  dialog?.addEventListener('close', () => {
    lastSearchOpener?.focus?.({ preventScroll: true })
    lastSearchOpener = null
  })
  searchInput?.addEventListener('input', renderResults)
  searchInput?.addEventListener('keydown', (event) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActiveResult(activeResult + 1)
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActiveResult(activeResult - 1)
    } else if (event.key === 'Enter' && activeResult >= 0) {
      event.preventDefault()
      searchResults.querySelectorAll('a')[activeResult]?.click()
    }
  })
  document.addEventListener('keydown', (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key.toLocaleLowerCase() === 'k') {
      event.preventDefault()
      openSearch(document.activeElement)
    }
    if (event.key === 'Escape' && docsSidebar?.classList.contains('is-open')) setDocsDrawer(false)
  })
})()
