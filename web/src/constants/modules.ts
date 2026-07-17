// 扫描模块的单一真相源。后端 plugin_seed.go / api/task.go / cronjob/runner.go
// 里的 moduleOrder 与此保持一致；改动时同步。
//
// - MODULE_ORDER: 扫描流水线中模块的执行顺序（前端渲染/后端 stages 派生都用它）
// - MODULE_LABELS: 中文标签
// - MODULE_ICONS: emoji 图标
// - STAGE_LABELS: pipeline 内部 stage 别名（nuclei→漏洞扫描 之类）
//
// 只有添加新扫描模块 / 改中文名 / 重排序时才需要动本文件。

export const MODULE_ORDER = [
  'search',
  'subdomain',
  'port',
  'http',
  'crawler',
  'vuln',
  'brute',
  'dir',
  'sensitive',
] as const

export type ModuleKey = typeof MODULE_ORDER[number]

export const MODULE_LABELS: Record<string, string> = {
  search: '在线搜索',
  subdomain: '子域名枚举',
  port: '端口扫描',
  http: 'HTTP 探测',
  crawler: '爬虫',
  vuln: '漏洞扫描',
  brute: '弱口令爆破',
  dir: '目录扫描',
  sensitive: '敏感信息',
}

export const MODULE_ICONS: Record<string, string> = {
  search: '🔎',
  subdomain: '🔍',
  port: '🔌',
  http: '🌐',
  crawler: '🕷️',
  vuln: '⚡',
  brute: '🔑',
  dir: '📁',
  sensitive: '🕵️',
}

// stage → 展示标签。stage 名和 module 名基本相同，但有个别别名（nuclei == vuln）。
export const STAGE_LABELS: Record<string, string> = {
  ...MODULE_LABELS,
  nuclei: '漏洞扫描',
  shuffledns: '子域名验证',
  bbot: '子域名枚举(bbot)',
  findomain: '子域名枚举(findomain)',
}

export function moduleLabel(mod: string): string {
  return MODULE_LABELS[mod] ?? mod
}

export function moduleIcon(mod: string): string {
  return MODULE_ICONS[mod] ?? '📦'
}

export function stageLabel(stage: string): string {
  return STAGE_LABELS[stage] ?? stage
}

// stage → module 映射。用于把 bbot / findomain / shuffledns 这类插件级 stage
// 归拢到所属 module 下（前端分组渲染任务详情）。
export const STAGE_TO_MODULE: Record<string, string> = {
  search: 'search',
  subdomain: 'subdomain',
  bbot: 'subdomain',
  findomain: 'subdomain',
  shuffledns: 'subdomain',
  port: 'port',
  http: 'http',
  crawler: 'crawler',
  vuln: 'vuln',
  nuclei: 'vuln',
  brute: 'brute',
  dir: 'dir',
  sensitive: 'sensitive',
}

export function stageModule(stage: string): string {
  return STAGE_TO_MODULE[stage] ?? stage
}
