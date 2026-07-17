<template>
  <div class="system-config">
    <div class="config-hero">
      <div class="hero-icon"><el-icon><Setting /></el-icon></div>
      <div class="hero-copy">
        <div class="hero-title-row"><h2>系统配置</h2><el-tag size="small" effect="plain" type="info">管理员</el-tag></div>
        <p>集中管理系统运行参数、AI 服务、通知渠道、扫描工具和访问控制。</p>
      </div>
    </div>

    <el-tabs v-model="activeTab" type="border-card" class="config-tabs">
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
.config-hero { display: flex; align-items: center; gap: 14px; padding: 22px 24px; margin-bottom: 18px; border: 1px solid var(--el-border-color-light); border-radius: 12px; background: linear-gradient(135deg, var(--el-bg-color-overlay), var(--el-color-primary-light-9)); }
.hero-icon { display: grid; place-items: center; width: 46px; height: 46px; border-radius: 12px; color: #fff; background: linear-gradient(135deg, #4080ff, #6d5dfc); font-size: 22px; box-shadow: 0 8px 18px rgb(64 128 255 / 22%); }
.hero-title-row { display: flex; align-items: center; gap: 10px; }
.hero-title-row h2 { margin: 0; font-size: 19px; font-weight: 650; color: var(--el-text-color-primary); }
.hero-copy p { margin: 6px 0 0; color: var(--el-text-color-secondary); font-size: 13px; }
.config-tabs { border: 0; box-shadow: none; }
.config-tabs :deep(.el-tabs__header) { border: 1px solid var(--el-border-color-light); border-radius: 10px; overflow: hidden; }
.config-tabs :deep(.el-tabs__nav-wrap) { padding: 0 8px; }
.config-tabs :deep(.el-tabs__item) { height: 48px; color: var(--el-text-color-secondary); }
.config-tabs :deep(.el-tabs__item.is-active) { color: var(--el-color-primary); font-weight: 600; background: var(--el-bg-color-overlay); }
.tab-label { display: inline-flex; align-items: center; gap: 6px; }
.config-tabs :deep(.el-tabs__content) { padding: 20px 0 0; }
.config-tabs :deep(.el-tab-pane > .page-card),
.config-tabs :deep(.el-tab-pane > div) { padding: 0; }
</style>
