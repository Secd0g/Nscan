<template>
  <div>
    <div class="page-card">
    <div class="page-header">
      <h2 class="page-title">任务管理</h2>
      <div class="header-actions">
        <el-input v-model="searchKeyword" placeholder="搜索任务名" clearable style="width:180px" @keyup.enter="filterAndFetch" @clear="filterAndFetch">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-select v-model="filterProjectId" clearable placeholder="按项目筛选" style="width:180px" @change="filterAndFetch">
          <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-select v-model="filterStatus" clearable placeholder="按状态筛选" style="width:130px" @change="filterAndFetch">
          <el-option label="排队中" value="queued" />
          <el-option label="运行中" value="running" />
          <el-option label="已完成" value="done" />
          <el-option label="失败" value="failed" />
        </el-select>
        <el-button @click="fetchList"><el-icon><Refresh /></el-icon></el-button>
        <el-popconfirm title="批量删除选中任务？" @confirm="batchDeleteTasks">
          <template #reference>
            <el-button type="danger" plain :disabled="!selectedTasks.length">
              <el-icon><Delete /></el-icon>批量删除({{ selectedTasks.length }})
            </el-button>
          </template>
        </el-popconfirm>
        <el-button type="primary" @click="openCreate">
          <el-icon><Plus /></el-icon>新建任务
        </el-button>
      </div>
    </div>

    <el-table :data="list" v-loading="loading" style="width:100%" size="default" :header-cell-style="{ background:'#f5f7fa', color:'#4e5969', fontWeight:'600' }" @selection-change="(rows: Task[]) => selectedTasks = rows">
      <el-table-column type="selection" width="42" />
      <el-table-column prop="name" label="任务名" min-width="130" show-overflow-tooltip />
      <el-table-column label="目标" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">
          <span style="font-family:monospace;font-size:12px">{{ row.targets?.join(', ') }}</span>
        </template>
      </el-table-column>
      <el-table-column label="模版" min-width="110">
        <template #default="{ row }">
          <el-tag v-if="row.template_name" size="small" type="info" effect="plain">{{ row.template_name }}</el-tag>
          <span v-else style="color:var(--el-text-color-disabled)">—</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="88">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)" size="small" effect="light">{{ statusLabel(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="进度" min-width="200">
        <template #default="{ row }">
          <template v-if="progressMap[row.id]">
            <el-progress
              :percentage="progressMap[row.id].percent"
              :status="progressMap[row.id].finished ? 'success' : undefined"
              :striped="!progressMap[row.id].finished"
              :striped-flow="!progressMap[row.id].finished"
              :duration="10"
              :stroke-width="7"
            />
          </template>
          <template v-else-if="row.progress">
            <el-progress
              :percentage="row.progress.percent"
              :status="row.status === 'done' ? 'success' : row.status === 'failed' ? 'exception' : undefined"
              :stroke-width="7"
            />
          </template>
          <span v-else-if="['running','dispatched'].includes(row.status)" style="color:var(--el-text-color-secondary);font-size:12px">
            <el-icon class="is-loading" style="margin-right:4px"><Loading /></el-icon>等待中…
          </span>
          <span v-else style="color:var(--el-text-color-disabled)">—</span>
        </template>
      </el-table-column>
      <el-table-column label="创建时间" min-width="150">
        <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="结束时间" min-width="150">
        <template #default="{ row }">
          <span v-if="row.done_at">{{ fmtTime(row.done_at) }}</span>
          <span v-else style="color:var(--el-text-color-disabled)">—</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="380" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="openDetail(row)">详情</el-button>
          <el-divider direction="vertical" />
          <el-button link size="small" @click="viewAssets(row)">资产</el-button>
          <el-divider direction="vertical" />
          <el-button link size="small" @click="copyTask(row)">复制</el-button>
          <el-divider direction="vertical" />
          <el-button v-if="['done','failed'].includes(row.status)" link size="small" @click="rescan(row)">重扫</el-button>
          <el-divider v-if="['done','failed'].includes(row.status)" direction="vertical" />
          <el-button v-if="['pending','queued','running','dispatched'].includes(row.status)" type="warning" link size="small" @click="cancelTask(row)">停止</el-button>
          <el-divider v-if="['pending','queued','running','dispatched'].includes(row.status)" direction="vertical" />
          <el-button type="danger" link size="small" @click="confirmRemove(row)">删除</el-button>
          <el-divider direction="vertical" />
          <el-button type="primary" link size="small" :disabled="row.status !== 'done'" @click="openAI(row)">
            AI 分析
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <div style="display:flex;justify-content:space-between;align-items:center;margin-top:16px">
      <span style="font-size:13px;color:var(--el-text-color-secondary)">共 {{ total }} 条记录</span>
      <el-pagination v-if="total > pageSize" v-model:current-page="page" :page-size="pageSize" :total="total"
        layout="prev, pager, next" @current-change="fetchList" />
    </div>
    </div><!-- /page-card -->

    <!-- 新建任务弹窗 -->
    <el-dialog v-model="dialogVisible" title="新建扫描任务" width="820px" destroy-on-close>
      <el-form :model="form" label-position="top" style="padding:0 4px">
        <!-- 基本信息 -->
        <el-row :gutter="16">
          <el-col :span="8">
            <el-form-item label="所属项目" required>
              <el-select v-model="form.project_id" placeholder="选择项目" style="width:100%">
                <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="任务名" required>
              <el-input v-model="form.name" placeholder="任务名称" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="扫描节点" required>
              <el-select v-model="form.node_ids" multiple collapse-tags collapse-tags-tooltip
                placeholder="请选择扫描节点" style="width:100%">
                <el-option v-for="n in onlineNodes" :key="n.id" :label="n.name || n.id.slice(0,12)" :value="n.id">
                  <span>{{ n.name || n.id.slice(0,12) }}</span>
                  <span style="float:right;color:var(--el-text-color-secondary);font-size:11px">{{ n.active_tasks }}/{{ n.max_tasks }}</span>
                </el-option>
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <!-- 扫描目标（紧跟项目） -->
        <el-form-item label="扫描目标" required>
          <el-input v-model="form.targetsText" type="textarea" :rows="5"
            placeholder="每行一个目标，或用逗号分隔，支持以下格式：&#10;域名：example.com&#10;IP：192.168.1.1&#10;CIDR：10.0.0.0/24&#10;带端口：192.168.1.1:8080"
            style="font-family:monospace;font-size:12px" />
          <div style="font-size:11px;color:var(--el-text-color-secondary);margin-top:4px">支持换行或逗号分隔</div>
        </el-form-item>

        <!-- 配置模式 -->
        <el-form-item label="插件配置" style="margin-bottom:8px">
          <el-radio-group v-model="form.configMode" style="margin-bottom:8px">
            <el-radio-button value="template">选择模板</el-radio-button>
            <el-radio-button value="custom">自定义配置</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <!-- 模板模式 -->
        <template v-if="form.configMode === 'template'">
          <el-form-item label="扫描模版" required>
            <el-select v-model="form.template_id" placeholder="选择模版" style="width:100%">
              <el-option v-for="t in templates" :key="t.id" :label="t.name" :value="t.id">
                <span>{{ t.name }}</span>
                <span style="float:right;color:var(--el-text-color-secondary);font-size:12px">{{ t.description }}</span>
              </el-option>
            </el-select>
          </el-form-item>
          <div v-if="selectedTemplate" class="tpl-preview">
            <el-tag v-for="s in tplStages(selectedTemplate)" :key="s" size="small" effect="plain">{{ stageLabel(s) }}</el-tag>
          </div>
        </template>

        <!-- 自定义配置模式 -->
        <template v-if="form.configMode === 'custom'">
          <PluginConfigEditor :model-value="customConfig" :plugins="filteredPluginsForTask" :dicts="allDicts" :disabled-plugins="disabledPlugins" />
        </template>
        <el-form-item label="扫描完成后" style="margin-top:12px">
          <el-checkbox v-model="form.ai_analysis_enabled">自动进行 AI 分析</el-checkbox>
          <div style="font-size:12px;color:var(--el-text-color-secondary);margin-top:4px">需要先在「AI 配置」中填写接口地址、Token 和模型</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">
          <el-icon><VideoPlay /></el-icon>提交任务
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="aiDialogVisible" title="" width="800px" destroy-on-close>
      <el-tabs v-model="aiActiveTab" class="ai-tabs">
        <el-tab-pane label="分析操作与日志" name="process">
          <div class="ai-toolbar">
            <el-tag v-if="aiTask" :type="aiStatusType(aiTask.ai_analysis_status)">{{ aiStatusLabel(aiTask.ai_analysis_status) }}</el-tag>
            <el-button v-if="aiTask?.ai_analysis_status !== 'running'" type="primary" :loading="aiLoading" @click="startAI">{{ aiTask?.ai_analysis_status === 'done' ? '重新分析' : '启动 AI 分析' }}</el-button>
            <el-button v-else type="danger" plain @click="stopAI">停止 AI 分析</el-button>
          </div>
          <div class="ai-log-title">分析过程日志</div>
          <div class="ai-log-box"><div v-for="(line, i) in (aiTask?.ai_analysis_log || [])" :key="i">{{ line }}</div><span v-if="!aiTask?.ai_analysis_log?.length">暂无日志</span></div>
        </el-tab-pane>
        <el-tab-pane label="分析结果与报告" name="result">
          <div class="result-toolbar">
            <el-button class="ai-export-button" type="primary" :disabled="aiTask?.ai_analysis_status !== 'done'" @click="exportAIReport">导出 AI 报告</el-button>
          </div>
          <div v-if="aiTask?.ai_analysis" class="ai-result" v-html="renderMarkdown(aiTask.ai_analysis)" />
          <el-empty v-else description="分析完成后显示结果" :image-size="72" />
        </el-tab-pane>
        <el-tab-pane label="自动化渗透" name="pentest">
          <el-alert type="warning" :closable="false" show-icon title="仅对你明确授权的目标执行。任务会在节点上运行 Claude Code，请确认目标范围和节点权限。" />
          <div class="ai-toolbar" style="margin-top:16px">
            <el-tag :type="aiStatusType(aiTask?.ai_pentest_status)">{{ aiStatusLabel(aiTask?.ai_pentest_status) }}</el-tag>
            <el-button v-if="aiTask?.ai_pentest_status !== 'running'" type="danger" :disabled="aiTask?.status !== 'done'" @click="startPentest">启动自动化渗透</el-button>
            <el-button v-else type="danger" plain @click="stopPentest">停止自动化渗透</el-button>
          </div>
          <div class="ai-log-box"><div v-for="(line, i) in (aiTask?.ai_pentest_log || [])" :key="i">{{ line }}</div><span v-if="!aiTask?.ai_pentest_log?.length">暂无日志</span></div>
          <pre v-if="aiTask?.ai_pentest_output" class="ai-result" style="white-space:pre-wrap">{{ aiTask.ai_pentest_output }}</pre>
          <el-empty v-else description="启动后显示 Claude Code 执行结果" :image-size="72" />
        </el-tab-pane>
      </el-tabs>
    </el-dialog>

    <!-- cscan 风格任务详情抽屉 -->
    <el-drawer v-model="detailVisible" :title="'任务详情'" size="720px" @close="closeDetail">
      <template #header>
        <div style="display:flex;align-items:center;justify-content:space-between;width:100%">
          <span style="font-size:16px;font-weight:600">任务详情</span>
        </div>
      </template>
      <template v-if="detailTask">
        <div class="detail-container">

          <!-- 顶部进度条 -->
          <div class="detail-progress-bar">
            <el-progress
              :percentage="detailPercent"
              :status="detailTask.status === 'done' ? 'success' : detailTask.status === 'failed' ? 'exception' : undefined"
              :striped="detailTask.status === 'running'"
              :striped-flow="detailTask.status === 'running'"
              :stroke-width="10"
              :show-text="false"
            />
            <span class="detail-progress-text">{{ detailPercent }}%</span>
          </div>

          <!-- 时间信息 -->
          <div class="detail-time-row">
            <div class="time-item">
              <el-icon style="color:#409eff"><Clock /></el-icon>
              <div>
                <div class="time-label">创建时间</div>
                <div class="time-value">{{ fmtTime(detailTask.created_at) }}</div>
              </div>
            </div>
            <div class="time-item">
              <el-icon style="color:#409eff"><VideoPlay /></el-icon>
              <div>
                <div class="time-label">开始时间</div>
                <div class="time-value">{{ detailTask.started_at ? fmtTime(detailTask.started_at) : '—' }}</div>
              </div>
            </div>
            <div class="time-item">
              <el-icon style="color:#67c23a"><CircleCheck /></el-icon>
              <div>
                <div class="time-label">结束时间</div>
                <div class="time-value">{{ detailTask.done_at ? fmtTime(detailTask.done_at) : '—' }}</div>
              </div>
            </div>
          </div>

          <!-- 扫描目标 -->
          <div class="detail-section">
            <div class="section-title"><el-icon><Aim /></el-icon> 扫描目标</div>
            <div class="targets-list">
              <el-tag v-for="t in detailTask.targets" :key="t" size="default" effect="plain" class="target-tag">{{ t }}</el-tag>
            </div>
          </div>

          <div class="detail-section" v-if="pipelineLogs.length">
            <div class="section-title">任务日志</div>
            <div class="stage-log-box">
              <div v-for="(line, i) in pipelineLogs" :key="`pipeline-${i}`" class="log-line">{{ line }}</div>
            </div>
          </div>

          <!-- 各模块（按 module 分组的折叠面板） -->
          <div class="detail-section" v-if="detailStageGroups.length">
            <el-collapse v-model="expandedModules">
              <el-collapse-item v-for="group in detailStageGroups" :key="group.module" :name="group.module">
                <template #title>
                  <div class="collapse-title">
                    <span class="module-card-icon">{{ group.icon }}</span>
                    <span style="font-weight:600">{{ group.label }}</span>
                    <el-tag v-if="group.status === 'finish'" size="small" type="success" style="margin-left:8px">完成</el-tag>
                    <el-tag v-else-if="group.status === 'process'" size="small" type="primary" style="margin-left:8px">进行中</el-tag>
                    <el-tag v-else-if="group.status === 'error'" size="small" type="danger" style="margin-left:8px">失败</el-tag>
                  </div>
                </template>
                <div class="module-detail-content">
                  <div class="stage-block">
                    <template v-if="group.stages.some(s => stageLogs[s.name]?.length)">
                      <div class="stage-log-box">
                        <template v-for="stage in group.stages" :key="stage.name">
                          <div v-for="(line, i) in stageLogs[stage.name]" :key="stage.name + i" class="log-line">{{ line }}</div>
                        </template>
                      </div>
                    </template>
                    <div v-else-if="group.status === 'process'" class="no-plugin-info" style="color:var(--el-text-color-secondary)">等待日志输出...</div>
                    <div v-else class="no-plugin-info" style="color:var(--el-text-color-secondary)">暂无日志</div>
                  </div>
                </div>
              </el-collapse-item>
            </el-collapse>
          </div>

          <!-- Subtask list (Phase 3 queue mode) -->
          <div class="detail-section" v-if="detailSubtasks.length">
            <div class="section-title">
              <el-icon><Connection /></el-icon> 子任务 ({{ detailSubtasks.length }})
              <el-button size="small" link @click="loadSubtasks(detailTask.id)" style="margin-left:8px">刷新</el-button>
            </div>
            <el-table :data="detailSubtasks" size="small" style="width:100%">
              <el-table-column prop="stage" label="阶段" width="100" />
              <el-table-column prop="status" label="状态" width="90">
                <template #default="{ row }">
                  <el-tag :type="subtaskStatusType(row.status)" size="small">{{ row.status }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="目标数" width="72">
                <template #default="{ row }">{{ row.targets?.length ?? 0 }}</template>
              </el-table-column>
              <el-table-column prop="leased_by" label="节点" min-width="80" show-overflow-tooltip />
              <el-table-column prop="attempt" label="重试" width="60" />
              <el-table-column prop="error_msg" label="错误" min-width="120" show-overflow-tooltip />
            </el-table>
          </div>

          <!-- Dead-letter panel (Phase 4) -->
          <div class="detail-section" v-if="deadLetterItems.length">
            <div class="section-title" style="color:var(--el-color-danger)">
              <el-icon><CircleClose /></el-icon> 死信队列 ({{ deadLetterItems.length }})
              <el-button size="small" link @click="loadDeadLetter(detailTask.id)" style="margin-left:8px">刷新</el-button>
              <el-button size="small" type="danger" @click="retryAllDeadLetter(detailTask.id)" style="margin-left:8px">全部重试</el-button>
            </div>
            <el-table :data="deadLetterItems" size="small" style="width:100%">
              <el-table-column prop="stage" label="阶段" width="100" />
              <el-table-column label="目标数" width="72">
                <template #default="{ row }">{{ row.targets?.length ?? 0 }}</template>
              </el-table-column>
              <el-table-column prop="attempt" label="尝试" width="60" />
              <el-table-column prop="error_msg" label="失败原因" min-width="140" show-overflow-tooltip />
              <el-table-column label="操作" width="80" fixed="right">
                <template #default="{ row }">
                  <el-button size="small" type="primary" link @click="retryDeadLetter(row.id, detailTask.id)">重试</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>

        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { taskApi, projectApi, scanTemplateApi, nodeApi, pluginApi, dictApi, settingsApi, toolDefApi, subscribeTaskProgress, assetApi, type Task, type Project, type ScanTemplate, type Node, type Plugin, type ProgressEvent, type DictEntry, type StagePlugin as StagePluginType, type ToolDef, type Subtask } from '@/api'
import { Loading, Clock, CircleCheck, CircleClose, Setting, Operation, Aim, Connection } from '@element-plus/icons-vue'
import { MODULE_ORDER, MODULE_LABELS, MODULE_ICONS, stageLabel as stageLabelHelper, stageModule, moduleLabel } from '@/constants/modules'
import PluginConfigEditor from '@/components/PluginConfigEditor.vue'

const router = useRouter()
const list = ref<Task[]>([])
const selectedTasks = ref<Task[]>([])
const projects = ref<Project[]>([])
const loading = ref(false)
const saving = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = 20
const filterProjectId = ref<string | undefined>(undefined)
const filterStatus = ref<string | undefined>(undefined)
const searchKeyword = ref('')
const dialogVisible = ref(false)
const progressMap = reactive<Record<string, { percent: number; stage: string; finished: boolean }>>({})
const wsMap: Record<string, WebSocket> = {}

function statusType(s: string) {
  const m: Record<string, string> = { done: 'success', running: 'primary', failed: 'danger', queued: 'warning', dispatched: 'warning' }
  return m[s] ?? 'info'
}
function statusLabel(s: string) {
  const m: Record<string, string> = { done: '完成', running: '运行中', failed: '失败', queued: '排队中', dispatched: '已分发', pending: '等待' }
  return m[s] ?? s
}
function stageLabel(s: string) { return stageLabelHelper(s) }

function tplStages(tpl: ScanTemplate): string[] {
  if (!tpl.modules) return []
  // 用全量 MODULE_ORDER（8 个模块），别再漏 search/brute/dir/sensitive。
  return MODULE_ORDER.filter(mod => tpl.modules[mod]?.some((p: any) => p.enabled))
}

function filterAndFetch() {
  page.value = 1
  fetchList()
}

async function fetchList() {
  loading.value = true
  try {
    const res = await taskApi.list({ project_id: filterProjectId.value, status: filterStatus.value, keyword: searchKeyword.value || undefined, limit: pageSize, skip: (page.value - 1) * pageSize })
    list.value = res.data ?? []; total.value = res.total
    for (const t of list.value) {
      if (['running', 'dispatched'].includes(t.status) && !wsMap[t.id]) subscribeInline(t.id)
    }
  } finally { loading.value = false }
}

function subscribeInline(taskId: string) {
  if (wsMap[taskId]) return
  const ws = subscribeTaskProgress(taskId, ev => {
    if (ev.kind === 'progress' && ev.stage) {
      // 总进度 = (已完成阶段数 + 当前阶段占比) / 总阶段数
      const item = list.value.find(t => t.id === taskId)
      const stages = item?.config?.stages ?? []
      const idx = stages.indexOf(ev.stage)
      const total = stages.length || 1
      const stagePct = ev.percent ?? 0
      const overall = idx >= 0
        ? Math.round(((idx + stagePct / 100) / total) * 100)
        : stagePct
      progressMap[taskId] = { percent: overall, stage: ev.stage, finished: false }
    }
    else if (ev.kind === 'log') {
      const item = list.value.find(t => t.id === taskId)
      if (item && item.status === 'dispatched') item.status = 'running'
    }
    else if (ev.kind === 'status') {
      if (['done', 'failed'].includes(ev.status ?? '')) {
        if (progressMap[taskId]) { progressMap[taskId].finished = true; progressMap[taskId].percent = 100 }
        ws.close(); delete wsMap[taskId]; fetchList()
      } else if (ev.status === 'running') {
        const item = list.value.find(t => t.id === taskId)
        if (item) item.status = 'running'
      }
    }
  })
  wsMap[taskId] = ws
}

// ── 模版 ─────────────────────────────────────────────────────────────────────
const templates = ref<ScanTemplate[]>([])
async function fetchTemplates() {
  try {
    const res = await scanTemplateApi.list({ limit: 200 })
    templates.value = res.data ?? []
  } catch {}
}

// ── 节点 & 插件 ──────────────────────────────────────────────────────────────
const nodes = ref<Node[]>([])
const allPlugins = ref<Plugin[]>([])
const onlineNodes = computed(() => nodes.value.filter(n => {
  const lastSeen = new Date(n.last_seen_at || n.last_seen).getTime()
  return Date.now() - lastSeen < 60000
}))

const moduleOrder = [...MODULE_ORDER]
const allModules = [...MODULE_ORDER]
const allModuleCount = allModules.length
const moduleLabels = MODULE_LABELS
const moduleIcons = MODULE_ICONS
function modulePlugins(mod: string) {
  return allPlugins.value.filter(p => p.module === mod && p.enabled)
}

async function fetchNodes() {
  try { nodes.value = await nodeApi.list() } catch {}
}
async function fetchPlugins() {
  try { allPlugins.value = await pluginApi.list() } catch {}
}
const toolDefs = ref<ToolDef[]>([])
async function fetchToolDefs() {
  try { toolDefs.value = await toolDefApi.list() } catch {}
}
function pluginToolName(pluginName: string): string | null {
  return toolDefs.value.find(t => t.name === pluginName)?.name ?? null
}

const onlinesearchReady = ref(false)
async function checkOnlineSearch() {
  try {
    const res = await settingsApi.getProviders('online_search')
    const keys = res.providers || {}
    const enabled = res.enabled || {}
    onlinesearchReady.value = Object.keys(keys).some(k => keys[k] && keys[k].length > 0 && enabled[k])
  } catch {
    onlinesearchReady.value = false
  }
}

const disabledPlugins = computed(() => {
  const result: Record<string, { disabled: boolean, reason: string, route?: string }> = {}
  
  for (const p of allPlugins.value) {
    if (p.name === 'onlinesearch' && !onlinesearchReady.value) {
      result[p.id] = { disabled: true, reason: '未配置任何 API Key', route: '/tool-config' }
    }
  }
  
  return result
})

const filteredPluginsForTask = computed(() => {
  if (form.value.node_ids.length === 0) return allPlugins.value.filter(p => p.enabled)
  const selectedNodes = onlineNodes.value.filter(n => form.value.node_ids.includes(n.id))
  return allPlugins.value.filter(p => {
    if (!p.enabled) return false
    const toolName = pluginToolName(p.name)
    if (!toolName) return true
    return selectedNodes.every(n => n.installed_tools?.includes(toolName))
  })
})

// dict-select 用：所有字典（按 category+service+kind 客户端过滤）
const allDicts = ref<DictEntry[]>([])
async function fetchDicts() {
  try {
    const r = await dictApi.list()
    allDicts.value = r.data || []
  } catch {}
}
function getDictOptions(param: any): DictEntry[] {
  return allDicts.value.filter(d => {
    if (param.dict_category && d.category !== param.dict_category) return false
    if (param.dict_service && (d.service || '') !== param.dict_service) return false
    if (param.dict_kind && (d.kind || '') !== param.dict_kind) return false
    return true
  })
}
function dictSelectPlaceholder(param: any): string {
  const opts = getDictOptions(param)
  return opts.length ? '选择字典' : '暂无匹配字典，请到「字典管理」添加'
}

// ── 新建任务表单 ─────────────────────────────────────────────────────────────
interface CreateForm {
  project_id: string; name: string; targetsText: string; template_id: string
  node_ids: string[]; configMode: 'template' | 'custom'; ai_analysis_enabled: boolean
}
const customConfig = reactive<Record<string, Record<string, { enabled: boolean; params: Record<string, any> }>>>({})

function initCustomConfig() {
  for (const mod of moduleOrder) {
    if (!customConfig[mod]) customConfig[mod] = {}
    for (const p of modulePlugins(mod)) {
      if (!customConfig[mod][p.id]) {
        const defaults: Record<string, any> = {}
        for (const param of (p.params || [])) {
          defaults[param.key] = param.default ?? (param.type === 'checkbox-group' || param.multiple ? [] : param.type === 'number' ? 0 : param.type === 'switch' ? false : '')
        }
        customConfig[mod][p.id] = { enabled: false, params: defaults }
      }
    }
  }
}

const defaultForm = (): CreateForm => ({
  project_id: projects.value[0]?.id ?? '', name: '', targetsText: '',
  template_id: templates.value[0]?.id ?? '', node_ids: [], configMode: 'template', ai_analysis_enabled: false,
})
const form = ref<CreateForm>(defaultForm())
const selectedTemplate = computed(() => templates.value.find(t => t.id === form.value.template_id) ?? null)

function parseTargets(text: string) { return text.split(/[\n,]+/).map(s => s.trim()).filter(Boolean) }

async function openCreate() {
  form.value = defaultForm()
  initCustomConfig()
  await fetchNodes()
  if (onlineNodes.value.length === 1) {
    form.value.node_ids = [onlineNodes.value[0].id]
  }
  checkOnlineSearch()
  await fetchToolDefs()
  dialogVisible.value = true
}

async function submit() {
  if (!form.value.project_id) { ElMessage.warning('请选择项目'); return }
  if (!form.value.name.trim()) { ElMessage.warning('请填写任务名'); return }
  if (!form.value.node_ids || form.value.node_ids.length === 0) { ElMessage.warning('请至少选择一个扫描节点'); return }
  const targets = parseTargets(form.value.targetsText)
  if (!targets.length) { ElMessage.warning('请输入至少一个目标'); return }

  let stages: string[] = []
  let modules: Record<string, any[]> | undefined
  let templateId = ''
  let templateName = ''

  if (form.value.configMode === 'template') {
    if (!form.value.template_id) { ElMessage.warning('请选择扫描模版'); return }
    const tpl = selectedTemplate.value!
    stages = tplStages(tpl)
    modules = tpl.modules
    templateId = tpl.id
    templateName = tpl.name
  } else {
    modules = {}
    for (const mod of moduleOrder) {
      const list: any[] = []
      const pluginsForMod = filteredPluginsForTask.value.filter(p => p.module === mod && p.enabled)
      for (const p of pluginsForMod) {
        const cfg = customConfig[mod]?.[p.id]
        if (cfg?.enabled) {
          if (disabledPlugins.value[p.id]?.disabled) {
            ElMessage.warning(`插件 [${p.name}] ${disabledPlugins.value[p.id].reason}，请取消勾选或解决问题`)
            return
          }
          list.push({ plugin_id: p.id, name: p.name, enabled: true, params: cfg.params || {} })
          if (!stages.includes(mod)) stages.push(mod)
        }
      }
      if (list.length) modules[mod] = list
    }
    if (!stages.length) { ElMessage.warning('请至少启用一个插件'); return }
  }

  saving.value = true
  try {
    await taskApi.create({
      project_id: form.value.project_id, name: form.value.name, targets,
      template_id: templateId, template_name: templateName,
      stages, modules,
      node_ids: form.value.node_ids,
      ai_analysis_enabled: form.value.ai_analysis_enabled,
    })
    ElMessage.success('任务已创建'); dialogVisible.value = false; fetchList()
  } catch (e: any) { ElMessage.error(e.message) } finally { saving.value = false }
}


async function cancelTask(row: Task) {
  try {
    await taskApi.cancel(row.id)
    ElMessage.success('停止指令已发送')
    if (detailTask.value?.id === row.id) {
      detailWs?.close(); detailWs = null
      const fresh = await taskApi.get(row.id)
      detailTask.value = fresh
      initDetailStages(fresh)
    }
    fetchList()
  } catch (e: any) { ElMessage.error(e.message) }
}

async function rescan(row: Task) {
  try {
    await taskApi.rescan(row.id)
    // Clear cached logs and progress so the new run starts clean.
    delete taskLogsCache.value[row.id]
    delete progressMap[row.id]
    // If the detail drawer is open for this task, reset stage statuses and
    // re-subscribe so the new run's events are reflected correctly.
    if (detailTask.value?.id === row.id) {
      detailWs?.close(); detailWs = null
      const fresh = await taskApi.get(row.id)
      detailTask.value = fresh
      initDetailStages(fresh)
      if (['running', 'dispatched', 'queued'].includes(fresh.status)) {
        detailWs = subscribeTaskProgress(fresh.id, handleDetailEvent)
      }
      // Refresh subtask list and replay log history so stages that already
      // completed before the WebSocket connected show the correct state.
      loadSubtasks(fresh.id)
      loadDeadLetter(fresh.id)
      taskApi.getLogs(fresh.id).then(logs => {
        if (!logs?.length || detailTask.value?.id !== fresh.id) return
        for (const ev of logs) handleDetailEvent(ev)
      }).catch(() => {})
    }
    ElMessage.success('重新扫描任务已创建')
    fetchList()
  } catch (e: any) { ElMessage.error(e.message) }
}

async function confirmRemove(row: Task) {
  try {
    await ElMessageBox.confirm(
      `确认删除任务「${row.name}」？该任务关联的资产数据将一并删除。`,
      '删除任务',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonClass: 'el-button--danger' }
    )
  } catch { return }
  try { await taskApi.remove(row.id, true); ElMessage.success('已删除'); fetchList() } catch (e: any) { ElMessage.error(e.message) }
}
async function batchDeleteTasks() {
  try {
    await ElMessageBox.confirm(
      `确认删除选中的 ${selectedTasks.value.length} 个任务？关联的资产数据将一并删除。`,
      '批量删除',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消', confirmButtonClass: 'el-button--danger' }
    )
  } catch { return }
  try {
    await taskApi.batchRemove(selectedTasks.value.map(t => t.id), true)
    ElMessage.success(`已删除 ${selectedTasks.value.length} 个任务`)
    selectedTasks.value = []
    fetchList()
  } catch (e: any) { ElMessage.error(e.message) }
}

// ── 复制任务 ─────────────────────────────────────────────────────────────────
async function copyTask(row: Task) {
  let fullTask: Task
  try {
    fullTask = await taskApi.get(row.id)
  } catch {
    fullTask = row
  }
  // 只保留仍在线的节点，防止提交后卡在"排队"永远等一个已离线/删除的节点。
  await fetchNodes()
  const onlineIds = new Set(onlineNodes.value.map(n => n.id))
  const filteredNodeIds = (fullTask.node_ids || []).filter(id => onlineIds.has(id))
  const droppedCount = (fullTask.node_ids?.length || 0) - filteredNodeIds.length
  if (droppedCount > 0) {
    ElMessage.warning(`已过滤 ${droppedCount} 个离线节点`)
  }
  form.value = {
    project_id: fullTask.project_id || projects.value[0]?.id || '',
    name: fullTask.name + ' (副本)',
    targetsText: fullTask.targets?.join('\n') || '',
    template_id: fullTask.template_id || '',
    node_ids: filteredNodeIds,
    configMode: fullTask.template_id ? 'template' : 'custom',
    ai_analysis_enabled: fullTask.ai_analysis_enabled ?? false,
  }
  initCustomConfig()
  const hasModules = fullTask.modules && Object.keys(fullTask.modules).length > 0
  if (hasModules) {
    if (fullTask.template_id) form.value.configMode = 'custom'
    for (const mod of moduleOrder) {
      const plugins = fullTask.modules![mod]
      if (!plugins) continue
      for (const sp of plugins) {
        if (!sp.enabled) continue
        const plugin = modulePlugins(mod).find(p => p.id === sp.plugin_id || p.name === sp.name)
        if (plugin && customConfig[mod]?.[plugin.id]) {
          customConfig[mod][plugin.id].enabled = true
          if (sp.params) {
            // 只接受插件 schema 里存在的 key，丢弃老数据里已废弃的参数（如 onlinesearch.query）
            const schemaKeys = new Set((plugin.params || []).map(p => p.key))
            for (const [k, v] of Object.entries(sp.params)) {
              if (schemaKeys.has(k)) customConfig[mod][plugin.id].params[k] = v
            }
          }
        }
      }
    }
  } else if (fullTask.config?.stages?.length) {
    form.value.configMode = 'custom'
    const params = fullTask.config.params || {}
    for (const stage of fullTask.config.stages) {
      const plugins = modulePlugins(stage)
      if (plugins.length > 0) {
        const plugin = plugins[0]
        if (customConfig[stage]?.[plugin.id]) {
          customConfig[stage][plugin.id].enabled = true
          const prefix = stage + '.'
          const schemaKeys = new Set((plugin.params || []).map((p: any) => p.key))
          for (const [k, v] of Object.entries(params)) {
            if (!k.startsWith(prefix)) continue
            const paramKey = k.slice(prefix.length)
            if (!schemaKeys.has(paramKey)) continue
            const paramDef = plugin.params?.find((p: any) => p.key === paramKey)
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
  }
  dialogVisible.value = true
}

// ── cscan 风格任务详情抽屉 ──────────────────────────────────────────────────
interface DetailStage { name: string; status: 'process' | 'finish' | 'error' | 'wait'; statusClass: string; desc: string }

const detailVisible = ref(false)
const detailTask = ref<Task | null>(null)
const detailStages = ref<DetailStage[]>([])
const expandedModules = ref<string[]>([])
let detailWs: WebSocket | null = null
// 按 taskId 缓存日志，关闭详情不清空，再次打开仍能看到历史
const taskLogsCache = ref<Record<string, Record<string, string[]>>>({})
const stageLogs = computed(() => taskLogsCache.value[detailTask.value?.id ?? ''] ?? {})
const pipelineLogs = computed(() => stageLogs.value._pipeline ?? [])

// 按 module 分组的 stage 列表：[{ module, label, icon, stages: [...] }]
const detailStageGroups = computed(() => {
  const groups: { module: string; label: string; icon: string; stages: DetailStage[]; status: DetailStage['status'] }[] = []
  const map = new Map<string, DetailStage[]>()
  for (const s of detailStages.value) {
    const mod = stageModule(s.name)
    if (!map.has(mod)) map.set(mod, [])
    map.get(mod)!.push(s)
  }
  for (const [mod, stages] of map) {
    // 组级状态：任一 error → error；任一 process → process；全部 finish → finish；否则 wait
    let status: DetailStage['status'] = 'wait'
    if (stages.some(s => s.status === 'error')) status = 'error'
    else if (stages.some(s => s.status === 'process')) status = 'process'
    else if (stages.every(s => s.status === 'finish')) status = 'finish'
    groups.push({ module: mod, label: moduleLabel(mod), icon: MODULE_ICONS[mod] || '📦', stages, status })
  }
  return groups
})

const detailTotalStages = computed(() => detailStages.value.length)
const detailFinishedStages = computed(() => detailStages.value.filter(s => s.status === 'finish').length)
const detailPercent = computed(() => {
  if (!detailTotalStages.value) return 0
  if (detailTask.value?.status === 'done') return 100
  const live = progressMap[detailTask.value?.id ?? '']
  if (live) return live.percent
  if (detailTask.value?.progress) return detailTask.value.progress.percent
  return Math.round((detailFinishedStages.value / detailTotalStages.value) * 100)
})
const detailEnabledModules = computed(() => {
  if (!detailTask.value) return 0
  return detailTask.value.config?.stages?.length || 0
})
const detailCurrentStage = computed(() => {
  const running = detailStages.value.find(s => s.status === 'process')
  if (running) return stageLabel(running.name)
  if (detailTask.value?.status === 'done') return '已完成'
  if (detailTask.value?.status === 'failed') return '失败'
  return ''
})

function isModuleEnabled(mod: string): boolean {
  return detailTask.value?.config?.stages?.includes(mod) ?? false
}

function getModulePlugins(mod: string): StagePluginType[] {
  if (!detailTask.value) return []
  const taskModules = detailTask.value.modules
  if (taskModules?.[mod]) {
    return taskModules[mod].filter(p => p.enabled)
  }
  const tpl = templates.value.find(t => t.id === detailTask.value?.template_id)
  if (tpl?.modules?.[mod]) {
    return tpl.modules[mod].filter((p: any) => p.enabled)
  }
  return []
}

function paramLabel(key: string): string {
  const m: Record<string, string> = {
    tool: '扫描工具', ports: '端口范围', rate: '扫描速率', port_threshold: '端口阈值',
    scan_type: '扫描类型', timeout: '超时时间', host_discovery: '跳过主机发现',
    exclude_cdn: '排除CDN/WAF', threads: '线程数', wordlist: '字典',
    severity: '漏洞等级', template_tags: '模板标签', concurrency: '并发数',
  }
  return m[key] ?? key
}

function formatParamValue(val: any): string {
  if (Array.isArray(val)) return val.join(', ') || '—'
  if (typeof val === 'boolean') return val ? '是' : '否'
  if (val === '' || val === null || val === undefined) return '—'
  return String(val)
}

function initDetailStages(task: Task) {
  const tpl = templates.value.find(t => t.id === task.template_id)
  const stages = task.config?.stages?.length ? task.config.stages
    : (tpl ? tplStages(tpl) : [...MODULE_ORDER])

  detailStages.value = stages.map(s => {
    let status: DetailStage['status'] = 'wait'
    if (task.status === 'done') status = 'finish'
    else if (task.status === 'failed') {
      if (task.progress?.stage === s) status = 'error'
      else {
        const idx = stages.indexOf(s)
        const failIdx = stages.indexOf(task.progress?.stage ?? '')
        status = idx < failIdx ? 'finish' : 'wait'
      }
    }
    return { name: s, status, statusClass: `wf-${status}`, desc: '' }
  })
  expandedModules.value = stages.length ? [stageModule(stages[0])] : []
}

function applyLogEvent(ev: ProgressEvent, tid: string) {
  const logStage = ev.stage || '_pipeline'
  if (logStage !== '_pipeline') {
    const si = detailStages.value.findIndex(s => s.name === logStage)
    if (si >= 0 && detailStages.value[si].status === 'wait') {
      detailStages.value[si].status = 'process'
      detailStages.value[si].statusClass = 'wf-process'
      const mod = stageModule(logStage)
      if (!expandedModules.value.includes(mod)) expandedModules.value = [mod]
    }
  }
  if (detailTask.value?.status === 'dispatched') detailTask.value.status = 'running'
  if (!taskLogsCache.value[tid]) taskLogsCache.value[tid] = {}
  if (!taskLogsCache.value[tid][logStage]) taskLogsCache.value[tid][logStage] = []
  const line = ev.log || ev.message || ''
  if (line) taskLogsCache.value[tid][logStage].push(line)
}

// Unified event handler used by both the WebSocket subscription and getLogs replay.
// Progress events drive stage status changes; log events populate the log cache.
// The backend persists pct=0 and pct=100 boundary events so replay can fully
// reconstruct stage states without any "mark all previous as finish" inference
// (which was the source of stale-event bugs).
function handleDetailEvent(ev: ProgressEvent) {
  if (!detailTask.value) return
  const tid = detailTask.value.id

  if (ev.kind === 'progress' && ev.stage) {
    const idx = detailStages.value.findIndex(s => s.name === ev.stage)
    if (idx < 0) return
    const pct = ev.percent ?? 0
    if (pct >= 100) {
      detailStages.value[idx].status = 'finish'
      detailStages.value[idx].statusClass = 'wf-finish'
    } else {
      // Only advance from wait/finish → process; never downgrade an already-finished stage.
      if (detailStages.value[idx].status !== 'finish') {
        detailStages.value[idx].status = 'process'
        detailStages.value[idx].statusClass = 'wf-process'
        const mod = stageModule(ev.stage)
        if (!expandedModules.value.includes(mod)) expandedModules.value = [mod]
      }
    }
    if (detailTask.value.status === 'dispatched') detailTask.value.status = 'running'
  } else if (ev.kind === 'log') {
    applyLogEvent(ev, tid)
  } else if (ev.kind === 'status') {
    if (ev.status === 'done') {
      detailStages.value.forEach(s => { if (s.status !== 'error') { s.status = 'finish'; s.statusClass = 'wf-finish' } })
      detailTask.value.status = 'done'
      fetchList()
    } else if (ev.status === 'failed') {
      detailTask.value.status = 'failed'
      fetchList()
    } else if (ev.status === 'running') {
      if (detailTask.value.status === 'dispatched' || detailTask.value.status === 'queued') {
        detailTask.value.status = 'running'
      }
    }
  }
}

function openDetail(task: Task) {
  taskApi.get(task.id).then(full => {
    detailTask.value = full
    initDetailStages(full)
    detailVisible.value = true
    loadSubtasks(full.id)
    loadDeadLetter(full.id)

    // Always reload log history when opening detail so stage states are current.
    taskApi.getLogs(full.id).then(logs => {
      if (!logs?.length || detailTask.value?.id !== full.id) return
      for (const ev of logs) handleDetailEvent(ev)
    }).catch(() => {})

    if (['running', 'dispatched'].includes(full.status)) {
      detailWs = subscribeTaskProgress(full.id, handleDetailEvent)
    }
  }).catch(() => {
    ElMessage.error('获取任务详情失败')
  })
}

function closeDetail() { detailWs?.close(); detailWs = null; detailSubtasks.value = []; deadLetterItems.value = [] }

// ── Subtasks (Phase 3 queue mode) ───────────────────────────────────────────
const detailSubtasks = ref<Subtask[]>([])

function loadSubtasks(taskId: string) {
  taskApi.getSubtasks(taskId).then(subs => {
    detailSubtasks.value = subs
  }).catch(() => {})
}

// ── Dead-letter (Phase 4) ────────────────────────────────────────────────────
const deadLetterItems = ref<Subtask[]>([])

function loadDeadLetter(taskId: string) {
  taskApi.getDeadLetter(taskId).then(items => {
    deadLetterItems.value = items
  }).catch(() => {})
}

async function retryDeadLetter(subtaskId: string, taskId: string) {
  try {
    await taskApi.retryDeadLetter(subtaskId)
    ElMessage.success('已重新入队')
    loadDeadLetter(taskId)
  } catch (e: any) {
    ElMessage.error(e?.message ?? '操作失败')
  }
}

async function retryAllDeadLetter(taskId: string) {
  const items = deadLetterItems.value
  if (!items.length) return
  await Promise.allSettled(items.map(i => taskApi.retryDeadLetter(i.id)))
  ElMessage.success(`已重新入队 ${items.length} 个`)
  loadDeadLetter(taskId)
}

function subtaskStatusType(s: string) {
  const m: Record<string, string> = { done: 'success', leased: 'primary', failed: 'danger', dead_letter: 'danger', pending: 'warning' }
  return m[s] ?? 'info'
}
function viewAssets(task: Task) { router.push(`/assets?task_id=${task.id}`) }

const aiDialogVisible = ref(false)
const aiLoading = ref(false)
const aiTask = ref<Task | null>(null)
const aiActiveTab = ref('process')
let aiPollTimer: ReturnType<typeof setInterval> | null = null
function openAI(row: Task) { aiTask.value = row; aiActiveTab.value = 'process'; aiDialogVisible.value = true; if (row.ai_analysis_status === 'running') startAIPoll(row.id) }
function aiStatusLabel(s?: string) { return ({ running: '分析中', done: '已完成', failed: '失败', cancelled: '已停止' } as Record<string, string>)[s || ''] || '未启动' }
function aiStatusType(s?: string) { return s === 'done' ? 'success' : s === 'failed' ? 'danger' : s === 'running' ? 'warning' : 'info' }
function renderMarkdown(md: string) {
  const escape = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  const inline = (s: string) => s.replace(/`([^`]+)`/g, '<code>$1</code>').replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  const lines = escape(md).split(/\r?\n/)
  const out: string[] = []; let list = false
  const closeList = () => { if (list) { out.push('</ul>'); list = false } }
  for (const line of lines) {
    if (/^\s*[-*_]{3,}\s*$/.test(line)) { closeList(); out.push('<hr>'); continue }
    const heading = line.match(/^\s*(#{1,4})\s+(.+)$/)
    if (heading) { closeList(); out.push(`<h${heading[1].length}>${inline(heading[2])}</h${heading[1].length}>`); continue }
    const bullet = line.match(/^\s*[-*]\s+(.+)$/)
    if (bullet) { if (!list) { out.push('<ul>'); list = true }; out.push(`<li>${inline(bullet[1])}</li>`); continue }
    closeList()
    if (!line.trim()) continue
    out.push(`<p>${inline(line)}</p>`)
  }
  closeList(); return out.join('')
}
function startAIPoll(id: string) { if (aiPollTimer) clearInterval(aiPollTimer); aiPollTimer = setInterval(async () => { try { const fresh = await taskApi.get(id); if (aiTask.value?.id === id) Object.assign(aiTask.value, fresh); if (['done','failed','cancelled'].includes(fresh.ai_analysis_status || '')) { clearInterval(aiPollTimer!); aiPollTimer = null } } catch {} }, 1500) }
async function startPentest() { if (!aiTask.value) return; try { const updated = await taskApi.startAIPentest(aiTask.value.id); Object.assign(aiTask.value, updated); startAIPoll(aiTask.value.id) } catch (e: any) { ElMessage.error(e.message || '启动失败') } }
async function stopPentest() { if (!aiTask.value) return; try { await taskApi.stopAIPentest(aiTask.value.id); Object.assign(aiTask.value, await taskApi.get(aiTask.value.id)) } catch (e: any) { ElMessage.error(e.message || '停止失败') } }
async function startAI() { if (!aiTask.value) return; aiLoading.value = true; try { const updated = await taskApi.analyze(aiTask.value.id); Object.assign(aiTask.value, updated); startAIPoll(aiTask.value.id) } catch (e: any) { ElMessage.error(e.message || '启动失败') } finally { aiLoading.value = false } }
async function stopAI() { if (!aiTask.value) return; try { await taskApi.stopAnalyze(aiTask.value.id); const fresh = await taskApi.get(aiTask.value.id); Object.assign(aiTask.value, fresh); if (aiPollTimer) { clearInterval(aiPollTimer); aiPollTimer = null } } catch (e: any) { ElMessage.error(e.message || '停止失败') }
}
async function exportAIReport() {
  if (!aiTask.value || aiTask.value.ai_analysis_status !== 'done') return
  try {
    const blob = await assetApi.exportAIReport(aiTask.value.id)
    const url = URL.createObjectURL(blob); const a = document.createElement('a'); a.href = url
    a.download = `nscan_task_${aiTask.value.id.slice(0, 8)}_${new Date().toISOString().slice(0, 10)}.xlsx`; a.click(); URL.revokeObjectURL(url)
  } catch (e: any) { ElMessage.error('导出失败：' + e.message) }
}
function fmtTime(iso: string) { if (!iso) return '—'; return new Date(iso).toLocaleString('zh-CN', { hour12: false }) }

onMounted(async () => {
  projects.value = (await projectApi.list({ limit: 200 }).catch(() => ({ data: [] }))).data ?? []
  fetchTemplates()
  await Promise.all([fetchPlugins(), fetchDicts()])
  fetchList()
})
onUnmounted(() => {
  for (const ws of Object.values(wsMap)) ws.close()
  if (detailWs) { detailWs.close(); detailWs = null }
  if (aiPollTimer) clearInterval(aiPollTimer)
})
</script>

<style scoped>
.tpl-preview {
  background: #f0f4ff; border-radius: 6px; padding: 8px 10px;
  display: flex; align-items: center; flex-wrap: wrap; gap: 6px;
  margin-bottom: 4px;
}
.custom-plugins {
  border: 1px solid var(--el-border-color-lighter); border-radius: 8px;
  overflow: hidden;
}
.module-section { border-bottom: 1px solid var(--el-border-color-lighter); }
.module-section:last-child { border-bottom: none; }
.module-header {
  background: #f5f7fa; padding: 8px 14px;
  display: flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 600;
}
.module-icon { font-size: 16px; }
.module-name { min-width: 80px; }
.plugin-row {
  padding: 8px 14px 8px 42px;
  display: flex; align-items: center; gap: 10px; font-size: 13px;
  border-top: 1px solid var(--el-border-color-extra-light);
}
.plugin-name-label { font-weight: 500; min-width: 90px; }
.plugin-ver { color: var(--el-text-color-secondary); font-size: 11px; min-width: 40px; }
.plugin-desc-text { color: var(--el-text-color-secondary); font-size: 12px; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.plugin-card { border-top: 1px solid var(--el-border-color-extra-light); }
.plugin-params {
  margin: 6px 14px 10px 42px;
  padding: 14px 18px 4px;
  background: var(--el-fill-color-lighter);
  border-radius: 8px;
  border: 1px solid var(--el-border-color-extra-light);
}
.plugin-params :deep(.el-form-item) { margin-bottom: 14px; }
.plugin-params :deep(.el-form-item__label) { font-size: 13px; color: var(--el-text-color-regular); padding-bottom: 4px; }
.param-help { font-size: 11px; color: var(--el-text-color-placeholder); margin-top: 4px; line-height: 1.4; }

/* ── cscan 风格任务详情 ──────────────────────────────────────────── */
.detail-container {
  display: flex; flex-direction: column; gap: 20px; padding: 0 4px;
}

.detail-progress-bar {
  position: relative;
}
.detail-progress-text {
  position: absolute; right: 0; top: 50%; transform: translateY(-50%);
  font-size: 13px; color: var(--el-text-color-secondary); font-weight: 500;
}

.detail-time-row {
  display: flex; gap: 0; background: #f5f7fa; border-radius: 10px; padding: 16px 20px;
}
.time-item {
  flex: 1; display: flex; align-items: center; gap: 10px;
}
.time-item .el-icon { font-size: 22px; }
.time-label { font-size: 12px; color: var(--el-text-color-secondary); }
.time-value { font-size: 13px; color: #1d2129; font-weight: 500; margin-top: 2px; }

.detail-section {
  margin-top: 4px;
}
.section-title {
  font-size: 14px; font-weight: 600; color: #1d2129;
  display: flex; align-items: center; gap: 6px; margin-bottom: 14px;
}

/* 工作流 */
.workflow-container {
  background: #f5f7fa; border-radius: 10px; padding: 20px 24px;
}
.workflow-flow {
  display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 4px;
}
.workflow-node {
  display: inline-flex; align-items: center;
  padding: 8px 18px; border-radius: 20px;
  font-size: 13px; font-weight: 500;
  background: #fff; border: 1px solid #e5e6eb; color: var(--el-text-color-secondary);
  transition: all .2s;
}
.workflow-node.wf-finish { border-color: #67c23a; color: #67c23a; background: #f0f9eb; }
.workflow-node.wf-process { border-color: #409eff; color: #409eff; background: #ecf5ff; }
.workflow-node.wf-error { border-color: #f56c6c; color: #f56c6c; background: #fef0f0; }
.workflow-arrow { display: flex; align-items: center; padding: 0 2px; }

/* 扫描策略 */
.strategy-card {
  background: #fff; border: 1px solid var(--el-border-color-lighter); border-radius: 10px;
  padding: 16px 20px; margin-bottom: 14px;
}
.strategy-header {
  display: flex; align-items: center; gap: 8px; margin-bottom: 14px;
}
.strategy-stats {
  display: flex; gap: 0;
}
.stat-item { flex: 1; }
.stat-label { font-size: 12px; color: var(--el-text-color-secondary); margin-bottom: 4px; }
.stat-value { font-size: 18px; font-weight: 700; color: #1d2129; }
.stat-value.primary { color: #409eff; }

/* 模块卡片网格 */
.module-grid {
  display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px;
}
.module-card {
  border: 1px solid var(--el-border-color-lighter); border-radius: 10px;
  padding: 14px 16px; background: #fff; transition: all .2s;
}
.module-card.enabled { border-color: #b7eb8f; background: #f6ffed; }
.module-card-header {
  display: flex; align-items: center; gap: 6px; margin-bottom: 6px;
}
.module-card-icon { font-size: 16px; }
.module-card-name { font-weight: 600; font-size: 13px; flex: 1; }
.module-card-plugins { display: flex; flex-wrap: wrap; gap: 4px; }
.plugin-chip {
  display: inline-block; padding: 2px 8px; border-radius: 4px;
  background: #e6f7e6; color: #389e0d; font-size: 11px;
}

/* 折叠面板 */
.collapse-title {
  display: flex; align-items: center; gap: 8px;
}
.module-detail-content { padding: 4px 0; }
.plugin-detail-card {
  border: 1px solid var(--el-border-color-extra-light); border-radius: 8px;
  padding: 12px 16px; margin-bottom: 10px; background: var(--el-fill-color-light);
}
.plugin-detail-header { margin-bottom: 10px; font-size: 13px; }
.plugin-detail-params { font-size: 12px; }
.no-plugin-info { font-size: 12px; color: var(--el-text-color-secondary); }
.stage-log-box { background: #0d1117; border-radius: 6px; padding: 12px; max-height: 400px; overflow-y: auto; font-family: monospace; font-size: 12px; line-height: 1.6; }
.log-line { color: #c9d1d9; white-space: pre-wrap; word-break: break-all; }
.stage-block { margin-bottom: 12px; }
.stage-block:last-child { margin-bottom: 0; }
.stage-block-header {
  display: flex; align-items: center; gap: 8px;
  padding: 6px 10px; margin-bottom: 6px;
  background: var(--el-fill-color-light); border-radius: 6px;
}
.stage-block-title { font-weight: 500; font-size: 13px; }

/* 扫描目标 */
.targets-list { display: flex; flex-wrap: wrap; gap: 6px; }
.target-tag { font-family: monospace; font-size: 12px; }
.ai-result { max-height: 48vh; overflow: auto; padding: 18px 22px; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; background: var(--el-fill-color-lighter); color: var(--el-text-color-primary); line-height: 1.75; word-break: break-word; }
.ai-result :deep(h1), .ai-result :deep(h2), .ai-result :deep(h3), .ai-result :deep(h4) { margin: 18px 0 8px; line-height: 1.4; color: var(--el-text-color-primary); }
.ai-result :deep(h1) { font-size: 20px; }.ai-result :deep(h2) { font-size: 18px; }.ai-result :deep(h3) { font-size: 16px; }.ai-result :deep(h4) { font-size: 14px; }
.ai-result :deep(p) { margin: 7px 0; }.ai-result :deep(ul) { margin: 8px 0; padding-left: 24px; }.ai-result :deep(li) { margin: 4px 0; }
.ai-result :deep(code) { padding: 2px 5px; border-radius: 4px; background: var(--el-fill-color); color: var(--el-color-primary); font: 0.9em monospace; }
.ai-result :deep(hr) { border: 0; border-top: 1px solid var(--el-border-color); margin: 16px 0; }
.ai-toolbar { display:flex; align-items:center; gap:12px; margin-bottom:16px; }
.result-toolbar { display:flex; justify-content:flex-end; margin-bottom:12px; }
.ai-export-button:not(:disabled) { color:#fff !important; background:#409eff !important; border-color:#409eff !important; }
.ai-tabs { min-height: 390px; }
.ai-log-title { font-size:13px; font-weight:600; margin:10px 0 8px; }
.ai-log-box { min-height:90px; max-height:180px; overflow:auto; padding:12px; background:#0d1117; color:#c9d1d9; border-radius:6px; font:12px/1.7 monospace; }
</style>
