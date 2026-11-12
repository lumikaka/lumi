import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'

const source = readFileSync(new URL('../../../internal/production/website_static/reader.js', import.meta.url), 'utf8')
function reader(format = 'classic_picture_book') {
  class Element {
    constructor(tag) { this.tagName = tag.toUpperCase(); this.children = []; this.listeners = {}; this.attributes = {} }
    append(...elements) { this.children.push(...elements) }
    replaceChildren(...elements) { this.children = elements }
    setAttribute(key, value) { this.attributes[key] = value }
    addEventListener(event, handler) { this.listeners[event] = handler }
    focus() {}
  }
  const app = new Element('main'), listeners = {}
  const pages = [1, 2, 3].map(number => ({ number, role: 'body', title: `Page ${number}`, src: `media/${number}.jpg`, width: 1000, height: 3000 }))
  const book = { title: '<Book>', format, missing_count: 0, chapters: [
    { uuid: 'chapter-one', code: 'vol01.ch99', title: '<One>', pages },
    { uuid: 'chapter-two', code: 'vol02.ch01', title: 'Two', pages: [pages[0]] },
    { uuid: 'chapter-hundred', code: 'vol01.ch100', title: 'Hundred', pages: [] },
  ] }
  let hash = '#chapter=chapter-one&page=1'
  const location = { get hash() { return hash }, set hash(value) { hash = value; listeners.hashchange?.() } }
  const window = { LUMI_WEBSITE: book, addEventListener: (event, handler) => { listeners[event] = handler }, scrollTo() {} }
  const document = { documentElement: new Element('html'), getElementById: () => app, createElement: tag => new Element(tag), title: '' }
  vm.runInNewContext(source, { window, document, location, URLSearchParams })
  const all = (node = app) => [node, ...node.children.flatMap(child => all(child))]
  return { app, window, location, document, all, key: key => listeners.keydown({ key, target: app, preventDefault() {} }) }
}

test('standalone reader supports bounded pages, keyboard, swipe and chapter selection', () => {
  const ui = reader()
  assert.equal(ui.document.documentElement.attributes['data-reader'], 'picture-book')
  assert.equal(ui.all().filter(el => el.tagName === 'FIGCAPTION').length, 1)
  const button = text => ui.all().find(el => el.tagName === 'BUTTON' && el.textContent.includes(text))
  assert.equal(button('Previous').disabled, true)
  button('Next').listeners.click()
  assert.match(ui.location.hash, /page=2$/)
  ui.key('ArrowRight')
  assert.match(ui.location.hash, /page=3$/)
  assert.equal(button('Next').disabled, true)
  ui.key('ArrowRight')
  assert.match(ui.location.hash, /page=3$/)
  ui.app.listeners.touchstart({ touches: [{ clientX: 50, clientY: 200 }] })
  ui.app.listeners.touchend({ changedTouches: [{ clientX: 250, clientY: 205 }] })
  assert.match(ui.location.hash, /page=2$/)
  ui.app.listeners.touchstart({ touches: [{ clientX: 250, clientY: 100 }] })
  ui.app.listeners.touchend({ changedTouches: [{ clientX: 240, clientY: 400 }] })
  assert.match(ui.location.hash, /page=2$/) // scrolling does not turn the page
  const select = ui.all().find(el => el.tagName === 'SELECT')
  select.value = 'chapter-two'; select.listeners.change()
  assert.equal(ui.location.hash, '#chapter=chapter-two&page=1')
  assert.match(ui.document.title, /Two/)
  ui.location.hash = '#chapter=chapter-one&page=999'
  assert.equal(button('Next').disabled, true)
  ui.location.hash = '#'
  assert.ok(ui.all().some(el => el.tagName === 'H1' && el.textContent === '<Book>'))
  assert.equal(ui.all().filter(el => el.className === 'chapter-card').length, 3)
})

for (const format of ['classic_picture_book', 'vertical_strip']) {
  test(`${format} contents toggle by volume and chapter number, highest first`, () => {
    const ui = reader(format)
    ui.location.hash = '#'
    const cardLinks = () => ui.all().filter(el => el.className === 'chapter-card').map(el => el.href)
    const sortButton = () => ui.all().find(el => el.tagName === 'BUTTON')
    const lowest = '#chapter=chapter-one&page=1'
    const middle = '#chapter=chapter-hundred&page=1'
    const highest = '#chapter=chapter-two&page=1'
    assert.deepEqual(cardLinks(), [highest, middle, lowest])
    assert.equal(sortButton().attributes['aria-pressed'], 'true')

    sortButton().listeners.click()
    assert.deepEqual(cardLinks(), [lowest, middle, highest])
    assert.equal(sortButton().attributes['aria-pressed'], 'false')
    assert.equal(ui.location.hash, '#')

    ui.location.hash = ui.all().find(el => el.className === 'primary').href
    assert.equal(ui.location.hash, lowest)
    const navigation = ui.all().find(el => el.className === 'chapter-navigation')
    assert.equal(navigation.children.find(el => el.textContent.includes('Next')).href, highest)
    ui.location.hash = '#'
    assert.deepEqual(cardLinks(), [lowest, middle, highest])
    sortButton().listeners.click()
    assert.deepEqual(cardLinks(), [highest, middle, lowest])
    assert.equal(sortButton().attributes['aria-pressed'], 'true')
  })
}

test('vertical reader shows ordered full-width pages without page navigation', () => {
  const ui = reader('vertical_strip')
  assert.equal(ui.app.className, 'reader reader--vertical')
  assert.equal(ui.document.documentElement.attributes['data-reader'], 'comic')
  assert.equal(ui.all().filter(el => el.className === 'comic-strip')[0].children.length, 3)
  assert.equal(ui.all().filter(el => el.tagName === 'FIGCAPTION').length, 0)
  assert.deepEqual(ui.all().filter(el => el.tagName === 'IMG').map(el => el.src), ['./media/1.jpg', './media/2.jpg', './media/3.jpg'])
  assert.equal(ui.all().filter(el => el.className === 'page-controls').length, 0)
  ui.key('ArrowRight')
  assert.match(ui.location.hash, /page=1$/)
  ui.location.hash = '#'
  assert.ok(ui.all().some(el => el.tagName === 'H2' && el.textContent === '全部话数 / Episodes'))
  assert.ok(ui.all().some(el => el.className === 'primary' && el.textContent === '开始看漫画 / Read the comic'))
})
