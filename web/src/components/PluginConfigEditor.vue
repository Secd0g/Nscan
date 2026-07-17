<!--
  PluginConfigEditor — 「自定义配置」模式下的插件+参数编辑器。
  Tasks.vue 和 Scheduled.vue 共享。

  modelValue 是 Record<module, Record<pluginId, {enabled, params}>>；
  组件直接原地修改 modelValue 的属性（modelValue 应为 reactive/ref 值）。
  为了避免与 Vue 的 v-model 顶层替换语义冲突，组件不 emit update:modelValue，
  父组件只需保证传入的对象本身是响应式的即可。

  组件负责：
    - 按 moduleOrder 分组渲染
    - 每个插件一个开关 + 展开的参数表单
    - 根据 plugin.params[i].type 渲染不同的输入控件
    - dict-select 从 props.dicts 里按 category/service/kind 过滤候选

  组件不负责：
    - 拉取插件/字典（由父组件传入）
    - 校验/提交
-->
<template>
  <div class="plugin-config-editor">
    <div v-for="mod in moduleOrder" :key="mod" class="module-section">
      <div class="module-header">
        <span class="module-icon">{{ moduleIcon(mod) }}</span>
        <span class="module-name">{{ moduleLabel(mod) }}</span>
        <el-tag size="small" type="info">{{ pluginsOf(mod).length }} 个插件</el-tag>
      </div>
      <div v-for="plugin in pluginsOf(mod)" :key="plugin.id" class="plugin-card">
        <div class="plugin-row">
          <el-switch v-model="entryOf(mod, plugin.id).enabled" size="small" :disabled="props.disabledPlugins?.[plugin.id]?.disabled" />
          <span class="plugin-name-label">{{ plugin.name }}</span>
          <span class="plugin-ver">{{ plugin.version }}</span>
          <span class="plugin-desc-text">{{ plugin.description }}</span>
          <div v-if="props.disabledPlugins?.[plugin.id]?.disabled" style="margin-left:auto; display:flex; align-items:center; gap:8px">
            <el-tag size="small" type="danger" effect="plain">{{ props.disabledPlugins[plugin.id].reason }}</el-tag>
            <el-button v-if="props.disabledPlugins[plugin.id].route" size="small" type="primary" link @click="$router.push(props.disabledPlugins[plugin.id].route!)">去配置</el-button>
          </div>
        </div>
        <div v-if="entryOf(mod, plugin.id).enabled && plugin.params?.length && !props.disabledPlugins?.[plugin.id]?.disabled" class="plugin-params">
          <el-row :gutter="12">
            <el-col v-for="param in plugin.params" :key="param.key" :span="param.span || 12">
              <el-form-item :label="param.label" style="margin-bottom:10px">
                <el-input v-if="param.type === 'text' || param.type === 'string'"
                  v-model="entryOf(mod, plugin.id).params[param.key]"
                  :placeholder="param.placeholder || param.help" />
                <el-input-number v-else-if="param.type === 'number'"
                  v-model="entryOf(mod, plugin.id).params[param.key]"
                  :min="param.min" :max="param.max" :step="param.step || 1"
                  style="width:100%" controls-position="right" />
                <el-select v-else-if="param.type === 'select'"
                  v-model="entryOf(mod, plugin.id).params[param.key]"
                  :multiple="!!param.multiple" collapse-tags collapse-tags-tooltip
                  style="width:100%">
                  <el-option v-for="opt in param.options" :key="String(opt.value)" :value="opt.value" :label="opt.label" />
                </el-select>
                <el-select v-else-if="param.type === 'dict-select'"
                  v-model="entryOf(mod, plugin.id).params[param.key]"
                  :multiple="!!param.multiple" collapse-tags collapse-tags-tooltip
                  style="width:100%"
                  :placeholder="dictSelectPlaceholder(param)">
                  <el-option v-for="d in dictOptions(param)" :key="d.id"
                    :value="d.id"
                    :label="`${d.name}${d.builtin ? ' (内置)' : ''} · ${d.count}行`" />
                </el-select>
                <el-checkbox-group v-else-if="param.type === 'checkbox-group'"
                  v-model="entryOf(mod, plugin.id).params[param.key]">
                  <el-checkbox v-for="opt in param.options" :key="String(opt.value)" :value="opt.value">{{ opt.label }}</el-checkbox>
                </el-checkbox-group>
                <el-input v-else-if="param.type === 'textarea'"
                  v-model="entryOf(mod, plugin.id).params[param.key]"
                  type="textarea" :rows="3" :placeholder="param.placeholder || param.help"
                  style="font-family:monospace;font-size:12px" />
                <el-switch v-else-if="param.type === 'switch'"
                  v-model="entryOf(mod, plugin.id).params[param.key]" />
                <div v-if="param.help" class="param-help">{{ param.help }}</div>
              </el-form-item>
            </el-col>
          </el-row>
        </div>
      </div>
      <div v-if="pluginsOf(mod).length === 0" class="module-empty">暂无可用插件</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import { useRouter } from 'vue-router'
import type { Plugin, PluginParam, DictEntry } from '@/api'
import { MODULE_ORDER, moduleLabel, moduleIcon } from '@/constants/modules'

export interface PluginEntry {
  enabled: boolean
  params: Record<string, any>
}
export type CustomConfig = Record<string, Record<string, PluginEntry>>

const props = defineProps<{
  modelValue: CustomConfig
  plugins: Plugin[]
  dicts?: DictEntry[]
  disabledPlugins?: Record<string, { disabled: boolean, reason: string, route?: string }>
}>()

const router = useRouter()
const moduleOrder = [...MODULE_ORDER]

function pluginsOf(mod: string): Plugin[] {
  return props.plugins.filter(p => p.module === mod && p.enabled)
}

// entryOf 保证父组件传入的 customConfig 里有对应槽位；没有则用插件 schema 的默认值填充。
// 直接就地修改 props.modelValue（父组件传入的应是 reactive/ref 对象），不 emit 顶层替换。
function entryOf(mod: string, pluginId: string): PluginEntry {
  const cfg = props.modelValue
  if (!cfg[mod]) cfg[mod] = {}
  if (!cfg[mod][pluginId]) {
    const plugin = props.plugins.find(p => p.id === pluginId)
    const params: Record<string, any> = {}
    for (const p of (plugin?.params || [])) {
      params[p.key] = defaultForParam(p)
    }
    cfg[mod][pluginId] = { enabled: false, params }
  }
  return cfg[mod][pluginId]
}

function defaultForParam(p: PluginParam): any {
  if (p.default !== undefined && p.default !== null) return p.default
  if (p.type === 'checkbox-group' || p.multiple) return []
  if (p.type === 'number') return 0
  if (p.type === 'switch') return false
  return ''
}

// 插件加载后为每个插件预填一次默认槽位，避免绑定到未初始化的 undefined。
watch(
  () => props.plugins,
  plugins => {
    for (const plugin of plugins) {
      entryOf(plugin.module, plugin.id)
    }
  },
  { immediate: true },
)

function dictOptions(param: PluginParam): DictEntry[] {
  const list = props.dicts || []
  return list.filter(d => {
    if (param.dict_category && d.category !== param.dict_category) return false
    if (param.dict_service && (d.service || '') !== param.dict_service) return false
    if (param.dict_kind && (d.kind || '') !== param.dict_kind) return false
    return true
  })
}

function dictSelectPlaceholder(param: PluginParam): string {
  return dictOptions(param).length ? '选择字典' : '暂无匹配字典，请到「字典管理」添加'
}
</script>

<style scoped>
.plugin-config-editor {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  overflow: hidden;
}
.module-section { border-bottom: 1px solid var(--el-border-color-lighter); }
.module-section:last-child { border-bottom: none; }
.module-header {
  background: var(--el-fill-color-light); padding: 8px 14px;
  display: flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 600;
}
.module-icon { font-size: 16px; }
.module-name { min-width: 90px; }
.plugin-card { border-top: 1px solid var(--el-border-color-extra-light); }
.plugin-row {
  padding: 8px 14px 8px 42px;
  display: flex; align-items: center; gap: 10px; font-size: 13px;
}
.plugin-name-label { font-weight: 500; min-width: 100px; }
.plugin-ver { color: var(--el-text-color-secondary); font-size: 11px; min-width: 40px; }
.plugin-desc-text { color: var(--el-text-color-secondary); font-size: 12px; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
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
.module-empty { padding: 12px 14px; color: var(--el-text-color-disabled); font-size: 12px; }
</style>
