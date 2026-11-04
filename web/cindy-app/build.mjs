import { build } from 'vite'
import react from '@vitejs/plugin-react'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { themeCSS } from './theme.js'

const root = fileURLToPath(new URL('.', import.meta.url))
export async function buildCindy({ entry = 'main.jsx', target = '../../cindy-plugins/lumi/workspace.html' } = {}) {
  const result = await build({ configFile: false, root, plugins: [react()], define: { 'process.env.NODE_ENV': JSON.stringify('production') }, build: { write: false, minify: true, cssCodeSplit: false, lib: { entry: `${root}${entry}`, name: 'LumiCindyWorkspace', formats: ['iife'] }, rolldownOptions: { output: { codeSplitting: false } } } })
  const outputs = result.output ?? result[0].output
  const js = outputs.filter(v => v.type === 'chunk').map(v => v.code).join('\n').replace(/<\/script/gi, '<\\/script')
  const css = themeCSS(outputs.filter(v => v.type === 'asset' && v.fileName.endsWith('.css')).map(v => v.source).join('\n'))
  const template = await readFile(new URL('template.html', import.meta.url), 'utf8')
  const output = new URL(target, import.meta.url)
  await mkdir(new URL('.', output), { recursive: true })
  await writeFile(output, template.replace('/*__CSS__*/', () => css).replace('/*__JS__*/', () => js))
}
if (process.argv[1] === fileURLToPath(import.meta.url)) await buildCindy()
