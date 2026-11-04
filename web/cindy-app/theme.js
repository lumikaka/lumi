// Convert only the independent Cindy artifact. Shared Sass and the Web/Codex
// outputs retain their established colors and default appearance.
const roles = {
  f3f3ef: 'panel-bg', eaeae4: 'panel-bg', '20231f': 'text-primary', '666b63': 'text-secondary', '747970': 'text-secondary',
  '2f6b55': 'accent', '1f513f': 'accent', e2eee7: 'hover-bg', d3e6dc: 'active-bg',
  ffffff: 'surface', f7f7f3: 'panel-bg', fbfbf8: 'surface', fdfdf9: 'surface', f5f5f0: 'hover-bg',
  dedfd8: 'border-default', c9cbc1: 'border-default', e9eae4: 'border-default',
  b42318: 'danger', '8f1d14': 'danger', fff1f0: 'surface', '15803d': 'success', eefbf2: 'surface',
  '8a5a00': 'warning', fff7ed: 'surface', d99153: 'warning', f5e8da: 'surface',
}
export function themeCSS(css) {
  return css.split(/([;{}])/).map(part => part.replace(/var\((?:[^()]|\([^)]*\))*\)|rgba?\(\s*[\d.,% /]+\)|#[0-9a-f]{3,8}\b/gi, token => {
    if (token.startsWith('var(')) return token
    let hex, alpha = 1
    if (token.startsWith('#')) {
      hex = token.slice(1).toLowerCase()
      if (hex.length === 3 || hex.length === 4) hex = [...hex].map(c => c + c).join('')
      if (hex.length === 8) { alpha = parseInt(hex.slice(6), 16) / 255; hex = hex.slice(0, 6) }
    }
    else {
      const values = token.match(/[\d.]+/g).map(Number)
      if (values.length < 3) return token
      hex = values.slice(0, 3).map(v => Math.round(v).toString(16).padStart(2, '0')).join(''); alpha = values[3] ?? 1
    }
    const role = hex === 'ffffff' && /^\s*color\s*:/.test(part) ? 'text-on-accent' : roles[hex]
    if (!role) return token
    const color = `var(--${role},#${hex})`
    return alpha === 1 ? color : `color-mix(in srgb,${color} ${Math.round(alpha * 10000) / 100}%,transparent)`
  })).join('')
}
