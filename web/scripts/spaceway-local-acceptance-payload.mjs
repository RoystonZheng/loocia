import { readFileSync } from 'node:fs'

const inputPath = process.argv[2]

if (!inputPath) {
  console.error('Usage: node scripts/spaceway-local-acceptance-payload.mjs <playwright-json-report>')
  process.exit(1)
}

const report = JSON.parse(readFileSync(inputPath, 'utf8'))
const testResults = new Map()

for (const entry of collectSpecs(report)) {
  const acceptId = entry.title.match(/ACCEPT-\d{3}/)?.[0]
  if (!acceptId) continue
  testResults.set(acceptId, {
    title: entry.title,
    status: normalizeStatus(entry),
  })
}

const mappings = [
  {
    acceptId: 'ACCEPT-001',
    reqCaseId: 6263451,
    caseUid: 'd3e9119b-efd7-43ca-99f0-3d8a37b3b086',
    caseName: 'SUP-CROSS-013 5178 到 8991 proxy',
  },
  {
    acceptId: 'ACCEPT-004',
    reqCaseId: 6263565,
    caseUid: 'e6b2a238-6350-4930-8096-b862c6cb4ff1',
    caseName: 'SUP-TOOL-001 工具池默认加载',
  },
  {
    acceptId: 'ACCEPT-004',
    reqCaseId: 6263495,
    caseUid: 'facab3fc-748f-429c-a8ed-babcd1833ba2',
    caseName: 'SUP-TOOL-002 purposeTag 筛选',
  },
  {
    acceptId: 'ACCEPT-005',
    reqCaseId: 6263997,
    caseUid: '7726f2a1-8932-4aeb-99e8-132845e535c6',
    caseName: 'SUP-CFG-005 actor 必填与已有配置回退',
  },
  {
    acceptId: 'ACCEPT-005',
    reqCaseId: 6263403,
    caseUid: '5a0a89f1-ce15-4e33-b98b-1d68ed06d68c',
    caseName: 'SUP-TOOL-008 repoUrl 必填为空',
  },
  {
    acceptId: 'ACCEPT-005',
    reqCaseId: 6263867,
    caseUid: '96bee681-696d-4c46-ac68-b390126ffb35',
    caseName: 'SUP-TOOL-011 URL 非法',
  },
  {
    acceptId: 'ACCEPT-006',
    reqCaseId: 6263861,
    caseUid: 'f6cddbd6-341f-4180-aadd-301546d524d4',
    caseName: 'TC-COMBO-025 本地触发器组合锁定副本环境',
  },
]

const stageId = 690715
const generatedAt = new Date().toISOString()

const payloads = mappings.map((mapping) => {
  const result = testResults.get(mapping.acceptId)
  const executionResult = result?.status === 'passed' ? 'PASS' : result?.status === 'skipped' ? 'UN_EXECUTED' : 'FAIL'
  return {
    acceptId: mapping.acceptId,
    caseName: mapping.caseName,
    executionPayload: {
      caseUid: mapping.caseUid,
      reqCaseId: mapping.reqCaseId,
      stageId,
      executionResult,
      splitTags: ['default_split_tag'],
    },
    remarkPayload: {
      caseUid: mapping.caseUid,
      reqCaseId: mapping.reqCaseId,
      stageId,
      stageNote: [
        `AI Cool local acceptance ${mapping.acceptId}: ${executionResult}`,
        `Generated at: ${generatedAt}`,
        `Source test: ${result?.title ?? 'not found in report'}`,
      ].join('\\n'),
    },
  }
})

console.log(JSON.stringify({
  dryRun: true,
  testPlanId: 1359221,
  stageId,
  generatedAt,
  endpoints: {
    saveExecution: 'https://eng.xiaojukeji.com/new-case-dream/dream/api/v1/test-plan/stage/execution/save',
    saveRemark: 'https://eng.xiaojukeji.com/new-case-dream/dream/api/v1/test-plan/stage/execution/remark/save',
  },
  payloads,
  unmappedAcceptance: [...testResults.keys()].filter((acceptId) => !mappings.some((mapping) => mapping.acceptId === acceptId)),
}, null, 2))

function* collectSpecs(node) {
  if (!node || typeof node !== 'object') return
  if (Array.isArray(node.specs)) {
    for (const spec of node.specs) {
      yield spec
      yield* collectSpecs(spec)
    }
  }
  for (const key of ['suites']) {
    if (!Array.isArray(node[key])) continue
    for (const child of node[key]) yield* collectSpecs(child)
  }
}

function normalizeStatus(specEntry) {
  if (specEntry.ok === true) return 'passed'
  if (specEntry.ok === false) return 'failed'
  const explicit = specEntry.status
  if (explicit) return explicit
  const tests = Array.isArray(specEntry.tests) ? specEntry.tests : []
  const results = tests.flatMap((test) => Array.isArray(test.results) ? test.results : [])
  if (results.some((result) => result.status === 'failed' || result.status === 'timedOut' || result.status === 'interrupted')) {
    return 'failed'
  }
  if (results.length > 0 && results.every((result) => result.status === 'skipped')) return 'skipped'
  if (results.some((result) => result.status === 'passed')) return 'passed'
  return 'unknown'
}
