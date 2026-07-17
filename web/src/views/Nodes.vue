<template>
  <div>
    <div class="page-header">
      <h2 class="page-title">扫描节点</h2>
      <div class="header-actions">
        <el-switch v-model="autoRefresh" active-text="自动刷新" inactive-text="手动" />
        <el-button :loading="loading" @click="fetchNodes">
          <el-icon><Refresh /></el-icon>刷新状态
        </el-button>
        <el-button type="primary" @click="showInstall = true">
          <el-icon><Download /></el-icon>部署节点
        </el-button>
      </div>
    </div>

    <el-table :data="nodes" v-loading="loading" style="width:100%">
      <el-table-column label="节点名称" width="160">
        <template #default="{ row }">
          <span class="editable-name" @click="openRename(row)">
            {{ row.name || row.id.slice(0,12) }}
            <el-icon class="edit-icon"><Edit /></el-icon>
          </span>
        </template>
      </el-table-column>
      <el-table-column prop="addr" label="地址" width="160" />
      <el-table-column label="CPU" width="140">
        <template #default="{ row }">
          <el-progress :percentage="row.cpu_percent ?? 0" :stroke-width="6"
            :color="loadColor(row.cpu_percent ?? 0)" style="width:110px" />
        </template>
      </el-table-column>
      <el-table-column label="内存" width="140">
        <template #default="{ row }">
          <el-progress :percentage="row.mem_percent ?? 0" :stroke-width="6"
            :color="loadColor(row.mem_percent ?? 0)" style="width:110px" />
        </template>
      </el-table-column>
      <el-table-column label="运行任务" width="90">
        <template #default="{ row }">
          <el-tag v-if="row.active_tasks > 0" size="small">{{ row.active_tasks }}</el-tag>
          <span v-else style="color:var(--el-text-color-disabled)">0</span>
        </template>
      </el-table-column>
      <el-table-column label="并发上限" width="90">
        <template #default="{ row }">
          <span class="editable-name" @click="openConcurrency(row)">
            {{ row.max_tasks ?? 5 }}<el-icon class="edit-icon"><Edit /></el-icon>
          </span>
        </template>
      </el-table-column>
      <el-table-column label="能力" min-width="180">
        <template #default="{ row }">
          <el-tag v-for="c in row.capabilities" :key="c" size="small" style="margin-right:4px;margin-bottom:2px">{{ c }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <span class="status-dot" :class="isOnline(row) ? 'online' : 'offline'" />
          <span style="margin-left:5px;font-size:13px">{{ isOnline(row) ? '在线' : '离线' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="最后心跳" width="170">
        <template #default="{ row }">{{ fmtTime(row.last_seen_at || row.last_seen) }}</template>
      </el-table-column>
      <el-table-column label="已安装工具" min-width="200">
        <template #default="{ row }">
          <template v-if="row.installed_tools?.length">
            <el-tag v-for="t in row.installed_tools" :key="t" size="small" type="success" style="margin-right:4px;margin-bottom:2px">{{ t }}</el-tag>
            <el-tag v-for="t in missingTools(row)" :key="t" size="small" type="info" effect="plain" style="margin-right:4px;margin-bottom:2px;opacity:0.5">{{ t }}</el-tag>
          </template>
          <span v-else style="color:var(--el-text-color-disabled);font-size:12px">未上报</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="290" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="openNodeLog(row)">
            <el-icon><Tickets /></el-icon>日志
          </el-button>
          <el-divider direction="vertical" />
          <el-button type="success" link size="small" @click="openToolInstall(row)">
            <el-icon><Setting /></el-icon>管理工具
          </el-button>
          <el-divider direction="vertical" />
          <el-popconfirm title="确认重启该节点？" teleported @confirm="restartNode(row)">
            <template #reference>
              <el-button type="warning" link size="small" :disabled="!isOnline(row)">重启</el-button>
            </template>
          </el-popconfirm>
          <el-divider direction="vertical" />
          <el-button type="danger" link size="small" @click="confirmDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!loading && nodes.length === 0" description="暂无节点，点击「部署节点」查看部署指引" style="padding:60px 0" />

    <!-- 重命名 -->
    <el-dialog v-model="showRename" title="修改节点名称" width="420px">
      <el-form label-position="top">
        <el-form-item label="原名称"><el-input :value="renameForm.oldName" disabled /></el-form-item>
        <el-form-item label="新名称"><el-input v-model="renameForm.newName" placeholder="输入新的节点名称" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showRename=false">取消</el-button>
        <el-button type="primary" @click="submitRename">确认</el-button>
      </template>
    </el-dialog>

    <!-- 并发数 -->
    <el-dialog v-model="showConcurrency" title="修改并发数" width="420px">
      <el-form label-position="top">
        <el-form-item label="节点"><el-input :value="concurrencyForm.name" disabled /></el-form-item>
        <el-form-item label="并发数">
          <el-input-number v-model="concurrencyForm.value" :min="1" :max="50" style="width:100%" controls-position="right" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showConcurrency=false">取消</el-button>
        <el-button type="primary" @click="submitConcurrency">确认</el-button>
      </template>
    </el-dialog>

    <!-- 部署节点弹窗 -->
    <el-dialog v-model="showInstall" title="部署扫描节点" width="680px" @open="loadToken">
      <el-alert type="info" show-icon :closable="false" style="margin-bottom:16px"
        description="首次部署已自带一个扫描节点。如需扩展，可在其他机器上添加远程节点。" />

      <el-descriptions :column="1" border size="small" style="margin-bottom:16px">
        <el-descriptions-item label="主控 gRPC 地址"><code>{{ serverAddr }}</code></el-descriptions-item>
        <el-descriptions-item label="认证 Key">
          <div style="display:flex;align-items:center;gap:8px">
            <code style="flex:1;word-break:break-all">{{ nodeToken || '加载中...' }}</code>
            <el-button size="small" @click="copy(nodeToken)" :disabled="!nodeToken">复制</el-button>
            <el-popconfirm title="重新生成 Key 后，已部署的节点需要更新 Token 才能重连，确认？" width="300" @confirm="regenerateToken">
              <template #reference>
                <el-button size="small" type="warning" :loading="tokenLoading">
                  <el-icon><RefreshRight /></el-icon>重新生成
                </el-button>
              </template>
            </el-popconfirm>
          </div>
        </el-descriptions-item>
      </el-descriptions>

      <div class="cmd-title">添加远程扫描节点（在目标机器上执行）</div>
      <div class="cmd-box"><code>{{ installCmd }}</code>
        <el-button size="small" @click="copy(installCmd)">复制</el-button></div>

      <el-collapse style="margin-top:12px">
        <el-collapse-item title="环境变量说明" name="env">
          <el-table :data="envParams" :show-header="true" size="small">
            <el-table-column prop="name" label="变量名" width="180" />
            <el-table-column prop="desc" label="说明" />
            <el-table-column prop="required" label="必填" width="80" />
          </el-table>
        </el-collapse-item>
      </el-collapse>

      <template #footer>
        <el-button @click="showInstall=false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 管理节点工具 -->
    <el-dialog v-model="showToolInstall" :title="`管理工具 — ${toolInstallNode?.name || toolInstallNode?.id?.slice(0,12)}`" width="720px" @close="closeToolInstall">
      <div style="max-height:65vh;overflow-y:auto;padding-right:4px">
          <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px"
          description="可在这里安装或重装节点工具。Claude Code 安装后可用于 AI 自动化渗透。" />
        <!-- 筛选栏 -->
        <div style="display:flex;gap:8px;align-items:center;margin-bottom:12px">
          <el-input v-model="toolSearch" placeholder="搜索插件名称" clearable size="small" style="width:200px">
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
        </div>

        <!-- 工具列表 -->
        <div class="tool-list">
          <div v-for="tool in filteredToolDefs" :key="tool.name" class="tool-item">
            <div class="tool-header">
              <el-tag :type="toolStatusTag(tool.name).type" size="small" effect="plain" style="min-width:70px;text-align:center">
                {{ toolStatusTag(tool.name).label }}
              </el-tag>
              <span class="tool-name">{{ tool.name }}</span>
              <span class="tool-desc">{{ tool.desc }}</span>
              <div style="margin-left:auto;display:flex;gap:6px">
                <el-button v-if="toolInstallStatus[tool.name] === 'installing'" size="small" loading disabled>
                  处理中…
                </el-button>
                <template v-else-if="isToolInstalled(tool.name)">
                  <el-button size="small" @click="installSingleTool(tool)">
                    重新安装
                  </el-button>
                </template>
                <el-button v-else size="small" type="primary" @click="installSingleTool(tool)">
                  安装
                </el-button>
              </div>
            </div>

            <!-- 安装日志：只在本次 session 触发过操作时显示 -->
            <div v-if="toolInstallLogs[tool.name]?.length && toolInstallStatus[tool.name] !== 'idle'" class="tool-log-area">
              <div class="tool-log-box" :ref="(el: any) => setLogRef(tool.name, el)">
                <div v-for="(line, i) in toolInstallLogs[tool.name]" :key="i"
                  :class="['tool-log-line', line.level === 'error' ? 'log-error' : '']">
                  {{ line.msg }}
                </div>
              </div>
            </div>

            <!-- 安装失败 → 手动安装提示 -->
            <div v-if="toolInstallStatus[tool.name] === 'failed'" class="tool-fail-hint">
              <el-alert type="error" :closable="false" style="margin-bottom:8px">
                <template #title>安装失败，请在节点机器上手动执行以下命令：</template>
              </el-alert>
              <div v-for="(cmd, i) in tool.cmds" :key="i" class="cmd-box">
                <code>{{ cmd }}</code>
                <el-button size="small" link @click="copy(cmd)">复制</el-button>
              </div>
            </div>
          </div>
        </div>

        <el-empty v-if="filteredToolDefs.length === 0" description="没有匹配的工具" :image-size="60" />
      </div>

      <template #footer>
        <el-button @click="showToolInstall = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 节点日志抽屉 -->
    <el-drawer v-model="logDrawerVisible" :title="`节点日志 — ${logNode?.name || logNode?.id?.slice(0,16)}`"
      size="680px" @close="closeNodeLog">
      <div style="display:flex;flex-direction:column;height:100%;gap:10px">

        <!-- 过滤栏 -->
        <div style="display:flex;gap:8px;align-items:center;flex-shrink:0">
          <el-select v-model="logFilter.level" clearable placeholder="日志级别" style="width:120px" size="small">
            <el-option label="INFO"  value="info" />
            <el-option label="WARN"  value="warn" />
            <el-option label="ERROR" value="error" />
            <el-option label="DEBUG" value="debug" />
          </el-select>
          <el-input v-model="logFilter.keyword" placeholder="关键字搜索" clearable size="small" style="width:180px">
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-checkbox v-model="logAutoScroll" size="small">自动滚动</el-checkbox>
          <div style="margin-left:auto;display:flex;gap:6px">
            <el-button size="small" @click="nodeLogs=[]">清空</el-button>
            <el-tag size="small" :type="wsConnected ? 'success' : 'info'" effect="plain">
              {{ wsConnected ? '已连接' : '已断开' }}
            </el-tag>
          </div>
        </div>

        <!-- 日志终端 -->
        <div ref="logBox" class="log-terminal">
          <div v-if="filteredLogs.length === 0" class="log-empty">
            {{ nodeLogs.length === 0 ? '等待日志输出…' : '无匹配日志' }}
          </div>
          <div v-for="(line, i) in filteredLogs" :key="i" :class="['log-line', `log-${line.level||'info'}`]">
            <span class="log-time">{{ fmtLogTime(line.time) }}</span>
            <span class="log-badge" :class="`badge-${line.level||'info'}`">{{ (line.level||'INFO').toUpperCase() }}</span>
            <span class="log-msg">{{ line.log }}</span>
          </div>
        </div>

        <div style="font-size:12px;color:var(--el-text-color-secondary);text-align:right;flex-shrink:0">共 {{ nodeLogs.length }} 条（显示 {{ filteredLogs.length }} 条）</div>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, nextTick, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { nodeApi, toolDefApi, type Node, type ToolDef } from '@/api'

const nodes = ref<Node[]>([])
const loading = ref(false)
const autoRefresh = ref(true)
const showInstall = ref(false)
const showRename = ref(false)
const showConcurrency = ref(false)
const renameForm = reactive({ oldName: '', newName: '', nodeId: '' })
const concurrencyForm = reactive({ name: '', nodeId: '', value: 5 })
let timer: ReturnType<typeof setInterval>

const serverAddr = location.hostname + ':9000'
const apiBase = `${location.protocol}//${location.host}`
const wsProto = location.protocol === 'https:' ? 'wss' : 'ws'
const wsBase = `${wsProto}://${location.host}`

const nodeToken = ref('')
const tokenLoading = ref(false)

function shellQuote(value: string) {
  return `'${value.split("'").join(`'"'"'`)}'`
}

const installCmd = computed(() =>
  nodeToken.value
    ? `curl -fsSL -H ${shellQuote(`X-Nscan-Token: ${nodeToken.value}`)} ${shellQuote(`${apiBase}/api/v1/nodes/install.sh`)} | sh`
    : '正在加载部署命令...'
)

async function loadToken() {
  try { nodeToken.value = await nodeApi.token() } catch {}
}
async function regenerateToken() {
  tokenLoading.value = true
  try {
    nodeToken.value = await nodeApi.regenerateToken()
    ElMessage.success('Key 已重新生成')
  } catch { ElMessage.error('生成失败') }
  finally { tokenLoading.value = false }
}

const envParams = [
  { name: 'SERVER_ADDR', desc: '主控 gRPC 地址（自动填充）', required: '自动' },
  { name: 'TOKEN', desc: '认证 Key（自动填充）', required: '自动' },
  { name: 'NODE_NAME', desc: '节点名称，留空自动生成', required: '否' },
  { name: 'MAX_TASKS', desc: '最大并发任务数，默认 5', required: '否' },
]

// ── 安装工具 ─────────────────────────────────────────────────────────────────
const showToolInstall = ref(false)
const toolInstallNode = ref<Node | null>(null)
const toolSearch = ref('')
const toolFilter = ref<'all' | 'installed' | 'missing'>('all')
const toolInstallStatus = reactive<Record<string, 'idle' | 'installing' | 'success' | 'failed'>>({})
const toolInstallLogs = reactive<Record<string, { msg: string; level: string }[]>>({})
const logRefs: Record<string, HTMLElement | null> = {}
// 挂在 window 上，HMR 热更新重新执行模块时仍能找到并关闭旧连接
declare global { interface Window { __toolInstallWs?: WebSocket | null } }
function getToolWs() { return window.__toolInstallWs ?? null }
function setToolWs(ws: WebSocket | null) { window.__toolInstallWs = ws }

const toolDefs = ref<{ name: string; desc: string; cmds: string[] }[]>([])
const allToolNames = computed(() => toolDefs.value.map(t => t.name))

async function fetchToolDefs() {
  try {
    const defs = await toolDefApi.list()
    toolDefs.value = defs.map(d => ({ name: d.name, desc: d.description, cmds: d.install_cmds }))
  } catch {}
}

function missingTools(node: Node): string[] {
  const installed = new Set(node.installed_tools ?? [])
  return allToolNames.value.filter(t => !installed.has(t))
}

function isToolInstalled(name: string): boolean {
  if (toolInstallStatus[name] === 'success') return true
  // 优先用 nodes.value 里的最新数据（心跳会更新它），避免快照过期
  const latest = nodes.value.find(n => n.id === toolInstallNode.value?.id)
  return (latest ?? toolInstallNode.value)?.installed_tools?.includes(name) ?? false
}

function toolStatusTag(name: string): { type: string; label: string } {
  const st = toolInstallStatus[name]
  if (st === 'installing') return { type: 'warning', label: '安装中' }
  if (st === 'success' || isToolInstalled(name)) return { type: 'success', label: '已安装' }
  if (st === 'failed') return { type: 'danger', label: '安装失败' }
  return { type: 'info', label: '未安装' }
}

const filteredToolDefs = computed(() => {
  return toolDefs.value.filter(t => {
    if (toolSearch.value && !t.name.includes(toolSearch.value.toLowerCase())) return false
    if (toolFilter.value === 'installed' && !isToolInstalled(t.name)) return false
    if (toolFilter.value === 'missing' && isToolInstalled(t.name)) return false
    return true
  })
})

function setLogRef(name: string, el: HTMLElement | null) {
  logRefs[name] = el
}

function scrollLogToBottom(name: string) {
  nextTick(() => {
    const el = logRefs[name]
    if (el) el.scrollTop = el.scrollHeight
  })
}

function openToolInstall(node: Node) {
  toolInstallNode.value = node
  toolSearch.value = ''
  toolFilter.value = 'all'
  // reset status
  for (const t of allToolNames.value) {
    toolInstallStatus[t] = 'idle'
    toolInstallLogs[t] = []
  }
  showToolInstall.value = true
  connectToolInstallWs(node.id)
}

function closeToolInstall() {
  const ws = getToolWs()
  if (ws) { ws.onmessage = null; ws.close(); setToolWs(null) }
}

function connectToolInstallWs(nodeId: string) {
  const old = getToolWs()
  if (old) { old.onmessage = null; old.close() }
  const token = localStorage.getItem('nscan_token') || ''
  const ws = new WebSocket(`${wsBase}/ws/nodes/${nodeId}/logs?token=${token}`)
  setToolWs(ws)
  // 记录连接时刻，用于区分历史回放消息和本次新消息
  const sessionStart = Date.now()
  ws.onmessage = (ev) => {
    try {
      const entry = JSON.parse(ev.data)
      // 历史回放的消息时间早于连接时刻，只用于 install_result（实时推送无时间戳延迟）
      const entryTime = entry.time ? new Date(entry.time).getTime() : Date.now()
      const isReplay = entryTime < sessionStart - 1000

      // handle install_result event（始终是实时推送，不会回放）
      if (entry.kind === 'install_result' && entry.data) {
        const d = entry.data
        const name = d.tool_name
        if (d.success) {
          toolInstallStatus[name] = 'success'
          ElMessage.success(`${name} 安装成功`)
          if (toolInstallNode.value && d.installed_tools) {
            toolInstallNode.value.installed_tools = d.installed_tools
            const n = nodes.value.find(x => x.id === toolInstallNode.value?.id)
            if (n) n.installed_tools = d.installed_tools
          }
          if (toolInstallLogs[name] !== undefined)
            toolInstallLogs[name].push({ msg: '安装完成', level: 'info' })
        } else {
          toolInstallStatus[name] = 'failed'
          ElMessage.error(`${name} 安装失败`)
          if (toolInstallLogs[name] !== undefined)
            toolInstallLogs[name].push({ msg: d.error || `${name} 重装失败`, level: 'error' })
        }
        return
      }

      // 历史回放日志不显示、不更新状态，避免旧操作污染当前视图
      if (isReplay) return

      // match install log lines: [安装工具/toolname] msg
      const log: string = entry.log || ''
      const match = log.match(/\[安装工具(?:\/(\w+))?\]\s*(.*)/)
      if (!match) return
      const toolName = match[1] || ''
      const msg = match[2] || log

      if (toolName && toolInstallLogs[toolName]) {
        if (toolInstallStatus[toolName] === 'idle') {
          toolInstallStatus[toolName] = 'installing'
        }
        toolInstallLogs[toolName].push({ msg, level: entry.level || 'info' })
        scrollLogToBottom(toolName)
      }
    } catch {}
  }
}

async function installSingleTool(tool: { name: string; desc: string; cmds: string[] }) {
  if (!toolInstallNode.value) return
  toolInstallStatus[tool.name] = 'installing'
  toolInstallLogs[tool.name] = []
  const fullCmd = tool.cmds.join(' && ')
  try {
    await nodeApi.installTool(toolInstallNode.value.id, tool.name, fullCmd, isToolInstalled(tool.name))
  } catch (e: any) {
    toolInstallStatus[tool.name] = 'failed'
    toolInstallLogs[tool.name].push({ msg: `请求失败: ${e.message}`, level: 'error' })
    ElMessage.error(`发送安装指令失败: ${e.message}`)
  }
}

// ── 节点日志 ──────────────────────────────────────────────────────────────────
interface LogEntry { time: string; node_id: string; level: string; log: string }

const logDrawerVisible = ref(false)
const logNode = ref<Node | null>(null)
const nodeLogs = ref<LogEntry[]>([])
const logAutoScroll = ref(true)
const wsConnected = ref(false)
const logFilter = reactive({ level: '', keyword: '' })
const logBox = ref<HTMLElement | null>(null)
let logWs: WebSocket | null = null

const filteredLogs = computed(() => {
  return nodeLogs.value.filter(l => {
    if (logFilter.level && l.level !== logFilter.level) return false
    if (logFilter.keyword && !l.log.includes(logFilter.keyword)) return false
    return true
  })
})

function fmtLogTime(iso: string) {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleTimeString('zh-CN', { hour12: false }) + '.' + String(d.getMilliseconds()).padStart(3, '0')
}

function openNodeLog(node: Node) {
  logNode.value = node
  nodeLogs.value = []
  logHistoryDone.value = false
  logDrawerVisible.value = true
  connectNodeLog(node.id)
}

const logHistoryDone = ref(false)

function connectNodeLog(nodeId: string) {
  if (logWs) { logWs.close(); logWs = null }
  const batch: LogEntry[] = []
  let historyTimer: ReturnType<typeof setTimeout> | null = null

  const token = localStorage.getItem('nscan_token') || ''
  logWs = new WebSocket(`${wsBase}/ws/nodes/${nodeId}/logs?token=${token}`)
  logWs.onopen = () => { wsConnected.value = true }
  logWs.onclose = () => { wsConnected.value = false }
  logWs.onmessage = (ev) => {
    try {
      const entry: LogEntry = JSON.parse(ev.data)
      if (!logHistoryDone.value) {
        batch.push(entry)
        if (historyTimer) clearTimeout(historyTimer)
        historyTimer = setTimeout(() => {
          nodeLogs.value = batch.splice(0)
          logHistoryDone.value = true
          nextTick(() => { if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight })
        }, 100)
      } else {
        nodeLogs.value.push(entry)
        if (nodeLogs.value.length > 2000) nodeLogs.value.shift()
        if (logAutoScroll.value) {
          nextTick(() => { if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight })
        }
      }
    } catch {}
  }
}

function closeNodeLog() {
  if (logWs) { logWs.close(); logWs = null }
  wsConnected.value = false
}

// ── 节点管理 ──────────────────────────────────────────────────────────────────

function isOnline(node: Node) {
  return node.status === 'online'
}
function loadColor(v: number) {
  if (v < 50) return '#2BA471'
  if (v < 80) return '#F0883A'
  return '#F54A45'
}
function fmtTime(iso: string) { if (!iso) return '—'; return new Date(iso).toLocaleString('zh-CN', { hour12: false }) }
function copy(text: string) { navigator.clipboard.writeText(text).then(() => ElMessage.success('已复制')) }

function openRename(node: Node) {
  renameForm.nodeId = node.id; renameForm.oldName = node.name || node.id.slice(0,12); renameForm.newName = node.name || ''; showRename.value = true
}
function submitRename() {
  if (!renameForm.newName.trim()) { ElMessage.warning('请输入新名称'); return }
  const n = nodes.value.find(x => x.id === renameForm.nodeId)
  if (n) n.name = renameForm.newName.trim()
  showRename.value = false; ElMessage.success('已重命名')
}
function openConcurrency(node: Node) {
  concurrencyForm.nodeId = node.id; concurrencyForm.name = node.name || node.id.slice(0,12); concurrencyForm.value = node.max_tasks ?? 5; showConcurrency.value = true
}
function submitConcurrency() {
  const n = nodes.value.find(x => x.id === concurrencyForm.nodeId)
  if (n) n.max_tasks = concurrencyForm.value
  showConcurrency.value = false; ElMessage.success('已更新')
}
async function restartNode(node: Node) {
  try { await nodeApi.restart(node.id); ElMessage.success(`已发送重启指令`) } catch { ElMessage.error('重启失败') }
}
async function confirmDelete(node: Node) {
  try {
    await ElMessageBox.confirm(`确认删除节点「${node.name}」？删除后节点需重新注册。`, '删除节点', {
      type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消',
      confirmButtonClass: 'el-button--danger',
    })
  } catch { return }
  try {
    await nodeApi.remove(node.id)
    nodes.value = nodes.value.filter(n => n.id !== node.id)
    ElMessage.success('节点已删除')
  } catch (e: any) { ElMessage.error(e.message || '删除失败') }
}
async function fetchNodes() {
  loading.value = true
  try { nodes.value = await nodeApi.list() } catch {} finally { loading.value = false }
}
onMounted(() => {
  fetchNodes()
  fetchToolDefs()
  timer = setInterval(() => { if (autoRefresh.value) fetchNodes() }, 10_000)
})
onUnmounted(() => { clearInterval(timer); closeNodeLog() })
</script>

<style scoped>
.editable-name { cursor: pointer; display: inline-flex; align-items: center; gap: 4px; }
.editable-name:hover { color: var(--el-color-primary); }
.editable-name:hover .edit-icon { opacity: 1; }
.edit-icon { opacity: 0; font-size: 12px; transition: opacity 0.15s; }
.status-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; }
.status-dot.online { background: #2BA471; }
.status-dot.offline { background: var(--el-text-color-disabled); }
.log-terminal {
  flex: 1;
  background: #0d1117;
  border-radius: 8px;
  padding: 12px 16px;
  font-family: 'JetBrains Mono', 'Fira Code', Menlo, monospace;
  font-size: 12px;
  line-height: 1.75;
  overflow-y: auto;
  color: #e6edf3;
  min-height: 300px;
  max-height: calc(100vh - 240px);
  border: 1px solid #21262d;
}
.log-empty { color: #484f58; padding: 4px 0; }
.log-line { display: flex; gap: 10px; align-items: baseline; }
.log-time { color: #484f58; flex-shrink: 0; font-size: 11px; }
.log-badge {
  flex-shrink: 0; font-size: 10px; font-weight: 700;
  padding: 1px 5px; border-radius: 3px; min-width: 38px; text-align: center;
}
.badge-info  { background: #1f6feb33; color: #58a6ff; }
.badge-warn  { background: #9e6a0333; color: #d29922; }
.badge-error { background: #da363333; color: #f85149; }
.badge-debug { background: #23863633; color: #3fb950; }
.log-msg { word-break: break-all; color: #c9d1d9; }
.tool-list { display: flex; flex-direction: column; gap: 8px; }
.tool-item { border: 1px solid var(--el-border-color-lighter); border-radius: 8px; overflow: hidden; }
.tool-header { display: flex; align-items: center; gap: 10px; padding: 10px 14px; background: var(--el-fill-color-light); }
.tool-name { font-weight: 600; font-size: 14px; min-width: 100px; }
.tool-desc { color: var(--el-text-color-secondary); font-size: 12px; }
.tool-log-area { padding: 0 14px 10px; }
.tool-log-box {
  background: #0d1117; border-radius: 6px; padding: 8px 12px;
  font-family: 'JetBrains Mono', 'Fira Code', Menlo, monospace;
  font-size: 11px; line-height: 1.6; color: #c9d1d9;
  max-height: 150px; overflow-y: auto;
}
.tool-log-line { white-space: pre-wrap; word-break: break-all; }
.tool-log-line.log-error { color: #f85149; }
.tool-fail-hint { padding: 8px 14px 12px; }
.cmd-title { font-size: 13px; font-weight: 600; margin-bottom: 6px; color: var(--el-text-color-regular); }
.cmd-box {
  display: flex; align-items: center; gap: 8px; padding: 10px 14px;
  background: #0d1117; border-radius: 6px; margin-bottom: 8px;
  font-family: 'JetBrains Mono', 'Fira Code', Menlo, monospace; font-size: 12px;
}
.cmd-box code { flex: 1; color: #e6edf3; word-break: break-all; white-space: pre-wrap; user-select: all; }
</style>
