<template>
  <div class="page-card mcp-config">
    <div class="page-heading">
      <div>
        <h2 class="page-title">MCP 配置</h2>
        <p class="hint">让支持 MCP 的 AI 平台查询 nscan 中的项目、任务和安全资产。</p>
      </div>
      <el-tag type="success" effect="light">只读查询</el-tag>
    </div>

    <el-alert title="MCP 接口已启用" description="平台通过当前登录账号的 Bearer Token 访问 MCP。Token 有效期为 24 小时，过期后请重新登录获取。" type="info" :closable="false" show-icon class="intro" />

    <el-form label-position="top" class="config-form">
      <el-form-item label="MCP 服务地址">
        <el-input :model-value="mcpUrl" readonly><template #append><el-button @click="copy(mcpUrl)">复制</el-button></template></el-input>
      </el-form-item>
      <el-form-item label="Bearer Token">
        <el-input :model-value="tokenPreview" readonly><template #append><el-button @click="copyToken">复制 Token</el-button></template></el-input>
        <div class="field-hint">使用当前登录会话 Token，不要把 Token 分享给其他人。</div>
      </el-form-item>
    </el-form>

    <div class="section-title">支持的查询能力</div>
    <div class="tool-grid">
      <div v-for="tool in tools" :key="tool.name" class="tool-item"><div class="tool-name">{{ tool.name }}</div><div class="tool-desc">{{ tool.description }}</div></div>
    </div>

    <div class="section-title">接入示例</div>
    <pre class="snippet">MCP URL: {{ mcpUrl }}
Authorization: Bearer {{ tokenPreview }}</pre>
    <div class="field-hint">如果平台支持导入 MCP 地址，填写上面的服务地址并选择 Bearer Token 鉴权即可。</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ElMessage } from 'element-plus'

const token = localStorage.getItem('nscan_token') ?? ''
const mcpUrl = computed(() => `${window.location.origin}/mcp`)
const tokenPreview = token ? `${token.slice(0, 12)}${token.length > 12 ? '••••••••' : ''}` : '当前会话无 Token'
const tools = [
  { name: 'list_projects', description: '查询项目列表' },
  { name: 'list_tasks', description: '按项目、状态和关键词查询任务' },
  { name: 'get_task', description: '查询任务详情' },
  { name: 'query_assets', description: '查询各类安全资产' },
  { name: 'asset_stats', description: '查询资产统计' },
]
async function copy(value: string) { try { await navigator.clipboard.writeText(value); ElMessage.success('已复制') } catch { ElMessage.error('复制失败，请手动复制') } }
function copyToken() { if (token) copy(token); else ElMessage.warning('当前登录会话无 Token') }
</script>

<style scoped>
.mcp-config { padding: 24px; max-width: 900px; }
.page-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.hint, .field-hint { color: var(--el-text-color-secondary); font-size: 13px; }
.hint { margin: 8px 0 20px; }.field-hint { margin-top: 5px; font-size: 12px; }
.intro { margin: 4px 0 24px; }.config-form { max-width: 720px; }
.section-title { margin: 28px 0 12px; color: var(--el-text-color-primary); font-size: 15px; font-weight: 600; }
.tool-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.tool-item { padding: 13px 15px; border: 1px solid var(--el-border-color-light); border-radius: 6px; background: var(--el-fill-color-lighter); }
.tool-name { color: var(--el-color-primary); font-family: monospace; font-size: 13px; }.tool-desc { margin-top: 5px; color: var(--el-text-color-secondary); font-size: 13px; }
.snippet { overflow-x: auto; margin: 0; padding: 14px 16px; border-radius: 6px; background: var(--el-fill-color-dark); color: var(--el-text-color-primary); font-size: 12px; line-height: 1.7; }
@media (max-width: 700px) { .tool-grid { grid-template-columns: 1fr; } }
</style>
