<template>
  <div>
    <div class="page-card">
      <div class="page-header">
        <h2 class="page-title">项目管理</h2>
        <div class="header-actions">
          <el-popconfirm title="批量删除选中项目？" @confirm="batchRemove">
            <template #reference>
              <el-button type="danger" plain :disabled="!selected.length">
                <el-icon><Delete /></el-icon>批量删除({{ selected.length }})
              </el-button>
            </template>
          </el-popconfirm>
          <el-button type="primary" @click="openCreate">
            <el-icon><Plus /></el-icon>新建项目
          </el-button>
        </div>
      </div>
    <el-table :data="list" v-loading="loading" style="width:100%" size="default" @selection-change="(rows: Project[]) => selected = rows">
      <el-table-column type="selection" width="42" />
      <el-table-column prop="name" label="项目名" min-width="150" show-overflow-tooltip />
      <el-table-column prop="description" label="描述" show-overflow-tooltip />
      <el-table-column label="创建时间" width="180">
        <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="140" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="openEdit(row)">编辑</el-button>
          <el-divider direction="vertical" />
          <el-popconfirm title="确认删除该项目？" @confirm="remove(row.id)">
            <template #reference>
              <el-button type="danger" link size="small">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-if="total > pageSize"
      v-model:current-page="page"
      :page-size="pageSize"
      :total="total"
      layout="total, prev, pager, next"
      style="margin-top:16px;justify-content:flex-end"
      @current-change="fetchList"
    />
    </div><!-- /page-card -->

    <el-dialog v-model="dialogVisible" :title="editing ? '编辑项目' : '新建项目'" width="460px">
      <el-form :model="form" label-position="top" style="margin-top:4px">
        <el-form-item label="项目名" required>
          <el-input v-model="form.name" placeholder="输入项目名称" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { projectApi, type Project } from '@/api'

const list = ref<Project[]>([])
const loading = ref(false)
const selected = ref<Project[]>([])
const saving = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = 20
const dialogVisible = ref(false)
const editing = ref<Project | null>(null)
const form = ref({ name: '', description: '' })

async function fetchList() {
  loading.value = true
  try {
    const res = await projectApi.list({ limit: pageSize, skip: (page.value - 1) * pageSize })
    list.value = res.data ?? []
    total.value = res.total
  } finally { loading.value = false }
}

function openCreate() {
  editing.value = null; form.value = { name: '', description: '' }; dialogVisible.value = true
}
function openEdit(row: Project) {
  editing.value = row; form.value = { name: row.name, description: row.description }; dialogVisible.value = true
}

async function save() {
  if (!form.value.name.trim()) { ElMessage.warning('项目名不能为空'); return }
  saving.value = true
  try {
    if (editing.value) { await projectApi.update(editing.value.id, form.value); ElMessage.success('更新成功') }
    else { await projectApi.create(form.value); ElMessage.success('创建成功') }
    dialogVisible.value = false; fetchList()
  } catch (e: any) { ElMessage.error(e.message) } finally { saving.value = false }
}

async function remove(id: string) {
  try { await projectApi.remove(id); ElMessage.success('已删除'); fetchList() }
  catch (e: any) { ElMessage.error(e.message) }
}
async function batchRemove() {
  try {
    await projectApi.batchRemove(selected.value.map(p => p.id))
    ElMessage.success(`已删除 ${selected.value.length} 个项目`)
    selected.value = []; fetchList()
  } catch (e: any) { ElMessage.error(e.message) }
}

function fmtTime(iso: string) { return new Date(iso).toLocaleString('zh-CN', { hour12: false }) }

onMounted(fetchList)
</script>

