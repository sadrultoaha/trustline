import { queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, isApiError } from './client'
import type { Me } from './types'

export const meQueryOptions = queryOptions({
  queryKey: ['me'],
  queryFn: async (): Promise<Me | null> => {
    try {
      return await api.get<Me>('/me')
    } catch (e) {
      if (isApiError(e) && e.status === 401) return null
      throw e
    }
  },
  staleTime: 30_000,
})

export function useMe() {
  return useQuery(meQueryOptions)
}

export function usePermissions() {
  const { data } = useMe()
  const perms = new Set(data?.permissions ?? [])
  return { can: (p: string) => perms.has(p), role: data?.role }
}

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { email: string; password: string }) => api.post<Me>('/auth/login', input),
    onSuccess: (me) => qc.setQueryData(['me'], me),
  })
}

export function useSignup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { email: string; password: string; name: string; org_name?: string; invite_token?: string }) => api.post<Me>('/auth/signup', input),
    onSuccess: (me) => qc.setQueryData(['me'], me),
  })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.post('/auth/logout'),
    onSuccess: () => {
      qc.setQueryData(['me'], null)
      qc.clear()
    },
  })
}

export function useSwitchOrg() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (org_id: string) => api.post<Me>('/me/switch-org', { org_id }),
    onSuccess: (me) => {
      qc.clear()
      qc.setQueryData(['me'], me)
    },
  })
}

export function useCreateOrg() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => api.post<Me>('/orgs', { name }),
    onSuccess: (me) => {
      qc.clear()
      qc.setQueryData(['me'], me)
    },
  })
}

export function useUpdateProfile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { name: string }) => api.patch<Me>('/me', input),
    onSuccess: (me) => qc.setQueryData(['me'], me),
  })
}

export function useChangePassword() {
  return useMutation({
    mutationFn: (input: { current_password: string; new_password: string }) => api.post('/me/password', input),
  })
}
