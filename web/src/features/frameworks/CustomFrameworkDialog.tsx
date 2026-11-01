import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Plus, Trash2, X } from 'lucide-react'
import { toast } from 'sonner'
import { errorMessage, isApiError } from '@/api/client'
import {
  controlLibraryQuery,
  useCreateCustomFramework,
  useUpdateCustomFramework,
  type CustomFrameworkInput,
  type CustomRequirementInput,
  type LibraryControl,
} from '@/api/compliance'
import { FormField } from '@/components/layout/FormField'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

type Row = CustomRequirementInput & { key: number }

let nextKey = 0
const emptyRow = (): Row => ({ key: nextKey++, code: '', section: '', title: '', description: '', controls: [] })

/** Creates a custom framework, or edits one when `id` and `initial` are given. */
export function CustomFrameworkDialog({ id, initial, onClose }: { id?: string; initial?: CustomFrameworkInput; onClose: () => void }) {
  const create = useCreateCustomFramework()
  const update = useUpdateCustomFramework(id ?? '')
  const navigate = useNavigate()
  const library = useQuery(controlLibraryQuery()).data?.items ?? []
  const [f, setF] = useState({ name: initial?.name ?? '', version: initial?.version ?? '', publisher: initial?.publisher ?? '', description: initial?.description ?? '' })
  const [rows, setRows] = useState<Row[]>(() => (initial?.requirements.length ? initial.requirements.map((r) => ({ ...r, key: nextKey++ })) : [emptyRow()]))
  const [fields, setFields] = useState<Record<string, string>>({})
  const setRow = (key: number, patch: Partial<Row>) => setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)))
  const err = (i: number, name: string) => fields[`requirements.${i}.${name}`]

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] max-w-3xl overflow-y-auto">
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            setFields({})
            const input: CustomFrameworkInput = {
              ...f,
              requirements: rows.map((r) => ({ code: r.code, section: r.section, title: r.title, description: r.description, controls: r.controls })),
            }
            const onError = (err: unknown) => {
              if (isApiError(err) && Object.keys(err.fields).length) {
                setFields(err.fields)
                toast.error('Fix the highlighted fields')
              } else toast.error(errorMessage(err))
            }
            if (id) {
              update.mutate(input, {
                onSuccess: () => {
                  toast.success(`${f.name} updated`)
                  onClose()
                },
                onError,
              })
            } else {
              create.mutate(input, {
                onSuccess: (d) => {
                  toast.success(`${d.name} created. Add it to scope to start tracking readiness.`)
                  onClose()
                  void navigate({ to: '/app/frameworks/$id', params: { id: d.id } })
                },
                onError,
              })
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>{id ? 'Edit framework' : 'New custom framework'}</DialogTitle>
            <DialogDescription>
              Write your own requirements, such as a customer contract or an internal standard, and map each one to controls from the library. Controls that already pass
              count toward it straight away.
            </DialogDescription>
          </DialogHeader>
          <FormField label="Name" htmlFor="cf-name" error={fields.name}>
            <Input id="cf-name" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="Acme customer security schedule" data-testid="cf-name" />
          </FormField>
          <div className="grid gap-3 sm:grid-cols-2">
            <FormField label="Publisher" htmlFor="cf-publisher" error={fields.publisher}>
              <Input id="cf-publisher" value={f.publisher} onChange={(e) => setF({ ...f, publisher: e.target.value })} placeholder="Acme Corp" />
            </FormField>
            <FormField label="Version" htmlFor="cf-version" error={fields.version}>
              <Input id="cf-version" value={f.version} onChange={(e) => setF({ ...f, version: e.target.value })} placeholder="2026" />
            </FormField>
          </div>
          <FormField label="Description" htmlFor="cf-description" error={fields.description}>
            <Textarea id="cf-description" rows={2} value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} />
          </FormField>

          <div className="space-y-3">
            <div className="flex items-baseline justify-between">
              <h3 className="text-sm font-medium">Requirements</h3>
              {fields.requirements ? <span className="text-xs text-destructive">{fields.requirements}</span> : null}
            </div>
            {rows.map((r, i) => (
              <div key={r.key} className="space-y-3 rounded-md border p-3" data-testid={`cf-req-${i}`}>
                <div className="grid gap-3 sm:grid-cols-[120px_1fr_auto]">
                  <FormField label="Code" htmlFor={`cf-code-${r.key}`} error={err(i, 'code')}>
                    <Input id={`cf-code-${r.key}`} value={r.code} onChange={(e) => setRow(r.key, { code: e.target.value })} placeholder={`REQ-${i + 1}`} data-testid={`cf-code-${i}`} />
                  </FormField>
                  <FormField label="Section" htmlFor={`cf-section-${r.key}`} error={err(i, 'section')}>
                    <Input id={`cf-section-${r.key}`} value={r.section} onChange={(e) => setRow(r.key, { section: e.target.value })} placeholder="Requirements" />
                  </FormField>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="self-end"
                    disabled={rows.length === 1}
                    onClick={() => setRows(rows.filter((x) => x.key !== r.key))}
                    aria-label={`Remove requirement ${i + 1}`}
                  >
                    <Trash2 />
                  </Button>
                </div>
                <FormField label="Title" htmlFor={`cf-title-${r.key}`} error={err(i, 'title')}>
                  <Input id={`cf-title-${r.key}`} value={r.title} onChange={(e) => setRow(r.key, { title: e.target.value })} data-testid={`cf-title-${i}`} />
                </FormField>
                <FormField label="Description" htmlFor={`cf-desc-${r.key}`} error={err(i, 'description')}>
                  <Textarea id={`cf-desc-${r.key}`} rows={2} value={r.description} onChange={(e) => setRow(r.key, { description: e.target.value })} />
                </FormField>
                <FormField label="Satisfied by controls" htmlFor={`cf-controls-${r.key}`} error={err(i, 'controls')}>
                  <ControlPicker id={`cf-controls-${r.key}`} library={library} value={r.controls} onChange={(controls) => setRow(r.key, { controls })} testId={`cf-controls-${i}`} />
                </FormField>
              </div>
            ))}
            <Button type="button" variant="outline" size="sm" onClick={() => setRows([...rows, emptyRow()])} data-testid="cf-add-requirement">
              <Plus /> Add requirement
            </Button>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={create.isPending || update.isPending} data-testid="cf-save">
              {id ? 'Save changes' : 'Create framework'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ControlPicker({ id, library, value, onChange, testId }: { id: string; library: LibraryControl[]; value: string[]; onChange: (v: string[]) => void; testId: string }) {
  const [q, setQ] = useState('')
  const byCode = Object.fromEntries(library.map((c) => [c.code, c]))
  const needle = q.trim().toLowerCase()
  const matches = needle ? library.filter((c) => !value.includes(c.code) && `${c.code} ${c.name} ${c.domain}`.toLowerCase().includes(needle)).slice(0, 8) : []
  const add = (code: string) => {
    onChange([...value, code])
    setQ('')
  }
  return (
    <div className="space-y-2">
      {value.length ? (
        <div className="flex flex-wrap gap-1.5">
          {value.map((code) => (
            <span key={code} className="inline-flex items-center gap-1 rounded border px-1.5 py-0.5 text-xs" title={byCode[code]?.name}>
              <code>{code}</code>
              <span className="max-w-48 truncate text-muted-foreground">{byCode[code]?.name}</span>
              <button type="button" onClick={() => onChange(value.filter((c) => c !== code))} aria-label={`Remove ${code}`} className="hover:text-destructive">
                <X className="h-3 w-3" />
              </button>
            </span>
          ))}
        </div>
      ) : null}
      <div className="relative">
        <Input
          id={id}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && matches.length) {
              e.preventDefault()
              add(matches[0].code)
            }
          }}
          placeholder="Search controls by code, name or domain"
          autoComplete="off"
          data-testid={testId}
        />
        {matches.length ? (
          <div className="absolute z-10 mt-1 max-h-64 w-full overflow-y-auto rounded-md border bg-popover p-1 shadow-md">
            {matches.map((c) => (
              <button
                key={c.code}
                type="button"
                className="flex w-full items-baseline gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-accent"
                onClick={() => add(c.code)}
                data-testid={`cf-option-${c.code}`}
              >
                <code className="text-xs font-semibold">{c.code}</code>
                <span className="flex-1 truncate">{c.name}</span>
                <span className="text-xs text-muted-foreground">{c.domain}</span>
              </button>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  )
}
