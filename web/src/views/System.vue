<template>
  <div class="system-config">
    <div class="config-hero">
      <div class="hero-icon"><el-icon><Setting /></el-icon></div>
      <div class="hero-copy">
        <div class="hero-title-row"><h2>系统配置</h2><el-tag size="small" effect="plain" type="info">管理员</el-tag></div>
        <p>集中管理系统运行参数、AI 服务、通知渠道、扫描工具和访问控制。</p>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="config-tabs">
      <el-tab-pane name="system" lazy>
        <template #label><span class="tab-label"><el-icon><Setting /></el-icon>系统</span></template>
        <SystemGeneral />
      </el-tab-pane>
      <el-tab-pane name="ai" lazy>
        <template #label><span class="tab-label"><el-icon><Connection /></el-icon>AI 服务</span></template>
        <AIConfig />
      </el-tab-pane>
      <el-tab-pane name="notify" lazy>
        <template #label><span class="tab-label"><el-icon><Bell /></el-icon>通知</span></template>
        <Notify />
      </el-tab-pane>
      <el-tab-pane name="tools" lazy>
        <template #label><span class="tab-label"><el-icon><SetUp /></el-icon>扫描工具</span></template>
        <ToolConfig />
      </el-tab-pane>
      <el-tab-pane name="mcp" lazy>
        <template #label><span class="tab-label"><el-icon><Connection /></el-icon>MCP</span></template>
        <MCPConfig />
      </el-tab-pane>
      <el-tab-pane name="blacklist" lazy>
        <template #label><span class="tab-label"><el-icon><Warning /></el-icon>黑名单</span></template>
        <Blacklist />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Bell, Connection, SetUp, Setting, Warning } from '@element-plus/icons-vue'
import AIConfig from './AIConfig.vue'
import Blacklist from './Blacklist.vue'
import MCPConfig from './MCPConfig.vue'
import Notify from './Notify.vue'
import SystemGeneral from './SystemGeneral.vue'
import ToolConfig from './ToolConfig.vue'

const activeTab = ref('system')
</script>

<style scoped>
.system-config { min-height: calc(100vh - 112px); padding: 4px 2px 24px; }
.config-hero { display: flex; align-items: center; gap: 14px; padding: 18px 22px; margin-bottom: 16px; border: 1px solid var(--el-border-color-light); border-radius: 12px; background: linear-gradient(135deg, var(--el-bg-color-overlay), var(--el-color-primary-light-9)); }
.hero-icon { display: grid; place-items: center; width: 44px; height: 44px; border-radius: 11px; color: #fff; background: linear-gradient(135deg, #4080ff, #6d5dfc); font-size: 21px; box-shadow: 0 6px 14px rgb(64 128 255 / 18%); }
.hero-title-row { display: flex; align-items: center; gap: 10px; }
.hero-title-row h2 { margin: 0; font-size: 18px; font-weight: 650; color: var(--el-text-color-primary); }
.hero-copy p { margin: 5px 0 0; color: var(--el-text-color-secondary); font-size: 12px; }
.config-tabs { border: 0; box-shadow: none; }
.config-tabs :deep(.el-tabs__header) { margin: 0; padding: 0 8px; border: 1px solid var(--el-border-color-light); border-radius: 10px; background: var(--el-fill-color-light); }
.config-tabs :deep(.el-tabs__nav-wrap) { padding: 0; }
.config-tabs :deep(.el-tabs__nav-wrap::after) { display: none; }
.config-tabs :deep(.el-tabs__nav) { border: 0; }
.config-tabs :deep(.el-tabs__active-bar) { display: none; }
.config-tabs :deep(.el-tabs__item) { position: relative; height: 44px; padding: 0 16px; margin-right: 2px; border: 0; border-radius: 0; color: var(--el-text-color-secondary); transition: color .2s, background .2s; }
.config-tabs :deep(.el-tabs__item:last-child) { margin-right: 0; }
.config-tabs :deep(.el-tabs__item:hover) { color: var(--el-color-primary); }
.config-tabs :deep(.el-tabs__item.is-active) { color: var(--el-color-primary); font-weight: 600; background: transparent; }
.config-tabs :deep(.el-tabs__item.is-active::after) { content: ''; position: absolute; left: 14px; right: 14px; bottom: 0; height: 2px; border-radius: 2px; background: var(--el-color-primary); }
.tab-label { display: inline-flex; align-items: center; gap: 7px; white-space: nowrap; }
.tab-label :deep(.el-icon) { font-size: 15px; }
.config-tabs :deep(.el-tabs__content) { padding: 14px 0 22px; }

@media (max-width: 900px) {
  .config-tabs :deep(.el-tabs__nav) { display: flex; width: 100%; overflow-x: auto; }
  .config-tabs :deep(.el-tabs__item) { flex: 0 0 auto; padding: 0 14px; }
}
</style>
