<template>
  <div class="page-card ai-config">
    <h2 class="page-title">AI 配置</h2>
    <p class="hint">用于任务扫描结果分析和 Claude Code 自动化渗透。API Key 保存在服务端配置中，启动任务时临时下发到节点。</p>
    <el-form label-position="top" class="ai-form" v-loading="loading">
      <el-form-item label="接口类型"><el-radio-group v-model="form.type" @change="switchProvider"><el-radio-button :value="'openai'">OpenAI 兼容</el-radio-button><el-radio-button :value="'gemini'">Google Gemini</el-radio-button><el-radio-button :value="'anthropic'">Anthropic / Claude</el-radio-button></el-radio-group></el-form-item>
      <div class="provider-note"><span class="provider-dot" />{{ providerMeta.name }}<span class="provider-note-text">{{ providerMeta.description }}</span></div>
      <el-form-item label="接口地址"><el-input v-model="form.base_url" :placeholder="providerMeta.baseUrl" /><div class="field-hint">{{ providerMeta.urlHint }}</div></el-form-item>
      <el-form-item label="API Key / Token"><el-input v-model="form.token" type="password" show-password /></el-form-item>
      <el-form-item label="模型"><el-input v-model="form.model" :placeholder="providerMeta.model" /></el-form-item>
      <el-form-item label="代理地址"><el-input v-model="form.proxy_url" placeholder="http://host.docker.internal:7890" /><div class="field-hint">可选。Docker 中访问宿主机代理请使用 host.docker.internal，例如 HTTP 代理 7890 端口。</div></el-form-item>
      <el-button type="primary" :loading="saving" @click="save">保存配置</el-button>
    </el-form>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { settingsApi, type AIConfig } from '@/api'
const loading = ref(true); const saving = ref(false)
const form = reactive<AIConfig>({ type: 'openai', base_url: '', token: '', model: '', proxy_url: '' })
type ProviderType = 'openai' | 'gemini' | 'anthropic'
const providerMetaMap = {
  openai: { name: 'OpenAI 兼容接口', description: '适用于 OpenAI 及兼容 OpenAI 协议的服务。', baseUrl: 'https://api.openai.com', model: 'gpt-4o-mini', urlHint: '可填写服务根地址，也可直接填写 /v1/chat/completions' },
  gemini: { name: 'Google Gemini', description: '使用 Gemini generateContent 接口进行分析。', baseUrl: 'https://generativelanguage.googleapis.com', model: 'gemini-2.0-flash', urlHint: '可填写 Gemini 完整 generateContent 地址，或填写服务根地址' },
  anthropic: { name: 'Anthropic / Claude', description: '适用于 Claude API 及 Claude Code 自动化渗透。', baseUrl: 'https://api.anthropic.com', model: 'claude-sonnet-4-20250514', urlHint: 'Claude Code 主要使用下方 API Key；此地址同时用于扫描结果分析' },
} as const
const providerMeta = computed(() => providerMetaMap[form.type as ProviderType] ?? { name: '自定义接口', description: '使用兼容接口协议的 AI 服务。', baseUrl: '', model: '', urlHint: '请填写服务地址和模型名称' })
const providerDrafts = reactive<Partial<Record<ProviderType, { base_url: string; model: string }>>>({})
const previousProvider = ref<ProviderType>('openai')
function switchProvider(nextType: string) {
  const next = nextType as ProviderType
  if (!providerMetaMap[next]) return
  providerDrafts[previousProvider.value] = { base_url: form.base_url, model: form.model }
  const draft = providerDrafts[next]
  form.base_url = draft?.base_url || providerMetaMap[next].baseUrl
  form.model = draft?.model || providerMetaMap[next].model
  previousProvider.value = next
}
onMounted(async () => {
  try {
    Object.assign(form, await settingsApi.getAI())
    previousProvider.value = (form.type as ProviderType) || 'openai'
    // 兼容旧版本：旧逻辑切换接口时可能把上一个接口的默认值保存到了当前接口。
    const current = providerMetaMap[previousProvider.value]
    if (current && ((form.type === 'openai' && (form.base_url.includes('googleapis.com') || form.model.startsWith('gemini'))) ||
      (form.type === 'gemini' && (form.base_url.includes('openai.com') || form.model.startsWith('gpt-'))) ||
      (form.type === 'anthropic' && (form.base_url.includes('openai.com') || form.model.startsWith('gpt-'))))) {
      form.base_url = current.baseUrl
      form.model = current.model
    }
    providerDrafts[previousProvider.value] = { base_url: form.base_url, model: form.model }
  } catch (e: any) { ElMessage.error(e.message) } finally { loading.value = false }
})
async function save() { saving.value = true; try { await settingsApi.saveAI(form); ElMessage.success('AI 配置已保存') } catch (e: any) { ElMessage.error(e.message) } finally { saving.value = false } }
</script>
<style scoped>
.ai-config { padding: 24px; }
.hint, .field-hint { color: var(--el-text-color-secondary); font-size: 13px; }
.hint { margin: 8px 0 20px; }.field-hint { margin-top: 4px; font-size: 12px; }
</style>
