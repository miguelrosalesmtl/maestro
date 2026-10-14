/** The Jira site's name: "acme" for acme.atlassian.net, the host for a self-hosted Jira. */
export function workspaceName(c?: { mode: string; site: string } | null): string {
  if (!c?.site) return c?.mode === 'connecting' ? 'Maestro' : 'Demo workspace'
  const cloud = c.site.match(/^([^.]+)\.(atlassian\.net|jira\.com)$/i)
  return cloud ? cloud[1] : c.site
}
