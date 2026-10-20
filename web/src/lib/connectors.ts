export function connectorName(type: string): string {
  return { aws: 'AWS', gcp: 'Google Cloud', github: 'GitHub', okta: 'Okta', google_workspace: 'Google Workspace', hris: 'HRIS' }[type] ?? type
}
