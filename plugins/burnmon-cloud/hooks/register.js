// burnmon-cloud 2.0.0: counts the tokens of a Cowork (Cloud) session for
// BurnMon on the linked computer. Plan:
// 02_roadmap/2026-11-24_cloud_mod_route_plan.md in the burnmon repo.
//
// Only inside a Cowork (Cloud) session (CLAUDE_CODE_ENTRYPOINT is
// remote_cowork) does it act; anywhere else every hook only passes the event
// on. Per model request (turn.step) it keeps one row of numbers and ids: no
// text, no tool input, no paths. The ids come from the session's own
// transcript, found by the uuid of the request's newest assistant row, so a
// request BurnMon also reads from a pulled transcript counts once (plan
// section 4a). The rows are rewritten whole into the session's outputs
// folder; at the end of a main turn, once the file has settled, the file is
// copied to ~/.burnmon/cloud on the computer with remote-devices.

const SOURCE = 'burnmon-cloud-mod'
const OUT_DIR = '/mnt/user-data/outputs'
const DEVICE_DIR = '~/.burnmon/cloud'
// Rows kept in the file. BurnMon keeps what it read before, so dropping the
// oldest rows loses nothing and keeps one file far under 4 MiB.
const MAX_ROWS = 10000
// The outputs folder uploads a write after a short delay; a copy 1 s after a
// write delivered the old content, 8 s after the new (measured 2026-10-12).
const SETTLE_MS = 10000
// How much of a transcript's end is searched for a request's row.
const TAIL_BYTES = 1048576

// The session's state; register starts it over on every load.
let st = { active: undefined, sid: undefined, timer: undefined, rows: [] }

export const stepKey = (turnId, index) => `turn-${turnId}-${index}`

// The uuid of the newest assistant row of a $.session.messages answer.
export function newestAssistantUuid(rows) {
  if (!Array.isArray(rows)) return undefined
  for (let i = rows.length - 1; i >= 0; i--) {
    if (rows[i] && rows[i].role === 'assistant') return rows[i].uuid
  }
  return undefined
}

// A subagent's transcript lies next to the main one, under subagents/.
export const subagentTranscript = (main, agentId) =>
  main.replace(/\.jsonl$/, '') + '/subagents/agent-' + agentId + '.jsonl'

// The request and message ids of the transcript row with this uuid, or
// undefined. Only the ids leave this function.
export function findIds(text, uuid) {
  let found
  for (const line of text.split('\n')) {
    if (!line.includes(uuid)) continue
    try {
      const o = JSON.parse(line)
      if (o.uuid === uuid && (o.requestId || (o.message && o.message.id))) {
        found = { requestId: o.requestId || '', messageId: (o.message && o.message.id) || '' }
      }
    } catch (_) { /* a cut line at the window's edge */ }
  }
  return found
}

// One file line: the drop format BurnMon reads (internal/adapter/claude/cloud.go,
// cloudRow), with step_key only while the ids are not known.
export function rowLine(row) {
  const out = {
    source: SOURCE, ts: row.ts, session_id: row.session_id, request_id: row.request_id,
    message_id: row.message_id, model: row.model, agent: row.agent,
    input_tokens: row.input_tokens, output_tokens: row.output_tokens,
    cache_creation_input_tokens: row.cache_creation_input_tokens, cache_read_input_tokens: row.cache_read_input_tokens,
  }
  if (!row.request_id && !row.message_id) out.step_key = row.step_key
  return JSON.stringify(out)
}

const outPath = () => `${OUT_DIR}/burnmon-cloud-${st.sid}.jsonl`

async function isActive($) {
  if (st.active === undefined) st.active = (await $.env.get('CLAUDE_CODE_ENTRYPOINT')) === 'remote_cowork'
  return st.active
}

// Looks up the ids of every row that has none yet; true when any was found.
async function resolveIds($) {
  const byFile = new Map()
  for (const r of st.rows) {
    if (r.request_id || r.message_id || !r.uuid || !r.file) continue
    if (!byFile.has(r.file)) byFile.set(r.file, [])
    byFile.get(r.file).push(r)
  }
  let changed = false
  for (const [file, todo] of byFile) {
    const { size } = await $.fs.stat(file)
    const offset = Math.max(0, size - TAIL_BYTES)
    const tail = await $.fs.read(file, { offset, length: size - offset })
    for (const r of todo) {
      const ids = findIds(tail, r.uuid)
      if (ids) {
        r.request_id = ids.requestId
        r.message_id = ids.messageId
        changed = true
      }
    }
  }
  return changed
}

async function writeRows($) {
  if (st.rows.length > MAX_ROWS) st.rows.splice(0, st.rows.length - MAX_ROWS)
  await $.fs.write(outPath(), st.rows.map(rowLine).join('\n') + '\n')
}

async function commitFile($) {
  await $.mcp.call('remote-devices', 'device_commit_files', {
    files: [{ devicePath: `${DEVICE_DIR}/mod-${st.sid}.jsonl`, stagedPath: outPath() }],
  })
}

// After the file settled: late ids first (they mean one more write and one
// more wait), else the copy to the computer.
async function afterSettle($) {
  try {
    if (await resolveIds($)) {
      await writeRows($)
      settleLater($)
      return
    }
    await commitFile($)
  } catch (_) { /* fail open: the rows stay in the outputs folder */ }
}

function settleLater($) {
  if (st.timer) st.timer.cancel()
  st.timer = $.clock.after(SETTLE_MS, () => afterSettle($))
}

async function recordStep($, e, r) {
  if (st.sid === undefined) st.sid = await $.session.id()
  const sub = e.agentId !== undefined
  const read = await $.session.messages(sub ? { agentId: e.agentId, last: 2 } : { last: 2 })
  const { transcriptPath } = await $.session.info()
  const u = r.usage
  st.rows.push({
    ts: new Date(await $.clock.now()).toISOString(), session_id: st.sid, request_id: '', message_id: '',
    model: u.model || e.model, agent: sub ? 'agent-' + e.agentId : 'main',
    input_tokens: u.input_tokens || 0, output_tokens: u.output_tokens || 0,
    cache_creation_input_tokens: u.cache_creation_input_tokens || 0,
    cache_read_input_tokens: u.cache_read_input_tokens || 0,
    step_key: stepKey(e.turnId, e.index),
    uuid: newestAssistantUuid(read),
    file: transcriptPath ? (sub ? subagentTranscript(transcriptPath, e.agentId) : transcriptPath) : '',
  })
  await resolveIds($)
  await writeRows($)
}

async function finishTurn($) {
  await resolveIds($)
  await writeRows($)
  settleLater($)
}

export const register = (on) => {
  st = { active: undefined, sid: undefined, timer: undefined, rows: [] }

  on('turn.step', async function* ($, e, next) {
    const r = yield* next(e)
    try {
      if (r && r.usage && (await isActive($))) await recordStep($, e, r)
    } catch (_) { /* fail open: the turn goes on without this row */ }
    return r
  })

  on('turn.complete', async ($, e, next) => {
    const res = await next(e)
    try {
      if (e.agentId === undefined && st.sid !== undefined && st.rows.length > 0 && (await isActive($))) await finishTurn($)
    } catch (_) { /* fail open */ }
    return res
  })
}
