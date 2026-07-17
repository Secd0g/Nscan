<template>
  <div class="page-card">
    <div class="page-header">
      <h2 class="page-title">敏感规则</h2>
      <div class="header-actions">
        <el-input v-model="search" placeholder="搜索规则" clearable size="small" style="width:200px;margin-right:8px" @change="fetchList" />
        <el-select v-model="severityFilter" placeholder="全部严重级别" clearable size="small" style="width:160px;margin-right:8px" @change="fetchList">
          <el-option v-for="s in severities" :key="s.value" :value="s.value" :label="s.label" />
        </el-select>
        <el-button type="primary" size="small" @click="openAdd">
          <el-icon style="margin-right:4px"><Plus /></el-icon>添加规则
        </el-button>
      </div>
    </div>

    <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px"
      description="敏感信息识别规则（正则表达式匹配 HTTP 响应体+响应头），扫描任务的 sensitive stage 会用所有 active=true 的规则做匹配。" />

    <el-table :data="rules" v-loading="loading" size="small" stripe max-height="640">
      <el-table-column prop="name" label="规则名称" min-width="180">
        <template #default="{ row }">
          <el-tag v-if="row.color" :color="row.color" style="color:white;border:none">{{ row.name }}</el-tag>
          <span v-else>{{ row.name }}</span>
        </template>
      </el-table-column>
      <el-table-column label="严重级别" width="100">
        <template #default="{ row }">
          <el-tag :type="sevTag(row.severity)" size="small" effect="plain">{{ row.severity || '-' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="pattern" label="正则表达式" min-width="260" show-overflow-tooltip>
        <template #default="{ row }">
          <code class="pattern-code">{{ row.pattern }}</code>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="说明" min-width="200" show-overflow-tooltip />
      <el-table-column label="类型" width="80">
        <template #default="{ row }">
          <el-tag :type="row.builtin ? 'info' : 'success'" size="small">{{ row.builtin ? '内置' : '自定义' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="启用" width="70" align="center">
        <template #default="{ row }">
          <el-switch v-model="row.active" size="small" @change="toggleActive(row)" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="140" align="center">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="openEdit(row)">编辑</el-button>
          <el-popconfirm v-if="!row.builtin" title="确认删除？" @confirm="deleteRule(row)">
            <template #reference>
              <el-button type="danger" link size="small">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination background layout="prev, pager, next, total"
      :total="total" :current-page="page" :page-size="pageSize"
      style="margin-top:12px;justify-content:flex-end" @current-change="onPageChange" />

    <!-- 添加/编辑对话框 -->
    <el-dialog v-model="dlgVisible" :title="editing ? '编辑规则' : '添加规则'" width="640px" @close="resetForm">
      <el-form :model="form" label-position="top">
        <el-form-item label="规则名称"><el-input v-model="form.name" placeholder="例：AWS Access Key" /></el-form-item>
        <el-form-item label="严重级别">
          <el-select v-model="form.severity" style="width:200px">
            <el-option v-for="s in severities" :key="s.value" :value="s.value" :label="s.label" />
          </el-select>
        </el-form-item>
        <el-form-item label="正则表达式">
          <el-input v-model="form.pattern" type="textarea" :rows="2"
            placeholder='例：AKIA[0-9A-Z]{16}' style="font-family:monospace" />
          <div class="hint">测试：<el-input v-model="testInput" size="small" placeholder="粘贴一段文本测试匹配" style="width:70%;margin-right:8px" /><el-button size="small" @click="doTest">测试</el-button></div>
          <div v-if="testResult" class="hint" :style="{color: testMatched ? 'var(--el-color-success)' : 'var(--el-color-danger)'}">{{ testResult }}</div>
        </el-form-item>
        <el-form-item label="说明"><el-input v-model="form.description" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.active" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlgVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { sensitiveRuleApi, type SensitiveRule } from '@/api'

const severities = [
  { value: 'critical', label: 'Critical' },
  { value: 'high',     label: 'High' },
  { value: 'medium',   label: 'Medium' },
  { value: 'low',      label: 'Low' },
]
function sevTag(sev: string): string {
  return ({ critical: 'danger', high: 'warning', medium: 'primary', low: 'info' } as Record<string, string>)[sev] || 'info'
}

const rules = ref<SensitiveRule[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const search = ref('')
const severityFilter = ref('')

async function fetchList() {
  loading.value = true
  try {
    const r = await sensitiveRuleApi.list({
      keyword: search.value,
      severity: severityFilter.value,
      limit: pageSize.value,
      skip: (page.value - 1) * pageSize.value,
    })
    rules.value = r.data || []
    total.value = r.total || 0
  } catch (e: any) {
    ElMessage.error('加载失败: ' + e.message)
  } finally {
    loading.value = false
  }
}

function onPageChange(p: number) { page.value = p; fetchList() }

const dlgVisible = ref(false)
const saving = ref(false)
const editing = ref<SensitiveRule | null>(null)
const form = reactive({
  name: '', pattern: '', description: '', severity: 'medium', active: true,
})
const testInput = ref('')
const testResult = ref('')
const testMatched = ref(false)

function resetForm() {
  form.name = ''; form.pattern = ''; form.description = ''; form.severity = 'medium'; form.active = true
  testInput.value = ''; testResult.value = ''; testMatched.value = false
  editing.value = null
}

function openAdd() { resetForm(); dlgVisible.value = true }

function openEdit(row: SensitiveRule) {
  editing.value = row
  form.name = row.name
  form.pattern = row.pattern
  form.description = row.description || ''
  form.severity = row.severity || 'medium'
  form.active = row.active
  dlgVisible.value = true
}

function doTest() {
  if (!form.pattern) { testResult.value = '请先填写正则'; testMatched.value = false; return }
  try {
    const re = new RegExp(form.pattern)
    const m = testInput.value.match(re)
    if (m) { testResult.value = '✓ 命中: ' + m[0].slice(0, 100); testMatched.value = true }
    else { testResult.value = '✗ 未命中'; testMatched.value = false }
  } catch (e: any) {
    testResult.value = '正则语法错误: ' + e.message; testMatched.value = false
  }
}

async function save() {
  if (!form.name || !form.pattern) { ElMessage.warning('规则名称和正则不能为空'); return }
  try { new RegExp(form.pattern) } catch (e: any) { ElMessage.error('正则语法错误: ' + e.message); return }
  saving.value = true
  try {
    if (editing.value) {
      await sensitiveRuleApi.update(editing.value.id, form)
      ElMessage.success('已保存')
    } else {
      await sensitiveRuleApi.create(form)
      ElMessage.success('已添加')
    }
    dlgVisible.value = false
    fetchList()
  } catch (e: any) {
    ElMessage.error('保存失败: ' + e.message)
  } finally {
    saving.value = false
  }
}

async function toggleActive(row: SensitiveRule) {
  try {
    await sensitiveRuleApi.update(row.id, { active: row.active })
    ElMessage.success(row.active ? '已启用' : '已停用')
  } catch (e: any) {
    row.active = !row.active
    ElMessage.error(e.message)
  }
}

async function deleteRule(row: SensitiveRule) {
  try {
    await sensitiveRuleApi.remove(row.id)
    ElMessage.success('已删除')
    fetchList()
  } catch (e: any) {
    ElMessage.error(e.message)
  }
}

onMounted(fetchList)
</script>

<style scoped>
.pattern-code {
  font-family: monospace; font-size: 12px;
  background: var(--el-fill-color-light); padding: 2px 6px; border-radius: 3px;
}
.hint { font-size: 12px; color: var(--el-text-color-placeholder); margin-top: 6px; }
</style>
