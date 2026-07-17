<template>
  <div class="page-card">
    <div class="page-header">
      <h2 class="page-title">指纹管理</h2>
      <div class="header-actions">
        <el-button size="small" type="success" :loading="syncing" @click="syncOnline">
          <el-icon><Download /></el-icon>重置内置指纹
        </el-button>
        <el-button size="small" @click="showImport = true">
          <el-icon><Upload /></el-icon>导入指纹库
        </el-button>
        <el-button type="primary" @click="openForm()">
          <el-icon><Plus /></el-icon>添加指纹
        </el-button>
      </div>
    </div>

    <el-tabs v-model="activeTab" @tab-change="onTabChange">
      <!-- ══ 被动指纹 ══ -->
      <el-tab-pane name="passive">
        <template #label>
          <span>被动指纹 <el-badge v-if="passiveTotal" :value="passiveTotal" :max="99999" class="tab-badge" /></span>
        </template>
      </el-tab-pane>

      <!-- ══ 主动指纹 ══ -->
      <el-tab-pane name="active">
        <template #label>
          <span>主动指纹 <el-badge v-if="activeTotal" :value="activeTotal" :max="99999" class="tab-badge" /></span>
        </template>
      </el-tab-pane>
    </el-tabs>

    <!-- 筛选栏 -->
    <div class="filter-bar">
      <el-input v-model="filter.keyword" clearable placeholder="搜索名称 / 厂商 / 关键词" style="width:220px" @keyup.enter="fetchList(true)">
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-select v-model="filter.parent_category" clearable placeholder="大类" style="width:160px" @change="fetchList(true)">
        <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
      </el-select>
      <el-select v-model="filter.location" clearable placeholder="匹配位置" style="width:130px" @change="fetchList(true)">
        <el-option value="header" label="Header" />
        <el-option value="body" label="Body" />
        <el-option value="title" label="Title" />
        <el-option value="favicon" label="Favicon" />
        <el-option value="cert" label="证书" />
        <el-option value="port" label="端口" />
      </el-select>
      <el-select v-model="filter.enabled" clearable placeholder="状态" style="width:100px" @change="fetchList(true)">
        <el-option value="true" label="已启用" />
        <el-option value="false" label="已停用" />
      </el-select>
      <el-button type="primary" @click="fetchList(true)">搜索</el-button>
      <el-button @click="resetFilter">重置</el-button>
      <div style="margin-left:auto;display:flex;gap:8px">
        <el-popconfirm :title="`确认清空所有${activeTab === 'passive' ? '被动' : '主动'}指纹？`" @confirm="clearAll">
          <template #reference>
            <el-button size="small" type="danger" plain>清空</el-button>
          </template>
        </el-popconfirm>
      </div>
    </div>

    <el-table :data="list" v-loading="loading" max-height="560" class="fp-table">
      <el-table-column prop="name" label="指纹名称" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="fp-name">{{ row.name }}</span>
        </template>
      </el-table-column>
      <el-table-column label="大类" width="140">
        <template #default="{ row }">
          <el-tag type="info" size="small">{{ row.parent_category }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="category" label="小类" width="130" show-overflow-tooltip>
        <template #default="{ row }">
          <span style="font-size:12px;color:var(--el-text-color-secondary)">{{ row.category }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="company" label="厂商" min-width="140" show-overflow-tooltip>
        <template #default="{ row }">
          <span style="font-size:12px;color:var(--el-text-color-secondary)">{{ row.company }}</span>
        </template>
      </el-table-column>
      <el-table-column label="匹配位置" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="locType(row.location)" size="small">{{ row.location }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="keyword" label="关键词" min-width="200" show-overflow-tooltip>
        <template #default="{ row }">
          <span class="fp-keyword">{{ row.keyword }}</span>
        </template>
      </el-table-column>
      <el-table-column label="来源" width="70" align="center">
        <template #default="{ row }">
          <el-tag :type="row.builtin ? 'info' : 'success'" size="small">{{ row.builtin ? '内置' : '自定' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="启用" width="70" align="center">
        <template #default="{ row }">
          <el-switch v-model="row.enabled" size="small" @change="toggleEnabled(row)" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="120" fixed="right" align="center">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="openForm(row)">编辑</el-button>
          <el-divider direction="vertical" />
          <el-popconfirm title="确认删除？" @confirm="doDelete(row)">
            <template #reference>
              <el-button type="danger" link size="small">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination v-model:current-page="page" :page-size="pageSize" :total="total"
      :page-sizes="[20, 50, 100, 200]" layout="total, sizes, prev, pager, next"
      style="margin-top:12px;justify-content:flex-end"
      @size-change="(s: number) => { pageSize=s; fetchList(true) }"
      @current-change="fetchList()" />

    <!-- ══ 添加/编辑指纹 ══ -->
    <el-dialog v-model="showForm" :title="editingId ? '编辑指纹' : '添加指纹'" width="540px">
      <el-form :model="form" label-position="top">
        <el-form-item label="指纹名称">
          <el-input v-model="form.name" placeholder="如: WordPress, Shiro" />
        </el-form-item>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="大类">
              <el-select v-model="form.parent_category" filterable allow-create style="width:100%">
                <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="小类">
              <el-input v-model="form.category" placeholder="如: CMS, Framework" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="厂商">
          <el-input v-model="form.company" placeholder="厂商名称" />
        </el-form-item>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="指纹类型">
              <el-radio-group v-model="form.fp_type">
                <el-radio value="passive">被动</el-radio>
                <el-radio value="active">主动</el-radio>
              </el-radio-group>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="匹配位置">
              <el-select v-model="form.location" style="width:100%">
                <el-option value="header" label="Header" />
                <el-option value="body" label="Body" />
                <el-option value="title" label="Title" />
                <el-option value="favicon" label="Favicon" />
                <el-option value="cert" label="证书" />
                <el-option value="port" label="端口" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="匹配类型">
          <el-radio-group v-model="form.match_type">
            <el-radio value="contains">包含</el-radio>
            <el-radio value="regex">正则</el-radio>
            <el-radio value="md5">MD5</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="关键词">
          <el-input v-model="form.keyword" type="textarea" :rows="3" placeholder="匹配的关键词、正则或 Hash" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showForm = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveForm">保存</el-button>
      </template>
    </el-dialog>

    <!-- ══ 导入指纹库 ══ -->
    <el-dialog v-model="showImport" title="导入指纹库" width="520px"
      :close-on-click-modal="!importLoading" :show-close="!importLoading">
      <template v-if="!importLoading">
        <el-form label-position="top">
          <el-form-item label="指纹类型">
            <el-radio-group v-model="importFpType">
              <el-radio value="passive">被动指纹</el-radio>
              <el-radio value="active">主动指纹</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="导入文件">
            <el-upload ref="importUploadRef" drag :auto-upload="false" :limit="1" accept=".json"
              :on-change="(f: any) => importFile = f.raw"
              :on-exceed="() => ElMessage.warning('只能上传一个文件')">
              <el-icon class="el-icon--upload" style="font-size:48px;color:#c0c4cc"><Upload /></el-icon>
              <div class="el-upload__text">拖拽 JSON 文件到此处，或 <em>点击上传</em></div>
              <template #tip><div class="el-upload__tip">支持 JSON 数组格式的指纹数据</div></template>
            </el-upload>
          </el-form-item>
        </el-form>
      </template>
      <template v-else>
        <el-progress :percentage="importProgress" :stroke-width="20" striped striped-flow
          :status="importDone ? 'success' : importFailed ? 'exception' : ''" />
        <div style="text-align:center;margin-top:12px;font-size:13px;color:var(--el-text-color-regular)">{{ importMsg }}</div>
      </template>
      <template #footer>
        <template v-if="importDone || importFailed">
          <el-button type="primary" @click="showImport=false;importLoading=false;importDone=false;importFailed=false">关闭</el-button>
        </template>
        <template v-else-if="!importLoading">
          <el-button @click="showImport=false">取消</el-button>
          <el-button type="primary" :disabled="!importFile" @click="doImport">开始导入</el-button>
        </template>
        <template v-else>
          <el-button disabled>导入中…</el-button>
        </template>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { fingerprintApi, type FingerprintEntry } from '@/api'

const activeTab = ref('passive')
const list = ref<FingerprintEntry[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)
const passiveTotal = ref(0)
const activeTotal = ref(0)
const categories = ref<string[]>([])
const filter = reactive({ keyword: '', parent_category: '', location: '', enabled: '' })

function onTabChange() {
  fetchList(true)
}

async function fetchList(reset = false) {
  if (reset) page.value = 1
  loading.value = true
  try {
    const r = await fingerprintApi.list({
      ...filter,
      fp_type: activeTab.value,
      limit: pageSize.value,
      skip: (page.value - 1) * pageSize.value,
    }).catch(() => ({ data: [] as FingerprintEntry[], total: 0 }))
    list.value = r.data || []
    total.value = r.total
  } finally { loading.value = false }
}

async function fetchCounts() {
  const [p, a] = await Promise.all([
    fingerprintApi.list({ fp_type: 'passive', limit: 1, skip: 0 }).catch(() => ({ total: 0 })),
    fingerprintApi.list({ fp_type: 'active', limit: 1, skip: 0 }).catch(() => ({ total: 0 })),
  ])
  passiveTotal.value = p.total
  activeTotal.value = a.total
}

async function fetchCategories() {
  categories.value = await fingerprintApi.categories().catch(() => [])
}

function resetFilter() {
  Object.assign(filter, { keyword: '', parent_category: '', location: '', enabled: '' })
  fetchList(true)
}

function locType(l: string) {
  return ({ header: 'primary', body: 'success', title: 'warning', favicon: 'info', cert: '', port: 'danger' } as Record<string, string>)[l] ?? 'info'
}

async function toggleEnabled(row: FingerprintEntry) {
  await fingerprintApi.update(row.id, { enabled: row.enabled }).catch(() => {})
}

async function doDelete(row: FingerprintEntry) {
  await fingerprintApi.remove(row.id)
  ElMessage.success('已删除')
  fetchList(); fetchCounts()
}

async function clearAll() {
  await fingerprintApi.clear()
  ElMessage.success('已清空')
  fetchList(true); fetchCounts()
}

// 在线同步
const syncing = ref(false)
async function syncOnline() {
  syncing.value = true
  try {
    const r = await fingerprintApi.syncOnline()
    ElMessage.success(`同步完成，共导入 ${r.count} 条指纹`)
    fetchList(true); fetchCounts(); fetchCategories()
  } catch (e: any) { ElMessage.error(e.message || '同步失败') }
  finally { syncing.value = false }
}

// 编辑表单
const showForm = ref(false)
const editingId = ref('')
const saving = ref(false)
const form = reactive({
  name: '', category: '', parent_category: 'Enterprise Application',
  company: '', match_type: 'contains', location: 'body',
  keyword: '', fp_type: 'passive',
})

function openForm(row?: FingerprintEntry) {
  if (row) {
    editingId.value = row.id
    Object.assign(form, {
      name: row.name, category: row.category, parent_category: row.parent_category,
      company: row.company, match_type: row.match_type, location: row.location,
      keyword: row.keyword, fp_type: row.fp_type,
    })
  } else {
    editingId.value = ''
    Object.assign(form, {
      name: '', category: '', parent_category: 'Enterprise Application',
      company: '', match_type: 'contains', location: 'body',
      keyword: '', fp_type: activeTab.value,
    })
  }
  showForm.value = true
}

async function saveForm() {
  if (!form.name) { ElMessage.warning('请填写指纹名称'); return }
  if (!form.keyword) { ElMessage.warning('请填写关键词'); return }
  saving.value = true
  try {
    if (editingId.value) {
      await fingerprintApi.update(editingId.value, { ...form })
    } else {
      await fingerprintApi.create({ ...form, enabled: true })
    }
    ElMessage.success('保存成功')
    showForm.value = false
    fetchList(); fetchCounts()
  } catch (e: any) { ElMessage.error(e.message || '保存失败') }
  finally { saving.value = false }
}

// 导入
const showImport = ref(false)
const importLoading = ref(false)
const importFile = ref<File | null>(null)
const importFpType = ref('passive')
const importProgress = ref(0)
const importMsg = ref('')
const importDone = ref(false)
const importFailed = ref(false)

async function doImport() {
  if (!importFile.value) return
  const fd = new FormData()
  fd.append('file', importFile.value)
  fd.append('fp_type', importFpType.value)
  importLoading.value = true; importProgress.value = 30; importMsg.value = '正在导入…'
  try {
    importProgress.value = 60
    const r = await fingerprintApi.import(fd)
    importProgress.value = 100; importDone.value = true
    importMsg.value = `导入完成，共 ${r.count} 条指纹`
    ElMessage.success(`导入 ${r.count} 条指纹`)
    fetchList(true); fetchCounts(); fetchCategories()
  } catch (e: any) {
    importFailed.value = true
    importMsg.value = e.message || '导入失败'
  }
}

onMounted(() => {
  fetchList(); fetchCounts(); fetchCategories()
})
</script>

<style scoped>
.tab-badge { margin-left: 4px; }
.tab-badge :deep(.el-badge__content) { font-size: 11px; }
.filter-bar { display: flex; align-items: center; gap: 8px; padding: 0 0 12px; flex-wrap: wrap; }
.fp-name { font-weight: 500; font-size: 13px; color: var(--el-text-color-primary); }
.fp-keyword { font-family: 'SF Mono', Monaco, Consolas, monospace; font-size: 12px; color: #6366f1; }
:deep(.el-upload-dragger) { padding: 24px; }
</style>
