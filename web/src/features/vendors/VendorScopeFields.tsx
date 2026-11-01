import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { frameworksQuery } from '@/api/compliance'
import { questionnaireLibraryQuery, type QuestionnaireSummary } from '@/api/phase3'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'

/** questionnaires: null follows the frameworks (the server's suggested set); a list is an explicit attachment. */
export interface VendorScope {
  frameworks: string[]
  questionnaires: string[] | null
}

/** Mirrors the server: general questionnaires plus those tied to a mapped framework. */
export function suggestedQuestionnaires(library: QuestionnaireSummary[], frameworks: string[]) {
  return library.filter((q) => q.frameworks.length === 0 || q.frameworks.some((f) => frameworks.includes(f))).map((q) => q.id)
}

export function VendorScopeFields({ value, onChange, errors, keep = [] }: { value: VendorScope; onChange: (v: VendorScope) => void; errors?: Record<string, string>; keep?: string[] }) {
  const fws = useQuery(frameworksQuery()).data?.items ?? []
  const library = useQuery(questionnaireLibraryQuery()).data?.items ?? []
  const names = Object.fromEntries(fws.map((f) => [f.id, f.name]))
  // In-scope frameworks, plus existing mappings whose framework has since left scope.
  const options = fws.filter((f) => f.adopted || keep.includes(f.id))
  const selected = value.questionnaires ?? suggestedQuestionnaires(library, value.frameworks)
  const total = library.filter((q) => selected.includes(q.id)).reduce((n, q) => n + q.question_count, 0)

  const toggleFramework = (id: string, on: boolean) => {
    const frameworks = on ? [...value.frameworks, id] : value.frameworks.filter((f) => f !== id)
    let questionnaires = value.questionnaires
    if (questionnaires) {
      const tied = library.filter((q) => q.frameworks.includes(id)).map((q) => q.id)
      questionnaires = on
        ? [...questionnaires, ...tied.filter((q) => !questionnaires!.includes(q))]
        : questionnaires.filter((q) => {
            const t = library.find((x) => x.id === q)
            return !t || t.frameworks.length === 0 || t.frameworks.some((f) => frameworks.includes(f))
          })
    }
    onChange({ frameworks, questionnaires })
  }
  const toggleQuestionnaire = (id: string, on: boolean) => onChange({ ...value, questionnaires: on ? [...selected, id] : selected.filter((q) => q !== id) })

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Frameworks</Label>
        {options.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No frameworks in scope yet.{' '}
            <Link to="/app/frameworks" className="underline">
              Add one
            </Link>{' '}
            to map vendors to it.
          </p>
        ) : (
          <div className="flex flex-wrap gap-2" data-testid="vendor-frameworks">
            {options.map((f) => {
              const on = value.frameworks.includes(f.id)
              return (
                <label key={f.id} className="flex cursor-pointer items-center gap-1.5 rounded-md border px-2 py-1 text-sm has-[:checked]:border-primary has-[:checked]:bg-primary/5">
                  <input type="checkbox" checked={on} onChange={(e) => toggleFramework(f.id, e.target.checked)} data-testid={`vendor-fw-${f.id}`} />
                  {f.name}
                  {!f.adopted ? <span className="text-xs text-muted-foreground">(not in scope)</span> : null}
                </label>
              )
            })}
          </div>
        )}
        {errors?.frameworks ? <p className="text-xs text-destructive">{errors.frameworks}</p> : null}
      </div>
      <div className="space-y-2">
        <div className="flex items-baseline justify-between">
          <Label>Questionnaires</Label>
          <span className="text-xs text-muted-foreground" data-testid="vendor-question-total">
            {total} questions{value.questionnaires ? '' : ' · suggested from frameworks'}
          </span>
        </div>
        <div className="max-h-64 divide-y overflow-y-auto rounded-md border">
          {library.map((q) => {
            const on = selected.includes(q.id)
            const suggested = q.frameworks.some((f) => value.frameworks.includes(f))
            return (
              <label key={q.id} className="flex cursor-pointer items-start gap-2 px-3 py-2 text-sm hover:bg-accent/50">
                <input type="checkbox" className="mt-0.5" checked={on} onChange={(e) => toggleQuestionnaire(q.id, e.target.checked)} data-testid={`vendor-q-${q.id}`} />
                <span className="flex-1">
                  <span className="font-medium">{q.title}</span> <span className="text-xs text-muted-foreground">· {q.question_count} questions</span>
                  <span className="mt-0.5 flex flex-wrap gap-1">
                    {q.frameworks.length === 0 ? <Badge variant="outline">General</Badge> : q.frameworks.map((f) => <Badge key={f} variant="outline">{names[f] ?? f}</Badge>)}
                    {suggested ? <Badge variant="success">Suggested</Badge> : null}
                  </span>
                </span>
              </label>
            )
          })}
        </div>
        {errors?.questionnaires ? <p className="text-xs text-destructive">{errors.questionnaires}</p> : null}
      </div>
    </div>
  )
}

/** The request fields for a scope: questionnaires are sent only when attached explicitly. */
export function scopeInput(s: VendorScope) {
  return s.questionnaires ? { frameworks: s.frameworks, questionnaires: s.questionnaires } : { frameworks: s.frameworks }
}
