<template>
  <div class="page-card">
    <div class="page-header">
      <h2 class="page-title">定时扫描</h2>
      <el-button type="primary" @click="openCreate">
        <el-icon><Plus /></el-icon>新建定时任务
      </el-button>
    </div>

    <el-table :data="jobs" v-loading="loading" style="width:100%">
      <el-table-column prop="name" label="任务名称" min-width="150" show-overflow-tooltip />
      <el-table-column label="执行计划" min-width="170">
        <template #default="{ row }"><el-tag type="info" size="small">{{ describeCron(row.cron) }}</el-tag></template>
      </el-table-column>
      <el-table-column prop="project_name" label="所属项目" width="130" show-overflow-tooltip />
      <el-table-column label="扫描模块" min-width="220">
        <template #default="{ row }">
          <el-tag v-for="s in jobStages(row)" :key="s" size="small" style="margin-right:4px">{{ stageLabel(s) }}</el-tag>
          <el-tag v-if="row.template_name" size="small" type="info" effect="plain" style="margin-left:4px">📋 {{ row.template_name }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <span class="status-dot" :class="row.enabled ? 'online' : 'offline'" />
          <span style="margin-left:5px">{{ row.enabled ? '启用' : '停用' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="下次运行" width="160">
        <template #default="{ row }">{{ fmt(row.next_run) }}</template>
      </el-table-column>
      <el-table-column label="上次运行" width="160">
        <template #default="{ row }">{{ fmt(row.last_run) }}</template>
      </el-table-column>
      <el-table-column prop="run_count" label="已运行" width="80" align="center" />
      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="runNow(row)">立即运行</el-button>
          <el-divider direction="vertical" />
          <el-button :type="row.enabled ? 'warning' : 'success'" link size="small" @click="toggleJob(row)">
            {{ row.enabled ? '停用' : '启用' }}
          </el-button>
          <el-divider direction="vertical" />
          <el-button type="primary" link size="small" @click="openEdit(row)">编辑</el-button>
          <el-divider direction="vertical" />
          <el-popconfirm title="确认删除该定时任务？" @confirm="deleteJob(row)">
            <template #reference>
              <el-button type="danger" link size="small">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!loading && jobs.length === 0" description="暂无定时任务" style="padding:60px 0" />

    <el-dialog v-model="showDialog" :title="editingId ? '编辑定时任务' : '新建定时扫描任务'" width="820px" destroy-on-close>
      <el-form :model="form" label-position="top" style="padding:0 4px">
        <!-- 配置模式（先选模式，UI 按模式收缩） -->
        <el-form-item label="配置方式" style="margin-bottom:8px">
          <el-radio-group v-model="form.configMode" @change="onConfigModeChange">
            <el-radio-button value="template">选择模板</el-radio-button>
            <el-radio-button value="task">关联现有任务</el-radio-button>
            <el-radio-button value="custom">自定义配置</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <!-- ═══════ 关联现有任务模式：只需选任务 + 起个名字，其他从任务继承 ═══════ -->
        <template v-if="form.configMode === 'task'">
          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="选择任务" required>
                <el-select v-model="linkedTaskId" placeholder="选择要周期性运行的任务" style="width:100%" filterable @change="onLinkTask">
                  <el-option v-for="t in taskList" :key="t.id" :label="t.name" :value="t.id">
                    <div style="display:flex;justify-content:space-between;align-items:center;width:100%">
                      <span>{{ t.name }}</span>
                      <span style="font-size:11px;color:var(--el-text-color-secondary)">{{ taskProjectName(t) }} · {{ (t.targets||[]).length }} 目标</span>
                    </div>
                  </el-option>
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="定时任务名称" required>
                <el-input v-model="form.name" placeholder="给这个定时任务起个名字" />
              </el-form-item>
            </el-col>
          </el-row>

          <!-- 只读摘要：让用户看到到底会跑什么，但不能改（要改去任务页改） -->
          <div v-if="linkedTask" class="linked-summary">
            <el-descriptions :column="1" border size="small">
              <el-descriptions-item label="所属项目">{{ taskProjectName(linkedTask) }}</el-descriptions-item>
              <el-descriptions-item label="扫描目标">
                <div style="font-family:monospace;font-size:12px;max-height:80px;overflow-y:auto;white-space:pre-line">{{ (linkedTask.targets || []).join('\n') || '—' }}</div>
              </el-descriptions-item>
              <el-descriptions-item label="扫描节点">
                <template v-if="linkedTask.node_ids?.length">
                  <el-tag v-for="id in linkedTask.node_ids" :key="id" size="small" style="margin-right:4px">
                    {{ nodes.find(n => n.id === id)?.name || id.slice(0,12) }}
                  </el-tag>
                </template>
                <span v-else style="color:var(--el-text-color-secondary)">自动分配</span>
              </el-descriptions-item>
              <el-descriptions-item label="扫描模块">
                <el-tag v-for="s in linkedTaskStages" :key="s" size="small" style="margin-right:4px">
                  {{ moduleIcon(s) }} {{ stageLabel(s) }}
                </el-tag>
              </el-descriptions-item>
            </el-descriptions>
            <div style="font-size:11px;color:var(--el-text-color-secondary);margin-top:6px">
              目标 / 节点 / 插件参数都从这个任务继承；如需调整请去「任务管理」修改该任务。
            </div>
          </div>
        </template>

        <!-- ═══════ 模板/自定义模式：需要用户填项目、目标、节点 ═══════ -->
        <template v-else>
          <el-row :gutter="16">
            <el-col :span="8">
              <el-form-item label="所属项目" required>
                <el-select v-model="form.project_id" placeholder="选择项目" style="width:100%" filterable>
                  <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="任务名称" required>
                <el-input v-model="form.name" placeholder="定时任务名称" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="扫描节点">
                <el-select v-model="form.node_ids" multiple collapse-tags collapse-tags-tooltip
                  placeholder="自动分配" style="width:100%">
                  <el-option v-for="n in onlineNodes" :key="n.id" :label="n.name || n.id.slice(0,12)" :value="n.id">
                    <span>{{ n.name || n.id.slice(0,12) }}</span>
                    <span style="float:right;color:var(--el-text-color-secondary);font-size:11px">{{ n.active_tasks }}/{{ n.max_tasks }}</span>
                  </el-option>
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>

          <el-form-item label="扫描目标" required>
            <el-input v-model="targetsText" type="textarea" :rows="4"
              placeholder="每行一个目标，支持域名、IP、CIDR&#10;example.com&#10;192.168.1.0/24&#10;10.0.0.1"
              style="font-family:monospace;font-size:12px" />
          </el-form-item>

          <!-- 模板模式 -->
          <template v-if="form.configMode === 'template'">
            <el-form-item label="扫描模版" required>
              <el-select v-model="form.template_id" placeholder="选择模版" style="width:100%" filterable>
                <el-option v-for="t in templates" :key="t.id" :label="t.name" :value="t.id">
                  <span>{{ t.name }}</span>
                  <span style="float:right;color:var(--el-text-color-secondary);font-size:12px">{{ t.description }}</span>
                </el-option>
              </el-select>
            </el-form-item>
            <div v-if="selectedTemplate" class="tpl-preview">
              <el-tag v-for="s in tplStages(selectedTemplate)" :key="s" size="small" effect="plain">
                {{ moduleIcon(s) }} {{ stageLabel(s) }}
              </el-tag>
            </div>
          </template>

          <!-- 自定义配置模式 -->
          <template v-else>
            <PluginConfigEditor :model-value="customConfig" :plugins="filteredPluginsForTask" :dicts="allDicts" />
          </template>
        </template>

        <!-- 通俗的执行计划配置，提交时转换成后端兼容的 Cron -->
        <el-row :gutter="16" style="margin-top:16px">
          <el-col :span="8">
            <el-form-item label="执行频率" required>
              <el-select v-model="form.scheduleType" style="width:100%" @change="onScheduleTypeChange">
                <el-option label="每隔一段时间" value="interval" />
                <el-option label="每天" value="daily" />
                <el-option label="每周" value="weekly" />
                <el-option label="每月" value="monthly" />
                <el-option v-if="form.scheduleType === 'advanced'" label="原有自定义计划" value="advanced" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8" v-if="form.scheduleType === 'interval'">
            <el-form-item label="间隔时间" required>
              <el-select v-model="form.intervalValue" style="width:48%">
                <el-option v-for="v in intervalValues" :key="v" :label="String(v)" :value="v" />
              </el-select>
              <el-select v-model="form.intervalUnit" style="width:48%;margin-left:4%">
                <el-option label="分钟" value="minute" />
                <el-option label="小时" value="hour" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8" v-else-if="form.scheduleType === 'weekly'">
            <el-form-item label="星期几" required>
              <el-select v-model="form.weekday" style="width:100%">
                <el-option v-for="d in weekdays" :key="d.value" :label="d.label" :value="d.value" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8" v-else-if="form.scheduleType === 'monthly'">
            <el-form-item label="每月第几天" required>
              <el-select v-model="form.monthday" style="width:100%">
                <el-option v-for="d in 31" :key="d" :label="`${d} 日`" :value="d" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8" v-if="['daily', 'weekly', 'monthly'].includes(form.scheduleType)">
            <el-form-item label="执行时间" required>
              <el-time-picker v-model="form.time" format="HH:mm" value-format="HH:mm" style="width:100%" />
            </el-form-item>
          </el-col>
          <el-col :span="8" v-if="form.scheduleType === 'advanced'">
            <el-form-item label="原有计划">
              <el-input :model-value="form.cron" disabled />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="创建后即启用">
              <el-switch v-model="form.enabled" />
            </el-form-item>
          </el-col>
        </el-row>
        <div class="schedule-preview">
          <span>{{ scheduleSummary }}</span>
          <span>下次运行：{{ nextPreview }}</span>
          <span v-if="form.scheduleType === 'advanced'" class="schedule-warning">该任务的原有计划暂不支持图形化编辑，保存时会保持不变</span>
        </div>
      </el-form>

      <template #footer>
        <el-button @click="showDialog=false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">{{ editingId ? '保存' : '创建' }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { projectApi, taskApi, scheduledApi, scanTemplateApi, nodeApi, pluginApi, dictApi, toolDefApi,
  type Project, type Task, type ScheduledJob, type ScanTemplate, type StagePlugin,
  type Node, type Plugin, type DictEntry, type ToolDef } from '@/api'
import { MODULE_ORDER, moduleIcon, stageLabel } from '@/constants/modules'
import PluginConfigEditor from '@/components/PluginConfigEditor.vue'

const jobs = ref<ScheduledJob[]>([])
const projects = ref<Project[]>([])
const taskList = ref<Task[]>([])
const templates = ref<ScanTemplate[]>([])
const nodes = ref<Node[]>([])
const allPlugins = ref<Plugin[]>([])
const allDicts = ref<DictEntry[]>([])
const toolDefs = ref<ToolDef[]>([])

const filteredPluginsForTask = computed(() => {
  if (form.value.node_ids.length === 0) return allPlugins.value.filter(p => p.enabled)
  const selectedNodes = onlineNodes.value.filter(n => form.value.node_ids.includes(n.id))
  return allPlugins.value.filter(p => {
    if (!p.enabled) return false
    const toolDef = toolDefs.value.find(t => t.name === p.name)
    if (!toolDef) return true
    return selectedNodes.every(n => n.installed_tools?.includes(toolDef.name))
  })
})
const loading = ref(false)
const saving = ref(false)
const showDialog = ref(false)
const editingId = ref<string | null>(null)
const targetsText = ref('')
const linkedTaskId = ref<string | null>(null)

const moduleOrder = [...MODULE_ORDER]

interface FormState {
  name: string
  project_id: string
  cron: string
  enabled: boolean
  node_ids: string[]
  template_id: string
  configMode: 'template' | 'task' | 'custom'
  scheduleType: 'interval' | 'daily' | 'weekly' | 'monthly' | 'advanced'
  intervalValue: number
  intervalUnit: 'minute' | 'hour'
  time: string
  weekday: number
  monthday: number
}
const emptyForm = (): FormState => ({
  name: '', project_id: '', cron: '0 2 * * *', enabled: true,
  node_ids: [], template_id: '', configMode: 'template',
  scheduleType: 'daily', intervalValue: 15, intervalUnit: 'minute', time: '02:00', weekday: 1, monthday: 1,
})
const form = ref<FormState>(emptyForm())

// customConfig 与 Tasks.vue 结构相同：mod → pluginId → {enabled, params}
const customConfig = reactive<Record<string, Record<string, { enabled: boolean; params: Record<string, any> }>>>({})

const onlineNodes = computed(() => nodes.value.filter(n => {
  const lastSeen = new Date(n.last_seen_at || n.last_seen).getTime()
  return Date.now() - lastSeen < 60000
}))

const selectedTemplate = computed(() => templates.value.find(t => t.id === form.value.template_id) ?? null)
const linkedTask = computed(() => taskList.value.find(t => t.id === linkedTaskId.value) ?? null)

const linkedTaskStages = computed(() => {
  const t = linkedTask.value
  if (!t) return []
  if (t.modules) return MODULE_ORDER.filter(mod => t.modules?.[mod]?.some((p: any) => p.enabled))
  return t.config?.stages || []
})

function tplStages(tpl: ScanTemplate): string[] {
  if (!tpl.modules) return []
  return MODULE_ORDER.filter(mod => tpl.modules[mod]?.some((p: any) => p.enabled))
}

function jobStages(job: ScheduledJob): string[] {
  if (job.modules) {
    return MODULE_ORDER.filter(mod => (job.modules as any)?.[mod]?.some((p: any) => p.enabled))
  }
  return job.stages || []
}

function modulePlugins(mod: string) {
  return allPlugins.value.filter(p => p.module === mod && p.enabled)
}

function taskProjectName(task: Task) {
  return projects.value.find(p => p.id === task.project_id)?.name || '—'
}

// 切换模式时清掉上一模式携带的引用，避免误用。
function onConfigModeChange() {
  if (form.value.configMode !== 'task') linkedTaskId.value = null
  if (form.value.configMode !== 'template') form.value.template_id = ''
  if (form.value.configMode === 'custom') {
    // 从模板/任务切到自定义时保留 customConfig 现值，用户可以基于已有值继续调
  }
}

// onLinkTask 选中任务后：
// - 只需把定时任务名字预填一下（用户可改），其余不动。
// - 提交时会直接从 linkedTask 读取项目/目标/节点/插件，UI 不给二次编辑的入口。
function onLinkTask(taskId: string) {
  const task = taskList.value.find(t => t.id === taskId)
  if (!task) return
  if (!form.value.name) form.value.name = `定时-${task.name}`
  // 下面的 customConfig 灌入只用于「未来切到自定义模式想继续调」时无缝过渡，可选行为
  // 重置 customConfig 后把任务里的插件配置写回
  initCustomConfig()
  if (task.modules) {
    for (const [mod, plugins] of Object.entries(task.modules)) {
      for (const sp of plugins as StagePlugin[]) {
        if (!sp.enabled) continue
        const plugin = allPlugins.value.find(p => p.id === sp.plugin_id || p.name === sp.name)
        if (plugin && customConfig[mod]?.[plugin.id]) {
          customConfig[mod][plugin.id].enabled = true
          if (sp.params) {
            // 只接受当前插件 schema 里的 key，丢弃老数据废弃参数
            const schemaKeys = new Set((plugin.params || []).map(p => p.key))
            for (const [k, v] of Object.entries(sp.params)) {
              if (schemaKeys.has(k)) customConfig[mod][plugin.id].params[k] = v
            }
          }
        }
      }
    }
  } else if (task.config?.params && task.config.stages?.length) {
    for (const stage of task.config.stages) {
      const plugins = modulePlugins(stage)
      if (!plugins.length) continue
      const plugin = plugins[0]
      if (!customConfig[stage]?.[plugin.id]) continue
      customConfig[stage][plugin.id].enabled = true
      const prefix = stage + '.'
      const schemaKeys = new Set((plugin.params || []).map(p => p.key))
      for (const [k, v] of Object.entries(task.config.params)) {
        if (!k.startsWith(prefix)) continue
        const paramKey = k.slice(prefix.length)
        if (!schemaKeys.has(paramKey)) continue
        const paramDef = plugin.params?.find(p => p.key === paramKey)
        if (paramDef && (paramDef.type === 'checkbox-group' || paramDef.multiple)) {
          customConfig[stage][plugin.id].params[paramKey] = String(v).split(',').filter(Boolean)
        } else if (paramDef?.type === 'number') {
          customConfig[stage][plugin.id].params[paramKey] = Number(v)
        } else if (paramDef?.type === 'switch') {
          customConfig[stage][plugin.id].params[paramKey] = v === 'true'
        } else {
          customConfig[stage][plugin.id].params[paramKey] = v
        }
      }
    }
  }
}

// initCustomConfig 用当前 allPlugins 预填 customConfig 的所有槽位（enabled=false + 默认参数）
function initCustomConfig() {
  for (const p of allPlugins.value) {
    if (!customConfig[p.module]) customConfig[p.module] = {}
    if (!customConfig[p.module][p.id]) {
      const defaults: Record<string, any> = {}
      for (const param of (p.params || [])) {
        defaults[param.key] = param.default ?? (param.type === 'checkbox-group' || param.multiple ? [] : param.type === 'number' ? 0 : param.type === 'switch' ? false : '')
      }
      customConfig[p.module][p.id] = { enabled: false, params: defaults }
    } else {
      // 已存在则重置为默认（编辑不同 job 时清理上次残留）
      customConfig[p.module][p.id].enabled = false
    }
  }
}

const intervalValues = [5, 10, 15, 30, 60]
const weekdays = [
  { label: '星期一', value: 1 }, { label: '星期二', value: 2 }, { label: '星期三', value: 3 },
  { label: '星期四', value: 4 }, { label: '星期五', value: 5 }, { label: '星期六', value: 6 }, { label: '星期日', value: 0 },
]

function cronFromSchedule(): string {
  if (form.value.scheduleType === 'advanced') return form.value.cron
  const [hour, minute] = form.value.time.split(':').map(Number)
  if (form.value.scheduleType === 'interval') {
    return form.value.intervalUnit === 'minute'
      ? `*/${form.value.intervalValue} * * * *`
      : `0 */${form.value.intervalValue} * * *`
  }
  if (form.value.scheduleType === 'weekly') return `${minute} ${hour} * * ${form.value.weekday}`
  if (form.value.scheduleType === 'monthly') return `${minute} ${hour} ${form.value.monthday} * *`
  return `${minute} ${hour} * * *`
}

function scheduleFromCron(expr: string): Pick<FormState, 'scheduleType' | 'intervalValue' | 'intervalUnit' | 'time' | 'weekday' | 'monthday'> {
  const fields = expr.trim().split(/\s+/)
  const result = { scheduleType: 'advanced' as FormState['scheduleType'], intervalValue: 15, intervalUnit: 'minute' as FormState['intervalUnit'], time: '02:00', weekday: 1, monthday: 1 }
  if (fields.length !== 5) return result
  const [minute, hour, dom, month, dow] = fields
  const time = /^\d+$/.test(minute) && /^\d+$/.test(hour) ? `${hour.padStart(2, '0')}:${minute.padStart(2, '0')}` : ''
  if (time && dom === '*' && month === '*' && dow === '*') return { ...result, scheduleType: 'daily', time }
  if (time && dom === '*' && month === '*' && /^\d$/.test(dow)) return { ...result, scheduleType: 'weekly', time, weekday: Number(dow) }
  if (time && /^\d+$/.test(dom) && month === '*' && dow === '*') return { ...result, scheduleType: 'monthly', time, monthday: Number(dom) }
  const minuteStep = /^\*\/(\d+)$/.exec(minute)
  if (minuteStep && hour === '*' && dom === '*' && month === '*' && dow === '*') return { ...result, scheduleType: 'interval', intervalValue: Number(minuteStep[1]), intervalUnit: 'minute' }
  const hourStep = /^\*\/(\d+)$/.exec(hour)
  if (minute === '0' && hourStep && dom === '*' && month === '*' && dow === '*') return { ...result, scheduleType: 'interval', intervalValue: Number(hourStep[1]), intervalUnit: 'hour' }
  if (minute === '0' && hour === '*' && dom === '*' && month === '*' && dow === '*') return { ...result, scheduleType: 'interval', intervalValue: 1, intervalUnit: 'hour' }
  return result
}

function onScheduleTypeChange() {
  if (form.value.scheduleType !== 'advanced') form.value.cron = cronFromSchedule()
}

const scheduleSummary = computed(() => form.value.scheduleType === 'advanced' ? '执行计划：保留原有设置' : `执行计划：${describeCron(cronFromSchedule())}`)

function describeCron(expr: string): string {
  const parsed = scheduleFromCron(expr)
  if (parsed.scheduleType === 'interval') return `每 ${parsed.intervalValue}${parsed.intervalUnit === 'minute' ? ' 分钟' : ' 小时'}`
  if (parsed.scheduleType === 'daily') return `每天 ${parsed.time}`
  if (parsed.scheduleType === 'weekly') return `每周${weekdays.find(d => d.value === parsed.weekday)?.label.replace('星期', '') || ''} ${parsed.time}`
  if (parsed.scheduleType === 'monthly') return `每月 ${parsed.monthday} 日 ${parsed.time}`
  return '自定义计划'
}

function fmt(t?: string | null) {
  if (!t) return '—'
  const d = new Date(t)
  if (isNaN(d.getTime())) return '—'
  return d.toLocaleString('zh-CN', { hour12: false })
}

const nextPreview = computed(() => {
  const t = nextCron(cronFromSchedule(), new Date())
  return t ? t.toLocaleString('zh-CN', { hour12: false }) : '表达式无效'
})

async function load() {
  loading.value = true
  try {
    const res = await scheduledApi.list({ limit: 200 })
    jobs.value = res.data ?? []
  } catch (e: any) {
    ElMessage.error(e.message || '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  const [pRes, tRes] = await Promise.all([
    projectApi.list({ limit: 200 }).catch(() => ({ data: [] as Project[], total: 0 })),
    taskApi.list({ limit: 200 }).catch(() => ({ data: [] as Task[], total: 0 })),
  ])
  projects.value = pRes.data ?? []
  taskList.value = tRes.data ?? []
  await Promise.all([
    load(),
    scanTemplateApi.list({ limit: 200 }).then(r => templates.value = r.data ?? []).catch(() => {}),
    nodeApi.list().then(l => nodes.value = l).catch(() => {}),
    pluginApi.list().then(l => allPlugins.value = l).catch(() => {}),
    toolDefApi.list().then(l => toolDefs.value = l).catch(() => {}),
    dictApi.list().then(r => allDicts.value = r.data || []).catch(() => {}),
  ])
})

function openCreate() {
  editingId.value = null
  form.value = emptyForm()
  targetsText.value = ''
  linkedTaskId.value = null
  initCustomConfig()
  // 刷新在线节点/任务列表，避免弹窗内看到过期列表
  nodeApi.list().then(l => nodes.value = l).catch(() => {})
  taskApi.list({ limit: 200 }).then(r => taskList.value = r.data || []).catch(() => {})
  showDialog.value = true
}

function openEdit(row: ScheduledJob) {
  editingId.value = row.id
  form.value = {
    name: row.name,
    project_id: row.project_id,
    cron: row.cron,
    enabled: row.enabled,
    node_ids: [...(row.node_ids || [])],
    template_id: row.template_id || '',
    // 编辑已有 job 时"关联任务"没有意义（关联关系不持久化），仅在 template/custom 之间切换
    configMode: row.template_id && !row.modules ? 'template' : 'custom',
    ...scheduleFromCron(row.cron),
  }
  targetsText.value = (row.targets ?? []).join('\n')
  linkedTaskId.value = null
  initCustomConfig()
  // 若历史 job 有 modules，展开回 customConfig；这样编辑时能直接改参数
  if (row.modules) {
    for (const [mod, plugins] of Object.entries(row.modules)) {
      for (const sp of plugins as StagePlugin[]) {
        if (!sp.enabled) continue
        const plugin = allPlugins.value.find(p => p.id === sp.plugin_id || p.name === sp.name)
        if (plugin && customConfig[mod]?.[plugin.id]) {
          customConfig[mod][plugin.id].enabled = true
          if (sp.params) {
            const schemaKeys = new Set((plugin.params || []).map(p => p.key))
            for (const [k, v] of Object.entries(sp.params)) {
              if (schemaKeys.has(k)) customConfig[mod][plugin.id].params[k] = v
            }
          }
        }
      }
    }
  } else if (row.params && row.stages?.length) {
    // 老数据仅 stages/params：把 params 按 stage.key 前缀分发到第一个匹配插件
    for (const stage of row.stages) {
      const plugins = modulePlugins(stage)
      if (!plugins.length) continue
      const plugin = plugins[0]
      if (!customConfig[stage]?.[plugin.id]) continue
      customConfig[stage][plugin.id].enabled = true
      const prefix = stage + '.'
      const schemaKeys = new Set((plugin.params || []).map(p => p.key))
      for (const [k, v] of Object.entries(row.params)) {
        if (!k.startsWith(prefix)) continue
        const paramKey = k.slice(prefix.length)
        if (!schemaKeys.has(paramKey)) continue
        const paramDef = plugin.params?.find(p => p.key === paramKey)
        if (paramDef && (paramDef.type === 'checkbox-group' || paramDef.multiple)) {
          customConfig[stage][plugin.id].params[paramKey] = String(v).split(',').filter(Boolean)
        } else if (paramDef?.type === 'number') {
          customConfig[stage][plugin.id].params[paramKey] = Number(v)
        } else if (paramDef?.type === 'switch') {
          customConfig[stage][plugin.id].params[paramKey] = v === 'true'
        } else {
          customConfig[stage][plugin.id].params[paramKey] = v
        }
      }
    }
  }
  nodeApi.list().then(l => nodes.value = l).catch(() => {})
  showDialog.value = true
}

async function submit() {
  if (!form.value.name.trim()) { ElMessage.warning('请填写任务名称'); return }
  const scheduleCron = cronFromSchedule()
  if (!scheduleCron.trim()) { ElMessage.warning('请设置执行计划'); return }

  let projectId = ''
  let targets: string[] = []
  let nodeIds: string[] | undefined = undefined
  let stages: string[] = []
  let modules: Record<string, StagePlugin[]> | undefined
  let templateId = ''
  let templateName = ''

  if (form.value.configMode === 'task') {
    // 关联任务模式：所有配置从任务继承，UI 不再暴露编辑入口
    const task = linkedTask.value
    if (!task) { ElMessage.warning('请选择要关联的任务'); return }
    projectId = task.project_id
    targets = task.targets || []
    // 过滤当前离线节点
    nodeIds = (task.node_ids || []).filter(id => onlineNodes.value.some(n => n.id === id))
    if (nodeIds.length === 0) nodeIds = undefined
    if (task.modules) {
      modules = task.modules
      stages = linkedTaskStages.value
    } else if (task.config?.stages?.length) {
      stages = task.config.stages
    }
    if (!targets.length) { ElMessage.warning('关联的任务没有扫描目标，无法定时'); return }
    if (!stages.length) { ElMessage.warning('关联的任务未启用任何模块'); return }
    templateId = task.template_id || ''
    templateName = task.template_name || ''
  } else {
    // 模板/自定义模式：需要用户填项目、目标
    if (!form.value.project_id) { ElMessage.warning('请选择所属项目'); return }
    projectId = form.value.project_id
    targets = targetsText.value.split(/[\n,]+/).map(s => s.trim()).filter(Boolean)
    if (!targets.length) { ElMessage.warning('请至少填写一个扫描目标'); return }
    nodeIds = form.value.node_ids.length ? form.value.node_ids : undefined

    if (form.value.configMode === 'template') {
      if (!form.value.template_id) { ElMessage.warning('请选择扫描模版'); return }
      const tpl = selectedTemplate.value!
      stages = tplStages(tpl)
      modules = tpl.modules
      templateId = tpl.id
      templateName = tpl.name
    } else {
      // custom
      modules = {}
      for (const mod of moduleOrder) {
        const list: StagePlugin[] = []
        for (const p of modulePlugins(mod)) {
          const cfg = customConfig[mod]?.[p.id]
          if (cfg?.enabled) {
            list.push({ plugin_id: p.id, name: p.name, enabled: true, params: cfg.params || {} })
            if (!stages.includes(mod)) stages.push(mod)
          }
        }
        if (list.length) modules[mod] = list
      }
      if (!stages.length) { ElMessage.warning('请至少启用一个插件'); return }
    }
  }

  saving.value = true
  try {
    const body: any = {
      name: form.value.name,
      project_id: projectId,
      cron: scheduleCron,
      enabled: form.value.enabled,
      targets,
      stages,
      modules,
      node_ids: nodeIds,
    }
    if (templateId) { body.template_id = templateId; body.template_name = templateName }
    if (editingId.value) {
      await scheduledApi.update(editingId.value, body)
      ElMessage.success('已保存')
    } else {
      await scheduledApi.create(body)
      ElMessage.success('定时任务已创建')
    }
    showDialog.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(e.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function toggleJob(row: ScheduledJob) {
  try {
    await scheduledApi.update(row.id, { enabled: !row.enabled })
    ElMessage.success(!row.enabled ? '已启用' : '已停用')
    await load()
  } catch (e: any) {
    ElMessage.error(e.message || '操作失败')
  }
}

async function deleteJob(row: ScheduledJob) {
  try {
    await scheduledApi.remove(row.id)
    ElMessage.success('已删除')
    await load()
  } catch (e: any) {
    ElMessage.error(e.message || '删除失败')
  }
}

async function runNow(row: ScheduledJob) {
  try {
    await scheduledApi.runNow(row.id)
    ElMessage.success('已触发一次扫描，可在任务页查看进度')
  } catch (e: any) {
    ElMessage.error(e.message || '触发失败')
  }
}

// 与后端 cron 包一致的标准5段解析，用于表单预览下次运行时间
function nextCron(expr: string, after: Date): Date | null {
  const fields = expr.trim().split(/\s+/)
  if (fields.length !== 5) return null
  const ranges = [[0, 59], [0, 23], [1, 31], [1, 12], [0, 7]] as const
  const sets: Set<number>[] = []
  for (let i = 0; i < 5; i++) {
    const s = parseField(fields[i], ranges[i][0], ranges[i][1])
    if (!s) return null
    sets.push(s)
  }
  const [min, hr, dom, mon, dow] = sets
  if (dow.has(7)) { dow.add(0); dow.delete(7) }
  const domR = fields[2] !== '*'
  const dowR = fields[4] !== '*'
  const t = new Date(after.getTime())
  t.setSeconds(0, 0)
  t.setMinutes(t.getMinutes() + 1)
  const limit = new Date(t.getTime()); limit.setFullYear(limit.getFullYear() + 1)
  while (t < limit) {
    const domHit = dom.has(t.getDate())
    const dowHit = dow.has(t.getDay())
    const dayOk = domR && dowR ? (domHit || dowHit) : domR ? domHit : dowR ? dowHit : true
    if (min.has(t.getMinutes()) && hr.has(t.getHours()) && mon.has(t.getMonth() + 1) && dayOk) return t
    t.setMinutes(t.getMinutes() + 1)
  }
  return null
}

function parseField(field: string, lo: number, hi: number): Set<number> | null {
  const out = new Set<number>()
  for (let part of field.split(',')) {
    let step = 1
    const slash = part.indexOf('/')
    if (slash !== -1) {
      step = parseInt(part.slice(slash + 1), 10)
      if (!(step > 0)) return null
      part = part.slice(0, slash)
    }
    let a = lo, b = hi
    if (part === '*') { /* full */ }
    else if (part.includes('-')) {
      const [x, y] = part.split('-'); a = parseInt(x, 10); b = parseInt(y, 10)
      if (isNaN(a) || isNaN(b)) return null
    } else {
      a = b = parseInt(part, 10)
      if (isNaN(a)) return null
    }
    if (a < lo || b > hi || a > b) return null
    for (let v = a; v <= b; v += step) out.add(v)
  }
  return out.size ? out : null
}
</script>

<style scoped>
.status-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; }
.status-dot.online { background: #2BA471; }
.status-dot.offline { background: var(--el-text-color-disabled); }
.tpl-preview {
  background: #f0f4ff; border-radius: 6px; padding: 8px 10px;
  display: flex; align-items: center; flex-wrap: wrap; gap: 6px;
  margin-bottom: 4px;
}
.linked-summary {
  margin-bottom: 8px;
  padding: 10px 12px;
  background: #f5f7fa;
  border-radius: 6px;
  border: 1px solid var(--el-border-color-lighter);
}
.schedule-preview {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  padding: 8px 10px;
  color: var(--el-text-color-secondary);
  background: #f5f7fa;
  border-radius: 6px;
  font-size: 12px;
}
.schedule-warning { color: var(--el-color-warning); }
</style>
