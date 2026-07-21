<template>
  <div class="system-general">
    <el-card class="system-card" shadow="never">
      <template #header>
        <div style="font-weight: 600">全局代理配置</div>
      </template>
      <el-alert
        title="此配置将作为环境变量注入给底层探测引擎（subfinder, httpx, naabu, nuclei等），适用于部分受限网络环境或隐匿扫描。"
        type="info"
        show-icon
        :closable="false"
        style="margin-bottom: 20px;"
      />
      <el-form :model="form" label-width="120px" v-loading="loading">
        <el-form-item label="代理地址" required>
          <el-input v-model="form.proxy" placeholder="如 http://127.0.0.1:7890 或 socks5://127.0.0.1:1080" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveConfig" :loading="saving">保存配置</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { http } from '@/api'

const form = ref({ proxy: '' })
const loading = ref(false)
const saving = ref(false)

async function loadConfig() {
  loading.value = true
  try {
    const res = await http.get('/settings/providers/system')
    form.value.proxy = res.data?.providers?.proxy?.[0] || ''
  } catch (err: any) {
    if (err.response?.status !== 404) ElMessage.error('加载系统配置失败: ' + err.message)
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  saving.value = true
  try {
    await http.put('/settings/providers/system', {
      providers: { proxy: [form.value.proxy] },
      enabled: { proxy: true },
    })
    ElMessage.success('保存成功')
  } catch (err: any) {
    ElMessage.error('保存失败: ' + err.message)
  } finally {
    saving.value = false
  }
}

onMounted(loadConfig)
</script>

<style scoped>
.system-card { width: 100%; }
</style>
