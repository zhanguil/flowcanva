export function createGenerationTaskId() {
  const uuid = globalThis.crypto?.randomUUID?.()
  if (uuid) return `task_${uuid}`

  // randomUUID is unavailable on plain HTTP LAN origins in Chromium. This ID
  // only correlates UI requests, so a timestamp plus random suffix is enough.
  return `task_${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`
}
