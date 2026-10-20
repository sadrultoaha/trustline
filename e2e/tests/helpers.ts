import { expect, type APIRequestContext, type Page } from '@playwright/test'

export const DEMO_PASSWORD = 'Trustline-demo-2026!'
export const ACME = {
  admin: { email: 'ada@acme.test', name: 'Ada Lovelace' },
  editor: { email: 'grace@acme.test', name: 'Grace Hopper' },
  auditor: { email: 'auditor@acme.test', name: 'Alan Auditor' },
  employee: { email: 'sam@acme.test', name: 'Sam Patel' },
}
export const GLOBEX = { admin: { email: 'linus@globex.test', name: 'Linus Torvalds' } }

const MAILPIT = process.env.MAILPIT_URL ?? 'http://localhost:8025'

export function unique(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 1e5)}`
}

export async function login(page: Page, email: string, password = DEMO_PASSWORD) {
  await page.goto('/login')
  await page.getByTestId('login-email').fill(email)
  await page.getByTestId('login-password').fill(password)
  await page.getByTestId('login-submit').click()
  await expect(page.getByTestId('current-org-name')).toBeVisible()
}

export async function logout(page: Page) {
  await page.getByTestId('user-menu').click()
  await page.getByTestId('logout').click()
  await expect(page).toHaveURL(/\/login/)
}

interface MailpitMessage {
  ID: string
  Subject: string
  To: { Address: string }[]
}

/** Poll mailpit for the newest message to `to` whose subject contains `subject`. */
export async function waitForEmail(request: APIRequestContext, to: string, subject: string, timeoutMs = 20_000): Promise<{ text: string; html: string; subject: string }> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const res = await request.get(`${MAILPIT}/api/v1/search?query=${encodeURIComponent(`to:${to}`)}`)
    if (res.ok()) {
      const body = (await res.json()) as { messages: MailpitMessage[] }
      const msg = body.messages.find((m) => m.Subject.includes(subject))
      if (msg) {
        const full = await request.get(`${MAILPIT}/api/v1/message/${msg.ID}`)
        const data = (await full.json()) as { Text: string; HTML: string; Subject: string }
        return { text: data.Text, html: data.HTML, subject: data.Subject }
      }
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  throw new Error(`no email to ${to} with subject containing "${subject}" within ${timeoutMs}ms`)
}

export function extractLink(text: string, marker: string): string {
  const m = text.match(new RegExp(`(https?://[^\\s"<]*${marker}[^\\s"<]*)`))
  if (!m) throw new Error(`no link containing ${marker} in: ${text}`)
  return m[1]
}
