import type { Edge, Node } from '../types'

export function connectionAncestors(edges: Edge[], source: string): Set<string> {
  const incoming = new Map<string, string[]>()
  for (const edge of edges) {
    const list = incoming.get(edge.target_node_id) || []
    list.push(edge.source_node_id); incoming.set(edge.target_node_id, list)
  }
  const visited = new Set<string>(), queue = [source]
  for (let index = 0; index < queue.length; index++) {
    const id = queue[index]
    if (visited.has(id)) continue
    visited.add(id)
    queue.push(...(incoming.get(id) || []))
  }
  return visited
}

// Node/Edge records contain only scalar fields; share unchanged immutable
// records across history entries without parsing megabytes of content JSON.
export function shareRecords<T extends Node | Edge>(records: T[], previous: T[] = []): T[] {
  const byId = new Map(previous.map(record => [record.id, record]))
  return records.map(record => {
    const old = byId.get(record.id)
    const keys = Object.keys(record) as (keyof T)[]
    if (old && keys.length === Object.keys(old).length && keys.every(key => record[key] === old[key])) return old
    return { ...record }
  })
}
