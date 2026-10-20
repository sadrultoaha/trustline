import type { Condition, Expr, Rule } from '@/api/compliance'

const opWords: Record<string, string> = {
  eq: 'is', neq: 'is not', gt: '>', gte: '≥', lt: '<', lte: '≤', in: 'is one of', nin: 'is not one of', contains: 'contains', not_contains: 'does not contain',
  exists: 'is set', not_exists: 'is not set', matches: 'matches', len_gt: 'has more items than', len_lt: 'has fewer items than',
}

export function isCondition(e: Expr): e is Condition {
  return (e as Condition).field !== undefined
}

export function describeExpr(e: Expr): string {
  if ('and' in e) return e.and.map(describeExpr).join(' AND ')
  if ('or' in e) return '(' + e.or.map(describeExpr).join(' OR ') + ')'
  if ('not' in e) return 'NOT (' + describeExpr(e.not) + ')'
  const v = e.op === 'exists' || e.op === 'not_exists' ? '' : ' ' + JSON.stringify(e.value)
  return `${e.field}${e.match === 'any' ? ' (any item)' : ''} ${opWords[e.op] ?? e.op}${v}`
}

export function describeRule(r: Rule): string {
  const q = { all: 'Every', any: 'At least one', none: 'No', count: 'The number of' }[r.quantifier]
  const scope = r.where ? ` where ${describeExpr(r.where)}` : ''
  if (r.quantifier === 'count') {
    const b = [r.min != null ? `at least ${r.min}` : '', r.max != null ? `at most ${r.max}` : ''].filter(Boolean).join(' and ')
    return `${q} ${r.resource}${scope} satisfying ${describeExpr(r.assert)} is ${b}`
  }
  return `${q} ${r.resource}${scope} ${r.quantifier === 'none' ? 'may satisfy' : 'satisfies'}: ${describeExpr(r.assert)}`
}
