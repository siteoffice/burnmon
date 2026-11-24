import { expect, mock, test } from 'claude-code/testing'

// What the engine beneath answers: one session, its transcript holding the
// rows of two requests (main and a subagent), each found by its uuid.
const MAIN = '/home/u/.claude/projects/p/sess-1.jsonl'
const SUB = '/home/u/.claude/projects/p/sess-1/subagents/agent-a1.jsonl'
const usage = (out: number) => ({
  input_tokens: 2, output_tokens: out, cache_creation_input_tokens: 10, cache_read_input_tokens: 100,
  model: 'claude-opus-5-5',
})
const line = (uuid: string, req: string, msg: string) =>
  JSON.stringify({ type: 'assistant', uuid, requestId: req, message: { id: msg, role: 'assistant', content: [{ type: 'text', text: 'never copied' }] } })

// The engine hands a path to the hooks beneath in the host's form (on Windows
// "C:\mnt\..."); the cloud runs on Linux, so the fake world compares in that form.
const norm = (p: string) => p.replace(/\\/g, '/').replace(/^[A-Za-z]:/, '')

function world(on: any, opts: { entry?: string; files?: Record<string, string> }) {
  const writes: { path: string; text: string }[] = []
  const calls: any[] = []
  const files = opts.files ?? {}
  mock.env(on, opts.entry ? { CLAUDE_CODE_ENTRYPOINT: opts.entry } : {})
  const clock = mock.clock(on, { now: Date.UTC(2026, 10, 24, 10, 0, 0) })
  on('session.id', () => ({ value: 'sess-1' }))
  on('session.info', () => ({ value: { transcriptPath: MAIN } }))
  on('session.messages', (_: any, e: any) => ({
    value: e.agentId === 'a1'
      ? [{ role: 'assistant', text: '', toolUses: [], uuid: 'u-sub' }]
      : [{ role: 'user', text: '', toolUses: [], uuid: 'u-0' }, { role: 'assistant', text: '', toolUses: [], uuid: 'u-main' }],
  }))
  on('fs.stat', (_: any, e: any) => ({ value: { size: (files[norm(e.path)] ?? '').length, mtimeMs: 0, kind: 'file' } }))
  on('fs.read', (_: any, e: any) => ({ value: (files[norm(e.path)] ?? '').slice(e.offset ?? 0) }))
  on('fs.write', (_: any, e: any) => { writes.push({ path: norm(e.path), text: e.text }); return { value: undefined } })
  on('mcp.call', (_: any, e: any) => { calls.push(e); return { value: { content: [], isError: false } } })
  on('turn.step', async function* (_: any, e: any) {
    return { turnId: e.turnId, index: e.index, answer: '', toolUses: [], stopReason: 'end_turn', usage: usage(e.agentId ? 7 : 120) }
  })
  on('turn.complete', (_: any, e: any) => ({ text: e.answer }))
  return { writes, calls, clock, files }
}

async function step($: any, input: any) {
  const s = $.turn.step(input)
  while (true) {
    const it = await s.next()
    if (it.done) return it.value
  }
}

const rowsOf = (text: string) => text.trim().split('\n').map((l) => JSON.parse(l))

test('outside a Cowork (Cloud) session the mod writes and calls nothing', async ($, on) => {
  const w = world(on, { entry: 'cli', files: { [MAIN]: line('u-main', 'req_1', 'msg_1') } })
  const r = await step($, { turnId: 't1', index: 0, model: 'claude-opus-5-5', messageCount: 3 })
  expect(r.usage.output_tokens).toBe(120)
  await $.turn.complete({ answer: 'x', durationMs: 1, isAborted: false, turnId: 't1', reason: 'answer' })
  await w.clock.advance(60000)
  expect(w.writes).toEqual([])
  expect(w.calls).toEqual([])
})

test('in a cloud session each request becomes one row with ids from the transcript, numbers only', async ($, on) => {
  const w = world(on, {
    entry: 'remote_cowork',
    files: { [MAIN]: line('u-main', 'req_1', 'msg_1') + '\n', [SUB]: line('u-sub', 'req_2', 'msg_2') + '\n' },
  })
  await step($, { turnId: 't1', index: 0, model: 'claude-opus-5-5', messageCount: 3 })
  await step($, { turnId: 't2', index: 0, model: 'claude-opus-5-5', messageCount: 1, agentId: 'a1' })
  const last = w.writes.at(-1)!
  expect(last.path).toBe('/mnt/user-data/outputs/burnmon-cloud-sess-1.jsonl')
  expect(last.text.includes('never copied')).toBe(false)
  const rows = rowsOf(last.text)
  expect(rows.length).toBe(2)
  expect(rows[0]).toEqual({
    source: 'burnmon-cloud-mod', ts: '2026-11-24T10:00:00.000Z', session_id: 'sess-1', request_id: 'req_1',
    message_id: 'msg_1', model: 'claude-opus-5-5', agent: 'main', input_tokens: 2, output_tokens: 120,
    cache_creation_input_tokens: 10, cache_read_input_tokens: 100,
  })
  expect(rows[1].agent).toBe('agent-a1')
  expect(rows[1].request_id).toBe('req_2')
})

test('a row the transcript does not hold yet carries a step key, and gets its ids on a later read', async ($, on) => {
  const w = world(on, { entry: 'remote_cowork', files: { [MAIN]: '' } })
  await step($, { turnId: 't9', index: 3, model: 'claude-opus-5-5', messageCount: 3 })
  let rows = rowsOf(w.writes.at(-1)!.text)
  expect(rows[0].request_id).toBe('')
  expect(rows[0].step_key).toBe('turn-t9-3')
  // The main transcript is written late: the row lands before the settle timer.
  w.files[MAIN] = line('u-main', 'req_9', 'msg_9') + '\n'
  await $.turn.complete({ answer: 'x', durationMs: 1, isAborted: false, turnId: 't9', reason: 'answer' })
  await w.clock.advance(60000)
  rows = rowsOf(w.writes.at(-1)!.text)
  expect(rows[0].request_id).toBe('req_9')
  expect(rows[0].step_key).toBeUndefined()
})

test('the end of a main turn copies the file to the computer after it settles', async ($, on) => {
  const w = world(on, { entry: 'remote_cowork', files: { [MAIN]: line('u-main', 'req_1', 'msg_1') + '\n' } })
  await step($, { turnId: 't1', index: 0, model: 'claude-opus-5-5', messageCount: 3 })
  await $.turn.complete({ answer: 'x', durationMs: 1, isAborted: false, turnId: 't1', reason: 'answer' })
  expect(w.calls).toEqual([])
  await w.clock.advance(60000)
  expect(w.calls.length).toBe(1)
  expect(w.calls[0]).toEqual({
    server: 'remote-devices', tool: 'device_commit_files',
    args: { files: [{ devicePath: '~/.burnmon/cloud/mod-sess-1.jsonl', stagedPath: '/mnt/user-data/outputs/burnmon-cloud-sess-1.jsonl' }] },
  })
})

test('a failing copy or write never stops the turn', async ($, on) => {
  mock.env(on, { CLAUDE_CODE_ENTRYPOINT: 'remote_cowork' })
  const clock = mock.clock(on, { now: 0 })
  on('session.id', () => { throw new Error('down') })
  on('turn.step', async function* (_: any, e: any) {
    return { turnId: e.turnId, index: e.index, answer: 'a', toolUses: [], stopReason: 'end_turn', usage: usage(1) }
  })
  on('turn.complete', (_: any, e: any) => ({ text: e.answer }))
  const r = await step($, { turnId: 't1', index: 0, model: 'claude-opus-5-5', messageCount: 1 })
  expect(r.answer).toBe('a')
  const c = await $.turn.complete({ answer: 'done', durationMs: 1, isAborted: false, turnId: 't1', reason: 'answer' })
  expect(c.text).toBe('done')
  await clock.advance(60000)
})
