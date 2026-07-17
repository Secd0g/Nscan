<template>
  <div>
    <h2 class="page-title" style="margin-bottom:16px">通知设置</h2>
    <el-card shadow="never" v-loading="loading" style="max-width:720px">
      <el-tabs v-model="activeTab" type="card">
        <el-tab-pane v-for="ch in channelList" :key="ch.key" :name="ch.key">
          <template #label>
            <div style="display:flex;align-items:center;gap:6px;padding:0 4px">
              <el-icon :style="{ color: ch.color, fontSize:'15px' }"><component :is="ch.icon" /></el-icon>
              <span>{{ ch.label }}</span>
              <el-badge v-if="channels[ch.key].enabled" is-dot type="success" style="margin-left:2px" />
            </div>
          </template>

          <div style="padding:4px 0 8px">
            <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:16px">
              <div>
                <span style="font-size:13px;color:var(--el-text-color-secondary)">渠道状态</span>
              </div>
              <div style="display:flex;align-items:center;gap:8px">
                <span style="font-size:13px;color:var(--el-text-color-secondary)">
                  {{ channels[ch.key].enabled ? '已启用' : '已禁用' }}
                </span>
                <el-switch v-model="channels[ch.key].enabled" />
              </div>
            </div>

            <el-form :model="channels[ch.key].config" label-position="top" :disabled="!channels[ch.key].enabled">
              <el-form-item v-for="field in ch.fields" :key="field.key" :label="field.label">
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

              <el-form-item label="通知事件">
                <div style="display:flex;flex-wrap:wrap;gap:8px">
                  <el-checkbox-button
                    v-for="ev in eventOptions"
                    :key="ev.value"
                    v-model="channels[ch.key].events"
                    :value="ev.value"
                    size="small"
                  >{{ ev.label }}</el-checkbox-button>
                </div>
              </el-form-item>

              <div style="display:flex;gap:8px;margin-top:4px">
                <el-button
                  size="small"
                  :loading="testing === ch.key"
                  :disabled="!channels[ch.key].enabled"
                  @click="test(ch.key)"
                >
                  <el-icon style="margin-right:4px"><Promotion /></el-icon>发送测试
                </el-button>
                <el-button
                  size="small"
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
