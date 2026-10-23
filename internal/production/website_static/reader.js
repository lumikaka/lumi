/* Standalone reader: no API calls, fonts, analytics, or remote resources. */
(() => {
  'use strict'
  const book = window.LUMI_WEBSITE
  const app = document.getElementById('app')
  if (!book || !Array.isArray(book.chapters)) { app.textContent = '作品数据不可用 / Book data unavailable'; return }
  const vertical = book.format === 'vertical_strip'
  document.documentElement.setAttribute('data-reader', vertical ? 'comic' : 'picture-book')
  const chapterLabel = vertical ? '话' : '章'
  const imageLabel = vertical ? '张' : '页'
  const node = (tag, text, className) => {
    const element = document.createElement(tag)
    if (text !== undefined) element.textContent = text
    if (className) element.className = className
    return element
  }
  const link = (text, href, className) => { const el = node('a', text, className); el.href = href; return el }
  const chapterURL = (chapter, page = 1) => `#chapter=${encodeURIComponent(chapter.uuid)}&page=${page}`
  const heading = (chapter) => [chapter.code, chapter.title].filter(Boolean).join(' · ')
  const pageLabel = (page, index) => page.role === 'front_cover' ? '封面 / Cover' : page.role === 'back_cover' ? '封底 / Back cover' : `第 ${page.number || index + 1} 页 / Page ${page.number || index + 1}`
  function picture(page, index, eager = false) {
    const figure = node('figure', undefined, 'page')
    const a = link('', `./${page.src}`)
    a.target = '_blank'; a.rel = 'noopener'
    a.setAttribute('aria-label', `${pageLabel(page, index)} · 打开大图 / Open image`)
    const image = node('img')
    image.src = `./${page.src}`; image.alt = page.title || pageLabel(page, index)
    image.width = page.width; image.height = page.height
    image.loading = eager ? 'eager' : 'lazy'; image.decoding = 'async'
    a.append(image); figure.append(a)
    if (!vertical) figure.append(node('figcaption', [pageLabel(page, index), page.title].filter(Boolean).join(' · ')))
    return figure
  }
  let turn = null
  function render() {
    app.replaceChildren(); turn = null
    const params = new URLSearchParams(location.hash.slice(1))
    const chapter = book.chapters.find((c) => c.uuid === params.get('chapter'))
    document.title = chapter ? `${heading(chapter)} · ${book.title}` : book.title
    if (!chapter) {
      app.className = 'home'
      const intro = node('section', undefined, 'intro')
      intro.append(node('p', vertical ? '条漫 · 向下滚动，故事继续 / Scroll into the story' : '一本故事，一段旅程 / A story to explore', 'eyebrow'), node('h1', book.title))
      const count = book.chapters.reduce((total, c) => total + c.pages.length, 0)
      intro.append(node('p', `${book.chapters.length} ${chapterLabel} · ${count} ${imageLabel} / ${book.chapters.length} ${vertical ? 'episodes' : 'chapters'} · ${count} ${vertical ? 'images' : 'pages'}`, 'muted'))
      if (book.missing_count) intro.append(node('p', `此版本仅包含已有图片，另有 ${book.missing_count} ${imageLabel}尚未收录。 / ${book.missing_count} unavailable images were omitted.`, 'notice'))
      const first = book.chapters.find((c) => c.pages.length)
      if (first) intro.append(link(vertical ? '开始看漫画 / Read the comic' : '开始阅读 / Start reading', chapterURL(first), 'primary'))
      app.append(intro, node('h2', vertical ? '全部话数 / Episodes' : '章节目录 / Contents'))
      const grid = node('div', undefined, 'chapters')
      book.chapters.forEach((c) => {
        const card = link('', chapterURL(c), 'chapter-card')
        if (c.pages[0]) {
          const cover = node('img'); cover.src = `./${c.pages[0].src}`; cover.alt = ''; cover.loading = 'lazy'
          card.append(cover)
        } else card.append(node('div', '尚无图片 / No images', 'empty-cover'))
        const detail = node('div', undefined, 'chapter-detail')
        detail.append(node('h3', heading(c)), node('p', `${c.pages.length} ${imageLabel} / ${vertical ? 'images' : 'pages'}`, 'muted'))
        card.append(detail); grid.append(card)
      })
      app.append(grid)
    } else {
      app.className = vertical ? 'reader reader--vertical' : 'reader'
      if (vertical) app.append(node('p', '条漫阅读 / Webcomic', 'eyebrow'))
      app.append(node('h1', heading(chapter)))
      const picker = node('label', vertical ? '选择话数 / Episode ' : '跳转章节 / Chapter ', 'chapter-picker')
      const select = node('select')
      book.chapters.forEach((c) => { const option = node('option', heading(c)); option.value = c.uuid; option.selected = c === chapter; select.append(option) })
      select.addEventListener('change', () => { location.hash = chapterURL(book.chapters.find((c) => c.uuid === select.value)) })
      picker.append(select); app.append(picker)
      if (!chapter.pages.length) app.append(node('p', `本${chapterLabel}暂时没有可阅读的图片。 / No images available.`, 'notice'))
      else if (vertical) {
        const strip = node('div', undefined, 'comic-strip')
        chapter.pages.forEach((page, index) => strip.append(picture(page, index, index === 0)))
        app.append(strip)
      }
      else {
        const requested = Number.parseInt(params.get('page'), 10)
        const index = Math.max(0, Math.min(chapter.pages.length - 1, (Number.isFinite(requested) ? requested : 1) - 1))
        const controls = node('nav', undefined, 'page-controls'); controls.setAttribute('aria-label', '翻页 / Page navigation')
        const previous = node('button', '← 上一页 / Previous'); previous.disabled = index === 0
        const next = node('button', '下一页 / Next →'); next.disabled = index === chapter.pages.length - 1
        turn = (direction) => { const position = index + direction; if (position >= 0 && position < chapter.pages.length) location.hash = chapterURL(chapter, position + 1) }
        previous.addEventListener('click', () => turn(-1)); next.addEventListener('click', () => turn(1))
        const counter = node('span', `${index + 1} / ${chapter.pages.length}`); counter.setAttribute('aria-live', 'polite')
        controls.append(previous, counter, next)
        app.append(controls, picture(chapter.pages[index], index, true))
        app.append(node('p', '可用左右方向键或轻扫翻页，点击图片查看大图。 / Use arrow keys or swipe; tap an image to enlarge.', 'hint'))
      }
      const navigation = node('nav', undefined, 'chapter-navigation'); navigation.setAttribute('aria-label', '章节导航 / Chapters')
      const position = book.chapters.indexOf(chapter)
      if (position > 0) navigation.append(link(vertical ? '← 上一话 / Previous episode' : '← 上一章 / Previous chapter', chapterURL(book.chapters[position - 1])))
      navigation.append(link('目录 / Contents', '#'))
      if (position < book.chapters.length - 1) navigation.append(link(vertical ? '下一话 / Next episode →' : '下一章 / Next chapter →', chapterURL(book.chapters[position + 1])))
      app.append(navigation)
    }
  }
  window.addEventListener('hashchange', () => { render(); window.scrollTo(0, 0); app.focus({ preventScroll: true }) })
  window.addEventListener('keydown', (event) => {
    if (!turn || /INPUT|SELECT|TEXTAREA/.test(event.target.tagName) || event.altKey || event.ctrlKey || event.metaKey) return
    if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') { event.preventDefault(); turn(event.key === 'ArrowLeft' ? -1 : 1) }
  })
  let touch = null
  app.addEventListener('touchstart', (event) => { touch = event.touches.length === 1 ? { x: event.touches[0].clientX, y: event.touches[0].clientY } : null }, { passive: true })
  app.addEventListener('touchend', (event) => {
    if (!touch || !turn || !event.changedTouches[0]) return
    const dx = event.changedTouches[0].clientX - touch.x, dy = event.changedTouches[0].clientY - touch.y
    touch = null
    if (Math.abs(dx) > 65 && Math.abs(dx) > Math.abs(dy) * 1.5) turn(dx < 0 ? 1 : -1)
  }, { passive: true })
  render()
})()
