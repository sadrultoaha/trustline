import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, XCircle } from 'lucide-react'
import { toast } from 'sonner'
import { errorMessage } from '@/api/client'
import { trainingQuery, useSubmitQuiz, type QuizResult } from '@/api/phase2'
import { PageHeader } from '@/components/layout/PageHeader'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'

export function TrainingPage() {
  const { data } = useQuery(trainingQuery())
  const submit = useSubmitQuiz()
  const [step, setStep] = useState(0)
  const [answers, setAnswers] = useState<Record<string, number>>({})
  const [result, setResult] = useState<QuizResult | null>(null)
  if (!data) return <p className="text-sm text-muted-foreground">Loading…</p>
  const c = data.course
  const quizStep = c.modules.length
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <PageHeader title={c.title} description={c.description} />
      <Progress value={(Math.min(step, quizStep) / quizStep) * 100} />
      {step < quizStep ? (
        <Card>
          <CardHeader>
            <CardTitle>
              {step + 1}. {c.modules[step]!.title}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="whitespace-pre-line text-sm leading-relaxed" data-testid="training-module">
              {c.modules[step]!.body}
            </div>
            <div className="flex justify-between">
              <Button variant="outline" disabled={step === 0} onClick={() => setStep(step - 1)}>
                Back
              </Button>
              <Button onClick={() => setStep(step + 1)} data-testid="training-next">
                {step === quizStep - 1 ? 'Take the quiz' : 'Next'}
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle>Quiz — {c.passing_score}% to pass</CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            {result ? (
              <Alert variant={result.passed ? 'success' : 'destructive'} data-testid="quiz-result">
                <AlertDescription>
                  {result.passed ? 'Passed' : 'Not passed'} — you scored {result.score}% ({result.required}% required).
                </AlertDescription>
              </Alert>
            ) : null}
            {c.questions.map((q, n) => {
              const fb = result?.feedback.find((f) => f.question_id === q.id)
              return (
                <fieldset key={q.id} className="space-y-2" data-testid={`quiz-${q.id}`}>
                  <legend className="text-sm font-medium">
                    {n + 1}. {q.question}
                  </legend>
                  {q.options.map((o, i) => (
                    <label key={i} className={cn('flex items-center gap-2 rounded-md border px-3 py-2 text-sm', fb && i === fb.correct_answer && 'border-success bg-success/10', fb && answers[q.id] === i && !fb.correct && 'border-destructive bg-destructive/10')}>
                      <input type="radio" name={q.id} checked={answers[q.id] === i} onChange={() => setAnswers({ ...answers, [q.id]: i })} disabled={!!result?.passed} data-testid={`quiz-${q.id}-${i}`} />
                      {o}
                    </label>
                  ))}
                  {fb ? (
                    <p className={cn('flex items-center gap-1 text-xs', fb.correct ? 'text-success' : 'text-destructive')}>
                      {fb.correct ? <CheckCircle2 className="h-3 w-3" /> : <XCircle className="h-3 w-3" />}
                      {fb.correct ? 'Correct' : 'Incorrect'}
                      {fb.explanation ? ` — ${fb.explanation}` : ''}
                    </p>
                  ) : null}
                </fieldset>
              )
            })}
            {!result?.passed ? (
              <Button
                disabled={Object.keys(answers).length < c.questions.length}
                loading={submit.isPending}
                data-testid="quiz-submit"
                onClick={() =>
                  submit.mutate(answers, {
                    onSuccess: (r) => {
                      setResult(r)
                      if (r.passed) toast.success('Training complete')
                    },
                    onError: (e) => toast.error(errorMessage(e)),
                  })
                }
              >
                Submit answers
              </Button>
            ) : null}
          </CardContent>
        </Card>
      )}
    </div>
  )
}
