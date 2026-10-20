import { queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import type { ActivityEntry, Invite, InvitePreview, Me, Member, Org, OrgSettings, Page, Role } from './types'

export const orgQuery = queryOptions({ queryKey: ['org'], queryFn: () => api.get<Org>('/org') })
export const membersQuery = queryOptions({ queryKey: ['org', 'members'], queryFn: () => api.get<{ items: Member[] }>('/org/members') })
export const invitesQuery = queryOptions({ queryKey: ['org', 'invites'], queryFn: () => api.get<{ items: Invite[] }>('/org/invites') })

export function useOrg() {
  return useQuery(orgQuery)
}
export function useMembers() {
  return useQuery(membersQuery)
}
export function useInvites() {
  return useQuery(invitesQuery)
}

export function useUpdateOrg() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { name?: string; settings?: OrgSettings }) => api.patch<Org>('/org', input),
    onSuccess: (org) => {
      qc.setQueryData(['org'], org)
      void qc.invalidateQueries({ queryKey: ['me'] })
      void qc.invalidateQueries({ queryKey: ['activity'] })
    },
  })
}

export function useCreateInvite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { email: string; role: Role }) => api.post<Invite>('/org/invites', input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['org', 'invites'] })
      void qc.invalidateQueries({ queryKey: ['activity'] })
    },
  })
}

export function useRevokeInvite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.delete(`/org/invites/${id}`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['org', 'invites'] })
      void qc.invalidateQueries({ queryKey: ['activity'] })
    },
  })
}

export function useResendInvite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.post<Invite>(`/org/invites/${id}/resend`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['org', 'invites'] }),
  })
}

export function useUpdateMemberRole() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { id: string; role: Role }) => api.patch<Member>(`/org/members/${input.id}`, { role: input.role }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['org', 'members'] })
      void qc.invalidateQueries({ queryKey: ['activity'] })
    },
  })
}

export function useRemoveMember() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.delete(`/org/members/${id}`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['org', 'members'] })
      void qc.invalidateQueries({ queryKey: ['activity'] })
    },
  })
}

export function invitePreviewQuery(token: string) {
  return queryOptions({ queryKey: ['invite', token], queryFn: () => api.get<InvitePreview>(`/invites/${token}`), retry: false })
}

export function useAcceptInvite() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (token: string) => api.post<Me>(`/invites/${token}/accept`),
    onSuccess: (me) => {
      qc.clear()
      qc.setQueryData(['me'], me)
    },
  })
}

export function activityQuery(params: { cursor?: string; action?: string; target_type?: string } = {}) {
  const qs = new URLSearchParams()
  if (params.cursor) qs.set('cursor', params.cursor)
  if (params.action) qs.set('action', params.action)
  if (params.target_type) qs.set('target_type', params.target_type)
  qs.set('limit', '50')
  return queryOptions({ queryKey: ['activity', params], queryFn: () => api.get<Page<ActivityEntry>>(`/activity?${qs.toString()}`) })
}
