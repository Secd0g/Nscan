<template>
  <div class="page-card">
    <div class="page-header">
      <h2 class="page-title">POC 管理</h2>
    </div>

    <el-tabs v-model="activeTab" @tab-change="onTabChange">

      <!-- ══ Nuclei 模板库 ══ -->
      <el-tab-pane name="templates">
        <template #label>
          <span>Nuclei 模板 <el-badge v-if="templateStats.total" :value="templateStats.total" :max="99999" class="tab-badge" /></span>
        </template>

        <!-- 统计条 + 操作栏 -->
        <div class="poc-toolbar">
          <div v-if="templateStats.total" class="stats-bar">
            <span class="sev-pill sev-critical">Critical {{ templateStats.critical || 0 }}</span>
            <span class="sev-pill sev-high">High {{ templateStats.high || 0 }}</span>
            <span class="sev-pill sev-medium">Medium {{ templateStats.medium || 0 }}</span>
            <span class="sev-pill sev-low">Low {{ templateStats.low || 0 }}</span>
            <span class="sev-pill sev-info">Info {{ templateStats.info || 0 }}</span>
          </div>
          <div style="display:flex;gap:8px;margin-left:auto">
            <el-button v-if="selectedTemplates.length" type="success" size="small"
              @click="showBatchValidate = true">
              批量验证 ({{ selectedTemplates.length }})
            </el-button>
            <el-button size="small" @click="clearTemplates">清空</el-button>
            <el-button type="primary" size="small" @click="showSyncDialog = true">
              <el-icon><Refresh /></el-icon>同步模板
            </el-button>
          </div>
        </div>

        <!-- 筛选栏 -->
        <div class="filter-bar">
          <el-select v-model="tplFilter.severity" clearable placeholder="等级" style="width:100px" @change="fetchTemplates(true)">
            <el-option value="critical" label="Critical" />
            <el-option value="high" label="High" />
            <el-option value="medium" label="Medium" />
            <el-option value="low" label="Low" />
            <el-option value="info" label="Info" />
          </el-select>
          <el-select v-model="tplFilter.category" clearable placeholder="分类" style="width:120px" @change="fetchTemplates(true)">
            <el-option v-for="c in templateCategories" :key="c" :label="c" :value="c" />
          </el-select>
          <el-input v-model="tplFilter.tag" clearable placeholder="标签: xss, rce" style="width:140px" @keyup.enter="fetchTemplates(true)">
            <template #prefix><el-icon><PriceTag /></el-icon></template>
          </el-input>
          <el-input v-model="tplFilter.keyword" clearable placeholder="搜索名称/ID" style="width:180px" @keyup.enter="fetchTemplates(true)">
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-button type="primary" @click="fetchTemplates(true)">搜索</el-button>
          <el-button @click="resetTplFilter">重置</el-button>
        </div>

        <el-table :data="templates" v-loading="tplLoading" max-height="560"
          @selection-change="selectedTemplates = $event" class="poc-table">
          <el-table-column type="selection" width="40" />
          <el-table-column label="模板 ID" width="220">
            <template #default="{ row }">
              <span class="tpl-id">{{ row.template_id }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="name" label="名称" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="poc-name">{{ row.name }}</span>
            </template>
          </el-table-column>
          <el-table-column label="等级" width="90" align="center">
            <template #default="{ row }">
              <span :class="['sev-dot', `sev-${row.severity}`]">{{ row.severity }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="category" label="分类" width="110">
            <template #default="{ row }">
              <span style="color:var(--el-text-color-secondary);font-size:12px">{{ row.category }}</span>
            </template>
          </el-table-column>
          <el-table-column label="标签" min-width="200">
            <template #default="{ row }">
              <div class="tag-wrap">
                <span v-for="t in (row.tags||[]).slice(0,5)" :key="t" class="poc-tag">{{ t }}</span>
                <span v-if="(row.tags||[]).length>5" class="poc-tag-more">+{{ row.tags.length-5 }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="author" label="作者" width="100" show-overflow-tooltip>
            <template #default="{ row }">
              <span style="font-size:12px;color:var(--el-text-color-secondary)">{{ row.author }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120" fixed="right" align="center">
            <template #default="{ row }">
              <el-button type="primary" link size="small" @click="viewTplContent(row)">查看</el-button>
              <el-divider direction="vertical" />
              <el-button type="success" link size="small" @click="openValidate(row, true)">验证</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-pagination v-model:current-page="tplPage" :page-size="tplPageSize" :total="tplTotal"
          :page-sizes="[50, 100, 200]" layout="total, sizes, prev, pager, next"
          style="margin-top:12px;justify-content:flex-end"
          @size-change="(s: number) => { tplPageSize=s; fetchTemplates(true) }"
          @current-change="fetchTemplates()" />
      </el-tab-pane>

      <!-- ══ 自定义 POC ══ -->
      <el-tab-pane name="custom">
        <template #label>
          <span>自定义 POC <el-badge v-if="pocTotal" :value="pocTotal" :max="99999" class="tab-badge" /></span>
        </template>

        <div class="poc-toolbar">
          <div style="display:flex;gap:8px;margin-left:auto">
            <el-button size="small" :loading="pocClearLoading" @click="clearPocs">清空</el-button>
            <el-button size="small" :loading="pocExportLoading" @click="exportPocs">
              <el-icon><Download /></el-icon>导出
            </el-button>
            <el-button size="small" @click="showImportDialog = true">
              <el-icon><Upload /></el-icon>批量导入
            </el-button>
            <el-button type="primary" size="small" @click="openPocForm()">
              <el-icon><Plus /></el-icon>添加 POC
            </el-button>
          </div>
        </div>

        <!-- 筛选栏 -->
        <div class="filter-bar">
          <el-input v-model="pocFilter.name" clearable placeholder="POC 名称" style="width:140px" @keyup.enter="fetchPocs(true)">
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-input v-model="pocFilter.template_id" clearable placeholder="模板 ID" style="width:150px" @keyup.enter="fetchPocs(true)" />
          <el-select v-model="pocFilter.severity" clearable placeholder="等级" style="width:100px" @change="fetchPocs(true)">
            <el-option value="critical" label="Critical" />
            <el-option value="high" label="High" />
            <el-option value="medium" label="Medium" />
            <el-option value="low" label="Low" />
            <el-option value="info" label="Info" />
          </el-select>
          <el-select v-model="pocFilter.enabled" clearable placeholder="状态" style="width:90px" @change="fetchPocs(true)">
            <el-option :value="true" label="已启用" />
            <el-option :value="false" label="已停用" />
          </el-select>
          <el-button type="primary" @click="fetchPocs(true)">搜索</el-button>
          <el-button @click="resetPocFilter">重置</el-button>
        </div>

        <el-table :data="pocs" v-loading="pocLoading" max-height="560" class="poc-table">
          <el-table-column label="名称" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="poc-name">{{ row.name }}</span>
            </template>
          </el-table-column>
          <el-table-column label="模板 ID" width="220" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="tpl-id">{{ row.template_id }}</span>
            </template>
          </el-table-column>
          <el-table-column label="等级" width="90" align="center">
            <template #default="{ row }">
              <span :class="['sev-dot', `sev-${row.severity}`]">{{ row.severity }}</span>
            </template>
          </el-table-column>
          <el-table-column label="标签" min-width="200">
            <template #default="{ row }">
              <div class="tag-wrap">
                <span v-for="t in (row.tags||[])" :key="t" class="poc-tag">{{ t }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="70" align="center">
            <template #default="{ row }">
              <span :class="['status-indicator', row.enabled ? 'enabled' : 'disabled']">{{ row.enabled ? '启用' : '停用' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="160" fixed="right" align="center">
            <template #default="{ row }">
              <el-button type="success" link size="small" @click="openValidate(row, false)">验证</el-button>
              <el-divider direction="vertical" />
              <el-button type="primary" link size="small" @click="openPocForm(row)">编辑</el-button>
              <el-divider direction="vertical" />
              <el-popconfirm title="确认删除此 POC？" @confirm="deletePoc(row)">
                <template #reference>
                  <el-button type="danger" link size="small">删除</el-button>
                </template>
              </el-popconfirm>
            </template>
          </el-table-column>
        </el-table>
        <el-pagination v-model:current-page="pocPage" :page-size="pocPageSize" :total="pocTotal"
          :page-sizes="[20, 50, 100]" layout="total, sizes, prev, pager, next"
          style="margin-top:12px;justify-content:flex-end"
          @size-change="(s: number) => { pocPageSize=s; fetchPocs(true) }"
          @current-change="fetchPocs()" />
      </el-tab-pane>

    </el-tabs>

    <!-- ══ 同步模板对话框 ══ -->
    <el-dialog v-model="showSyncDialog" title="同步 Nuclei 模板库" width="520px"
      :close-on-click-modal="!syncLoading" :show-close="!syncLoading">
      <template v-if="!syncLoading">
        <el-radio-group v-model="syncMode" style="margin-bottom:16px">
          <el-radio-button value="online">在线同步</el-radio-button>
          <el-radio-button value="upload">手动导入</el-radio-button>
        </el-radio-group>

        <template v-if="syncMode === 'online'">
          <el-alert type="info" :closable="false" style="margin-bottom:12px">
            从 GitHub 自动下载最新 nuclei-templates 并导入，文件较大（~50MB），请耐心等待。
          </el-alert>
          <div style="font-size:13px;color:var(--el-text-color-secondary)">
            数据来源：<a href="https://github.com/projectdiscovery/nuclei-templates" target="_blank" style="color:#409EFF">projectdiscovery/nuclei-templates</a>
          </div>
        </template>
        <template v-else>
          <el-alert type="info" :closable="false" style="margin-bottom:12px">
            上传 nuclei-templates 的 ZIP 压缩包，可从
            <a href="https://github.com/projectdiscovery/nuclei-templates/releases" target="_blank" style="color:#409EFF">GitHub Releases</a>
            下载。
          </el-alert>
          <el-upload ref="syncUploadRef" drag :auto-upload="false" :limit="1" accept=".zip"
            :on-change="(f: any) => syncZipFile = f.raw"
            :on-exceed="() => ElMessage.warning('只能上传一个文件')">
            <el-icon class="el-icon--upload" style="font-size:48px;color:#c0c4cc"><Upload /></el-icon>
            <div class="el-upload__text">拖拽 ZIP 到此处，或 <em>点击上传</em></div>
            <template #tip><div class="el-upload__tip">仅支持 .zip 格式</div></template>
          </el-upload>
          <div v-if="syncZipFile" style="margin-top:8px">
            <el-tag type="success">{{ (syncZipFile as File).name }} ({{ fmtBytes((syncZipFile as File).size) }})</el-tag>
          </div>
        </template>
      </template>
      <template v-else>
        <el-progress :percentage="syncProgress" :stroke-width="20" striped striped-flow
          :status="syncStatus === 'done' ? 'success' : syncStatus === 'failed' ? 'exception' : ''" />
        <div style="text-align:center;margin-top:12px;font-size:13px;color:var(--el-text-color-regular)">{{ syncMsg }}</div>
      </template>
      <template #footer>
        <template v-if="syncStatus === 'done' || syncStatus === 'failed'">
          <el-button type="primary" @click="showSyncDialog=false;syncLoading=false;syncStatus=''">关闭</el-button>
        </template>
        <template v-else-if="!syncLoading">
          <el-button @click="showSyncDialog=false">取消</el-button>
          <el-button v-if="syncMode === 'online'" type="primary" @click="doSyncOnline">开始在线同步</el-button>
          <el-button v-else type="primary" :disabled="!syncZipFile" @click="doSync">开始同步</el-button>
        </template>
        <template v-else>
          <el-button disabled>处理中…</el-button>
        </template>
      </template>
    </el-dialog>

    <!-- ══ 查看模板内容 ══ -->
    <el-dialog v-model="showTplContent" :title="currentTpl.name || '模板内容'" width="900px">
      <el-descriptions :column="2" border size="small" style="margin-bottom:14px">
        <el-descriptions-item label="模板 ID">{{ currentTpl.template_id }}</el-descriptions-item>
        <el-descriptions-item label="等级">
          <el-tag :type="sevType(currentTpl.severity||'')" size="small">{{ currentTpl.severity }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="分类">{{ currentTpl.category }}</el-descriptions-item>
        <el-descriptions-item label="作者">{{ currentTpl.author }}</el-descriptions-item>
        <el-descriptions-item label="标签" :span="2">
          <el-tag v-for="t in (currentTpl.tags||[])" :key="t" size="small" style="margin-right:4px">{{ t }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item v-if="currentTpl.description" label="描述" :span="2">{{ currentTpl.description }}</el-descriptions-item>
      </el-descriptions>
      <el-input :model-value="currentTpl.content" type="textarea" :rows="20" readonly
        style="font-family:monospace;font-size:12.5px" />
      <template #footer>
        <el-button @click="showTplContent=false">关闭</el-button>
        <el-button type="primary" @click="copyContent(currentTpl.content)">复制内容</el-button>
      </template>
    </el-dialog>

    <!-- ══ 自定义 POC 编辑 ══ -->
    <el-dialog v-model="showPocForm" :title="pocForm.id ? '编辑 POC' : '添加 POC'" width="900px">
      <el-form :model="pocForm" label-position="top">
        <el-form-item label="YAML 内容">
          <p style="font-size:12px;color:var(--el-text-color-secondary);margin:0 0 8px">粘贴 Nuclei YAML 模板，系统自动解析字段</p>
          <el-input v-model="pocForm.content" type="textarea" :rows="18"
            placeholder="id: my-poc&#10;info:&#10;  name: My POC&#10;  severity: high&#10;  ..."
            style="font-family:monospace;font-size:12.5px"
            @input="parseYaml" />
        </el-form-item>
        <el-divider content-position="left">解析结果</el-divider>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="模板 ID">
              <el-input v-model="pocForm.template_id" placeholder="从 YAML 中解析" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="名称">
              <el-input v-model="pocForm.name" placeholder="从 YAML 中解析" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="16">
          <el-col :span="8">
            <el-form-item label="危险等级">
              <el-select v-model="pocForm.severity" style="width:100%">
                <el-option value="critical" label="Critical" />
                <el-option value="high" label="High" />
                <el-option value="medium" label="Medium" />
                <el-option value="low" label="Low" />
                <el-option value="info" label="Info" />
              </el-select>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="作者">
              <el-input v-model="pocForm.author" placeholder="从 YAML 中解析" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="启用">
              <el-switch v-model="pocForm.enabled" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="标签（逗号分隔）">
          <el-input v-model="pocForm.tagsInput" placeholder="rce, apache, cve-2024-xxxx" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="pocForm.description" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showPocForm=false">取消</el-button>
        <el-button @click="parseYaml">重新解析</el-button>
        <el-button type="primary" :loading="pocSaving" @click="savePoc">保存</el-button>
      </template>
    </el-dialog>

    <!-- ══ 批量导入 POC ══ -->
    <el-dialog v-model="showImportDialog" title="批量导入 POC" width="520px"
      :close-on-click-modal="!importLoading" :show-close="!importLoading">
      <template v-if="!importLoading">
        <el-checkbox v-model="importEnabled" style="margin-bottom:12px">导入后自动启用</el-checkbox>
        <el-upload ref="importUploadRef" drag :auto-upload="false" :limit="1" accept=".zip"
          :on-change="(f: any) => importZipFile = f.raw"
          :on-exceed="() => ElMessage.warning('只能上传一个文件')">
          <el-icon class="el-icon--upload" style="font-size:48px;color:#c0c4cc"><Upload /></el-icon>
          <div class="el-upload__text">拖拽包含 .yaml 文件的 ZIP，或 <em>点击上传</em></div>
          <template #tip><div class="el-upload__tip">支持单层或多层目录的 ZIP 包</div></template>
        </el-upload>
        <div v-if="importZipFile" style="margin-top:8px">
          <el-tag type="success">{{ (importZipFile as File).name }} ({{ fmtBytes((importZipFile as File).size) }})</el-tag>
        </div>
      </template>
      <template v-else>
        <el-progress :percentage="importProgress" :stroke-width="20" striped striped-flow
          :status="importStatus === 'done' ? 'success' : importStatus === 'failed' ? 'exception' : ''" />
        <div style="text-align:center;margin-top:12px;font-size:13px;color:var(--el-text-color-regular)">{{ importMsg }}</div>
      </template>
      <template #footer>
        <template v-if="importStatus === 'done' || importStatus === 'failed'">
          <el-button type="primary" @click="showImportDialog=false;importLoading=false;importStatus=''">关闭</el-button>
        </template>
        <template v-else-if="!importLoading">
          <el-button @click="showImportDialog=false">取消</el-button>
          <el-button type="primary" :disabled="!importZipFile" @click="doImport">开始导入</el-button>
        </template>
        <template v-else>
          <el-button disabled>处理中…</el-button>
        </template>
      </template>
    </el-dialog>

    <!-- ══ POC 验证 ══ -->
    <el-dialog v-model="showValidate" title="验证 POC" width="700px" @close="validateLogs=[];validateResult=null">
      <el-form label-width="80px">
        <el-form-item label="POC">
          <el-input :model-value="validateTarget.name || validateTarget.id" disabled />
        </el-form-item>
        <el-form-item label="目标 URL">
          <el-input v-model="validateUrl" placeholder="https://example.com" clearable />
        </el-form-item>
      </el-form>

      <div v-if="validateLoading || validateLogs.length" class="validate-logs">
        <div class="logs-header">
          <span>执行日志</span>
          <el-tag v-if="validateLoading" type="warning" size="small">执行中</el-tag>
          <el-tag v-else-if="validateResult?.matched" type="danger" size="small">发现漏洞</el-tag>
          <el-tag v-else-if="validateResult" type="info" size="small">完成</el-tag>
        </div>
        <div class="logs-content" ref="logsRef">
          <div v-for="(log, i) in validateLogs" :key="i" :class="['log-line', `log-${(log.level||'').toLowerCase()}`]">
            <span class="log-time">{{ log.timestamp }}</span>
            <span class="log-level">[{{ log.level }}]</span>
            <span class="log-msg">{{ log.message }}</span>
          </div>
        </div>
      </div>

      <div v-if="validateResult && !validateLoading" class="validate-result">
        <el-tag :type="validateResult.matched ? 'danger' : 'info'" size="large">
          {{ validateResult.matched ? '✓ 发现漏洞' : '✗ 未匹配' }}
        </el-tag>
        <pre v-if="validateResult.details" class="result-pre">{{ validateResult.details }}</pre>
      </div>
      <template #footer>
        <el-button @click="showValidate=false">关闭</el-button>
        <el-button type="primary" :loading="validateLoading" :disabled="!validateUrl" @click="doValidate">开始验证</el-button>
      </template>
    </el-dialog>

    <!-- ══ 批量验证 ══ -->
    <el-dialog v-model="showBatchValidate" title="批量验证模板" width="820px" @close="batchResults=[]">
      <el-form label-width="100px">
        <el-form-item label="已选模板">
          <div style="display:flex;flex-wrap:wrap;gap:4px">
            <el-tag v-for="t in selectedTemplates.slice(0,10)" :key="t.id" size="small">{{ t.name||t.id }}</el-tag>
            <span v-if="selectedTemplates.length>10" style="font-size:12px;color:var(--el-text-color-secondary)">+{{ selectedTemplates.length-10 }} 个</span>
          </div>
        </el-form-item>
        <el-form-item label="目标 URL">
          <el-input v-model="batchUrls" type="textarea" :rows="4"
            placeholder="每行一个 URL&#10;https://example.com&#10;https://test.com" />
        </el-form-item>
      </el-form>
      <div v-if="batchLoading || batchResults.length" class="batch-progress">
        <div style="display:flex;align-items:center;gap:16px;margin-bottom:10px">
          <span style="font-size:13px">进度: {{ batchDone }}/{{ batchTotal }}</span>
          <el-progress :percentage="batchTotal>0 ? Math.round(batchDone/batchTotal*100) : 0"
            style="flex:1" :status="batchLoading ? '' : 'success'" />
          <el-tag v-if="batchResults.filter(r=>r.matched).length" type="danger" size="small">
            发现 {{ batchResults.filter((r: any)=>r.matched).length }} 个漏洞
          </el-tag>
        </div>
        <el-table v-if="batchResults.length" :data="batchResults" max-height="280" size="small">
          <el-table-column prop="pocName" label="模板名称" min-width="160" show-overflow-tooltip />
          <el-table-column label="等级" width="80">
            <template #default="{ row }"><el-tag :type="sevType(row.severity)" size="small">{{ row.severity }}</el-tag></template>
          </el-table-column>
          <el-table-column label="结果" width="80">
            <template #default="{ row }">
              <el-tag :type="row.matched ? 'danger' : 'info'" size="small">{{ row.matched ? '匹配' : '未匹配' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="matchedUrl" label="匹配 URL" min-width="200" show-overflow-tooltip />
        </el-table>
      </div>
      <template #footer>
        <el-button @click="showBatchValidate=false">关闭</el-button>
        <el-button type="primary" :loading="batchLoading" :disabled="!batchUrls.trim()" @click="doBatchValidate">开始验证</el-button>
      </template>
    </el-dialog>

  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { pocApi, type NucleiTemplate, type CustomPoc, type TemplateStats } from '@/api'

// ── Tab 状态 ─────────────────────────────────────
const activeTab = ref('templates')

function onTabChange(tab: string) {
  if (tab === 'templates' && templates.value.length === 0) fetchTemplates()
  if (tab === 'custom' && pocs.value.length === 0) fetchPocs()
}

// ── Nuclei 模板 ───────────────────────────────────
const templates = ref<NucleiTemplate[]>([])
const tplLoading = ref(false)
const tplPage = ref(1)
const tplPageSize = ref(50)
const tplTotal = ref(0)
const templateStats = ref<TemplateStats>({ total: 0, critical: 0, high: 0, medium: 0, low: 0, info: 0 })
const templateCategories = ref<string[]>([])
const selectedTemplates = ref<NucleiTemplate[]>([])
const tplFilter = reactive({ category: '', severity: '', tag: '', keyword: '' })

async function fetchTemplates(reset = false) {
  if (reset) tplPage.value = 1
  tplLoading.value = true
  try {
    const r = await pocApi.templates({
      ...tplFilter,
      limit: tplPageSize.value,
      skip: (tplPage.value - 1) * tplPageSize.value,
    }).catch(() => ({ data: [] as NucleiTemplate[], total: 0 }))
    templates.value = r.data; tplTotal.value = r.total
  } finally { tplLoading.value = false }
}
async function fetchTemplateStats() {
  templateStats.value = await pocApi.templateStats().catch(() => ({ total: 0, critical: 0, high: 0, medium: 0, low: 0, info: 0 }))
}
async function fetchTemplateCategories() {
  templateCategories.value = await pocApi.templateCategories().catch(() => [])
}
function resetTplFilter() { Object.assign(tplFilter, { category: '', severity: '', tag: '', keyword: '' }); fetchTemplates(true) }

// 查看模板内容
const showTplContent = ref(false)
const currentTpl = ref<Partial<NucleiTemplate>>({})
async function viewTplContent(row: NucleiTemplate) {
  currentTpl.value = { ...row }
  showTplContent.value = true
  if (!row.content) {
    const r = await pocApi.templateContent(row.template_id).catch(() => null)
    if (r) currentTpl.value = r
  }
}

// 同步模板
const showSyncDialog = ref(false)
const syncLoading = ref(false)
const syncZipFile = ref<File|null>(null)
const syncProgress = ref(0)
const syncStatus = ref('')
const syncMsg = ref('')
const syncMode = ref<'online'|'upload'>('online')

async function doSync() {
  if (!syncZipFile.value) return
  const fd = new FormData(); fd.append('file', syncZipFile.value)
  syncLoading.value = true; syncProgress.value = 10; syncMsg.value = '正在上传…'
  try {
    syncProgress.value = 40; syncMsg.value = '正在解压并导入…'
    await pocApi.syncTemplates(fd)
    syncProgress.value = 100; syncStatus.value = 'done'; syncMsg.value = '同步完成！'
    fetchTemplates(true); fetchTemplateStats()
    ElMessage.success('模板同步完成')
  } catch (e: any) {
    syncStatus.value = 'failed'; syncMsg.value = e.message || '同步失败'
  }
}

async function doSyncOnline() {
  syncLoading.value = true; syncProgress.value = 10; syncMsg.value = '正在从 GitHub 下载模板…'
  try {
    syncProgress.value = 30; syncMsg.value = '正在下载 nuclei-templates.zip（约50MB）…'
    const timer = setInterval(() => {
      if (syncProgress.value < 85) syncProgress.value += 2
    }, 3000)
    await pocApi.syncTemplatesOnline()
    clearInterval(timer)
    syncProgress.value = 100; syncStatus.value = 'done'; syncMsg.value = '在线同步完成！'
    fetchTemplates(true); fetchTemplateStats(); fetchTemplateCategories()
    ElMessage.success('在线同步完成')
  } catch (e: any) {
    syncStatus.value = 'failed'; syncMsg.value = e.message || '在线同步失败，请尝试手动导入'
  }
}

async function clearTemplates() {
  await ElMessageBox.confirm('确认清空所有 Nuclei 模板？此操作不可恢复。', '清空模板', { type: 'warning' })
  await pocApi.clearTemplates().catch(() => {})
  ElMessage.success('已清空'); fetchTemplates(true); fetchTemplateStats()
}

// ── 自定义 POC ────────────────────────────────────
const pocs = ref<CustomPoc[]>([])
const pocLoading = ref(false)
const pocPage = ref(1)
const pocPageSize = ref(20)
const pocTotal = ref(0)
const pocFilter = reactive<{ name: string; template_id: string; severity: string; enabled: boolean|undefined }>({
  name: '', template_id: '', severity: '', enabled: undefined,
})
const pocClearLoading = ref(false)
const pocExportLoading = ref(false)

async function fetchPocs(reset = false) {
  if (reset) pocPage.value = 1
  pocLoading.value = true
  try {
    const r = await pocApi.pocs({
      ...pocFilter,
      limit: pocPageSize.value,
      skip: (pocPage.value - 1) * pocPageSize.value,
    }).catch(() => ({ data: [] as CustomPoc[], total: 0 }))
    pocs.value = r.data; pocTotal.value = r.total
  } finally { pocLoading.value = false }
}
function resetPocFilter() {
  Object.assign(pocFilter, { name: '', template_id: '', severity: '', enabled: undefined })
  fetchPocs(true)
}

async function deletePoc(row: CustomPoc) {
  await pocApi.pocDelete(row.id).catch((e: any) => { throw e })
  ElMessage.success('已删除'); fetchPocs()
}
async function clearPocs() {
  await ElMessageBox.confirm('确认清空所有自定义 POC？', '清空 POC', { type: 'warning' })
  pocClearLoading.value = true
  try { await pocApi.pocClear(); ElMessage.success('已清空'); fetchPocs(true) }
  catch { ElMessage.error('清空失败') }
  finally { pocClearLoading.value = false }
}
async function exportPocs() {
  pocExportLoading.value = true
  try {
    const blob = await pocApi.pocExport()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a'); a.href = url; a.download = 'custom-pocs.zip'; a.click()
    URL.revokeObjectURL(url)
  } catch { ElMessage.error('导出失败') }
  finally { pocExportLoading.value = false }
}

// POC 编辑表单
const showPocForm = ref(false)
const pocSaving = ref(false)
const pocForm = reactive({
  id: '', name: '', template_id: '', severity: 'high', author: '', description: '',
  tagsInput: '', content: '', enabled: true,
})

function openPocForm(row?: CustomPoc) {
  if (row) {
    Object.assign(pocForm, { ...row, tagsInput: (row.tags||[]).join(', ') })
  } else {
    Object.assign(pocForm, { id:'', name:'', template_id:'', severity:'high', author:'', description:'', tagsInput:'', content:'', enabled:true })
  }
  showPocForm.value = true
}

function parseYaml() {
  if (!pocForm.content) return
  try {
    const lines = pocForm.content.split('\n')
    const id = lines.find(l => /^id:\s+/.test(l))?.replace(/^id:\s+/, '').trim()
    const name = lines.find(l => /^\s+name:\s+/.test(l))?.replace(/^\s+name:\s+/, '').trim()
    const sev = lines.find(l => /^\s+severity:\s+/.test(l))?.replace(/^\s+severity:\s+/, '').trim()
    const author = lines.find(l => /^\s+author:\s+/.test(l))?.replace(/^\s+author:\s+/, '').trim()
    const tags = lines.find(l => /^\s+tags:\s+/.test(l))?.replace(/^\s+tags:\s+/, '').trim()
    const desc = lines.find(l => /^\s+description:\s+/.test(l))?.replace(/^\s+description:\s+/, '').trim()
    if (id) pocForm.template_id = id
    if (name) pocForm.name = name
    if (sev) pocForm.severity = sev
    if (author) pocForm.author = author
    if (tags) pocForm.tagsInput = tags
    if (desc) pocForm.description = desc
  } catch {}
}

async function savePoc() {
  if (!pocForm.content) { ElMessage.warning('请填写 YAML 内容'); return }
  pocSaving.value = true
  const body = {
    name: pocForm.name,
    template_id: pocForm.template_id,
    severity: pocForm.severity,
    author: pocForm.author,
    description: pocForm.description,
    tags: pocForm.tagsInput.split(',').map((t: string) => t.trim()).filter(Boolean),
    content: pocForm.content,
    enabled: pocForm.enabled,
  }
  try {
    if (pocForm.id) await pocApi.pocUpdate(pocForm.id, body)
    else await pocApi.pocCreate(body as any)
    ElMessage.success('保存成功'); showPocForm.value = false; fetchPocs()
  } catch (e: any) { ElMessage.error(e.message || '保存失败') }
  finally { pocSaving.value = false }
}

// 批量导入
const showImportDialog = ref(false)
const importLoading = ref(false)
const importZipFile = ref<File|null>(null)
const importEnabled = ref(true)
const importProgress = ref(0)
const importStatus = ref('')
const importMsg = ref('')

async function doImport() {
  if (!importZipFile.value) return
  const fd = new FormData(); fd.append('file', importZipFile.value); fd.append('enabled', String(importEnabled.value))
  importLoading.value = true; importProgress.value = 20; importMsg.value = '正在上传并解压…'
  try {
    importProgress.value = 60; importMsg.value = '正在导入 POC…'
    await pocApi.pocImport(fd)
    importProgress.value = 100; importStatus.value = 'done'; importMsg.value = '导入完成！'
    ElMessage.success('POC 批量导入完成'); fetchPocs(true)
  } catch (e: any) {
    importStatus.value = 'failed'; importMsg.value = e.message || '导入失败'
  }
}

// POC 验证
const showValidate = ref(false)
const validateTarget = ref<Partial<NucleiTemplate & CustomPoc>>({})
const validateIsTemplate = ref(false)
const validateUrl = ref('')
const validateLoading = ref(false)
const validateLogs = ref<any[]>([])
const validateResult = ref<any>(null)
const logsRef = ref<HTMLElement>()

function openValidate(row: any, isTemplate: boolean) {
  validateTarget.value = row; validateIsTemplate.value = isTemplate
  validateUrl.value = ''; validateLogs.value = []; validateResult.value = null
  showValidate.value = true
}

async function doValidate() {
  if (!validateUrl.value) return
  validateLoading.value = true; validateLogs.value = []; validateResult.value = null
  try {
    const r = await pocApi.pocValidate((validateTarget.value as any).template_id || validateTarget.value.id!, validateUrl.value, validateIsTemplate.value)
    const taskId = r.task_id
    let retry = 0
    const poll = async () => {
      const res = await pocApi.pocValidateResult(taskId).catch(() => null)
      if (res?.logs) validateLogs.value = res.logs
      await nextTick(); if (logsRef.value) logsRef.value.scrollTop = logsRef.value.scrollHeight
      if (res?.status === 'done' || res?.status === 'failed' || retry++ > 60) {
        validateResult.value = res; validateLoading.value = false
      } else { setTimeout(poll, 1000) }
    }
    setTimeout(poll, 800)
  } catch (e: any) {
    validateLogs.value.push({ level: 'ERROR', timestamp: new Date().toLocaleTimeString(), message: (e as any).message })
    validateLoading.value = false
  }
}

// 批量验证
const showBatchValidate = ref(false)
const batchUrls = ref('')
const batchLoading = ref(false)
const batchResults = ref<any[]>([])
const batchDone = ref(0)
const batchTotal = ref(0)

async function doBatchValidate() {
  const urls = batchUrls.value.split('\n').map((s: string) => s.trim()).filter(Boolean)
  if (!urls.length) { ElMessage.warning('请填写目标 URL'); return }
  batchLoading.value = true; batchResults.value = []; batchDone.value = 0
  batchTotal.value = selectedTemplates.value.length * urls.length
  for (const tpl of selectedTemplates.value) {
    for (const url of urls) {
      try {
        const r = await pocApi.pocValidate(tpl.template_id || tpl.id, url, true)
        let retry = 0
        await new Promise<void>(resolve => {
          const poll = async () => {
            const res = await pocApi.pocValidateResult(r.task_id).catch(() => null)
            if (res?.status === 'done' || res?.status === 'failed' || retry++ > 30) {
              batchResults.value.push({ pocName: tpl.name||tpl.template_id, severity: tpl.severity, matched: res?.matched??false, matchedUrl: res?.matched?url:'' })
              batchDone.value++; resolve()
            } else { setTimeout(poll, 1000) }
          }
          setTimeout(poll, 500)
        })
      } catch { batchDone.value++ }
    }
  }
  batchLoading.value = false
}

// ── 工具函数 ──────────────────────────────────────
function sevType(s: string) {
  return ({ critical:'danger', high:'warning', medium:'', low:'info', info:'success' } as Record<string,string>)[s] ?? 'info'
}
function fmtBytes(n: number) {
  if (!n) return '—'
  if (n < 1024) return `${n} B`; if (n < 1048576) return `${(n/1024).toFixed(1)} KB`
  return `${(n/1048576).toFixed(1)} MB`
}
function countLines(s: string) { return s ? s.split('\n').filter((l: string) => l.trim()).length : 0 }
function copyContent(text?: string) {
  if (!text) return
  navigator.clipboard.writeText(text).then(() => ElMessage.success('已复制')).catch(() => ElMessage.error('复制失败'))
}

onMounted(() => {
  fetchTemplates(); fetchTemplateStats(); fetchTemplateCategories()
})
</script>

<style scoped>
.tab-badge { margin-left: 4px; }
.tab-badge :deep(.el-badge__content) { font-size: 11px; }

/* 工具栏 */
.poc-toolbar { display: flex; align-items: center; gap: 12px; padding: 0 0 12px; flex-wrap: wrap; }
.stats-bar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }

/* 严重等级药丸 */
.sev-pill {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 2px 10px; border-radius: 10px; font-size: 12px; font-weight: 500; line-height: 1.6;
}
.sev-critical { background: #fef2f2; color: #dc2626; }
.sev-high { background: #fff7ed; color: #ea580c; }
.sev-medium { background: #fefce8; color: #ca8a04; }
.sev-low { background: #eff6ff; color: #2563eb; }
.sev-info { background: #f0fdf4; color: #16a34a; }

/* 筛选栏 */
.filter-bar { display: flex; align-items: center; gap: 8px; padding: 0 0 12px; flex-wrap: wrap; }

/* 表格内样式 */
.poc-table :deep(.el-table__row) { cursor: default; }
.poc-name { font-weight: 500; font-size: 13px; color: var(--el-text-color-primary); }
.tpl-id { font-family: 'SF Mono', Monaco, Consolas, monospace; font-size: 12px; color: #6366f1; }

/* 表格内严重等级 */
.sev-dot { font-size: 12px; font-weight: 600; }
.sev-dot.sev-critical { color: #dc2626; }
.sev-dot.sev-high { color: #ea580c; }
.sev-dot.sev-medium { color: #ca8a04; }
.sev-dot.sev-low { color: #2563eb; }
.sev-dot.sev-info { color: #16a34a; }

/* 标签 */
.tag-wrap { display: flex; flex-wrap: wrap; gap: 4px; }
.poc-tag {
  display: inline-block; padding: 1px 8px; border-radius: 3px;
  font-size: 11px; line-height: 1.6;
  background: #f4f4f5; color: var(--el-text-color-regular);
}
.poc-tag-more { font-size: 11px; color: var(--el-text-color-secondary); line-height: 1.6; padding: 1px 4px; }

/* 启用状态 */
.status-indicator { font-size: 12px; font-weight: 500; }
.status-indicator.enabled { color: #16a34a; }
.status-indicator.disabled { color: #9ca3af; }

:deep(.el-upload-dragger) { padding: 24px; }

/* 验证日志 */
.validate-logs { margin-top: 16px; border: 1px solid #ebeef5; border-radius: 6px; overflow: hidden; }
.logs-header { display: flex; align-items: center; justify-content: space-between; padding: 8px 12px; background: #f5f7fa; font-size: 12.5px; font-weight: 600; border-bottom: 1px solid #ebeef5; }
.logs-content { max-height: 200px; overflow-y: auto; padding: 8px 12px; background: #1d2129; }
.log-line { font-family: monospace; font-size: 12px; line-height: 1.7; display: flex; gap: 8px; }
.log-time { color: #5c6370; flex-shrink: 0; }
.log-level { flex-shrink: 0; }
.log-info .log-level { color: #61afef; }
.log-warning .log-level { color: #e5c07b; }
.log-error .log-level { color: #e06c75; }
.log-success .log-level { color: #98c379; }
.log-msg { color: #abb2bf; word-break: break-all; }

.validate-result { margin-top: 14px; }
.result-pre { margin: 10px 0 0; padding: 10px; background: #f5f7fa; border-radius: 6px; font-size: 12px; font-family: monospace; white-space: pre-wrap; word-break: break-all; max-height: 160px; overflow-y: auto; }
.batch-progress { margin-top: 16px; }
</style>
