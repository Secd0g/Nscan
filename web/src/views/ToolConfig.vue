<template>
  <div class="page-card">
    <div class="page-header">
      <div>
        <h2 class="page-title">工具配置</h2>
        <p class="page-desc">配置扫描工具使用的第三方 API 数据源。</p>
      </div>
    </div>

    <div class="config-grid">
      <el-card shadow="never">
        <template #header><div class="card-title">Subfinder API</div></template>
        <p>配置被动子域名收集使用的数据源，每个数据源可填写多个 Key。</p>
        <el-button type="primary" @click="openProviderDialog">配置数据源</el-button>
      </el-card>
      <el-card shadow="never">
        <template #header><div class="card-title">在线搜索 API</div></template>
        <p>配置 Fofa、Hunter、Quake、Shodan 等网络空间搜索服务。</p>
        <el-button type="primary" @click="openOnlineSearchDialog">配置数据源</el-button>
      </el-card>
      <el-card shadow="never">
        <template #header><div class="card-title">BBOT API</div></template>
        <p>配置 BBOT 使用的第三方数据源，增强子域名和资产收集效果。</p>
        <el-button type="primary" @click="openBbotDialog">配置数据源</el-button>
      </el-card>
    </div>

    <el-dialog v-model="providerDialogVisible" title="Subfinder API 数据源配置" width="900px" destroy-on-close>
      <el-alert type="info" :closable="false" style="margin-bottom:16px">每个服务可配置多个 API Key（每行一个）。</el-alert>
      <el-table v-loading="providerLoading" :data="subfinderProviders" max-height="400" border>
        <el-table-column label="数据源" width="130"><template #default="{ row }">{{ row.name }}</template></el-table-column>
        <el-table-column label="API Key" min-width="300"><template #default="{ row }">
          <el-input v-model="row.keys" type="textarea" :rows="1" :autosize="{ minRows: 1, maxRows: 3 }" :placeholder="row.placeholder" :disabled="!row.enabled" />
        </template></el-table-column>
        <el-table-column label="启用" width="70" align="center"><template #default="{ row }"><el-switch v-model="row.enabled" size="small" /></template></el-table-column>
      </el-table>
      <template #footer><el-button @click="providerDialogVisible = false">取消</el-button><el-button type="primary" :loading="providerSaving" @click="saveProviders">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="onlineSearchDialogVisible" title="在线搜索 API 配置" width="720px" destroy-on-close>
      <el-alert type="info" :closable="false" style="margin-bottom:12px">配置后可在扫描任务中启用对应的在线搜索数据源。</el-alert>
      <el-tabs v-model="onlineSearchTab" type="border-card">
        <el-tab-pane v-for="name in ['fofa', 'hunter', 'quake', 'shodan']" :key="name" :label="name" :name="name">
          <el-form label-position="top"><el-form-item label="API Key"><el-input v-model="onlineSearchForm[name as keyof typeof onlineSearchForm].key" show-password :placeholder="`${name} API Key`" /></el-form-item><el-form-item label="启用"><el-switch v-model="onlineSearchForm[name as keyof typeof onlineSearchForm].enabled" /></el-form-item></el-form>
        </el-tab-pane>
      </el-tabs>
      <template #footer><el-button @click="onlineSearchDialogVisible = false">取消</el-button><el-button type="primary" :loading="onlineSearchSaving" @click="saveOnlineSearch">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="bbotDialogVisible" title="BBOT API 数据源配置" width="760px" destroy-on-close>
      <el-table v-loading="bbotLoading" :data="bbotProviders" max-height="400" border>
        <el-table-column label="数据源" width="180"><template #default="{ row }"><div>{{ row.name }}</div><small>{{ row.desc }}</small></template></el-table-column>
        <el-table-column label="API Key" min-width="260"><template #default="{ row }"><el-input v-model="row.key" :placeholder="row.placeholder" :disabled="!row.enabled" clearable /></template></el-table-column>
        <el-table-column label="启用" width="70" align="center"><template #default="{ row }"><el-switch v-model="row.enabled" size="small" /></template></el-table-column>
      </el-table>
      <template #footer><el-button @click="bbotDialogVisible = false">取消</el-button><el-button type="primary" :loading="bbotSaving" @click="saveBbotProviders">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { settingsApi } from '@/api'

const providerDefs = [
  ['shodan', 'Shodan 搜索引擎', 'Shodan API Key'], ['censys', 'Censys 证书搜索', 'API_ID:API_SECRET'], ['virustotal', 'VirusTotal 威胁情报', 'VirusTotal API Key'],
  ['securitytrails', 'SecurityTrails DNS 数据', 'SecurityTrails API Key'], ['chaos', 'ProjectDiscovery Chaos', 'Chaos API Key'], ['github', 'GitHub 代码搜索', 'ghp_xxxx'], ['fofa', 'FOFA 网络空间搜索', 'Fofa API Key'],
  ['quake', '360 Quake 网络空间搜索', 'Quake API Key'], ['hunter', '鹰图 Hunter 搜索', 'Hunter API Key'], ['zoomeye', 'ZoomEye 搜索引擎', 'ZoomEye API Key'], ['binaryedge', 'BinaryEdge 互联网扫描', 'BinaryEdge API Key'],
  ['passivetotal', 'PassiveTotal/RiskIQ', 'email,key'], ['fullhunt', 'FullHunt 攻击面管理', 'FullHunt API Key'], ['bevigil', 'BeVigil 移动应用安全', 'BeVigil API Key'],
]
const subfinderProviders = reactive(providerDefs.map(([name, desc, placeholder]) => ({ name, desc, placeholder, keys: '', enabled: false })))
const providerDialogVisible = ref(false), providerLoading = ref(false), providerSaving = ref(false)

async function openProviderDialog() {
  providerDialogVisible.value = true; providerLoading.value = true
  try { const cfg = await settingsApi.getProviders('subfinder'); for (const p of subfinderProviders) { p.keys = cfg.providers?.[p.name]?.join('\n') || ''; p.enabled = cfg.enabled?.[p.name] ?? false } }
  catch (e: any) { ElMessage.error('加载配置失败: ' + e.message) } finally { providerLoading.value = false }
}
async function saveProviders() {
  providerSaving.value = true
  try { const providers: Record<string, string[]> = {}, enabled: Record<string, boolean> = {}; for (const p of subfinderProviders) { const keys = p.keys.split('\n').map(s => s.trim()).filter(Boolean); if (keys.length) providers[p.name] = keys; enabled[p.name] = p.enabled }; await settingsApi.saveProviders('subfinder', providers, enabled); ElMessage.success('API 配置已保存'); providerDialogVisible.value = false }
  catch (e: any) { ElMessage.error('保存失败: ' + e.message) } finally { providerSaving.value = false }
}

const onlineSearchDialogVisible = ref(false), onlineSearchSaving = ref(false), onlineSearchTab = ref('fofa')
const onlineSearchForm = reactive({ fofa: { key: '', enabled: false }, hunter: { key: '', enabled: false }, quake: { key: '', enabled: false }, shodan: { key: '', enabled: false } })
async function openOnlineSearchDialog() {
  onlineSearchDialogVisible.value = true
  try { const [osCfg, subCfg] = await Promise.all([settingsApi.getProviders('online_search').catch(() => ({ providers: {}, enabled: {} } as any)), settingsApi.getProviders('subfinder').catch(() => ({ providers: {}, enabled: {} } as any))]); const pick = (n: string) => osCfg.providers?.[n]?.[0] || subCfg.providers?.[n]?.[0] || ''; for (const n of ['fofa', 'hunter', 'quake', 'shodan'] as const) { onlineSearchForm[n].key = pick(n); onlineSearchForm[n].enabled = !!osCfg.enabled?.[n] } }
  catch (e: any) { ElMessage.error('加载配置失败: ' + e.message) }
}
async function saveOnlineSearch() {
  onlineSearchSaving.value = true
  try { const providers: Record<string, string[]> = {}, enabled: Record<string, boolean> = {}; for (const n of ['fofa', 'hunter', 'quake', 'shodan'] as const) { if (onlineSearchForm[n].key) providers[n] = [onlineSearchForm[n].key]; enabled[n] = onlineSearchForm[n].enabled }; await settingsApi.saveProviders('online_search', providers, enabled); ElMessage.success('API 配置已保存'); onlineSearchDialogVisible.value = false }
  catch (e: any) { ElMessage.error('保存失败: ' + e.message) } finally { onlineSearchSaving.value = false }
}

const bbotProviderDefs = [
  ['shodan_dns', 'Shodan DNS 子域名', 'Shodan API Key'], ['censys_dns', 'Censys 证书搜索', 'API_ID:API_SECRET'], ['virustotal', 'VirusTotal 威胁情报', 'VirusTotal API Key'], ['securitytrails', 'SecurityTrails DNS', 'SecurityTrails API Key'], ['chaos', 'ProjectDiscovery Chaos', 'Chaos API Key'], ['bevigil', 'BeVigil 移动安全', 'BeVigil API Key'], ['fullhunt', 'FullHunt 攻击面管理', 'FullHunt API Key'], ['hunterio', 'Hunter.io 邮件/域名', 'Hunter.io API Key'], ['github_codesearch', 'GitHub 代码搜索', 'ghp_xxxx'], ['github_org', 'GitHub 组织枚举', 'ghp_xxxx'], ['leakix', 'LeakIX 泄露数据', 'LeakIX API Key'], ['otx', 'AlienVault OTX', 'OTX API Key'], ['bufferoverrun', 'BufferOver.run DNS', 'BufferOver.run API Key'], ['builtwith', 'BuiltWith 技术指纹', 'BuiltWith API Key'], ['c99', 'C99 子域名查找', 'C99 API Key'], ['postman', 'Postman 公开集合', 'Postman API Key'], ['postman_download', 'Postman 集合下载', 'Postman API Key'], ['subdomainradar', 'SubdomainRadar', 'SubdomainRadar API Key'], ['trickest', 'Trickest 子域名', 'Trickest API Key'],
]
const bbotProviders = reactive(bbotProviderDefs.map(([name, desc, placeholder]) => ({ name, desc, placeholder, key: '', enabled: false })))
const bbotDialogVisible = ref(false), bbotLoading = ref(false), bbotSaving = ref(false)
async function openBbotDialog() {
  bbotDialogVisible.value = true; bbotLoading.value = true
  try { const cfg = await settingsApi.getProviders('bbot'); for (const p of bbotProviders) { p.key = cfg.providers?.[p.name]?.[0] || ''; p.enabled = cfg.enabled?.[p.name] ?? false } }
  catch (e: any) { ElMessage.error('加载配置失败: ' + e.message) } finally { bbotLoading.value = false }
}
async function saveBbotProviders() {
  bbotSaving.value = true
  try { const providers: Record<string, string[]> = {}, enabled: Record<string, boolean> = {}; for (const p of bbotProviders) { if (p.key.trim()) providers[p.name] = [p.key.trim()]; enabled[p.name] = p.enabled }; await settingsApi.saveProviders('bbot', providers, enabled); ElMessage.success('API 配置已保存'); bbotDialogVisible.value = false }
  catch (e: any) { ElMessage.error('保存失败: ' + e.message) } finally { bbotSaving.value = false }
}
</script>

<style scoped>
.page-desc { margin: 6px 0 0; color: var(--el-text-color-secondary); font-size: 13px; }
.config-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 16px; }
.card-title { font-weight: 600; }
.config-grid p { min-height: 42px; color: var(--el-text-color-secondary); font-size: 13px; line-height: 1.6; }
small { color: var(--el-text-color-secondary); }
</style>
