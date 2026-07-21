<template>
  <div class="page-card">
    <div class="page-header">
      <h2 class="page-title">字典管理</h2>
    </div>

    <el-tabs v-model="activeTab" class="dict-tabs" @tab-change="fetchDicts">
      <el-tab-pane v-for="cat in categories" :key="cat.value" :label="cat.label" :name="cat.value">
        <div class="tab-toolbar">
          <div class="tab-desc">{{ cat.desc }}</div>
          <el-button type="primary" size="small" @click="openAdd(cat.value)">
            <el-icon style="margin-right:4px"><Plus /></el-icon>添加字典
          </el-button>
        </div>
        <el-table :data="dicts" v-loading="loading" size="small" style="margin-top:12px">
          <el-table-column prop="name" label="字典名称" min-width="180" />
          <el-table-column v-if="activeTab === 'password'" label="关联协议" width="110">
            <template #default="{ row }">
              <el-tag v-if="row.service" size="small" type="primary" effect="plain">{{ row.service.toUpperCase() }}</el-tag>
              <el-tag v-else size="small" type="info" effect="plain">通用</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="词条数" width="100">
            <template #default="{ row }">{{ (row.count || 0).toLocaleString() }}</template>
          </el-table-column>
          <el-table-column label="类型" width="90">
            <template #default="{ row }">
              <el-tag :type="row.builtin ? 'info' : 'success'" size="small">{{ row.builtin ? '内置' : '自定义' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="说明" min-width="200">
            <template #default="{ row }">
              <span style="color:var(--el-text-color-secondary);font-size:12px">{{ row.description }}</span>
            </template>
          </el-table-column>
          <el-table-column label="启用状态" width="90" align="center">
            <template #default="{ row }">
              <el-switch v-model="row.active" size="small" :disabled="row.builtin && row.category !== 'password'" @change="toggleDict(row)" />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="180" align="center">
            <template #default="{ row }">
              <el-button v-if="!row.builtin || row.category === 'password'" type="primary" link size="small" @click="editDict(row)">编辑</el-button>
              <el-button type="primary" link size="small" @click="previewDict(row)">预览</el-button>
              <el-popconfirm v-if="!row.builtin" title="确认删除？" @confirm="deleteDict(row)">
                <template #reference>
                  <el-button type="danger" link size="small">删除</el-button>
                </template>
              </el-popconfirm>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <!-- 添加字典 -->
    <el-dialog v-model="showAdd" title="添加字典" width="520px" @close="resetForm">
      <el-form :model="form" label-position="top">
        <template v-if="form.category === 'password'">
          <el-form-item label="关联协议">
            <el-select v-model="form.service" placeholder="选择协议（决定字典在哪个爆破插件里出现）" style="width:100%">
              <el-option value="" label="通用（所有协议可选）" />
              <el-option v-for="s in bruteServices" :key="s.value" :value="s.value" :label="s.label" />
            </el-select>
          </el-form-item>
        </template>
        <el-form-item label="字典名称">
          <el-input v-model="form.name" placeholder="例如：常见后台路径" />
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="form.description" placeholder="可选，简要描述字典用途" />
        </el-form-item>
        <el-form-item label="导入方式">
          <el-radio-group v-model="form.method">
            <el-radio value="paste">粘贴内容</el-radio>
            <el-radio value="upload">上传文件</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.method === 'paste'" label="内容">
          <el-input v-model="form.content" type="textarea" :rows="8"
            :placeholder="form.category === 'password' ? '每行一组凭据，格式 user:pass\\n例:\\nroot:root\\nadmin:admin123\\n:6379password  (redis 空用户)' : '每行一个词条'" />
          <div style="font-size:11px;color:var(--el-text-color-secondary);margin-top:4px">当前 {{ lineCount }} 行</div>
        </el-form-item>
        <el-form-item v-else label="文件">
          <el-upload
            ref="uploadRef"
            action="#"
            :auto-upload="false"
            accept=".txt,.csv,.dic"
            :limit="1"
            :on-change="onFileChange"
          >
            <el-button size="small">选择文件（.txt / .csv / .dic）</el-button>
          </el-upload>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAdd = false">取消</el-button>
        <el-button type="primary" :loading="addLoading" @click="addDict">确认添加</el-button>
      </template>
    </el-dialog>

    <!-- 编辑字典（内容/说明） -->
    <el-dialog v-model="showEdit" :title="`编辑字典 — ${editForm.name}`" width="640px" top="6vh">
      <el-form :model="editForm" label-position="top">
        <el-form-item label="字典名称">
          <el-input v-model="editForm.name" />
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="editForm.description" />
        </el-form-item>
        <el-form-item v-if="editForm.category === 'password'" label="关联协议">
          <el-select v-model="editForm.service" style="width:100%">
            <el-option value="" label="通用" />
            <el-option v-for="s in bruteServices" :key="s.value" :value="s.value" :label="s.label" />
          </el-select>
        </el-form-item>
        <el-form-item label="内容">
          <el-input v-model="editForm.content" type="textarea" :rows="14"
            :placeholder="editForm.category === 'password' ? '每行 user:pass，例:\\nroot:root\\nadmin:admin123' : '每行一个词条'"
            style="font-family:monospace;font-size:12px" />
          <div style="font-size:11px;color:var(--el-text-color-secondary);margin-top:4px">当前 {{ editLineCount }} 行</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEdit = false">取消</el-button>
        <el-button type="primary" :loading="editSaving" @click="saveEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 预览字典 -->
    <el-drawer v-model="showPreview" :title="`预览 — ${previewName}`" size="480px">
      <div v-if="previewLines.length" class="preview-content">
        <div v-for="(line, i) in previewLines" :key="i" class="preview-line">
          <span class="line-num">{{ i + 1 }}</span>
          <span>{{ line }}</span>
        </div>
        <div v-if="previewTotal > previewLines.length" class="preview-more">
          ... 共 {{ previewTotal.toLocaleString() }} 条，仅显示前 200 条
        </div>
      </div>
      <el-empty v-else description="字典为空" />
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { dictApi, type DictEntry } from '@/api'

type DictCategory = 'subdomain' | 'directory' | 'password'

const categories: { value: DictCategory; label: string; desc: string }[] = [
  { value: 'subdomain', label: '子域名字典', desc: '用于子域名爆破（ksubdomain 等插件），包含常见子域名前缀' },
  { value: 'directory', label: '目录字典', desc: '用于 Web 目录/路径爆破，包含常见后台路径、敏感文件等' },
  { value: 'password',  label: '弱口令字典', desc: '用于弱口令检测，按协议分开管理用户名/密码（brute-ssh、brute-mysql 等插件用）' },
]

const bruteServices = [
  { value: 'ssh', label: 'SSH' },
  { value: 'ftp', label: 'FTP' },
  { value: 'mysql', label: 'MySQL' },
  { value: 'redis', label: 'Redis' },
  { value: 'mongodb', label: 'MongoDB' },
  { value: 'postgresql', label: 'PostgreSQL' },
  { value: 'mssql', label: 'MSSQL' },
]

const activeTab = ref('subdomain')
const dicts = ref<DictEntry[]>([])
const loading = ref(false)

async function fetchDicts() {
  loading.value = true
  try {
    const r = await dictApi.list(activeTab.value).catch(() => ({ data: [] as DictEntry[] }))
    dicts.value = r.data || []
  } finally { loading.value = false }
}

// 添加字典
const showAdd = ref(false)
const addLoading = ref(false)
const form = ref({
  category: 'subdomain' as 'subdomain' | 'directory' | 'password',
  service: '',
  name: '', description: '',
  method: 'paste' as 'paste' | 'upload', content: '',
})

function openAdd(category: 'subdomain' | 'directory' | 'password') {
  form.value.category = category
  form.value.service = ''
  form.value.name = ''
  form.value.description = ''
  form.value.method = 'paste'
  form.value.content = ''
  fileContent.value = ''
  showAdd.value = true
}

// ── 编辑字典 ────────────────────────────────────────────────────────────
const showEdit = ref(false)
const editSaving = ref(false)
const editForm = ref<{ id: string; name: string; description: string; category: string; service: string; builtin: boolean; content: string }>({
  id: '', name: '', description: '', category: '', service: '', builtin: false, content: '',
})
const editLineCount = computed(() => editForm.value.content ? editForm.value.content.split('\n').filter(Boolean).length : 0)

async function editDict(row: DictEntry) {
  editSaving.value = false
  editForm.value = {
    id: row.id, name: row.name, description: row.description || '',
    category: row.category, service: row.service || '', builtin: !!row.builtin,
    content: '',
  }
  showEdit.value = true
  try {
    const r = await dictApi.getContent(row.id)
    editForm.value.content = r.content || ''
  } catch (e: any) {
    ElMessage.error('加载字典内容失败: ' + e.message)
  }
}

async function saveEdit() {
  editSaving.value = true
  try {
    const meta: Record<string, any> = {
      name: editForm.value.name,
      description: editForm.value.description,
    }
    if (editForm.value.category === 'password') meta.service = editForm.value.service
    await dictApi.update(editForm.value.id, meta)
    await dictApi.setContent(editForm.value.id, editForm.value.content)
    ElMessage.success('已保存')
    showEdit.value = false
    fetchDicts()
  } catch (e: any) {
    ElMessage.error('保存失败: ' + e.message)
  } finally {
    editSaving.value = false
  }
}
const fileContent = ref('')

const lineCount = computed(() => {
  const text = form.value.method === 'paste' ? form.value.content : fileContent.value
  return text ? text.split('\n').filter(Boolean).length : 0
})

function onFileChange(file: any) {
  const reader = new FileReader()
  reader.onload = (e) => { fileContent.value = (e.target?.result as string) || '' }
  reader.readAsText(file.raw)
}

function resetForm() {
  form.value = {
    category: activeTab.value as any,
    service: '',
    name: '', description: '', method: 'paste', content: '',
  }
  fileContent.value = ''
}

async function addDict() {
  if (!form.value.name) { ElMessage.warning('请填写字典名称'); return }
  const text = form.value.method === 'paste' ? form.value.content : fileContent.value
  if (!text || text.split('\n').filter(Boolean).length === 0) { ElMessage.warning('字典内容不能为空'); return }
  addLoading.value = true
  try {
    await dictApi.create({
      category: form.value.category,
      service: form.value.category === 'password' ? form.value.service : '',
      name: form.value.name,
      description: form.value.description,
      content: text,
    })
    showAdd.value = false
    resetForm()
    ElMessage.success('字典已添加')
    fetchDicts()
  } catch (e: any) { ElMessage.error(e.message || '添加失败') }
  finally { addLoading.value = false }
}

async function toggleDict(d: DictEntry) {
  await dictApi.update(d.id, { active: d.active }).catch(() => {})
  ElMessage.success(d.active ? `已启用: ${d.name}` : `已停用: ${d.name}`)
}

async function deleteDict(d: DictEntry) {
  await dictApi.remove(d.id)
  ElMessage.success('已删除')
  fetchDicts()
}

// 预览
const showPreview = ref(false)
const previewName = ref('')
const previewLines = ref<string[]>([])
const previewTotal = ref(0)

async function previewDict(d: DictEntry) {
  previewName.value = d.name
  previewLines.value = []
  previewTotal.value = 0
  showPreview.value = true
  try {
    const r = await dictApi.preview(d.id, { limit: 200 })
    previewLines.value = r.lines || []
    previewTotal.value = r.total
  } catch (e: any) {
    ElMessage.error('预览失败: ' + (e?.message || e))
  }
}

onMounted(() => { fetchDicts() })
</script>

<style scoped>
.dict-tabs { margin-top: 8px; }
.tab-toolbar { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 4px; }
.tab-desc { font-size: 13px; color: var(--el-text-color-secondary); line-height: 1.5; flex: 1; }
.preview-content { font-family: 'SF Mono', Monaco, Consolas, monospace; font-size: 12px; line-height: 1.8; }
.preview-line { display: flex; gap: 12px; padding: 0 4px; border-bottom: 1px solid var(--el-border-color-extra-light); }
.preview-line:hover { background: var(--el-fill-color-light); }
.line-num { color: var(--el-text-color-placeholder); min-width: 36px; text-align: right; user-select: none; }
.preview-more { margin-top: 12px; text-align: center; color: var(--el-text-color-secondary); font-size: 12px; }
</style>
