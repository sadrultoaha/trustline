import { useState } from 'react'
import { useParams } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, ShieldCheck } from 'lucide-react'
import { toast } from 'sonner'
import { errorMessage } from '@/api/client'
import { portalQuery, usePortalSave, type PortalView } from '@/api/phase3'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { cn, formatDate } from '@/lib/utils'

export function VendorPortalPage() {
  const { token } = useParams({ from: '/vendor-portal/$token' })
  const { data, isError } = useQuery(portalQuery(token))
  return (
    <div className="min-h-screen bg-muted/40 px-4 py-10">
      <div className="mx-auto max-w-3xl space-y-6">
        <div className="flex items-center gap-2 font-semibold">
          <span className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <ShieldCheck className="h-4 w-4" />
          </span>
          Trustline vendor portal
        </div>
        {isError ? (
          <Card>
            <CardContent className="p-6 text-sm">This link is invalid. Ask your contact to send a new questionnaire.</CardContent>
          </Card>
        ) : data ? (
          <PortalForm key={data.status} token={token} view={data} />
        ) : (
          <p className="text-sm text-muted-foreground">Loading…</p>
        )}
      </div>
    </div>
  )
}

function PortalForm({ token, view }: { token: string; view: PortalView }) {
  const save = usePortalSave(token)
  const [answers, setAnswers] = useState<Record<string, string>>(view.answers)
  const locked = view.status === 'submitted' || view.status === 'expired'
  const questions = view.templates.flatMap((t) => t.questions)
  const answered = questions.filter((q) => answers[q.id]).length
  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{view.templates.length === 1 ? view.templates[0].title : 'Security review'}</CardTitle>
          <CardDescription>
            {view.org_name} asks {view.vendor_name} to complete {view.templates.length === 1 ? 'this questionnaire' : `${view.templates.length} questionnaires (${questions.length} questions)`}.{' '}
            {view.templates.length === 1 ? `${view.templates[0].description} ` : ''}Your answers are saved as you go{locked ? '' : `; the link expires ${formatDate(view.expires_at)}`}.
          </CardDescription>
        </CardHeader>
      </Card>
      {view.status === 'submitted' ? (
        <Alert variant="success" data-testid="portal-submitted">
          <CheckCircle2 className="h-4 w-4" />
          <AlertDescription>Thank you — your answers were submitted to {view.org_name}.</AlertDescription>
        </Alert>
      ) : null}
      {view.status === 'expired' ? (
        <Alert variant="destructive">
          <AlertDescription>This questionnaire has expired.</AlertDescription>
        </Alert>
      ) : null}
      {view.templates.map((t) => (
        <section key={t.id} className="space-y-4" data-testid={`portal-template-${t.id}`}>
          {view.templates.length > 1 ? (
            <div>
              <h2 className="text-lg font-semibold">{t.title}</h2>
              <p className="text-sm text-muted-foreground">{t.description}</p>
            </div>
          ) : null}
          {[...new Set(t.questions.map((q) => q.section))].map((sec) => (
            <Card key={`${t.id}-${sec}`}>
              <CardHeader>
                <CardTitle className="text-base">{sec}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-5">
                {t.questions
                  .filter((q) => q.section === sec)
                  .map((q) => (
                    <div key={q.id} className="space-y-2" data-testid={`portal-q-${q.id}`}>
                      <div className="text-sm font-medium">{q.question}</div>
                      {q.type === 'text' ? (
                        <Textarea value={answers[q.id] ?? ''} disabled={locked} onChange={(e) => setAnswers({ ...answers, [q.id]: e.target.value })} data-testid={`portal-a-${q.id}`} />
                      ) : (
                        <div className="flex flex-wrap gap-2">
                          {(q.type === 'yes_no' ? ['Yes', 'No'] : (q.options ?? [])).map((o) => (
                            <button
                              type="button"
                              key={o}
                              disabled={locked}
                              onClick={() => setAnswers({ ...answers, [q.id]: o })}
                              className={cn('rounded-md border px-3 py-1.5 text-sm', answers[q.id] === o ? 'border-primary bg-primary text-primary-foreground' : 'hover:bg-accent')}
                              data-testid={`portal-a-${q.id}-${o}`}
                            >
                              {o}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                  ))}
              </CardContent>
            </Card>
          ))}
        </section>
      ))}
      {!locked ? (
        <div className="sticky bottom-4 flex items-center justify-between rounded-lg border bg-background p-3 shadow">
          <span className="text-sm text-muted-foreground" data-testid="portal-progress">
            {answered}/{questions.length} answered
          </span>
          <div className="flex gap-2">
            <Button variant="outline" loading={save.isPending && !save.variables?.submit} onClick={() => save.mutate({ answers, submit: false }, { onSuccess: () => toast.success('Saved') })} data-testid="portal-save">
              Save draft
            </Button>
            <Button loading={save.isPending && save.variables?.submit} onClick={() => save.mutate({ answers, submit: true }, { onError: (e) => toast.error(errorMessage(e)) })} data-testid="portal-submit">
              Submit
            </Button>
          </div>
        </div>
      ) : null}
    </>
  )
}
