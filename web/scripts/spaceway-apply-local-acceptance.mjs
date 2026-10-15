import { request } from '@playwright/test'
import { readFileSync } from 'node:fs'

const args = process.argv.slice(2)
const commit = args.includes('--commit')
const payloadPath = valueAfter('--payload') ?? '/tmp/ai-cool-acceptance/spaceway-local-acceptance-payload.json'
const cookieFile = valueAfter('--cookie-file')
const cookieHeader = process.env.SPACEWAY_COOKIE_HEADER ?? (cookieFile ? readFileSync(cookieFile, 'utf8').trim() : '')

if (!cookieHeader) {
  console.error('Missing Spaceway login cookie. Pass --cookie-file or SPACEWAY_COOKIE_HEADER.')
  process.exit(1)
}

const payload = JSON.parse(readFileSync(payloadPath, 'utf8'))
const api = await request.newContext({
  baseURL: 'https://eng.xiaojukeji.com',
  ignoreHTTPSErrors: true,
  extraHTTPHeaders: {
    cookie: cookieHeader,
    origin: 'https://spaceway.intra.xiaojukeji.com',
    referer: 'https://spaceway.intra.xiaojukeji.com/task/woody-case/33910/qa-case/require-case/case-detail?testPlanId=1359221&topTab=executionSet',
    'content-type': 'application/json',
    'user-agent': 'Mozilla/5.0',
  },
})

try {
  const before = await passRate()
  const writes = []

  for (const item of payload.payloads) {
    if (!commit) {
      writes.push({ caseName: item.caseName, skipped: true, result: item.executionPayload.executionResult })
      continue
    }

    const execution = await postJson('/new-case-dream/dream/api/v1/test-plan/stage/execution/save', {
      splitTags: ['default_split_tag'],
      ...item.executionPayload,
    })
    const remark = await postJson('/new-case-dream/dream/api/v1/test-plan/stage/execution/remark/save', item.remarkPayload)
    writes.push({
      caseName: item.caseName,
      result: item.executionPayload.executionResult,
      execution,
      remark,
    })
  }

  const after = await passRate()
  const cases = await fetchTargetCases(payload.payloads.map((item) => item.executionPayload.reqCaseId))

  console.log(JSON.stringify({
    committed: commit,
    testPlanId: payload.testPlanId,
    stageId: payload.stageId,
    before,
    after,
    writes,
    verifiedCases: cases.map((item) => ({
      reqCaseId: item.id,
      caseName: item.case_name,
      executionResult: item.execution_result,
      executor: item.executor,
      executionTime: item.execution_time,
    })),
  }, null, 2))
} finally {
  await api.dispose()
}

async function postJson(path, body) {
  const response = await api.post(path, { data: body })
  const text = await response.text()
  let json
  try {
    json = JSON.parse(text)
  } catch {
    throw new Error(`${path} returned non-JSON ${response.status()}: ${text.slice(0, 200)}`)
  }
  if (!response.ok() || !isSuccess(json)) {
    throw new Error(`${path} failed: HTTP ${response.status()} ${JSON.stringify(json).slice(0, 500)}`)
  }
  return summarizeResponse(json)
}

async function passRate() {
  const json = await postJson('/new-case-dream/dream/api/v2/case/pass-rate', {
    testPlanId: payload.testPlanId,
    stageId: payload.stageId,
  })
  return json.data ?? json.Data ?? {}
}

async function fetchTargetCases(reqCaseIds) {
  const target = new Set(reqCaseIds.map(String))
  const found = []
  for (let pageNum = 1; pageNum <= 10 && found.length < target.size; pageNum++) {
    const json = await postJson('/new-case-dream/dream/api/req/case/select', {
      requirementId: payload.testPlanId,
      pageNum,
      pageSize: 100,
      stageId: payload.stageId,
      itemUid: '6777b71e-5eed-4c0a-8886-8da63d477db9',
    })
    const data = json.data ?? json.Data ?? []
    found.push(...data.filter((item) => target.has(String(item.id))))
    const total = json.Total ?? json.total ?? 0
    if (pageNum * 100 >= total) break
  }
  return found
}

function isSuccess(json) {
  return json.status === 0 || json.Status === 0 || json.code === 0 || json.Code === 0 || json.errno === 0
}

function summarizeResponse(json) {
  if (Array.isArray(json.data) || Array.isArray(json.Data)) return json
  return {
    status: json.status ?? json.Status ?? json.code ?? json.Code,
    msg: json.msg ?? json.Msg ?? json.message ?? json.Message,
    data: json.data ?? json.Data,
    Total: json.Total,
    Data: json.Data,
  }
}

function valueAfter(flag) {
  const index = args.indexOf(flag)
  if (index === -1) return undefined
  return args[index + 1]
}
