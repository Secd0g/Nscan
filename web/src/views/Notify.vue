<template>
  <div class="notify-page">
    <el-card class="notify-card" shadow="never" v-loading="loading">
      <template #header>
        <div class="notify-card-header">
          <div>
            <h2 class="page-title">通知设置</h2>
            <p>配置任务完成、漏洞发现和资产变更等事件的通知渠道。</p>
          </div>
          <span class="notify-count">{{ channelList.length }} 个渠道</span>
        </div>
      </template>
      <el-tabs v-model="activeTab" class="channel-tabs">
        <el-tab-pane v-for="ch in channelList" :key="ch.key" :name="ch.key">
          <template #label>
            <div class="channel-tab">
              <el-icon :style="{ color: ch.color }"><component :is="ch.icon" /></el-icon>
              <span>{{ ch.label }}</span>
              <i v-if="channels[ch.key].enabled" class="enabled-dot" />
            </div>
          </template>

          <div class="channel-content">
            <div class="status-row">
              <div>
                <strong>渠道状态</strong>
                <span>启用后将按下方事件发送通知</span>
              </div>
              <div class="status-control">
                <span :class="{ 'is-enabled': channels[ch.key].enabled }">
                  {{ channels[ch.key].enabled ? '已启用' : '已禁用' }}
                </span>
                <el-switch v-model="channels[ch.key].enabled" />
              </div>
            </div>

            <el-form class="channel-form" :model="channels[ch.key].config" label-position="top" :disabled="!channels[ch.key].enabled">
              <div class="field-grid">
              <el-form-item v-for="field in ch.fields" :key="field.key" :label="field.label" class="channel-field">
                <template v-if="field.hint" #label>
                  <span>{{ field.label }}</span>
                  <el-tooltip :content="field.hint" placement="top">
                    <el-icon style="margin-left:4px;color:var(--el-text-color-placeholder);cursor:help"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </template>
                <el-input
                  v-if="field.type === 'password'"
                  v-model="channels[ch.key].config[field.key]"
                  type="password"
                  show-password
                  :placeholder="field.placeholder"
                />
                <el-input v-else v-model="channels[ch.key].config[field.key]" :placeholder="field.placeholder" />
              </el-form-item>
              </div>

              <el-form-item label="通知事件" class="event-field">
                <div class="event-options">
                  <el-checkbox-button
                    v-for="ev in eventOptions"
                    :key="ev.value"
                    v-model="channels[ch.key].events"
                    :value="ev.value"
                    size="small"
                  >{{ ev.label }}</el-checkbox-button>
                </div>
              </el-form-item>

              <div class="form-actions">
                <el-button
                  :loading="testing === ch.key"
                  :disabled="!channels[ch.key].enabled"
                  @click="test(ch.key)"
                >
                  <el-icon style="margin-right:4px"><Promotion /></el-icon>发送测试
                </el-button>
                <el-button
                  type="primary"
                  :loading="saving === ch.key"
                  @click="save(ch.key)"
                >
                  <el-icon style="margin-right:4px"><Check /></el-icon>保存
                </el-button>
              </div>
            </el-form>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { QuestionFilled, Promotion, Check } from '@element-plus/icons-vue'
import { notifyApi } from '@/api'

interface FieldDef {
  key: string
  label: string
  type: 'text' | 'password'
  placeholder: string
  hint?: string
}
interface ChannelState { enabled: boolean; events: string[]; config: Record<string, string> }

const eventOptions = [
  { value: 'task_done', label: '任务完成' },
  { value: 'task_failed', label: '任务失败' },
  { value: 'vuln_found', label: '发现漏洞' },
  { value: 'asset_changed', label: '资产变更' },
  { value: 'scan_diff', label: '扫描差异摘要' },
]

const channelList: { key: string; label: string; icon: string; color: string; fields: FieldDef[] }[] = [
  {
    key: 'wecom', label: '企业微信', icon: 'ChatDotRound', color: '#07c160',
    fields: [
      { key: 'webhook', label: 'Webhook URL', type: 'text', placeholder: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...' },
    ],
  },
  {
    key: 'dingtalk', label: '钉钉', icon: 'Bell', color: '#1677ff',
    fields: [
      { key: 'webhook', label: 'Webhook URL', type: 'text', placeholder: 'https://oapi.dingtalk.com/robot/send?access_token=...' },
      { key: 'secret', label: '加签密钥（可选）', type: 'password', placeholder: 'SECxxx' },
    ],
  },
  {
    key: 'slack', label: 'Slack', icon: 'ChatLineSquare', color: '#4a154b',
    fields: [
      { key: 'webhook', label: 'Webhook URL', type: 'text', placeholder: 'https://hooks.slack.com/services/...' },
    ],
  },
  {
    key: 'telegram', label: 'Telegram', icon: 'Promotion', color: '#0088cc',
    fields: [
      {
        key: 'bot_token', label: 'Bot Token', type: 'password', placeholder: '1234567890:ABCdef...',
        hint: '向 @BotFather 发送 /newbot 获取 Token',
      },
      {
        key: 'chat_id', label: 'Chat ID', type: 'text', placeholder: '-1001234567890 或 @channel_name',
        hint: '转发一条消息给 @userinfobot 可获取你的 Chat ID；频道请填 @频道用户名',
      },
    ],
  },
  {
    key: 'email', label: '邮件', icon: 'Message', color: '#e24329',
    fields: [
      { key: 'smtp_host', label: 'SMTP 服务器', type: 'text', placeholder: 'smtp.example.com:465' },
      { key: 'from', label: '发件人', type: 'text', placeholder: 'noreply@example.com' },
      { key: 'password', label: '密码 / 授权码', type: 'password', placeholder: 'SMTP 密码' },
      { key: 'to', label: '收件人', type: 'text', placeholder: '多个邮箱用逗号分隔' },
    ],
  },
]

function defaults(): Record<string, ChannelState> {
  return {
    wecom:    { enabled: false, events: ['vuln_found'], config: { webhook: '' } },
    dingtalk: { enabled: false, events: ['vuln_found'], config: { webhook: '', secret: '' } },
    slack:    { enabled: false, events: ['task_done', 'vuln_found'], config: { webhook: '' } },
    telegram: { enabled: false, events: ['vuln_found'], config: { bot_token: '', chat_id: '' } },
    email:    { enabled: false, events: [], config: { smtp_host: '', from: '', password: '', to: '' } },
  }
}

const channels = reactive<Record<string, ChannelState>>(defaults())
const activeTab = ref('wecom')
const loading = ref(false)
const saving = ref('')
const testing = ref('')

onMounted(async () => {
  loading.value = true
  try {
    const list = await notifyApi.list()
    for (const c of list) {
      if (channels[c.key]) {
        channels[c.key].enabled = c.enabled
        channels[c.key].events = c.events ?? []
        channels[c.key].config = { ...channels[c.key].config, ...(c.config ?? {}) }
      }
    }
    // 自动跳到第一个已启用的渠道
    const enabled = channelList.find(ch => channels[ch.key].enabled)
    if (enabled) activeTab.value = enabled.key
  } catch (e: any) {
    ElMessage.error(e.message || '加载通知配置失败')
  } finally {
    loading.value = false
  }
})

async function save(key: string) {
  saving.value = key
  try {
    const c = channels[key]
    await notifyApi.save(key, { enabled: c.enabled, events: c.events, config: c.config })
    ElMessage.success('通知设置已保存')
  } catch (e: any) {
    ElMessage.error(e.message || '保存失败')
  } finally {
    saving.value = ''
  }
}

async function test(key: string) {
  testing.value = key
  try {
    await notifyApi.test(key, channels[key].config)
    ElMessage.success('测试消息已发送，请检查是否收到')
  } catch (e: any) {
    ElMessage.error(e.message || '发送失败')
  } finally {
    testing.value = ''
  }
}
</script>

<style scoped>
.notify-page { min-height: calc(100vh - 112px); padding: 0 0 24px; }
.notify-card-header { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.notify-card-header .page-title { margin: 0; font-size: 16px; }
.notify-card-header p { margin: 5px 0 0; color: var(--el-text-color-secondary); font-size: 12px; }
.notify-count { padding: 5px 10px; border: 1px solid var(--el-border-color-light); border-radius: 6px; color: var(--el-text-color-secondary); background: var(--el-fill-color-light); font-size: 12px; }
.notify-card { width: 100%; max-width: none; border-radius: 8px; }
.notify-card :deep(.el-card__header) { padding: 16px 20px 14px; }
.notify-card :deep(.el-card__body) { padding: 16px 20px 22px; }
.channel-tabs :deep(.el-tabs__header) { margin: 0 0 22px; padding: 4px; border: 1px solid var(--el-border-color-light); border-radius: 9px; background: var(--el-fill-color-light); }
.channel-tabs :deep(.el-tabs__nav-wrap::after) { display: none; }
.channel-tabs :deep(.el-tabs__nav) { border: 0; }
.channel-tabs :deep(.el-tabs__item) { height: 40px; padding: 0 18px; border: 0; border-radius: 6px; color: var(--el-text-color-secondary); }
.channel-tabs :deep(.el-tabs__item:hover) { color: var(--el-color-primary); }
.channel-tabs :deep(.el-tabs__item.is-active) { color: var(--el-color-primary); background: var(--el-bg-color-overlay); box-shadow: 0 1px 4px rgb(31 50 81 / 8%); }
.channel-tabs :deep(.el-tabs__active-bar) { display: none; }
.channel-tab { display: inline-flex; align-items: center; gap: 7px; white-space: nowrap; }
.channel-tab .el-icon { font-size: 16px; }
.enabled-dot { width: 6px; height: 6px; margin-left: 1px; border-radius: 50%; background: #22c55e; }
.channel-content { padding: 0 4px; }
.status-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 16px; margin-bottom: 22px; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; background: var(--el-fill-color-light); }
.status-row strong { display: block; color: var(--el-text-color-primary); font-size: 14px; font-weight: 600; }
.status-row strong + span { display: block; margin-top: 4px; color: var(--el-text-color-secondary); font-size: 12px; }
.status-control { display: flex; align-items: center; gap: 9px; color: var(--el-text-color-secondary); font-size: 12px; white-space: nowrap; }
.status-control .is-enabled { color: #16a34a; font-weight: 600; }
.channel-form :deep(.el-form-item) { margin-bottom: 18px; }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: 18px; }
.field-grid .channel-field:only-child { grid-column: 1 / -1; }
.channel-form :deep(.el-form-item__label) { padding-bottom: 6px; color: var(--el-text-color-regular); font-size: 13px; font-weight: 500; line-height: 1.3; }
.channel-form :deep(.el-input__wrapper) { min-height: 36px; }
.event-field { padding-top: 2px; }
.event-options { display: flex; flex-wrap: wrap; gap: 8px; }
.event-options :deep(.el-checkbox-button__inner) { padding: 8px 13px; border-radius: 6px !important; border-left: 1px solid var(--el-border-color) !important; }
.form-actions { display: flex; gap: 10px; padding-top: 2px; }
.form-actions .el-button { min-height: 34px; padding: 0 15px; }

@media (max-width: 700px) {
  .notify-card-header { align-items: flex-start; flex-direction: column; }
  .notify-card :deep(.el-card__body) { padding: 14px; }
  .channel-tabs :deep(.el-tabs__nav) { display: flex; width: 100%; overflow-x: auto; }
  .channel-tabs :deep(.el-tabs__item) { flex: 0 0 auto; padding: 0 12px; }
  .field-grid { grid-template-columns: 1fr; }
  .field-grid .channel-field:only-child { grid-column: auto; }
}
</style>
