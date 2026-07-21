<template>
  <div>
    <!-- 第一行：运营统计 -->
    <el-row :gutter="16" style="margin-bottom:16px">
      <el-col :xs="12" :sm="6" v-for="s in opStats" :key="s.label">
        <el-card shadow="never" class="stat-card">
          <div class="stat-inner">
            <div class="stat-icon" :style="{ background: s.bg }">
              <el-icon :style="{ color: s.color, fontSize: '18px' }"><component :is="s.icon" /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-label">{{ s.label }}</div>
              <div class="stat-value">{{ s.value }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 第二行：资产统计 -->
    <el-row :gutter="16" style="margin-bottom:16px">
      <el-col :xs="8" :sm="4" v-for="a in assetStats" :key="a.label">
        <div class="asset-stat" @click="$router.push(a.route)">
          <div class="asset-stat-value">{{ fmtNum(a.value) }}</div>
          <div class="asset-stat-label">{{ a.label }}</div>
        </div>
      </el-col>
    </el-row>

    <!-- 第三行：图表 -->
    <el-row :gutter="16" style="margin-bottom:16px">
      <el-col :span="10">
        <el-card shadow="never">
          <template #header><span style="font-weight:600">漏洞等级分布</span></template>
          <div ref="vulnPieRef" style="height:220px" />
        </el-card>
      </el-col>
      <el-col :span="14">
        <el-card shadow="never">
          <template #header><span style="font-weight:600">近 7 天资产新增趋势</span></template>
          <div ref="trendLineRef" style="height:220px" />
        </el-card>
      </el-col>
    </el-row>

    <!-- 第四行：节点 + 最近任务 -->
    <el-row :gutter="16" style="margin-bottom:16px">
      <el-col :span="8">
        <el-card shadow="never" style="height:100%">
          <template #header>
            <div style="display:flex;align-items:center;justify-content:space-between">
              <span style="font-weight:600">扫描节点</span>
              <el-tag type="success" size="small">{{ onlineCount }} 在线</el-tag>
            </div>
          </template>
          <el-skeleton v-if="loading.nodes" :rows="3" animated />
          <el-empty v-else-if="nodes.length === 0" description="暂无节点" :image-size="60" />
          <div v-else>
            <div v-for="node in nodes" :key="node.id" class="node-row">
              <span class="status-dot" :class="isOnline(node) ? 'online' : 'offline'" />
              <div class="node-info">
                <span class="node-name">{{ node.name || node.id.slice(0,12) }}</span>
                <span class="node-addr">{{ node.addr }}</span>
              </div>
              <el-tag size="small" style="margin-left:auto">{{ node.active_tasks }} 任务</el-tag>
            </div>
          </div>
        </el-card>
      </el-col>

      <el-col :span="16">
        <el-card shadow="never">
          <template #header>
            <span style="font-weight:600">最近任务</span>
            <span style="font-size:11px;font-weight:400;color:var(--el-text-color-secondary);margin-left:8px">每 10s 自动刷新</span>
          </template>
          <el-table :data="recentTasks" v-loading="loading.tasks" size="small" style="width:100%">
            <el-table-column prop="name" label="任务名" show-overflow-tooltip />
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="statusType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="进度" width="160">
              <template #default="{ row }">
                <el-progress v-if="progressMap[row.id]"
                  :percentage="progressMap[row.id].percent"
                  :status="progressMap[row.id].done ? 'success' : undefined"
                  :stroke-width="6" style="width:120px" />
                <span v-else style="color:var(--el-text-color-disabled)">—</span>
              </template>
            </el-table-column>
            <el-table-column label="创建时间" width="160">
              <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
    </el-row>

    <!-- 第四行：资产变更动态 -->
    <el-card shadow="never">
      <template #header>
        <div style="display:flex;align-items:center;justify-content:space-between">
          <span style="font-weight:600">资产变更动态</span>
          <el-tag size="small" type="info">最近 {{ changes.length }} 条</el-tag>
        </div>
      </template>
      <el-skeleton v-if="loading.changes" :rows="4" animated />
      <el-empty v-else-if="changes.length === 0" description="暂无资产变更记录" :image-size="60">
        <template #description>
          <span style="color:var(--el-text-color-secondary)">执行扫描任务后，资产字段变更会在这里显示</span>
        </template>
      </el-empty>
      <div v-else class="change-list">
        <div v-for="c in changes" :key="c.id" class="change-item">
          <div class="change-left">
            <el-tag :type="changeTagType(c.asset_type)" size="small" class="change-type-tag">{{ changeTypeLabel(c.asset_type) }}</el-tag>
            <span v-if="c.asset_label" class="change-label" :title="c.asset_label">{{ c.asset_label }}</span>
          </div>
          <div class="change-center">
            <div v-for="ch in c.changes" :key="ch.field" class="change-field">
              <span class="field-name">{{ ch.field }}</span>
              <span v-if="ch.old" class="field-old">{{ ch.old }}</span>
              <el-icon v-if="ch.old" style="color:var(--el-text-color-disabled);font-size:11px;flex-shrink:0"><Right /></el-icon>
              <span class="field-new">{{ ch.new }}</span>
            </div>
          </div>
          <div class="change-time">{{ fmtTimeAgo(c.created_at) }}</div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, nextTick } from 'vue'
import * as echarts from 'echarts/core'
import { PieChart, LineChart } from 'echarts/charts'
import { TitleComponent, TooltipComponent, LegendComponent, GridComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import {
  nodeApi, taskApi, projectApi, assetApi,
  subscribeTaskProgress,
  type Node, type Task, type AssetChangeLog, type DashboardCounts, type VulnSeverityStat, type DailyTrendItem,
} from '@/api'

echarts.use([PieChart, LineChart, TitleComponent, TooltipComponent, LegendComponent, GridComponent, CanvasRenderer])

const nodes = ref<Node[]>([])
const recentTasks = ref<Task[]>([])
const changes = ref<AssetChangeLog[]>([])
const counts = ref<DashboardCounts>({ subdomains: 0, ports: 0, http: 0, vulns: 0, dirs: 0, sensitive: 0 })
const loading = ref({ nodes: false, tasks: false, changes: false })
const progressMap = reactive<Record<string, { percent: number; done: boolean }>>({})
const taskTotal = ref(0)
const projectTotal = ref(0)
const wsMap: Record<string, WebSocket> = {}
let timer: ReturnType<typeof setInterval>

// chart refs
const vulnPieRef = ref<HTMLElement | null>(null)
const trendLineRef = ref<HTMLElement | null>(null)
let vulnPieChart: echarts.ECharts | null = null
let trendLineChart: echarts.ECharts | null = null

const SEVERITY_COLORS: Record<string, string> = {
  critical: '#ff4d4f',
  high:     '#ff7a45',
  medium:   '#ffa940',
  low:      '#40a9ff',
  info:     '#bfbfbf',
}

function initVulnPie(data: VulnSeverityStat[]) {
  if (!vulnPieRef.value) return
  if (!vulnPieChart) vulnPieChart = echarts.init(vulnPieRef.value)
  vulnPieChart.setOption({
    tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
    legend: { bottom: 0, left: 'center', itemWidth: 10, itemHeight: 10, textStyle: { fontSize: 12 } },
    series: [{
      type: 'pie', radius: ['40%', '65%'], top: -20,
      label: { show: false },
      emphasis: { label: { show: true, fontSize: 13, fontWeight: 'bold' } },
      data: data.length
        ? data.map(d => ({ name: d.severity, value: d.count, itemStyle: { color: SEVERITY_COLORS[d.severity] ?? '#8884d8' } }))
        : [{ name: '暂无数据', value: 1, itemStyle: { color: '#f0f0f0' } }],
    }],
  })
}

function initTrendLine(data: DailyTrendItem[]) {
  if (!trendLineRef.value) return
  if (!trendLineChart) trendLineChart = echarts.init(trendLineRef.value)
  const dates = data.map(d => d.date.slice(5))
  trendLineChart.setOption({
    tooltip: { trigger: 'axis' },
    legend: { top: 0, right: 0, itemWidth: 10, itemHeight: 10, textStyle: { fontSize: 11 } },
    grid: { left: 40, right: 20, top: 28, bottom: 24 },
    xAxis: { type: 'category', data: dates, axisLabel: { fontSize: 11 } },
    yAxis: { type: 'value', axisLabel: { fontSize: 11 }, minInterval: 1 },
    series: [
      { name: '子域名', type: 'line', smooth: true, data: data.map(d => d.subdomain), lineStyle: { width: 2 }, symbolSize: 4 },
      { name: '端口', type: 'line', smooth: true, data: data.map(d => d.port), lineStyle: { width: 2 }, symbolSize: 4 },
      { name: 'HTTP', type: 'line', smooth: true, data: data.map(d => d.http), lineStyle: { width: 2 }, symbolSize: 4 },
      { name: '漏洞', type: 'line', smooth: true, data: data.map(d => d.vuln), lineStyle: { width: 2 }, symbolSize: 4, itemStyle: { color: '#ff4d4f' } },
    ],
  })
}

function waitForWidth(el: HTMLElement, cb: () => void) {
  if (el.clientWidth > 0) { cb(); return }
  const ro = new ResizeObserver(() => {
    if (el.clientWidth > 0) { ro.disconnect(); cb() }
  })
  ro.observe(el)
}

async function fetchCharts() {
  const [severityRes, trendRes] = await Promise.all([
    assetApi.vulnSeverityStats().catch(() => ({ data: [] as VulnSeverityStat[] })),
    assetApi.dailyTrend(7).catch(() => ({ data: [] as DailyTrendItem[] })),
  ])
  await nextTick()
  if (vulnPieRef.value) waitForWidth(vulnPieRef.value, () => initVulnPie(severityRes.data ?? []))
  if (trendLineRef.value) waitForWidth(trendLineRef.value, () => initTrendLine(trendRes.data ?? []))
}

const onlineCount = computed(() => nodes.value.filter(isOnline).length)

const opStats = computed(() => [
  { label: '扫描节点', value: `${onlineCount.value}/${nodes.value.length}`, icon: 'Share', color: 'var(--c-node-fg)', bg: 'var(--c-node-bg)' },
  { label: '任务总数', value: taskTotal.value, icon: 'List', color: 'var(--c-task-fg)', bg: 'var(--c-task-bg)' },
  { label: '项目总数', value: projectTotal.value, icon: 'Folder', color: 'var(--c-proj-fg)', bg: 'var(--c-proj-bg)' },
  { label: '漏洞数量', value: counts.value.vulns, icon: 'WarnTriangleFilled', color: 'var(--c-vuln-fg)', bg: 'var(--c-vuln-bg)' },
])

const assetStats = computed(() => [
  { label: '子域名', value: counts.value.subdomains, route: '/assets?tab=subdomain' },
  { label: '端口', value: counts.value.ports, route: '/assets?tab=ip' },
  { label: 'HTTP 资产', value: counts.value.http, route: '/assets?tab=asset' },
  { label: '漏洞', value: counts.value.vulns, route: '/assets?tab=vuln' },
  { label: '目录', value: counts.value.dirs, route: '/assets?tab=dir' },
  { label: '敏感信息', value: counts.value.sensitive, route: '/assets?tab=sensitive' },
])

function isOnline(node: Node) {
  const ts = (node as any).last_seen_at || (node as any).last_seen
  if (!ts) return true
  return Date.now() - new Date(ts).getTime() < 60_000
}
function statusType(s: string) {
  const m: Record<string, string> = { done: 'success', running: 'primary', failed: 'danger', queued: 'warning', dispatched: 'warning' }
  return m[s] ?? 'info'
}
function statusLabel(s: string) {
  const m: Record<string, string> = { done: '完成', running: '运行中', failed: '失败', queued: '排队', dispatched: '已分发', pending: '等待' }
  return m[s] ?? s
}
function changeTypeLabel(t: string) {
  const m: Record<string, string> = { subdomain: '子域名', port: '端口', http: 'HTTP' }
  return m[t] ?? t
}
function changeTagType(t: string) {
  const m: Record<string, string> = { subdomain: 'primary', port: 'success', http: 'warning' }
  return (m[t] ?? 'info') as any
}
function fmtNum(n: number) {
  if (n >= 10000) return (n / 10000).toFixed(1) + 'w'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
  return n.toString()
}
function fmtTime(iso: string) { return new Date(iso).toLocaleString('zh-CN', { hour12: false }) }
function fmtTimeAgo(iso: string) {
  if (!iso) return ''
  const diff = Date.now() - new Date(iso).getTime()
  const m = Math.floor(diff / 60000)
  const h = Math.floor(diff / 3600000)
  const d = Math.floor(diff / 86400000)
  if (d > 0) return `${d}天前`
  if (h > 0) return `${h}小时前`
  if (m > 0) return `${m}分钟前`
  return '刚刚'
}

async function fetchAll() {
  loading.value.nodes = true; loading.value.tasks = true
  try {
    const [n, t, p, c] = await Promise.all([
      nodeApi.list().catch(() => []),
      taskApi.list({ limit: 10 }).catch(() => ({ data: [], total: 0 })),
      projectApi.list({ limit: 1 }).catch(() => ({ data: [], total: 0 })),
      assetApi.dashboardCounts().catch(() => ({ subdomains: 0, ports: 0, http: 0, vulns: 0, dirs: 0, sensitive: 0 })),
    ])
    nodes.value = n
    recentTasks.value = t.data ?? []
    taskTotal.value = t.total
    projectTotal.value = p.total
    counts.value = c
    for (const task of recentTasks.value) {
      if (['running', 'dispatched'].includes(task.status) && !wsMap[task.id]) {
        const ws = subscribeTaskProgress(task.id, ev => {
          if (ev.kind === 'progress') progressMap[task.id] = { percent: ev.percent ?? 0, done: false }
          else if (ev.kind === 'status' && ['done', 'failed'].includes(ev.status ?? '')) {
            if (progressMap[task.id]) progressMap[task.id].done = true
            ws.close(); delete wsMap[task.id]; fetchAll()
          }
        })
        wsMap[task.id] = ws
      }
    }
  } finally { loading.value.nodes = false; loading.value.tasks = false }
}

async function fetchChanges() {
  loading.value.changes = true
  try {
    const res = await assetApi.recentChanges(20).catch(() => ({ data: [] }))
    changes.value = res.data ?? []
  } finally { loading.value.changes = false }
}

onMounted(() => {
  fetchAll()
  fetchChanges()
  fetchCharts()
  timer = setInterval(fetchAll, 10_000)
})
onUnmounted(() => {
  clearInterval(timer)
  for (const ws of Object.values(wsMap)) ws.close()
  vulnPieChart?.dispose()
  trendLineChart?.dispose()
})
</script>

<style scoped>
.stat-card {
  border: 1px solid var(--el-border-color-lighter);
  --c-node-fg: #1456F0; --c-node-bg: #EBF0FF;
  --c-task-fg: #7c3aed; --c-task-bg: #f5f3ff;
  --c-proj-fg: #d97706; --c-proj-bg: #fffbeb;
  --c-vuln-fg: #dc2626; --c-vuln-bg: #fef2f2;
}
:global(html.dark) .stat-card {
  --c-node-fg: #6fa3ff; --c-node-bg: rgba(20,86,240,0.15);
  --c-task-fg: #a78bfa; --c-task-bg: rgba(124,58,237,0.15);
  --c-proj-fg: #fbbf24; --c-proj-bg: rgba(217,119,6,0.15);
  --c-vuln-fg: #f87171; --c-vuln-bg: rgba(220,38,38,0.15);
}
.stat-inner { display: flex; align-items: center; gap: 14px; }
.stat-icon { display: flex; align-items: center; justify-content: center; width: 42px; height: 42px; border-radius: 10px; flex-shrink: 0; }
.stat-info { flex: 1; }
.stat-label { font-size: 12px; color: var(--el-text-color-secondary); margin-bottom: 4px; }
.stat-value { font-size: 22px; font-weight: 700; color: var(--el-text-color-primary); line-height: 1; }

/* 资产统计行 */
.asset-stat {
  text-align: center; padding: 14px 8px; border-radius: 8px; cursor: pointer;
  background: var(--el-bg-color); border: 1px solid var(--el-border-color-lighter);
  transition: border-color 0.15s, box-shadow 0.15s;
}
.asset-stat:hover { border-color: #4080ff; box-shadow: 0 2px 8px rgba(64,128,255,0.1); }
.asset-stat-value { font-size: 20px; font-weight: 700; color: var(--el-text-color-primary); line-height: 1.2; }
.asset-stat-label { font-size: 11px; color: var(--el-text-color-secondary); margin-top: 4px; }

/* 节点 */
.node-row { display: flex; align-items: center; gap: 8px; padding: 8px 0; border-bottom: 1px solid var(--el-border-color-lighter); }
.node-row:last-child { border-bottom: none; }
.node-info { display: flex; flex-direction: column; flex: 1; min-width: 0; }
.node-name { font-size: 13px; font-weight: 500; color: var(--el-text-color-primary); }
.node-addr { font-size: 11px; color: var(--el-text-color-secondary); }
.status-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
.status-dot.online { background: #2BA471; }
.status-dot.offline { background: var(--el-text-color-disabled); }

/* 变更动态 */
.change-list { display: flex; flex-direction: column; }
.change-item {
  display: flex; align-items: flex-start; gap: 12px; padding: 10px 0;
  border-bottom: 1px solid var(--el-border-color-lighter);
}
.change-item:last-child { border-bottom: none; }
.change-left { display: flex; align-items: center; gap: 6px; flex-shrink: 0; min-width: 120px; }
.change-type-tag { font-size: 11px !important; }
.change-label { max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; font-weight: 600; color: var(--el-text-color-primary); }
.change-center { flex: 1; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.change-field { display: flex; align-items: center; gap: 4px; font-size: 12px; flex-wrap: wrap; }
.field-name { font-weight: 600; color: var(--el-text-color-primary); flex-shrink: 0; }
.field-name::after { content: ':'; }
.field-old { color: var(--el-text-color-secondary); text-decoration: line-through; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.field-new { color: #2BA471; font-weight: 500; max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.change-time { font-size: 11px; color: var(--el-text-color-disabled); flex-shrink: 0; white-space: nowrap; }
</style>
