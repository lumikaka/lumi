import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const adminPathPattern = /^\/admin(?:\/|$)/

export function rewriteAdminRequest(req, _res, next) {
  if (!['GET', 'HEAD'].includes(req.method)) {
    next()
    return
  }

  const requestUrl = new URL(req.url, 'http://vite.local')
  let pathname
  try {
    pathname = decodeURIComponent(requestUrl.pathname)
  } catch {
    next()
    return
  }

  const lastSegment = pathname.slice(pathname.lastIndexOf('/') + 1)
  const accept = req.headers.accept || ''
  const acceptsHtml = !accept || accept === '*/*' || accept.includes('text/html')
  if (adminPathPattern.test(pathname) && !lastSegment.includes('.') && acceptsHtml) {
    req.url = `/admin.html${requestUrl.search}`
  }
  next()
}

function adminHtmlFallback() {
  return {
    name: 'admin-html-fallback',
    configureServer(server) {
      server.middlewares.use(rewriteAdminRequest)
    },
    configurePreviewServer(server) {
      server.middlewares.use(rewriteAdminRequest)
    },
  }
}

export default defineConfig({
  plugins: [adminHtmlFallback(), react()],
  server: {
    host: '127.0.0.1',
    port: 5802,
    strictPort: true,
  },
  preview: {
    host: '127.0.0.1',
    port: 5802,
    strictPort: true,
  },
  build: {
    rollupOptions: {
      input: {
        main: 'index.html',
        admin: 'admin.html',
      },
    },
  },
})
