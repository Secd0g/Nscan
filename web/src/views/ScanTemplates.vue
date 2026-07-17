<template>
  <div>
    <div v-if="!showEditor" class="page-card">
    <div class="page-header">
      <h2 class="page-title">扫描模版</h2>
      <div class="header-actions">
        <el-input v-model="search" placeholder="搜索模版名称" clearable style="width:200px">
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-popconfirm title="批量删除选中模版？" @confirm="batchDelete">
          <template #reference>
            <el-button type="danger" plain :disabled="!selected.length">
              <el-icon><Delete /></el-icon>批量删除({{ selected.length }})
            </el-button>
          </template>
        </el-popconfirm>
        <el-button type="primary" @click="openCreate">
          <el-icon><Plus /></el-icon>新建模版
        </el-button>
      </div>
    </div>
    <el-table v-loading="loading" :data="filteredTemplates" style="width:100%" @selection-change="(rows: ScanTemplate[]) => selected = rows">
      <el-table-column type="selection" width="42" />
      <el-table-column prop="name" label="模版名称" width="160" />
      <el-table-column prop="description" label="描述" show-overflow-tooltip />
      <el-table-column label="扫描模块" width="360">
        <template #default="{ row }">
          <template v-for="(plugins, mod) in (row.modules || {})" :key="String(mod)">
            <el-tag v-for="p in plugins.filter((x: StagePlugin) => x.enabled)" :key="p.plugin_id" size="small" style="margin-right:4px;margin-bottom:2px">
              {{ moduleLabel(String(mod)) }} / {{ p.name }}
            </el-tag>
          </template>
        </template>
      </el-table-column>
      <el-table-column label="更新时间" width="170">
        <template #default="{ row }">{{ fmtTime(row.updated_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="120" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="openEdit(row)">编辑</el-button>
          <el-divider direction="vertical" />
          <el-popconfirm title="���认删除该模版？" @confirm="deleteTemplate(row.id)">
            <template #reference>
              <el-button type="danger" link size="small">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>
    </div>

    <!-- 模版编辑器 -->
    <div v-else class="page-card">
      <div class="page-header">
        <div class="editor-title">
          <el-button link @click="closeEditor"><el-icon><ArrowLeft /></el-icon>返回</el-button>
          <h2 class="page-title">{{ editing ? `编辑模版 — ${editing.name}` : '新建扫描模版' }}</h2>
        </div>
      </div>
      <el-form :model="form" label-position="top">

        <!-- 基本信息 -->
        <el-card shadow="never" style="margin-bottom:16px">
          <template #header><span class="card-title">基本信息</span></template>
          <el-row :gutter="16">
            <el-col :span="8">
              <el-form-item label="模版名称" required>
                <el-input v-model="form.name" placeholder="如：快速扫描、完整渗透" />
              </el-form-item>
            </el-col>
            <el-col :span="16">
              <el-form-item label="描述">
                <el-input v-model="form.description" placeholder="简要说明该模版的用途" />
              </el-form-item>
            </el-col>
          </el-row>
        </el-card>

        <!-- 每个模块的插件卡片 -->
        <el-card
          v-for="mod in moduleOrder"
          :key="mod"
          shadow="never"
          :class="{ 'stage-active': hasEnabledPlugin(mod) }"
          style="margin-bottom:16px"
        >
          <template #header>
            <div class="stage-header">
              <span class="card-title">{{ moduleIcon(mod) }} {{ moduleLabel(mod) }}</span>
              <el-tag size="small" type="info" style="margin-left:8px">{{ getModulePlugins(mod).length }} 个插件可用</el-tag>
            </div>
          </template>

          <!-- 该模块下的插件列表 -->
          <div v-for="plugin in getModulePlugins(mod)" :key="plugin.id" class="plugin-block">
            <div class="plugin-header">
              <el-switch v-model="getFormPlugin(mod, plugin).enabled" />
              <span class="plugin-name">{{ plugin.name }}</span>
              <el-tag size="small" type="info">{{ plugin.version }}</el-tag>
              <span class="plugin-desc">{{ plugin.description }}</span>
            </div>

            <!-- 动态参数表单 -->
            <div v-show="getFormPlugin(mod, plugin).enabled" class="plugin-params">
              <el-row :gutter="16">
                <el-col v-for="param in plugin.params" :key="param.key" :span="param.span || 12">
                  <el-form-item :label="param.label">
                    <!-- string -->
                    <el-input
                      v-if="param.type === 'string'"
                      v-model="getFormPlugin(mod, plugin).params[param.key]"
                      :placeholder="param.placeholder"
                      style="font-family:monospace"
                    />
                    <!-- number -->
                    <el-input-number
                      v-else-if="param.type === 'number'"
                      v-model="getFormPlugin(mod, plugin).params[param.key]"
                      :min="param.min" :max="param.max" :step="param.step || 1"
                      style="width:100%" controls-position="right"
                    />
                    <!-- select -->
                    <el-select
                      v-else-if="param.type === 'select'"
                      v-model="getFormPlugin(mod, plugin).params[param.key]"
                      :multiple="!!param.multiple" collapse-tags collapse-tags-tooltip
                      style="width:100%"
                    >
                      <el-option v-for="opt in param.options" :key="String(opt.value)" :value="opt.value" :label="opt.label" />
                    </el-select>
                    <!-- dict-select: 从字典管理按 category+service+kind 过滤 -->
                    <el-select
                      v-else-if="param.type === 'dict-select'"
                      v-model="getFormPlugin(mod, plugin).params[param.key]"
                      :multiple="!!param.multiple" collapse-tags collapse-tags-tooltip
                      style="width:100%"
                      :placeholder="dictSelectPlaceholder(param)"
                    >
                      <el-option
                        v-for="d in getDictOptions(param)" :key="d.id"
                        :value="d.id"
                        :label="`${d.name}${d.builtin ? ' (内置)' : ''} · ${d.count}行`"
                      />
                    </el-select>
                    <!-- checkbox-group -->
                    <el-checkbox-group
                      v-else-if="param.type === 'checkbox-group'"
                      v-model="getFormPlugin(mod, plugin).params[param.key]"
                    >
                      <el-checkbox v-for="opt in param.options" :key="String(opt.value)" :value="opt.value">{{ opt.label }}</el-checkbox>
                    </el-checkbox-group>
                    <!-- textarea -->
                    <el-input
                      v-else-if="param.type === 'textarea'"
                      v-model="getFormPlugin(mod, plugin).params[param.key]"
                      type="textarea" :rows="3"
                      :placeholder="param.placeholder"
                      style="font-family:monospace;font-size:12px"
                    />
                    <!-- switch -->
                    <el-switch
                      v-else-if="param.type === 'switch'"
                      v-model="getFormPlugin(mod, plugin).params[param.key]"
                    />
                    <div v-if="param.help" class="param-help">{{ param.help }}</div>
                  </el-form-item>
                </el-col>
              </el-row>
            </div>
          </div>

          <el-empty v-if="getModulePlugins(mod).length === 0" description="暂无可用插件" :image-size="40" />
        </el-card>

        <div class="editor-footer">
          <el-button @click="closeEditor">取消</el-button>
          <el-button v-if="editing" :loading="saving" @click="saveAs">另存为新模版</el-button>
          <el-button type="primary" :loading="saving" @click="save">保存模版</el-button>
        </div>
      </el-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, reactive } from 'vue'
import { ElMessage } from 'element-plus'
import { scanTemplateApi, pluginApi, dictApi } from '@/api'
import type { ScanTemplate, Plugin, StagePlugin, DictEntry } from '@/api'
import { MODULE_ORDER, moduleLabel, moduleIcon } from '@/constants/modules'

const moduleOrder = [...MODULE_ORDER]

const loading = ref(false)
const search = ref('')
const showEditor = ref(false)
const selected = ref<ScanTemplate[]>([])
const editing = ref<ScanTemplate | null>(null)
const saving = ref(false)

// 所有已注册插件
const allPlugins = ref<Plugin[]>([])
// dict-select 用：所有字典（按 category+service+kind 客户端过滤）
const allDicts = ref<DictEntry[]>([])

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
  return opts.length ? '选择字典（不选则用同协议全部启用的内置字典）' : '暂无匹配字典，请到「字典管理」添加'
}

function getModulePlugins(mod: string) {
  return allPlugins.value.filter(p => p.module === mod && p.enabled)
}

// ── 表单状态 ─────────────────────────────────────────────────────────────────

interface FormState {
  name: string
  description: string
  modules: Record<string, Record<string, { enabled: boolean; params: Record<string, any> }>>
}

const form = reactive<FormState>({
  name: '',
  description: '',
  modules: {},
})

function getFormPlugin(mod: string, plugin: Plugin) {
  if (!form.modules[mod]) form.modules[mod] = {}
  if (!form.modules[mod][plugin.id]) {
    const defaults: Record<string, any> = {}
    for (const p of plugin.params) {
      defaults[p.key] = p.default ?? (p.type === 'checkbox-group' || p.multiple ? [] : p.type === 'number' ? 0 : p.type === 'switch' ? false : '')
    }
    form.modules[mod][plugin.id] = { enabled: false, params: defaults }
  }
  return form.modules[mod][plugin.id]
}

function hasEnabledPlugin(mod: string): boolean {
  const m = form.modules[mod]
  if (!m) return false
  return Object.values(m).some(p => p.enabled)
}

// ── 列表 ─────────────────────────────────────────────────────────────────────

const templates = ref<ScanTemplate[]>([])

async function fetchList() {
  loading.value = true
  try {
    const res = await scanTemplateApi.list({ limit: 200 })
    templates.value = res.data ?? []
  } catch (e: any) { ElMessage.error(e.message) } finally { loading.value = false }
}

async function fetchPlugins() {
  try {
    allPlugins.value = await pluginApi.list()
  } catch (e: any) { ElMessage.error('加载插件列表失败: ' + e.message) }
}

async function fetchDicts() {
  try {
    const r = await dictApi.list()
    allDicts.value = r.data || []
  } catch {}
}

onMounted(async () => {
  await Promise.all([fetchList(), fetchPlugins(), fetchDicts()])
})

const filteredTemplates = computed(() => {
  const q = search.value.toLowerCase()
  return q ? templates.value.filter(t => t.name.toLowerCase().includes(q) || t.description.toLowerCase().includes(q)) : templates.value
})

// ── 编辑器 ───────────────────────────────────────────────────────────────────

function resetForm() {
  form.name = ''
  form.description = ''
  form.modules = {}
  // 初始化所有插件的默认状态
  for (const plugin of allPlugins.value) {
    getFormPlugin(plugin.module, plugin)
  }
}

function openCreate() {
  editing.value = null
  resetForm()
  showEditor.value = true
}

function closeEditor() { showEditor.value = false }

function openEdit(tpl: ScanTemplate) {
  editing.value = tpl
  form.name = tpl.name
  form.description = tpl.description
  form.modules = {}

  // 先初始化所有插件默认值
  for (const plugin of allPlugins.value) {
    getFormPlugin(plugin.module, plugin)
  }

  // 再覆盖模版中已保存的配置。只接受插件 schema 里存在的 key —— 老数据里被移除的参数
  // （比如旧版 onlinesearch.query）会被自然丢弃，避免用户看不见但仍生效的隐式覆盖。
  if (tpl.modules) {
    for (const [mod, stagePlugins] of Object.entries(tpl.modules)) {
      for (const sp of stagePlugins) {
        const plugin = allPlugins.value.find(p => p.id === sp.plugin_id || p.name === sp.name)
        if (plugin && form.modules[mod]?.[plugin.id]) {
          form.modules[mod][plugin.id].enabled = sp.enabled
          const schemaKeys = new Set((plugin.params || []).map(p => p.key))
          for (const [k, v] of Object.entries(sp.params || {})) {
            if (schemaKeys.has(k)) form.modules[mod][plugin.id].params[k] = v
          }
        }
      }
    }
  }
  showEditor.value = true
}

// buildPayload 从当前表单构造 API payload；返回 null 表示校验未通过（已提示）。
function buildPayload(): { name: string; description: string; modules: Record<string, StagePlugin[]> } | null {
  if (!form.name.trim()) { ElMessage.warning('请填写模版名称'); return null }

  const modules: Record<string, StagePlugin[]> = {}
  let hasAny = false
  for (const mod of moduleOrder) {
    const m = form.modules[mod]
    if (!m) continue
    const list: StagePlugin[] = []
    for (const [pluginId, state] of Object.entries(m)) {
      if (!state.enabled) continue
      const plugin = allPlugins.value.find(p => p.id === pluginId)
      list.push({
        plugin_id: pluginId,
        name: plugin?.name ?? '',
        enabled: true,
        params: { ...state.params },
      })
      hasAny = true
    }
    if (list.length) modules[mod] = list
  }
  if (!hasAny) { ElMessage.warning('请至少启用一个插件'); return null }
  return { name: form.name, description: form.description, modules }
}

async function save() {
  const payload = buildPayload()
  if (!payload) return
  saving.value = true
  try {
    if (editing.value) {
      await scanTemplateApi.update(editing.value.id, payload)
    } else {
      await scanTemplateApi.create(payload)
    }
    showEditor.value = false; ElMessage.success('已保存'); fetchList()
  } catch (e: any) { ElMessage.error(e.message) } finally { saving.value = false }
}

// saveAs 在编辑模式下把当前编辑器状态另存为新模版，原模版不改动。
async function saveAs() {
  const payload = buildPayload()
  if (!payload) return
  // 若名字未改，追加"(副本)"避免和原模版重名。
  if (editing.value && payload.name === editing.value.name) {
    payload.name = payload.name + ' (副本)'
  }
  saving.value = true
  try {
    await scanTemplateApi.create(payload)
    showEditor.value = false
    ElMessage.success('已另存为新模版')
    fetchList()
  } catch (e: any) { ElMessage.error(e.message) } finally { saving.value = false }
}

async function deleteTemplate(id: string) {
  try {
    await scanTemplateApi.remove(id); ElMessage.success('已删除'); fetchList()
  } catch (e: any) { ElMessage.error(e.message) }
}
async function batchDelete() {
  const ids = selected.value.map(t => t.id)
  try {
    await scanTemplateApi.batchRemove(ids)
    ElMessage.success(`已删除 ${ids.length} 个模版`); selected.value = []; fetchList()
  } catch (e: any) { ElMessage.error(e.message) }
}
function fmtTime(iso: string) { return new Date(iso).toLocaleString('zh-CN', { hour12: false }) }
</script>

<style scoped>
.editor-title { display: flex; align-items: center; gap: 6px; }
.stage-header { display: flex; align-items: center; }
.card-title { font-weight: 600; font-size: 14px; }
.stage-active { border-color: #409EFF !important; }
.stage-active :deep(.el-card__header) { background: #ecf5ff; }
.editor-footer { display: flex; justify-content: flex-end; gap: 12px; padding: 20px 0 4px; }

.plugin-block { padding: 14px 0; border-bottom: 1px solid var(--el-border-color-lighter); }
.plugin-block:last-child { border-bottom: none; }
.plugin-header { display: flex; align-items: center; gap: 10px; }
.plugin-name { font-weight: 600; font-size: 14px; }
.plugin-desc { color: var(--el-text-color-secondary); font-size: 12px; margin-left: auto; max-width: 60%; text-align: right; line-height: 1.4; }
.plugin-params {
  margin-top: 14px; margin-left: 44px;
  padding: 14px 18px 4px;
  background: var(--el-fill-color-lighter);
  border-radius: 8px;
  border: 1px solid var(--el-border-color-extra-light);
}
.plugin-params :deep(.el-form-item) { margin-bottom: 14px; }
.plugin-params :deep(.el-form-item__label) { font-size: 13px; color: var(--el-text-color-regular); padding-bottom: 4px; }
.param-help { font-size: 11px; color: var(--el-text-color-placeholder); margin-top: 4px; line-height: 1.4; }
</style>
