import { build } from 'vite'
import react from '@vitejs/plugin-react'
import { readFile, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
const root = fileURLToPath(new URL('.', import.meta.url))
const result = await build({ configFile: false, root, plugins: [react()], define: { 'process.env.NODE_ENV': JSON.stringify('production') }, build: { write: false, minify: true, cssCodeSplit: false, lib: { entry: `${root}main.jsx`, name: 'LumiReview', formats: ['iife'] }, rollupOptions: { output: { inlineDynamicImports: true } } } })
const outputs = result.output ?? result[0].output
const js = outputs.filter((v) => v.type === 'chunk').map((v) => v.code).join('\n').replace(/<\/script/gi, '<\\/script')
const css = outputs.filter((v) => v.type === 'asset' && v.fileName.endsWith('.css')).map((v) => v.source).join('\n')
const template = await readFile(new URL('template.html', import.meta.url), 'utf8')
await writeFile(new URL('../../internal/textmcp/appdist/review.html', import.meta.url), template.replace('/*__CSS__*/', () => css).replace('/*__JS__*/', () => js))
