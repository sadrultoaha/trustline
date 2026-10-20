// Writes dist/_redirects for Netlify: proxy backend paths to TRUSTLINE_API_URL
// and fall back to index.html for client-side routes.
import { writeFileSync } from 'node:fs'

const api = (process.env.TRUSTLINE_API_URL ?? '').replace(/\/+$/, '')
if (!/^https:\/\/.+/.test(api)) {
  console.error('TRUSTLINE_API_URL must be set to the backend origin, e.g. https://trustline-api.example.com')
  process.exit(1)
}
const proxied = ['/api/*', '/auth/*', '/agent/*', '/healthz', '/readyz']
const lines = proxied.map((p) => `${p}  ${api}${p.replace('*', ':splat')}  200`)
lines.push('/*  /index.html  200')
writeFileSync(new URL('../dist/_redirects', import.meta.url), lines.join('\n') + '\n')
console.log(`_redirects: proxying ${proxied.join(', ')} to ${api}`)
