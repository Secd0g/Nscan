<template>
  <div class="page-card">
    <div class="page-header">
      <h2 class="page-title">扫描黑名单</h2>
      <div class="header-actions">
        <el-popconfirm title="确认清空所有黑名单？" @confirm="clearAll">
          <template #reference>
            <el-button type="danger" plain :disabled="!list.length" size="small">清空</el-button>
          </template>
        </el-popconfirm>
        <el-button size="small" @click="showBatchAdd = true">
          <el-icon><Upload /></el-icon>批量导入
        </el-button>
        <el-button type="primary" size="small" @click="showAdd = true">
          <el-icon><Plus /></el-icon>添加规则
        </el-button>
      </div>
    </div>

    <el-alert type="info" :closable="false" style="margin-bottom:16px">
      黑名单中的目标在扫描时将被自动跳过，支持域名、IP、CIDR 和通配符匹配。
    </el-alert>

    <!-- 筛选 -->
    <div class="filter-bar" style="margin-bottom:12px">
      <el-input v-model="search" placeholder="搜索" clearable style="width:200px">
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-select v-model="filterType" clearable placeholder="类型" style="width:120px">
        <el-option label="域名" value="domain" />
        <el-option label="IP" value="ip" />
        <el-option label="CIDR" value="cidr" />
        <el-option label="通配符" value="wildcard" />
      </el-select>
      <span style="font-size:12px;color:var(--el-text-color-secondary)">共 {{ filteredList.length }} 条</span>
    </div>

    <el-table :data="pagedList" v-loading="loading" style="width:100%">
      <el-table-column label="类型" width="100">
        <template #default="{ row }">
          <el-tag :type="typeTag(row.type)" size="small">{{ typeLabel(row.type) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="value" label="值" min-width="280">
        <template #default="{ row }">
          <code style="font-size:13px">{{ row.value }}</code>
        </template>
      </el-table-column>
      <el-table-column prop="remark" label="备注" min-width="200" show-overflow-tooltip>
        <template #default="{ row }">
          <span style="color:var(--el-text-color-secondary);font-size:12px">{{ row.remark || '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="添加时间" width="170">
        <template #default="{ row }">{{ fmt(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="80" fixed="right" align="center">
        <template #default="{ row }">
          <el-popconfirm title="确认删除？" @confirm="remove(row)">
            <template #reference>
              <el-button type="danger" link size="small">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination v-if="filteredList.length > pageSize" v-model:current-page="page" :page-size="pageSize"
      :total="filteredList.length" layout="total, prev, pager, next"
      style="margin-top:12px;justify-content:flex-end" />

    <el-empty v-if="!loading && list.length === 0" description="暂无黑名单规则" style="padding:40px 0" />

    <!-- 添加单条 -->
    <el-dialog v-model="showAdd" title="添加黑名单规则" width="460px">
      <el-form :model="addForm" label-position="top">
        <el-form-item label="类型">
          <el-select v-model="addForm.type" style="width:100%">
            <el-option label="域名" value="domain" />
            <el-option label="IP 地址" value="ip" />
            <el-option label="CIDR 网段" value="cidr" />
            <el-option label="通配符" value="wildcard" />
          </el-select>
        </el-form-item>
        <el-form-item label="值">
          <el-input v-model="addForm.value" :placeholder="addPlaceholder" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="addForm.remark" placeholder="可选" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAdd=false">取消</el-button>
        <el-button type="primary" :loading="addLoading" @click="submitAdd">添加</el-button>
      </template>
    </el-dialog>

    <!-- 批量导入 -->
    <el-dialog v-model="showBatchAdd" title="批量导入黑名单" width="520px">
      <el-form label-position="top">
        <el-form-item label="类型">
          <el-select v-model="batchType" style="width:100%">
            <el-option label="域名" value="domain" />
            <el-option label="IP 地址" value="ip" />
            <el-option label="CIDR 网段" value="cidr" />
            <el-option label="通配符" value="wildcard" />
          </el-select>
        </el-form-item>
        <el-form-item label="内容（每行一条）">
          <el-input v-model="batchText" type="textarea" :rows="8" placeholder="每行一个，如：&#10;*.gov.cn&#10;10.0.0.0/8&#10;192.168.0.0/16" style="font-family:monospace" />
          <div style="font-size:11px;color:var(--el-text-color-secondary);margin-top:4px">当前 {{ batchCount }} 条</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showBatchAdd=false">取消</el-button>
        <el-button type="primary" :loading="batchLoading" @click="submitBatch">导入</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { blacklistApi, type BlacklistEntry } from '@/api'

const list = ref<BlacklistEntry[]>([])
const loading = ref(false)
const search = ref('')
const filterType = ref('')
const page = ref(1)
const pageSize = 20

const filteredList = computed(() => {
  return list.value.filter(e => {
    if (filterType.value && e.type !== filterType.value) return false
    if (search.value && !e.value.includes(search.value) && !(e.remark || '').includes(search.value)) return false
    return true
  })
})

const pagedList = computed(() =>
  filteredList.value.slice((page.value - 1) * pageSize, page.value * pageSize)
)

function typeLabel(t: string) {
  return ({ domain: '域名', ip: 'IP', cidr: 'CIDR', wildcard: '通配符' } as Record<string, string>)[t] ?? t
}
function typeTag(t: string) {
  return ({ domain: '', ip: 'success', cidr: 'warning', wildcard: 'info' } as Record<string, string>)[t] ?? 'info'
}
function fmt(t?: string) {
  if (!t) return '—'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

async function load() {
  loading.value = true
  try {
    const res = await blacklistApi.list()
    list.value = res.data ?? []
  } catch { list.value = [] }
  finally { loading.value = false }
}

// 添加
const showAdd = ref(false)
const addLoading = ref(false)
const addForm = ref({ type: 'domain', value: '', remark: '' })
const addPlaceholder = computed(() => {
  return ({ domain: 'example.com', ip: '192.168.1.1', cidr: '10.0.0.0/8', wildcard: '*.gov.cn' } as Record<string, string>)[addForm.value.type] ?? ''
})

async function submitAdd() {
  if (!addForm.value.value.trim()) { ElMessage.warning('请填写值'); return }
  addLoading.value = true
  try {
    await blacklistApi.add(addForm.value)
    ElMessage.success('已添加')
    showAdd.value = false
    addForm.value = { type: 'domain', value: '', remark: '' }
    await load()
  } catch (e: any) { ElMessage.error(e.message || '添加失败') }
  finally { addLoading.value = false }
}

// 批量导入
const showBatchAdd = ref(false)
const batchLoading = ref(false)
const batchType = ref('domain')
const batchText = ref('')
const batchCount = computed(() => batchText.value.split('\n').filter(l => l.trim()).length)

async function submitBatch() {
  const lines = batchText.value.split('\n').map(l => l.trim()).filter(Boolean)
  if (!lines.length) { ElMessage.warning('内容不能为空'); return }
  batchLoading.value = true
  try {
    const items = lines.map(v => ({ type: batchType.value, value: v, remark: '' }))
    await blacklistApi.batchAdd({ items })
    ElMessage.success(`已导入 ${lines.length} 条`)
    showBatchAdd.value = false
    batchText.value = ''
    await load()
  } catch (e: any) { ElMessage.error(e.message || '导入失败') }
  finally { batchLoading.value = false }
}

// 删除
async function remove(row: BlacklistEntry) {
  try {
    await blacklistApi.remove(row.id)
    ElMessage.success('已删除')
    await load()
  } catch (e: any) { ElMessage.error(e.message || '删除失败') }
}

async function clearAll() {
  try {
    await blacklistApi.clear()
    ElMessage.success('已清空')
    list.value = []
  } catch (e: any) { ElMessage.error(e.message || '清空失败') }
}

onMounted(load)
</script>

<style scoped>
.filter-bar { display: flex; align-items: center; gap: 8px; }
</style>
