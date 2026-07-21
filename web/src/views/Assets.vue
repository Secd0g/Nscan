<template>
  <div class="assets-wrap">
    <!-- 全局筛选 -->
    <div class="global-filter">
      <el-select v-model="filter.project_id" clearable placeholder="项目" style="width:140px" @change="onProjectChange">
        <el-option v-for="p in projects" :key="p.id" :label="p.name" :value="p.id" />
      </el-select>
      <el-select v-model="filter.task_id" clearable placeholder="任务" style="width:180px" @change="onGlobalFilter">
        <el-option v-for="t in tasks" :key="t.id" :label="t.name" :value="t.id" />
      </el-select>
      <button class="export-btn" :disabled="exporting" style="margin-left:auto" @click="exportAll">
        <svg v-if="!exporting" xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
        <svg v-else xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" class="spin"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>
        导出报告
      </button>
    </div>

    <el-tabs v-model="activeTab" type="border-card" class="asset-tabs" @tab-change="onTabChange">

      <!-- ══ 资产（HTTP）══ -->
      <el-tab-pane name="asset">
        <template #label><span>资产</span></template>

        <!-- 筛选栏（单行，flex-wrap） -->
        <div class="search-bar">
          <template v-if="!assetExprMode">
            <el-select v-model="assetForm.field" style="width:120px" size="default">
              <el-option v-for="f in assetFields" :key="f.value" :label="f.label" :value="f.value" />
            </el-select>
            <el-input v-model="assetForm.value" clearable style="width:200px"
              :placeholder="`输入${assetFields.find(f=>f.value===assetForm.field)?.label ?? ''}…`"
              @keyup.enter="addAssetChip">
            </el-input>
            <el-button @click="addAssetChip">添加筛选</el-button>
            <el-button v-if="assetChips.length" type="danger" plain size="small" @click="clearAssetChips">清除筛选</el-button>
            <el-tag v-for="c in assetChips" :key="c.raw" closable size="small" @close="removeAssetChip(c.raw)">
              {{ c.label }}="{{ c.val }}"
            </el-tag>
          </template>
          <template v-else>
            <el-autocomplete v-model="assetExpr" style="width:480px"
              placeholder='表达式 如: app="nginx" && port="443" || title!="404"'
              :fetch-suggestions="(q: string, cb: any) => exprSuggest(q, assetFields, cb)"
              :trigger-on-focus="false"
              highlight-first-item
              @select="(item: any) => onExprSelect(item, 'asset')"
              @keyup.enter="fetchAsset(true)"
              clearable>
              <template #default="{ item }">
                <span style="font-weight:500">{{ item.value }}</span>
                <span style="margin-left:8px;color:var(--el-text-color-secondary);font-size:12px">{{ item.label }}</span>
              </template>
            </el-autocomplete>
            <el-button type="primary" @click="fetchAsset(true)">搜索</el-button>
          </template>
          <el-button text size="small" @click="assetExprMode = !assetExprMode; clearAssetChips()">
            {{ assetExprMode ? '筛选模式' : '表达式' }}
          </el-button>
          <!-- 表格/卡片切换 -->
          <div class="view-switch" style="margin-left:auto">
            <span class="view-btn" :class="{ active: asset.view==='table' }" @click="asset.view='table'">
              <el-icon><List /></el-icon>
            </span>
            <span class="view-btn" :class="{ active: asset.view==='card' }" @click="asset.view='card'">
              <el-icon><Grid /></el-icon>
            </span>
          </div>
        </div>

        <!-- 主体：左侧统计 + 右侧内容 -->
        <div class="asset-body">

          <!-- 左侧统计 -->
          <div v-if="!statHidden" class="stat-panel">
            <div class="stat-header">
              <span>资产总数 <b>{{ asset.total }}</b></span>
              <span class="stat-hide" @click="statHidden=true">收起</span>
            </div>
            <el-collapse v-model="statOpen">
              <el-collapse-item title="端口" name="port">
                <div v-for="p in stats.ports" :key="p.value" class="stat-item" @click="addTag('port', p.value)">
                  <el-tag size="small">{{ p.value }}</el-tag>
                  <span class="stat-num">{{ p.count }}</span>
                </div>
              </el-collapse-item>
              <el-collapse-item title="应用" name="product">
                <div v-for="t in stats.techs" :key="t.value" class="stat-item" @click="addTag('app', t.value)">
                  <el-tag type="success" size="small">{{ t.value }}</el-tag>
                  <span class="stat-num">{{ t.count }}</span>
                </div>
              </el-collapse-item>
            </el-collapse>
          </div>
          <div v-else class="stat-collapsed" @click="statHidden=false">
            <el-icon><Expand /></el-icon>
          </div>

          <!-- 右侧 -->
          <div class="asset-right">

            <!-- 表格视图 -->
            <template v-if="asset.view==='table'">
              <div class="tab-toolbar">
                <el-popconfirm title="批量删除选中资产？" @confirm="batchDelete('http', assetSelected, fetchAsset)">
                  <template #reference>
                    <el-button type="danger" plain :disabled="!assetSelected.length" size="small">
                      <el-icon><Delete /></el-icon>批量删除({{ assetSelected.length }})
                    </el-button>
                  </template>
                </el-popconfirm>
              </div>
              <el-table ref="assetTableRef" class="asset-table" :data="asset.list" v-loading="asset.loading" style="width:100%" size="small" stripe border
                :header-cell-style="{ background: 'var(--el-fill-color-light)', color: 'var(--el-text-color-primary)', fontWeight: 600, fontSize: '13px' }"
                @scroll="syncAssetTableHeader"
                @selection-change="(rows: any[]) => assetSelected = rows">
                <el-table-column type="selection" width="42" />
                <el-table-column type="index" label="序号" width="55" />
                <el-table-column label="域名" min-width="220"><template #default="{ row }"><a class="asset-link full-domain" :href="assetURL(row.url)" target="_blank" rel="noopener noreferrer">{{ normalizeAssetURL(row.domain || row.url) }}</a></template></el-table-column>
                <el-table-column label="IP" width="140"><template #default="{ row }">{{ row.ip || '-' }}</template></el-table-column>
                <el-table-column label="端口/服务" width="100"><template #default="{ row }"><el-tag v-if="row.port" type="info" size="small" @click="addTag('port', row.port)">{{ row.port }}</el-tag><span v-else>-</span></template></el-table-column>
                <el-table-column label="状态码" width="80"><template #default="{ row }"><span v-if="row.status_code" class="status-code" :style="{ color: statusColor(row.status_code) }">{{ row.status_code }}</span><span v-else>-</span></template></el-table-column>
                <el-table-column label="标题" min-width="130"><template #default="{ row }">{{ row.title || '-' }}</template></el-table-column>
                <el-table-column label="响应头" min-width="110"><template #default="{ row }">{{ row.banner || '-' }}</template></el-table-column>
                <el-table-column label="应用/组件" min-width="140"><template #default="{ row }"><el-tag v-for="t in uniqueTech(row.tech)" :key="t" type="success" size="small" style="margin-right:4px" @click="addTag('app', t)">{{ t }}</el-tag><span v-if="!uniqueTech(row.tech).length">-</span></template></el-table-column>
                <el-table-column prop="content_len" label="大小" width="90" align="center">
                  <template #default="{ row }">{{ row.content_len > 0 ? fmtBytes(row.content_len) : '-' }}</template>
                </el-table-column>
                <el-table-column prop="screenshot" label="截图" width="140" align="center">
                  <template #default="{ row }">
                    <img v-if="row.screenshot" :src="`/images/screenshots/${row.screenshot}.png`"
                      style="display:block;width:120px;height:auto;max-height:80px;margin:auto;cursor:pointer;border-radius:4px;object-fit:contain"
                      @click="previewSrc=`/images/screenshots/${row.screenshot}.png`" />
                    <span v-else>-</span>
                  </template>
                </el-table-column>
                <el-table-column prop="created_at" label="发现时间" width="170" align="center">
                  <template #default="{ row }">{{ row.created_at ? fmtTime(row.created_at) : '-' }}</template>
                </el-table-column>
                <el-table-column label="操作" width="70" align="center">
                  <template #default="{ row }"><el-button type="primary" link size="small" @click="openAssetDetail(row)">详情</el-button></template>
                </el-table-column>
              </el-table>
            </template>

            <!-- 卡片视图 -->
            <template v-else>
              <div v-loading="asset.loading" class="card-grid">
                <div v-for="item in asset.list" :key="item.id" class="asset-card" @click="openAssetDetail(item)">
                  <div class="card-img-wrap">
                    <img v-if="item.screenshot" :src="`/images/screenshots/${item.screenshot}.png`"
                      class="card-img"
                      @error="(e: any) => { e.target.style.display='none'; (e.target.previousSibling as any).style.display='flex' }" />
                    <div class="card-no-img"><el-icon style="font-size:28px;color:var(--el-text-color-disabled)"><Chrome /></el-icon></div>
                  </div>
                  <div class="card-footer">
                    <div class="card-row1">
                      <span class="card-title">{{ item.title || '无标题' }}</span>
                      <span v-if="item.status_code" :style="{ color: statusColor(item.status_code), fontWeight: 600, fontSize: '12px' }">{{ item.status_code }}</span>
                    </div>
                    <a class="card-url" :href="assetURL(item.url)" target="_blank" @click.stop>{{ normalizeAssetURL(item.url) }}</a>
                    <div class="card-tags">
                      <el-tag v-if="item.port" size="small" type="info" style="font-family:monospace">{{ item.port }}</el-tag>
                      <el-tag v-for="t in (item.tech||[]).slice(0,2)" :key="t" size="small" type="success">{{ t }}</el-tag>
                    </div>
                  </div>
                </div>
              </div>
              <el-empty v-if="!asset.loading && asset.list.length===0" description="暂无资产数据" style="padding:60px 0" />
            </template>

            <el-pagination v-if="asset.total > pageSize" v-model:current-page="asset.page" :page-size="pageSize"
              :total="asset.total" layout="total, prev, pager, next"
              style="margin-top:14px;justify-content:flex-end" @current-change="fetchAsset()" />
          </div>
        </div>
      </el-tab-pane>

      <!-- ══ IP / 端口 ══ -->
      <el-tab-pane name="ip">
        <template #label><span>IP/端口</span></template>
        <div class="search-bar">
          <template v-if="!ipExprMode">
            <el-select v-model="ipForm.field" style="width:120px" size="default">
              <el-option v-for="f in ipFields" :key="f.value" :label="f.label" :value="f.value" />
            </el-select>
            <el-input v-model="ipForm.value" clearable style="width:200px"
              :placeholder="`输入${ipFields.find(f=>f.value===ipForm.field)?.label ?? ''}…`"
              @keyup.enter="addIpChip">
            </el-input>
            <el-button @click="addIpChip">添加筛选</el-button>
            <el-button v-if="ipChips.length" type="danger" plain size="small" @click="clearIpChips">清除筛选</el-button>
            <el-tag v-for="c in ipChips" :key="c.raw" closable size="small" @close="removeIpChip(c.raw)">
              {{ c.label }}="{{ c.val }}"
            </el-tag>
          </template>
          <template v-else>
            <el-autocomplete v-model="ipExpr" style="width:480px"
              placeholder='表达式 如: ip="192.168.1" && port="80" || service!="http"'
              :fetch-suggestions="(q: string, cb: any) => exprSuggest(q, ipFields, cb)"
              :trigger-on-focus="false"
              highlight-first-item
              @select="(item: any) => onExprSelect(item, 'ip')"
              @keyup.enter="fetchIP(true)"
              clearable>
              <template #default="{ item }">
                <span style="font-weight:500">{{ item.value }}</span>
                <span style="margin-left:8px;color:var(--el-text-color-secondary);font-size:12px">{{ item.label }}</span>
              </template>
            </el-autocomplete>
            <el-button type="primary" @click="fetchIP(true)">搜索</el-button>
          </template>
          <el-button text size="small" @click="ipExprMode = !ipExprMode; clearIpChips()">
            {{ ipExprMode ? '筛选模式' : '表达式' }}
          </el-button>
          <!-- IP聚合/扁平切换 -->
          <div class="view-switch" style="margin-left:auto">
            <span class="view-btn" :class="{ active: ip.view==='agg' }" @click="ip.view='agg'; fetchIP(true)">聚合</span>
            <span class="view-btn" :class="{ active: ip.view==='flat' }" @click="ip.view='flat'; fetchIP(true)">扁平</span>
          </div>
        </div>
        <div class="tab-toolbar">
          <el-popconfirm v-if="ip.view==='flat'" title="批量删除选中端口？" @confirm="batchDelete('port', ipSelected, fetchIP)">
            <template #reference>
              <el-button type="danger" plain :disabled="!ipSelected.length" size="small">
                <el-icon><Delete /></el-icon>批量删除({{ ipSelected.length }})
              </el-button>
            </template>
          </el-popconfirm>
        </div>

        <!-- 聚合视图 -->
        <template v-if="ip.view==='agg'">
          <el-table :data="ipAgg.list" v-loading="ipAgg.loading" style="width:100%" border
            :header-cell-style="{ background: 'var(--el-fill-color-light)', color: 'var(--el-text-color-primary)', fontWeight: 600, fontSize: '13px' }"
            :cell-style="{ fontSize: '13px' }"
            :span-method="ipSpanMethod">
            <el-table-column label="IP" width="150">
              <template #default="{ row }"><span style="font-family:monospace">{{ row.ip }}</span></template>
            </el-table-column>
            <el-table-column label="端口" width="80">
              <template #default="{ row }"><el-tag v-if="row.port" type="info" size="small" style="font-family:monospace">{{ row.port }}</el-tag></template>
            </el-table-column>
            <el-table-column label="服务" width="100">
              <template #default="{ row }"><el-tag v-if="row.service" size="small">{{ row.service }}</el-tag></template>
            </el-table-column>
            <el-table-column label="域名" min-width="240">
              <template #default="{ row }">
                <span v-if="row.domains?.length" class="full-domain">{{ row.domains.join(', ') }}</span>
                <span v-else style="color:var(--el-text-color-disabled)">—</span>
              </template>
            </el-table-column>
            <el-table-column label="Web Server" min-width="140" show-overflow-tooltip>
              <template #default="{ row }">
                <span v-if="row.webServer" style="font-size:12px;color:var(--el-text-color-secondary)">{{ row.webServer }}</span>
                <span v-else style="color:var(--el-text-color-disabled)">—</span>
              </template>
            </el-table-column>
            <el-table-column label="产品" min-width="200">
              <template #default="{ row }">
                <el-tag v-for="p in row.products" :key="p" type="success" size="small" style="margin-right:4px">{{ p }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="时间" width="170">
              <template #default="{ row }">{{ row.time }}</template>
            </el-table-column>
          </el-table>
          <el-pagination v-if="ipAgg.total > ipAggPageSize" v-model:current-page="ipAgg.page" :page-size="ipAggPageSize"
            :total="ipAgg.total" layout="total, prev, pager, next"
            style="margin-top:14px;justify-content:flex-end" @current-change="fetchIPAgg()" />
        </template>

        <!-- 扁平视图（原始端口列表） -->
        <template v-else>
          <el-table :data="ip.list" v-loading="ip.loading" style="width:100%" stripe
            :header-cell-style="{ background: 'var(--el-fill-color-light)', color: 'var(--el-text-color-primary)', fontWeight: 600, fontSize: '13px' }"
            :cell-style="{ fontSize: '13px' }"
            @selection-change="(rows: any[]) => ipSelected = rows">
            <el-table-column type="selection" width="42" />
            <el-table-column label="IP" width="150">
              <template #default="{ row }"><span style="font-family:monospace">{{ row.ip }}</span></template>
            </el-table-column>
            <el-table-column label="端口" width="80">
              <template #default="{ row }"><el-tag type="info" size="small" style="font-family:monospace">{{ row.port }}</el-tag></template>
            </el-table-column>
            <el-table-column prop="protocol" label="协议" width="70" />
            <el-table-column label="服务" width="100">
              <template #default="{ row }"><el-tag v-if="row.service" size="small">{{ row.service }}</el-tag></template>
            </el-table-column>
            <el-table-column label="产品" min-width="180">
              <template #default="{ row }">
                <el-tag v-for="p in row.products" :key="p" type="success" size="small" style="margin-right:4px">{{ p }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="banner" label="Banner" min-width="200" show-overflow-tooltip />
            <el-table-column label="来源" min-width="140">
              <template #default="{ row }">
                <span v-if="!row.sources?.length" style="color:var(--el-text-color-disabled)">—</span>
                <template v-else>
                  <el-tag v-for="s in row.sources" :key="s" size="small" :type="sourceTagType(s)" effect="plain" style="margin-right:3px;margin-bottom:2px">{{ s }}</el-tag>
                </template>
              </template>
            </el-table-column>
            <el-table-column label="发现时间" width="170">
              <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
            </el-table-column>
          </el-table>
          <el-pagination v-if="ip.total > pageSize" v-model:current-page="ip.page" :page-size="pageSize"
            :total="ip.total" layout="total, prev, pager, next"
            style="margin-top:14px;justify-content:flex-end" @current-change="fetchIP()" />
        </template>
      </el-tab-pane>

      <!-- ══ 子域名 ══ -->
      <el-tab-pane name="subdomain">
        <template #label><span>子域名</span></template>
        <div class="search-bar">
          <el-select v-model="subdomainForm.field" style="width:120px" size="default">
            <el-option v-for="f in subdomainFields" :key="f.value" :label="f.label" :value="f.value" />
          </el-select>
          <el-input v-model="subdomainForm.value" clearable style="width:200px"
            :placeholder="`输入${subdomainFields.find(f=>f.value===subdomainForm.field)?.label ?? ''}…`"
            @keyup.enter="addSubdomainChip">
          </el-input>
          <el-button @click="addSubdomainChip">添加筛选</el-button>
          <el-button v-if="subdomainChips.length" type="danger" plain size="small" @click="clearSubdomainChips">清除筛选</el-button>
          <el-tag v-for="c in subdomainChips" :key="c.raw" closable size="small" @close="removeSubdomainChip(c.raw)">
            {{ c.label }}="{{ c.val }}"
          </el-tag>
        </div>
        <div class="tab-toolbar">
          <el-popconfirm title="批量删除选中子域名？" @confirm="batchDelete('subdomain', subSelected, fetchSubdomain)">
            <template #reference>
              <el-button type="danger" plain :disabled="!subSelected.length" size="small">
                <el-icon><Delete /></el-icon>批量删除({{ subSelected.length }})
              </el-button>
            </template>
          </el-popconfirm>
        </div>
        <el-table :data="subdomain.list" v-loading="subdomain.loading" style="width:100%" stripe
          :header-cell-style="{ background: 'var(--el-fill-color-light)', color: 'var(--el-text-color-primary)', fontWeight: 600, fontSize: '13px' }"
          :cell-style="{ fontSize: '13px' }"
          @selection-change="(rows: any[]) => subSelected = rows">
          <el-table-column type="selection" width="42" />
          <el-table-column label="域名" min-width="300">
            <template #default="{ row }">
              <a class="asset-link" :href="assetURL(row.domain)" target="_blank">{{ normalizeAssetURL(row.domain) }}</a>
            </template>
          </el-table-column>
          <el-table-column label="类型" width="80" align="center">
            <template #default="{ row }"><el-tag :type="dnsType(row.dns_type)" size="small" effect="plain" round>{{ row.dns_type || 'A' }}</el-tag></template>
          </el-table-column>
          <el-table-column label="IP" min-width="160">
            <template #default="{ row }">
              <div class="ip-cell">
                <span v-for="ipVal in row.ips" :key="ipVal" class="ip-badge">{{ ipVal }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="解析值" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">
              <span style="font-family:monospace;font-size:12px;color:var(--el-text-color-regular)">{{ row.value?.join(', ') }}</span>
            </template>
          </el-table-column>
          <el-table-column label="来源" min-width="160">
            <template #default="{ row }">
              <span v-if="!row.sources?.length" style="color:var(--el-text-color-disabled)">—</span>
              <template v-else>
                <el-tag v-for="s in row.sources" :key="s" size="small" :type="sourceTagType(s)" effect="plain" style="margin-right:3px;margin-bottom:2px">{{ s }}</el-tag>
              </template>
            </template>
          </el-table-column>
          <el-table-column label="发现时间" width="170">
            <template #default="{ row }">
              <span style="font-size:12px;color:var(--el-text-color-secondary)">{{ fmtTime(row.created_at) }}</span>
            </template>
          </el-table-column>
        </el-table>
        <el-pagination v-if="subdomain.total > pageSize" v-model:current-page="subdomain.page" :page-size="pageSize"
          :total="subdomain.total" layout="total, prev, pager, next"
          style="margin-top:14px;justify-content:flex-end" @current-change="fetchSubdomain()" />
      </el-tab-pane>

      <!-- ══ 漏洞 ══ -->
      <el-tab-pane name="vuln">
        <template #label><span>漏洞</span></template>
        <div class="search-bar">
          <el-select v-model="vulnForm.field" style="width:120px" size="default">
            <el-option v-for="f in vulnFields" :key="f.value" :label="f.label" :value="f.value" />
          </el-select>
          <el-input v-model="vulnForm.value" clearable style="width:200px"
            :placeholder="`输入${vulnFields.find(f=>f.value===vulnForm.field)?.label ?? ''}…`"
            @keyup.enter="addVulnChip">
          </el-input>
          <el-select v-model="vuln.severity" clearable placeholder="危险等级" style="width:110px" @change="fetchVuln(true)">
            <el-option value="critical" label="严重" />
            <el-option value="high" label="高危" />
            <el-option value="medium" label="中危" />
            <el-option value="low" label="低危" />
            <el-option value="info" label="信息" />
          </el-select>
          <el-button @click="addVulnChip">添加筛选</el-button>
          <el-button v-if="vulnChips.length" type="danger" plain size="small" @click="clearVulnChips">清除筛选</el-button>
          <el-tag v-for="c in vulnChips" :key="c.raw" closable size="small" @close="removeVulnChip(c.raw)">
            {{ c.label }}="{{ c.val }}"
          </el-tag>
        </div>
        <div class="tab-toolbar">
          <el-popconfirm title="批量删除选中漏洞？" @confirm="batchDelete('vuln', vulnSelected, fetchVuln)">
            <template #reference>
              <el-button type="danger" plain :disabled="!vulnSelected.length" size="small">
                <el-icon><Delete /></el-icon>批量删除({{ vulnSelected.length }})
              </el-button>
            </template>
          </el-popconfirm>
        </div>
        <el-table :data="vuln.list" v-loading="vuln.loading" style="width:100%" stripe
          :header-cell-style="{ background: 'var(--el-fill-color-light)', color: 'var(--el-text-color-primary)', fontWeight: 600, fontSize: '13px' }"
          :cell-style="{ fontSize: '13px' }"
          @row-click="openVulnDetail" @selection-change="(rows: any[]) => vulnSelected = rows">
          <el-table-column type="selection" width="42" />
          <el-table-column label="漏洞名称" min-width="220" show-overflow-tooltip>
            <template #default="{ row }"><span class="vuln-name">{{ row.name }}</span></template>
          </el-table-column>
          <el-table-column label="等级" width="80">
            <template #default="{ row }">
              <el-tag :type="sevType(row.severity)" size="small" style="font-weight:600">{{ sevLabel(row.severity) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="目标" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">
              <a class="asset-link" :href="assetURL(row.target)" target="_blank" rel="noopener noreferrer" @click.stop>{{ row.target }}</a>
            </template>
          </el-table-column>
          <el-table-column prop="matched_at" label="匹配位置" min-width="180" show-overflow-tooltip />
          <el-table-column prop="template_id" label="模板" width="160" show-overflow-tooltip />
          <el-table-column label="处理状态" width="120">
            <template #default="{ row }">
              <el-select :model-value="row.status||1" size="small" style="width:100px"
                @change="(v: number) => updateVulnStatus(row, v)" @click.stop>
                <el-option :value="1" label="待处理" />
                <el-option :value="2" label="处理中" />
                <el-option :value="3" label="已忽略" />
                <el-option :value="4" label="疑似" />
                <el-option :value="5" label="已确认" />
                <el-option :value="6" label="已处理" />
              </el-select>
            </template>
          </el-table-column>
          <el-table-column label="发现时间" width="170">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="70" fixed="right">
            <template #default="{ row }">
              <el-button type="primary" link size="small" @click.stop="openVulnDetail(row)">详情</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-pagination v-if="vuln.total > pageSize" v-model:current-page="vuln.page" :page-size="pageSize"
          :total="vuln.total" layout="total, prev, pager, next"
          style="margin-top:14px;justify-content:flex-end" @current-change="fetchVuln()" />
      </el-tab-pane>

      <!-- ══ 目录扫描 ══ -->
      <el-tab-pane name="dir">
        <template #label><span>目录</span></template>
        <div class="search-bar">
          <span style="color:var(--el-text-color-regular);font-size:13px">状态码</span>
          <el-select
            v-model="dir.statusCodes"
            multiple collapse-tags collapse-tags-tooltip
            placeholder="全部" clearable style="width:280px" size="default"
            @change="onDirStatusChange">
            <el-option v-for="o in DIR_STATUS_OPTIONS" :key="o.code" :label="o.label" :value="o.code" />
          </el-select>
          <el-button v-if="dir.statusCodes.length" size="small" plain @click="dir.statusCodes = []; fetchDir(true)">清除</el-button>
        </div>
        <div class="tab-toolbar">
          <el-popconfirm title="批量删除选中目录？" @confirm="batchDelete('dir', dirSelected, fetchDir)">
            <template #reference>
              <el-button type="danger" plain :disabled="!dirSelected.length" size="small">
                <el-icon><Delete /></el-icon>批量删除({{ dirSelected.length }})
              </el-button>
            </template>
          </el-popconfirm>
        </div>
        <el-table class="dir-table" :data="dir.list" v-loading="dir.loading" style="width:100%" stripe
          :header-cell-style="{ background: 'var(--el-fill-color-light)', color: 'var(--el-text-color-primary)', fontWeight: 600, fontSize: '13px' }"
          :cell-style="{ fontSize: '13px' }"
          @selection-change="(rows: any[]) => dirSelected = rows"
          @sort-change="onDirSort">
          <el-table-column type="selection" width="42" />
          <el-table-column label="URL" min-width="280" show-overflow-tooltip>
            <template #default="{ row }">
              <a class="asset-link" :href="assetURL(row.url)" target="_blank">{{ normalizeAssetURL(row.url) }}</a>
            </template>
          </el-table-column>
          <el-table-column prop="path" label="路径" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">
              <code style="font-family:monospace;font-size:12px;color:var(--el-text-color-regular)">{{ row.path }}</code>
            </template>
          </el-table-column>
          <el-table-column prop="status_code" label="状态码" width="110" min-width="110" align="center" sortable="custom">
            <template #default="{ row }">
              <span class="status-code" :style="{ color: statusColor(row.status_code), fontWeight: 600 }">{{ row.status_code }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="content_len" label="大小" width="90" align="right" sortable="custom">
            <template #default="{ row }">{{ fmtBytes(row.content_len) }}</template>
          </el-table-column>
          <el-table-column prop="content_type" label="Content-Type" min-width="180" show-overflow-tooltip />
          <el-table-column label="跳转" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">
              <span v-if="row.redirect_url" style="color:var(--el-text-color-secondary);font-size:12px">{{ row.redirect_url }}</span>
              <span v-else style="color:var(--el-text-color-disabled)">—</span>
            </template>
          </el-table-column>
          <el-table-column label="发现时间" width="170">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>
        <el-pagination v-if="dir.total > pageSize" v-model:current-page="dir.page" :page-size="pageSize"
          :total="dir.total" layout="total, prev, pager, next"
          style="margin-top:14px;justify-content:flex-end" @current-change="fetchDir()" />
      </el-tab-pane>

      <el-tab-pane name="crawler">
        <template #label><span>🕷️ 爬虫</span></template>
        <div class="tab-toolbar">
          <el-popconfirm title="批量删除选中爬虫页面？" @confirm="batchDelete('crawler', crawlerSelected, fetchCrawler)">
            <template #reference>
              <el-button type="danger" plain :disabled="!crawlerSelected.length" size="small">
                <el-icon><Delete /></el-icon>批量删除({{ crawlerSelected.length }})
              </el-button>
            </template>
          </el-popconfirm>
        </div>
        <el-table :data="crawler.list" v-loading="crawler.loading" style="width:100%" stripe
          :header-cell-style="{ background: 'var(--el-fill-color-light)', color: 'var(--el-text-color-primary)', fontWeight: 600, fontSize: '13px' }"
          :cell-style="{ fontSize: '13px' }"
          @selection-change="(rows: any[]) => crawlerSelected = rows"
          @sort-change="onCrawlerSort">
          <el-table-column type="selection" width="42" />
          <el-table-column label="URL" min-width="300" show-overflow-tooltip>
            <template #default="{ row }">
              <a class="asset-link" :href="assetURL(row.url)" target="_blank" @click.stop>{{ normalizeAssetURL(row.url) }}</a>
            </template>
          </el-table-column>
          <el-table-column label="标题" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">
              <span v-if="row.title">{{ row.title }}</span>
              <span v-else style="color:var(--el-text-color-disabled)">—</span>
            </template>
          </el-table-column>
          <el-table-column label="类型" width="140" show-overflow-tooltip>
            <template #default="{ row }">
              <span style="font-size:12px;color:var(--el-text-color-secondary)">{{ row.content_type || '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="content_len" label="大小" width="90" align="right" sortable="custom">
            <template #default="{ row }">{{ fmtBytes(row.content_len) }}</template>
          </el-table-column>
          <el-table-column prop="depth" label="深度" width="60" align="center" sortable="custom">
            <template #default="{ row }">{{ row.depth }}</template>
          </el-table-column>
          <el-table-column label="来源" width="90" align="center">
            <template #default="{ row }">
              <el-tag v-if="row.source === 'headless'" type="warning" size="small">Headless</el-tag>
              <el-tag v-else-if="row.source === 'pdf'" type="danger" size="small">PDF</el-tag>
              <el-tag v-else type="info" size="small">静态</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="发现时间" width="170">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>
        <el-pagination v-if="crawler.total > pageSize" v-model:current-page="crawler.page" :page-size="pageSize"
          :total="crawler.total" layout="total, prev, pager, next"
          style="margin-top:14px;justify-content:flex-end" @current-change="fetchCrawler()" />
      </el-tab-pane>

      <el-tab-pane name="sensitive">
        <template #label><span>敏感信息</span></template>
        <div class="search-bar">
          <el-select v-model="sensitiveForm.field" style="width:120px" size="default">
            <el-option v-for="f in sensitiveFields" :key="f.value" :label="f.label" :value="f.value" />
          </el-select>
          <el-input v-model="sensitiveForm.value" clearable style="width:200px"
            :placeholder="`输入${sensitiveFields.find(f=>f.value===sensitiveForm.field)?.label ?? ''}…`"
            @keyup.enter="addSensitiveChip">
          </el-input>
          <el-button @click="addSensitiveChip">添加筛选</el-button>
          <el-button v-if="sensitiveChips.length" type="danger" plain size="small" @click="clearSensitiveChips">清除筛选</el-button>
          <el-tag v-for="c in sensitiveChips" :key="c.raw" closable size="small" @close="removeSensitiveChip(c.raw)">
            {{ c.label }}="{{ c.val }}"
          </el-tag>
        </div>
        <div class="tab-toolbar">
          <el-popconfirm title="批量删除选中敏感信息？" @confirm="batchDelete('sensitive', sensitiveSelected, fetchSensitive)">
            <template #reference>
              <el-button type="danger" plain :disabled="!sensitiveSelected.length" size="small">
                <el-icon><Delete /></el-icon>批量删除({{ sensitiveSelected.length }})
              </el-button>
            </template>
          </el-popconfirm>
        </div>
        <div class="sensitive-body">
          <!-- 左侧聚合面板 -->
          <div v-if="sensitiveAgg.length" class="sens-agg-panel">
            <div class="stat-header">
              <span>规则聚合 <b>{{ sensitiveAgg.length }}</b></span>
            </div>
            <el-scrollbar max-height="520px">
              <div v-for="item in sensitiveAgg" :key="item.rule_name" class="sens-agg-item" @click="addSensitiveRuleFilter(item.rule_name)">
                <el-tag :type="sensSevType(item.severity)" size="small" effect="plain" style="max-width:130px;overflow:hidden;text-overflow:ellipsis">{{ item.rule_name }}</el-tag>
                <span class="stat-num">{{ item.count }}</span>
              </div>
            </el-scrollbar>
          </div>
          <!-- 右侧表格 -->
          <div class="sensitive-right">
            <el-table :data="sensitive.list" v-loading="sensitive.loading" style="width:100%" stripe
              :header-cell-style="{ background: 'var(--el-fill-color-light)', color: 'var(--el-text-color-primary)', fontWeight: 600, fontSize: '13px' }"
              :cell-style="{ fontSize: '13px' }"
              @selection-change="(rows: any[]) => sensitiveSelected = rows">
              <el-table-column type="selection" width="42" />
              <el-table-column label="规则" min-width="180">
                <template #default="{ row }">
                  <span style="font-weight:600;cursor:pointer" @click="addSensitiveRuleFilter(row.rule_name)">{{ row.rule_name }}</span>
                </template>
              </el-table-column>
              <el-table-column label="等级" width="90">
                <template #default="{ row }">
                  <el-tag :type="sensSevType(row.severity)" size="small" style="font-weight:600">{{ row.severity }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="URL" min-width="240" show-overflow-tooltip>
                <template #default="{ row }">
                  <a class="asset-link" :href="assetURL(row.url)" target="_blank" @click.stop>{{ normalizeAssetURL(row.url) }}</a>
                </template>
              </el-table-column>
              <el-table-column label="命中内容" min-width="240" show-overflow-tooltip>
                <template #default="{ row }">
                  <code style="font-family:monospace;font-size:12px">{{ row.matched }}</code>
                </template>
              </el-table-column>
              <el-table-column label="上下文" min-width="280" show-overflow-tooltip>
                <template #default="{ row }">
                  <span style="color:var(--el-text-color-secondary);font-size:12px">{{ row.context }}</span>
                </template>
              </el-table-column>
              <el-table-column label="来源" width="100" align="center">
                <template #default="{ row }">
                  <el-tag v-if="row.source === 'trufflehog'" type="warning" size="small">TruffleHog</el-tag>
                  <el-tag v-else type="info" size="small">正则</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="验证" width="80" align="center">
                <template #default="{ row }">
                  <el-tag v-if="row.verified === true" type="success" size="small">有效</el-tag>
                  <el-tag v-else-if="row.verified === false" type="danger" size="small">无效</el-tag>
                  <span v-else style="color:var(--el-text-color-disabled)">—</span>
                </template>
              </el-table-column>
              <el-table-column label="发现时间" width="170">
                <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
              </el-table-column>
            </el-table>
            <el-pagination v-if="sensitive.total > pageSize" v-model:current-page="sensitive.page" :page-size="pageSize"
              :total="sensitive.total" layout="total, prev, pager, next"
              style="margin-top:14px;justify-content:flex-end" @current-change="fetchSensitive()" />
          </div>
        </div>
      </el-tab-pane>

    </el-tabs>

    <!-- 图片预览 -->
    <Teleport to="body">
      <div v-if="previewSrc" class="img-preview-mask" @click="previewSrc=''">
        <img :src="previewSrc" class="img-preview-img" @click.stop />
      </div>
    </Teleport>

    <!-- 资产详情抽屉 -->
    <el-drawer v-model="assetDrawer" :title="normalizeAssetURL(assetDetail?.url)" size="720px">
      <div v-if="assetDetail">
        <div v-if="assetDetail.screenshot" class="detail-screenshot">
          <img :src="`/images/screenshots/${assetDetail.screenshot}.png`" class="detail-img"
            @error="(e: any) => e.target.parentElement.style.display='none'" />
        </div>
        <div class="detail-section">
          <div class="detail-label">基础信息</div>
          <el-descriptions :column="2" border size="small">
            <el-descriptions-item label="URL" :span="2">
              <a class="asset-link" :href="assetURL(assetDetail.url)" target="_blank">{{ normalizeAssetURL(assetDetail.url) }}</a>
            </el-descriptions-item>
            <el-descriptions-item label="状态码">
              <span :style="{ color: statusColor(assetDetail.status_code), fontWeight: 600 }">{{ assetDetail.status_code }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="大小">{{ fmtBytes(assetDetail.content_len) }}</el-descriptions-item>
            <el-descriptions-item label="标题" :span="2">{{ assetDetail.title || '—' }}</el-descriptions-item>
            <el-descriptions-item label="Server" :span="2">{{ assetDetail.server || '—' }}</el-descriptions-item>
            <el-descriptions-item label="IP">{{ assetDetail.ip || '—' }}</el-descriptions-item>
            <el-descriptions-item label="Port">{{ assetDetail.port || '—' }}</el-descriptions-item>
          </el-descriptions>
        </div>
        <div v-if="assetDetail.tech?.length" class="detail-section">
          <div class="detail-label">技术栈</div>
          <el-tag v-for="t in assetDetail.tech" :key="t" type="success" style="margin-right:6px;margin-bottom:4px">{{ t }}</el-tag>
        </div>
        <div class="detail-section">
          <div class="detail-label">变更历史</div>
          <div v-if="changesLoading" style="font-size:12px;color:var(--el-text-color-secondary)">加载中…</div>
          <el-empty v-else-if="!changes.length" description="未记录变更" :image-size="60" />
          <el-timeline v-else>
            <el-timeline-item v-for="c in changes" :key="c.id"
              :timestamp="fmtTime(c.created_at)" placement="top" type="primary">
              <div v-for="ch in c.changes" :key="ch.field" class="change-row">
                <span class="change-field">{{ ch.field }}</span>：
                <span class="change-old">{{ ch.old || '(空)' }}</span>
                <span class="change-arrow"> → </span>
                <span class="change-new">{{ ch.new || '(空)' }}</span>
              </div>
            </el-timeline-item>
          </el-timeline>
        </div>
      </div>
    </el-drawer>

    <!-- 漏洞详情抽屉 -->
    <el-drawer v-model="vulnDrawer" title="漏洞详情" size="780px" destroy-on-close>
      <div v-if="selectedVuln" v-loading="vulnDetailLoading" class="vuln-detail-wrap">

        <!-- 顶部：严重程度 + 名称 -->
        <div class="vuln-detail-header">
          <span class="vuln-sev-dot" :class="'sev-' + selectedVuln.severity"></span>
          <span class="vuln-sev-label">{{ sevLabel(selectedVuln.severity) }}</span>
          <h3 class="vuln-detail-title">{{ selectedVuln.name }}</h3>
        </div>

        <!-- 基本信息 -->
        <el-descriptions :column="2" border size="small" class="vuln-detail-desc">
          <el-descriptions-item label="目标" :span="2">
            <span class="mono-text">{{ selectedVuln.target }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="匹配位置" :span="2">
            <span class="mono-text">{{ selectedVuln.matched_at }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="模板 ID">
            <code class="tpl-code">{{ selectedVuln.template_id }}</code>
          </el-descriptions-item>
          <el-descriptions-item label="发现时间">
            {{ selectedVuln.created_at ? new Date(selectedVuln.created_at).toLocaleString() : '—' }}
          </el-descriptions-item>
          <el-descriptions-item label="处理状态" :span="2">
            <el-select :model-value="selectedVuln.status||1" style="width:130px"
              @change="(v: number) => updateVulnStatus(selectedVuln!, v)">
              <el-option :value="1" label="待处理" />
              <el-option :value="2" label="处理中" />
              <el-option :value="3" label="已忽略" />
              <el-option :value="5" label="已确认" />
              <el-option :value="6" label="已处理" />
            </el-select>
          </el-descriptions-item>
        </el-descriptions>

        <!-- Request / Response -->
        <template v-if="selectedVuln.request || selectedVuln.response">
          <el-divider style="margin:20px 0 12px" />
          <div class="req-resp-grid" :class="{ 'req-only': !selectedVuln.response, 'resp-only': !selectedVuln.request }">
            <div v-if="selectedVuln.request" class="code-panel">
              <div class="code-panel-title">
                <span class="code-dot req-dot"></span>Request
              </div>
              <el-scrollbar max-height="380px">
                <pre class="code-block req-block">{{ selectedVuln.request }}</pre>
              </el-scrollbar>
            </div>
            <div v-if="selectedVuln.response" class="code-panel">
              <div class="code-panel-title">
                <span class="code-dot resp-dot"></span>Response
              </div>
              <el-scrollbar max-height="380px">
                <pre class="code-block resp-block">{{ selectedVuln.response }}</pre>
              </el-scrollbar>
            </div>
          </div>
        </template>
        <el-empty v-else-if="!vulnDetailLoading" description="暂无请求/响应数据" style="padding:40px 0" />

      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  assetApi, projectApi, taskApi, subscribeTaskProgress,
  type Project, type Task, type HTTPAsset, type VulnAsset, type VulnStatus, type StatItem, type IPAssetFlat,
} from '@/api'

const route = useRoute()
const projects = ref<Project[]>([])
const tasks = ref<Task[]>([])
const filter = reactive({ project_id: undefined as string|undefined, task_id: undefined as string|undefined })
const activeTab = ref('asset')
const pageSize = 20

const statHidden = ref(false)
const statOpen = ref(['port', 'product'])
const stats = reactive({
  ports: [] as StatItem[],
  techs: [] as StatItem[],
})
const previewSrc = ref('')
const assetTableRef = ref<{ doLayout: () => void; $el?: HTMLElement } | null>(null)

function syncAssetTableHeader(position?: { scrollLeft?: number }) {
  nextTick(() => {
    const root = assetTableRef.value?.$el
    if (!root) return
    const header = root.querySelector<HTMLElement>('.el-table__header-wrapper')
    const body = root.querySelector<HTMLElement>('.el-table__body-wrapper .el-scrollbar__wrap')
    if (!header) return
    const scrollLeft = position?.scrollLeft ?? body?.scrollLeft ?? 0
    if (header.scrollLeft !== scrollLeft) header.scrollLeft = scrollLeft
  })
}

function layoutAssetTable() {
  nextTick(() => {
    assetTableRef.value?.doLayout()
    syncAssetTableHeader()
  })
}

function normalizeAssetURL(value: string | null | undefined): string {
  const raw = String(value || '').trim()
  if (!raw) return ''
  return raw
    // 扫描结果偶尔会把协议重复写入，例如 https://https://host。
    .replace(/^(https?):\/\/(?:https?:\/\/)+/i, '$1://')
    // 同一个端口被拼接两次，例如 host:7443:7443。
    .replace(/:(\d+):\1(?=$|\/)/, ':$1')
}

function assetURL(value: string): string {
  const repaired = normalizeAssetURL(value)
  if (!repaired) return '#'
  if (/^https?:\/\//i.test(repaired)) return repaired
  const port = repaired.match(/:(\d+)$/)?.[1]
  return `${port === '443' ? 'https' : 'http'}://${repaired}`
}

function uniqueTech(values: string[] | null | undefined): string[] {
  if (!Array.isArray(values)) return []
  const seen = new Set<string>()
  return values.filter(value => {
    const key = String(value || '').trim().toLowerCase()
    if (!key || seen.has(key)) return false
    seen.add(key)
    return true
  })
}

// ── Chip interface ─────────────────────────────────────────────────────────────
interface Chip { label: string; val: string; raw: string }

// ── 表达式自动补全 ────────────────────────────────────────────────────────────
function exprSuggest(input: string, fields: { label: string; value: string }[], cb: (list: any[]) => void) {
  const parts = input.split(/(\s*&&\s*|\s*\|\|\s*)/)
  const lastPart = (parts[parts.length - 1] || '').trim()
  if (!lastPart || lastPart.includes('=') || lastPart.includes('"')) {
    cb([])
    return
  }
  const kw = lastPart.toLowerCase()
  const suggestions = fields
    .filter(f => f.value.includes(kw) || f.label.includes(kw))
    .map(f => ({ value: f.value, label: f.label }))
  cb(suggestions)
}

function onExprSelect(item: { value: string }, target: 'asset' | 'ip') {
  const ref = target === 'asset' ? assetExpr : ipExpr
  const parts = ref.value.split(/(\s*&&\s*|\s*\|\|\s*)/)
  parts[parts.length - 1] = `${item.value}=""`
  ref.value = parts.join('')
  nextTick(() => {
    const el = document.querySelector(
      target === 'asset'
        ? '.search-bar .el-autocomplete input'
        : '.search-bar .el-autocomplete input'
    ) as HTMLInputElement | null
    if (el) {
      const pos = ref.value.length - 1
      el.focus()
      el.setSelectionRange(pos, pos)
    }
  })
}

// ── 资产 tab 筛选 ──────────────────────────────────────────────────────────────
const assetFields = [
  { label: '域名', value: 'domain' },
  { label: 'URL', value: 'url' },
  { label: 'IP', value: 'ip' },
  { label: '端口', value: 'port' },
  { label: '状态码', value: 'statuscode' },
  { label: '标题', value: 'title' },
  { label: '应用组件', value: 'app' },
  { label: '响应头', value: 'banner' },
]
const assetForm = reactive({ field: 'domain', value: '' })
const assetChips = ref<Chip[]>([])
const assetExprMode = ref(false)
const assetExpr = ref('')

function buildAssetQ() {
  if (assetExprMode.value) return assetExpr.value.trim()
  return assetChips.value.map(c => c.raw).join(' && ')
}
function addAssetChip() {
  if (!assetForm.value.trim()) return
  const label = assetFields.find(f => f.value === assetForm.field)?.label ?? assetForm.field
  const raw = `${assetForm.field}="${assetForm.value.trim()}"`
  if (!assetChips.value.find(c => c.raw === raw)) {
    assetChips.value.push({ label, val: assetForm.value.trim(), raw })
    fetchAsset(true)
  }
  assetForm.value = ''
}
function removeAssetChip(raw: string) { assetChips.value = assetChips.value.filter(c => c.raw !== raw); fetchAsset(true) }
function clearAssetChips() { assetChips.value = []; fetchAsset(true) }

// addTag 供侧边栏统计面板点击使用（添加到资产 chips）
function addTag(type: string, value: any) {
  const fieldMap: Record<string, string> = { port: 'port', service: 'service', app: 'app', tag: 'app' }
  const field = fieldMap[type] ?? type
  const label = assetFields.find(f => f.value === field)?.label ?? field
  const raw = `${field}="${value}"`
  if (!assetChips.value.find(c => c.raw === raw)) {
    assetChips.value.push({ label, val: String(value), raw })
    fetchAsset(true)
  }
}

// ── IP/端口 tab 筛选 ───────────────────────────────────────────────────────────
const ipFields = [
  { label: 'IP', value: 'ip' },
  { label: '端口', value: 'port' },
  { label: '服务', value: 'service' },
  { label: 'Banner', value: 'banner' },
]
const ipForm = reactive({ field: 'ip', value: '' })
const ipChips = ref<Chip[]>([])
const ipExprMode = ref(false)
const ipExpr = ref('')

function buildIpQ() {
  if (ipExprMode.value) return ipExpr.value.trim()
  return ipChips.value.map(c => c.raw).join(' && ')
}
function addIpChip() {
  if (!ipForm.value.trim()) return
  const label = ipFields.find(f => f.value === ipForm.field)?.label ?? ipForm.field
  const raw = `${ipForm.field}="${ipForm.value.trim()}"`
  if (!ipChips.value.find(c => c.raw === raw)) {
    ipChips.value.push({ label, val: ipForm.value.trim(), raw })
    fetchIP(true)
  }
  ipForm.value = ''
}
function removeIpChip(raw: string) { ipChips.value = ipChips.value.filter(c => c.raw !== raw); fetchIP(true) }
function clearIpChips() { ipChips.value = []; fetchIP(true) }

// ── 子域名 tab 筛选 ────────────────────────────────────────────────────────────
const subdomainFields = [
  { label: '域名', value: 'domain' },
  { label: 'IP', value: 'ip' },
  { label: 'DNS类型', value: 'dns_type' },
]
const subdomainForm = reactive({ field: 'domain', value: '' })
const subdomainChips = ref<Chip[]>([])

function buildSubdomainQ() { return subdomainChips.value.map(c => c.raw).join(' && ') }
function addSubdomainChip() {
  if (!subdomainForm.value.trim()) return
  const label = subdomainFields.find(f => f.value === subdomainForm.field)?.label ?? subdomainForm.field
  const raw = `${subdomainForm.field}="${subdomainForm.value.trim()}"`
  if (!subdomainChips.value.find(c => c.raw === raw)) {
    subdomainChips.value.push({ label, val: subdomainForm.value.trim(), raw })
    fetchSubdomain(true)
  }
  subdomainForm.value = ''
}
function removeSubdomainChip(raw: string) { subdomainChips.value = subdomainChips.value.filter(c => c.raw !== raw); fetchSubdomain(true) }
function clearSubdomainChips() { subdomainChips.value = []; fetchSubdomain(true) }

// ── 敏感信息 tab 筛选 ──────────────────────────────────────────────────────────
const sensitiveFields = [
  { label: '规则名', value: 'rule_name' },
  { label: 'URL', value: 'url' },
  { label: '命中内容', value: 'matched' },
]
const sensitiveForm = reactive({ field: 'rule_name', value: '' })
const sensitiveChips = ref<Chip[]>([])

function buildSensitiveQ() { return sensitiveChips.value.map(c => c.raw).join(' && ') }
function addSensitiveChip() {
  if (!sensitiveForm.value.trim()) return
  const label = sensitiveFields.find(f => f.value === sensitiveForm.field)?.label ?? sensitiveForm.field
  const raw = `${sensitiveForm.field}="${sensitiveForm.value.trim()}"`
  if (!sensitiveChips.value.find(c => c.raw === raw)) {
    sensitiveChips.value.push({ label, val: sensitiveForm.value.trim(), raw })
    fetchSensitive(true)
  }
  sensitiveForm.value = ''
}
function removeSensitiveChip(raw: string) { sensitiveChips.value = sensitiveChips.value.filter(c => c.raw !== raw); fetchSensitive(true) }
function clearSensitiveChips() { sensitiveChips.value = []; fetchSensitive(true) }
function addSensitiveRuleFilter(ruleName: string) {
  const raw = `rule_name="${ruleName}"`
  if (!sensitiveChips.value.find(c => c.raw === raw)) {
    sensitiveChips.value.push({ label: '规则名', val: ruleName, raw })
    fetchSensitive(true)
  }
}

// ── 漏洞 tab 筛选 ─────────────────────────────────────────────────────────────
const vulnFields = [
  { label: '漏洞名', value: 'name' },
  { label: '目标', value: 'target' },
  { label: '模板ID', value: 'template_id' },
]
const vulnForm = reactive({ field: 'name', value: '' })
const vulnChips = ref<Chip[]>([])

function buildVulnQ() { return vulnChips.value.map(c => c.raw).join(' && ') }
function addVulnChip() {
  if (!vulnForm.value.trim()) return
  const label = vulnFields.find(f => f.value === vulnForm.field)?.label ?? vulnForm.field
  const raw = `${vulnForm.field}="${vulnForm.value.trim()}"`
  if (!vulnChips.value.find(c => c.raw === raw)) {
    vulnChips.value.push({ label, val: vulnForm.value.trim(), raw })
    fetchVuln(true)
  }
  vulnForm.value = ''
}
function removeVulnChip(raw: string) { vulnChips.value = vulnChips.value.filter(c => c.raw !== raw); fetchVuln(true) }
function clearVulnChips() { vulnChips.value = []; fetchVuln(true) }

// ── State ─────────────────────────────────────────────────────────────────────
const asset     = reactive({ list: [] as HTTPAsset[], total: 0, loading: false, page: 1, view: 'table' as 'table'|'card' })
const ip        = reactive({ list: [] as any[], total: 0, loading: false, page: 1, view: 'agg' as 'agg'|'flat' })
const ipAgg     = reactive({ list: [] as IPAssetFlat[], total: 0, loading: false, page: 1 })
const ipAggPageSize = 10
const subdomain = reactive({ list: [] as any[], total: 0, loading: false, page: 1 })
const vuln      = reactive({ list: [] as VulnAsset[], total: 0, loading: false, page: 1, severity: undefined as string|undefined })
const sensitive = reactive({ list: [] as any[], total: 0, loading: false, page: 1 })
const sensitiveAgg = ref<{ rule_name: string; severity: string; count: number }[]>([])
const crawler   = reactive({ list: [] as any[], total: 0, loading: false, page: 1, sortBy: '' as string, sortOrder: '' as string })
const dir       = reactive({ list: [] as any[], total: 0, loading: false, page: 1, statusCodes: [] as number[], sortBy: '' as string, sortOrder: '' as string })
const DIR_STATUS_OPTIONS = [
  { code: 200, label: '200 OK', tone: 'success' },
  { code: 201, label: '201 Created', tone: 'success' },
  { code: 204, label: '204 No Content', tone: 'success' },
  { code: 301, label: '301 Moved', tone: 'primary' },
  { code: 302, label: '302 Found', tone: 'primary' },
  { code: 307, label: '307 Redirect', tone: 'primary' },
  { code: 401, label: '401 Unauthorized', tone: 'warning' },
  { code: 403, label: '403 Forbidden', tone: 'warning' },
  { code: 404, label: '404 Not Found', tone: 'info' },
  { code: 500, label: '500 Server Error', tone: 'danger' },
  { code: 502, label: '502 Bad Gateway', tone: 'danger' },
  { code: 503, label: '503 Unavailable', tone: 'danger' },
]

const assetSelected = ref<any[]>([])
const ipSelected    = ref<any[]>([])
const subSelected   = ref<any[]>([])
const vulnSelected  = ref<any[]>([])
const dirSelected       = ref<any[]>([])
const crawlerSelected   = ref<any[]>([])
const sensitiveSelected = ref<any[]>([])

const exporting = ref(false)
async function exportAssets(type: string) {
  exporting.value = true
  try {
    const q = { task_id: filter.task_id, project_id: filter.project_id }
    const blob = await assetApi.exportAssets(type, q)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `nscan_${type}_${new Date().toISOString().slice(0,10)}.xlsx`
    a.click()
    URL.revokeObjectURL(url)
  } catch (e: any) { ElMessage.error('导出失败: ' + e.message) }
  finally { exporting.value = false }
}

async function exportAll() {
  exporting.value = true
  try {
    const q = { task_id: filter.task_id, project_id: filter.project_id }
    const blob = await assetApi.exportAllAssets(q)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `nscan_assets_${new Date().toISOString().slice(0,10)}.xlsx`
    a.click()
    URL.revokeObjectURL(url)
  } catch (e: any) { ElMessage.error('导出失败: ' + e.message) }
  finally { exporting.value = false }
}

async function batchDelete(type: 'http'|'port'|'subdomain'|'vuln'|'dir'|'crawler'|'sensitive', rows: any[], refresh: () => void) {
  const ids = rows.map(r => r.id)
  try {
    await assetApi.batchDelete(type, ids)
    ElMessage.success(`已删除 ${ids.length} 条`)
    refresh()
  } catch (e: any) { ElMessage.error(e.message) }
}

const vulnDrawer = ref(false)
const selectedVuln = ref<VulnAsset|null>(null)
const assetDrawer = ref(false)
const assetDetail = ref<HTTPAsset|null>(null)

function baseQ(page: number) {
  return { project_id: filter.project_id, task_id: filter.task_id, limit: pageSize, skip: (page-1)*pageSize }
}

async function fetchAsset(reset = false) {
  if (reset) asset.page = 1
  asset.loading = true
  const q = buildAssetQ() || undefined
  try {
    const [r, s] = await Promise.all([
      assetApi.http({ ...baseQ(asset.page), q }),
      assetApi.stats({ task_id: filter.task_id, project_id: filter.project_id }),
    ])
    asset.list = r.data ?? []; asset.total = r.total
    stats.ports = s.ports ?? []; stats.techs = s.techs ?? []
  } finally {
    asset.loading = false
    layoutAssetTable()
  }
}
async function fetchIP(reset = false) {
  if (ip.view === 'agg') { fetchIPAgg(reset); return }
  if (reset) ip.page = 1
  ip.loading = true
  const q = buildIpQ() || undefined
  try { const r = await assetApi.ports({ ...baseQ(ip.page), q }); ip.list = r.data ?? []; ip.total = r.total }
  finally { ip.loading = false }
}
async function fetchIPAgg(reset = false) {
  if (reset) ipAgg.page = 1
  ipAgg.loading = true
  const q = buildIpQ() || undefined
  try {
    const r = await assetApi.ipAggregated({ project_id: filter.project_id, task_id: filter.task_id, q, limit: ipAggPageSize, skip: (ipAgg.page-1)*ipAggPageSize })
    ipAgg.list = r.data ?? []; ipAgg.total = r.total
  } finally { ipAgg.loading = false }
}
function ipSpanMethod({ row, columnIndex }: { row: IPAssetFlat; column: any; rowIndex: number; columnIndex: number }) {
  if (columnIndex === 0) { // IP
    if (row.ipRowSpan > 0) return [row.ipRowSpan, 1]
    return [0, 0]
  }
  if (columnIndex === 1) { // Port
    if (row.portRowSpan > 0) return [row.portRowSpan, 1]
    return [0, 0]
  }
}
async function fetchSubdomain(reset = false) {
  if (reset) subdomain.page = 1
  subdomain.loading = true
  const q = buildSubdomainQ() || undefined
  try { const r = await assetApi.subdomains({ ...baseQ(subdomain.page), q }); subdomain.list = r.data ?? []; subdomain.total = r.total }
  finally { subdomain.loading = false }
}
async function fetchVuln(reset = false) {
  if (reset) vuln.page = 1
  vuln.loading = true
  const q = buildVulnQ() || undefined
  try { const r = await assetApi.vulns({ ...baseQ(vuln.page), q, severity: vuln.severity }); vuln.list = r.data ?? []; vuln.total = r.total }
  finally { vuln.loading = false }
}
async function fetchSensitive(reset = false) {
  if (reset) sensitive.page = 1
  sensitive.loading = true
  const q = buildSensitiveQ() || undefined
  try {
    const [r, agg] = await Promise.all([
      assetApi.sensitive({ ...baseQ(sensitive.page), q }),
      assetApi.sensitiveAgg({ task_id: filter.task_id, project_id: filter.project_id, q }),
    ])
    sensitive.list = r.data ?? []; sensitive.total = r.total
    sensitiveAgg.value = agg.data ?? []
  } finally { sensitive.loading = false }
}
async function fetchDir(reset = false) {
  if (reset) dir.page = 1
  dir.loading = true
  const status_codes = dir.statusCodes.length ? dir.statusCodes.join(',') : undefined
  const sort_by = dir.sortBy || undefined
  const sort_order = dir.sortOrder || undefined
  try { const r = await assetApi.dirs({ ...baseQ(dir.page), status_codes, sort_by, sort_order }); dir.list = r.data ?? []; dir.total = r.total }
  finally { dir.loading = false }
}
function onDirSort({ prop, order }: { prop: string; order: string | null }) {
  dir.sortBy = order ? prop : ''
  dir.sortOrder = order ?? ''
  fetchDir(true)
}
async function fetchCrawler(reset = false) {
  if (reset) crawler.page = 1
  crawler.loading = true
  const sort_by = crawler.sortBy || undefined
  const sort_order = crawler.sortOrder || undefined
  try { const r = await assetApi.crawler({ ...baseQ(crawler.page), sort_by, sort_order }); crawler.list = r.data ?? []; crawler.total = r.total }
  finally { crawler.loading = false }
}
function onCrawlerSort({ prop, order }: { prop: string; order: string | null }) {
  crawler.sortBy = order ? prop : ''
  crawler.sortOrder = order ?? ''
  fetchCrawler(true)
}
function onDirStatusChange() { fetchDir(true) }
function sensSevType(sev: string): string {
  return ({ critical: 'danger', high: 'warning', medium: '', low: 'info' } as Record<string, string>)[sev] || 'info'
}

function onTabChange(tab: string) {
  if (tab === 'asset') { fetchAsset(); layoutAssetTable() }
  else if (tab === 'ip') fetchIP()
  else if (tab === 'subdomain') fetchSubdomain()
  else if (tab === 'vuln') fetchVuln()
  else if (tab === 'dir') fetchDir()
  else if (tab === 'crawler') fetchCrawler()
  else if (tab === 'sensitive') fetchSensitive()
}

function refreshActiveTab() {
  onTabChange(activeTab.value)
}

let assetTaskWs: WebSocket | null = null
function subscribeFilteredTask(taskID?: string) {
  assetTaskWs?.close()
  assetTaskWs = null
  if (!taskID) return
  assetTaskWs = subscribeTaskProgress(taskID, event => {
    // Refresh once when persisted scan results for the task are complete.
    if (event.kind === 'status' && ['done', 'failed'].includes(event.status ?? '')) {
      refreshActiveTab()
      assetTaskWs?.close()
      assetTaskWs = null
    }
  })
}

function refreshWhenVisible() {
  if (document.visibilityState === 'visible') { refreshActiveTab(); layoutAssetTable() }
}
function onGlobalFilter() {
  asset.page = 1; ip.page = 1; subdomain.page = 1; vuln.page = 1; sensitive.page = 1; dir.page = 1; crawler.page = 1
  fetchAsset(); fetchIP(); fetchSubdomain(); fetchVuln(); fetchSensitive(); fetchDir(); fetchCrawler()
}
async function onProjectChange() {
  filter.task_id = undefined
  tasks.value = (await taskApi.list({ project_id: filter.project_id, limit: 200 }).catch(() => ({ data: [] }))).data ?? []
  onGlobalFilter()
}

const vulnDetailLoading = ref(false)
async function openVulnDetail(row: VulnAsset) {
  selectedVuln.value = row
  vulnDrawer.value = true
  vulnDetailLoading.value = true
  try {
    const detail = await assetApi.vulnDetail(row.id)
    selectedVuln.value = { ...row, ...detail }
  } catch { /* 降级用列表数据 */ }
  finally { vulnDetailLoading.value = false }
}
async function updateVulnStatus(record: VulnAsset, status: number) {
  record.status = status as VulnStatus
  try { await assetApi.updateVulnStatus(record.id, status); ElMessage.success('状态已更新') }
  catch { ElMessage.error('更新失败') }
}
function openAssetDetail(record: HTTPAsset) {
  assetDetail.value = record
  assetDrawer.value = true
  loadChanges('http', record.id)
}

const changes = ref<any[]>([])
const changesLoading = ref(false)
async function loadChanges(type: string, id: string) {
  changesLoading.value = true
  changes.value = []
  try {
    const r = await assetApi.changes(type, id)
    changes.value = r.data || []
  } catch {} finally {
    changesLoading.value = false
  }
}

function statusColor(code: number) {
  if (!code) return 'var(--el-text-color-disabled)'
  if (code < 300) return '#2BA471'; if (code < 400) return '#1456F0'
  if (code < 500) return '#F0883A'; return '#F54A45'
}
function dnsType(t: string) {
  return ({ A: '', CNAME: 'warning', NS: 'danger', TXT: 'success', MX: 'info' } as Record<string, string>)[t] ?? ''
}
// 来源标签配色：外部情报源用暖色（warning/danger），本地探测工具用冷色（info/success）
function sourceTagType(src: string) {
  return ({
    fofa: 'warning', hunter: 'warning', quake: 'warning', shodan: 'danger',
    httpx: 'success', naabu: 'info', tcp: 'info',
    subfinder: 'success', 'crt.sh': 'success', 'dns-brute': '', 'dns-record': 'info',
    baidu: '', bing: '',
  } as Record<string, string>)[src] ?? 'info'
}
function sevType(s: string) {
  return ({ critical: 'danger', high: 'warning', medium: '', low: 'info', info: 'info' } as Record<string, string>)[s] ?? 'info'
}
function sevLabel(s: string) {
  return ({ critical:'严重', high:'高危', medium:'中危', low:'低危', info:'信息' } as Record<string, string>)[s] ?? s
}
function fmtTime(iso: string) { if (!iso) return '—'; return new Date(iso).toLocaleString('zh-CN', { hour12: false }) }
function fmtBytes(n: number) {
  if (!n) return '—'
  if (n < 1024) return `${n} B`; if (n < 1048576) return `${(n/1024).toFixed(1)} KB`
  return `${(n/1048576).toFixed(1)} MB`
}

onMounted(async () => {
  const [pRes, tRes] = await Promise.all([
    projectApi.list({ limit: 200 }).catch(() => ({ data: [], total: 0 })),
    taskApi.list({ limit: 200 }).catch(() => ({ data: [], total: 0 })),
  ])
  projects.value = pRes.data ?? []; tasks.value = tRes.data ?? []
  const requestedTab = String(route.query.tab || '')
  if (['asset', 'ip', 'subdomain', 'vuln', 'dir', 'crawler', 'sensitive'].includes(requestedTab)) {
    activeTab.value = requestedTab
  }
  if (route.query.task_id) filter.task_id = route.query.task_id as string
  if (route.query.project_id) filter.project_id = route.query.project_id as string
  fetchAsset(); fetchIP(); fetchSubdomain(); fetchVuln(); fetchSensitive(); fetchDir(); fetchCrawler()
  subscribeFilteredTask(filter.task_id)
  window.addEventListener('focus', refreshActiveTab)
  window.addEventListener('resize', layoutAssetTable)
  document.addEventListener('visibilitychange', refreshWhenVisible)
  layoutAssetTable()
})

watch(
  () => [route.query.project_id, route.query.task_id, route.query.tab],
  ([projectID, taskID, tab]) => {
    filter.project_id = projectID ? String(projectID) : undefined
    filter.task_id = taskID ? String(taskID) : undefined
    const requestedTab = String(tab || '')
    if (['asset', 'ip', 'subdomain', 'vuln', 'dir', 'crawler', 'sensitive'].includes(requestedTab)) {
      activeTab.value = requestedTab
    }
    subscribeFilteredTask(filter.task_id)
    onGlobalFilter()
  },
)

watch(() => filter.task_id, taskID => subscribeFilteredTask(taskID))

onBeforeUnmount(() => {
  assetTaskWs?.close()
  window.removeEventListener('focus', refreshActiveTab)
  window.removeEventListener('resize', layoutAssetTable)
  document.removeEventListener('visibilitychange', refreshWhenVisible)
})
</script>

<style scoped>
.assets-wrap { }
.tab-toolbar { display:flex; align-items:center; gap:8px; margin-bottom:10px; }
.batch-tip { font-size:13px; color:#856404; margin-right:4px; }
.global-filter { display: flex; gap: 8px; margin-bottom: 12px; align-items: center; }

.export-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 16px;
  height: 32px;
  font-size: 13px;
  font-weight: 500;
  color: #fff;
  background: linear-gradient(135deg, #4a90e2 0%, #2563eb 100%);
  border: none;
  border-radius: 6px;
  cursor: pointer;
  box-shadow: 0 1px 4px rgba(37,99,235,.25);
  transition: opacity .15s, box-shadow .15s;
  white-space: nowrap;
}
.export-btn:hover:not(:disabled) { opacity: .88; box-shadow: 0 3px 10px rgba(37,99,235,.35); }
.export-btn:active:not(:disabled) { opacity: .78; box-shadow: none; }
.export-btn:disabled { opacity: .55; cursor: not-allowed; }
.spin { animation: spin .8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

.asset-tabs :deep(.el-tabs__content) { padding: 0; }


.search-bar { display: flex; align-items: center; gap: 8px; padding: 12px 0; flex-wrap: wrap; }
.view-switch { display: flex; align-items: center; border: 1px solid var(--el-border-color-lighter); border-radius: 6px; overflow: hidden; }
.view-btn { padding: 5px 10px; cursor: pointer; color: var(--el-text-color-secondary); font-size: 14px; display: flex; align-items: center; transition: background 0.15s, color 0.15s; }
.view-btn:hover { color: var(--el-color-primary); }
.view-btn.active { background: var(--el-color-primary); color: #fff; }

.asset-body { display: flex; gap: 0; align-items: flex-start; width: 100%; min-width: 0; }

.stat-panel { width: 180px; flex-shrink: 0; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; background: var(--el-bg-color-overlay); margin-right: 14px; overflow: hidden; }
.stat-header { display: flex; justify-content: space-between; align-items: center; padding: 10px 12px; font-size: 12.5px; border-bottom: 1px solid var(--el-border-color-lighter); background: var(--el-fill-color-light); }
.stat-header b { color: var(--el-color-primary); }
.stat-hide { font-size: 11px; color: var(--el-text-color-secondary); cursor: pointer; }
.stat-hide:hover { color: var(--el-color-primary); }
.stat-panel :deep(.el-collapse-item__header) { padding: 0 12px; font-size: 12.5px; font-weight: 600; height: 36px; }
.stat-panel :deep(.el-collapse-item__content) { padding: 0 8px 8px; }
.stat-item { display: flex; align-items: center; justify-content: space-between; padding: 3px 0; cursor: pointer; border-radius: 4px; }
.stat-item:hover { background: var(--el-color-primary-light-9); }
.stat-num { font-size: 11px; color: var(--el-text-color-secondary); min-width: 24px; text-align: right; }

.stat-collapsed { width: 28px; flex-shrink: 0; margin-right: 14px; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; background: var(--el-fill-color-light); display: flex; flex-direction: column; align-items: center; padding: 12px 0; cursor: pointer; color: var(--el-text-color-secondary); font-size: 14px; }
.stat-collapsed:hover { color: var(--el-color-primary); border-color: var(--el-color-primary-light-5); }

.asset-right { flex: 1 1 0; width: 0; min-width: 0; overflow: hidden; }
.asset-right :deep(.asset-table) { width: 100%; max-width: 100%; }
.asset-right :deep(.el-table__header-wrapper),
.asset-right :deep(.el-table__body-wrapper) { max-width: 100%; }

/* 卡片网格 */
.card-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; }
@media (max-width: 1400px) { .card-grid { grid-template-columns: repeat(3, 1fr); } }
@media (max-width: 1000px) { .card-grid { grid-template-columns: repeat(2, 1fr); } }

.asset-card { border: 1px solid var(--el-border-color-lighter); border-radius: 10px; overflow: hidden; cursor: pointer; background: var(--el-bg-color-overlay); transition: box-shadow 0.18s, transform 0.18s, border-color 0.18s; }
.asset-card:hover { box-shadow: var(--el-box-shadow-light); transform: translateY(-2px); border-color: var(--el-color-primary-light-5); }
.card-img-wrap { position: relative; width: 100%; height: 160px; background: var(--el-fill-color-light); overflow: hidden; }
.card-img { width: 100%; height: 100%; object-fit: cover; display: block; }
.card-no-img { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; }
.card-footer { padding: 10px 12px; }
.card-row1 { display: flex; align-items: center; justify-content: space-between; gap: 6px; margin-bottom: 4px; }
.card-title { font-size: 13px; font-weight: 500; color: var(--el-text-color-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex: 1; }
.card-url { display: block; font-size: 12px; color: var(--el-color-primary); text-decoration: none; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-bottom: 6px; }
.card-url:hover { text-decoration: underline; }
.card-tags { display: flex; flex-wrap: wrap; gap: 4px; }

.asset-link { color: var(--el-color-primary); text-decoration: none; font-size: 13px; }
.full-domain { white-space: normal; overflow-wrap: anywhere; }
.asset-link:hover { text-decoration: underline; }
.status-code { font-family: monospace; font-weight: 600; }
.ip-cell { display: flex; flex-direction: column; gap: 2px; }
.ip-badge { display: inline-block; background: var(--el-fill-color-light); border-radius: 4px; padding: 2px 8px; font-size: 12px; font-family: monospace; margin: 1px 0; color: var(--el-text-color-regular); }
.vuln-name { color: var(--el-text-color-primary); font-weight: 500; cursor: pointer; }
.vuln-name:hover { color: var(--el-color-primary); }

/* 图片预览 */
.img-preview-mask { position: fixed; inset: 0; background: rgba(0,0,0,0.75); z-index: 9999; display: flex; align-items: center; justify-content: center; cursor: zoom-out; }
.img-preview-img { max-width: 90vw; max-height: 90vh; object-fit: contain; cursor: default; }

/* 详情抽屉 */
.detail-screenshot { width: 100%; background: #1d2129; display: flex; align-items: center; justify-content: center; max-height: 280px; overflow: hidden; }
.detail-img { width: 100%; object-fit: contain; max-height: 280px; display: block; }
.detail-section { padding: 14px 0; border-bottom: 1px solid var(--el-border-color-lighter); }
.detail-label { font-size: 11px; font-weight: 600; color: var(--el-text-color-secondary); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 10px; }
/* ── 旧样式保留兼容 ─────────────────────────── */
.vuln-summary { padding: 0 0 16px; border-bottom: 1px solid var(--el-border-color-lighter); }
.vuln-title { margin: 10px 0 12px; font-size: 16px; font-weight: 600; color: var(--el-text-color-primary); }
.code-title { padding: 8px 0; font-size: 12px; font-weight: 600; color: var(--el-text-color-secondary); text-transform: uppercase; letter-spacing: 0.5px; border-top: 1px solid var(--el-border-color-lighter); margin-top: 12px; }
.code-block { margin: 0; padding: 12px; font-family: monospace; font-size: 12px; line-height: 1.7; color: var(--el-text-color-primary); white-space: pre-wrap; word-break: break-all; background: var(--el-fill-color-light); border-radius: 6px; }
/* ── 新漏洞详情 ─────────────────────────────── */
.vuln-detail-wrap { padding: 4px 0; }
.vuln-detail-header { display: flex; align-items: center; gap: 8px; margin-bottom: 16px; flex-wrap: wrap; }
.vuln-sev-dot { width: 10px; height: 10px; border-radius: 50%; flex-shrink: 0; }
.sev-critical .vuln-sev-dot, .vuln-sev-dot.sev-critical { background: #e74c3c; }
.sev-high .vuln-sev-dot, .vuln-sev-dot.sev-high { background: #e67e22; }
.sev-medium .vuln-sev-dot, .vuln-sev-dot.sev-medium { background: #f1c40f; }
.sev-low .vuln-sev-dot, .vuln-sev-dot.sev-low { background: #3498db; }
.sev-info .vuln-sev-dot, .vuln-sev-dot.sev-info { background: #2ecc71; }
.vuln-sev-label { font-size: 12px; font-weight: 600; color: var(--el-text-color-secondary); text-transform: uppercase; letter-spacing: 0.5px; }
.vuln-detail-title { margin: 0; font-size: 16px; font-weight: 600; color: var(--el-text-color-primary); line-height: 1.4; }
.vuln-detail-desc { margin-bottom: 4px; }
.mono-text { font-family: monospace; font-size: 12px; word-break: break-all; }
.tpl-code { background: var(--el-fill-color); padding: 2px 6px; border-radius: 4px; font-size: 12px; }
.req-resp-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.req-resp-grid.req-only, .req-resp-grid.resp-only { grid-template-columns: 1fr; }
.code-panel { border: 1px solid var(--el-border-color); border-radius: 6px; overflow: hidden; }
.code-panel-title { display: flex; align-items: center; gap: 6px; padding: 7px 12px; font-size: 12px; font-weight: 600; color: var(--el-text-color-secondary); background: var(--el-fill-color-light); border-bottom: 1px solid var(--el-border-color); text-transform: uppercase; letter-spacing: 0.5px; }
.code-dot { width: 8px; height: 8px; border-radius: 50%; }
.req-dot { background: #409EFF; }
.resp-dot { background: #67C23A; }
.req-block, .resp-block { margin: 0; padding: 12px; font-family: 'JetBrains Mono', 'Fira Code', monospace; font-size: 12px; line-height: 1.65; color: var(--el-text-color-primary); white-space: pre-wrap; word-break: break-all; background: transparent; }
.change-row { font-size: 12px; line-height: 1.8; word-break: break-all; }
.change-field { font-weight: 600; color: var(--el-color-primary); }
.change-old { color: var(--el-color-danger); text-decoration: line-through; }
.change-arrow { color: var(--el-text-color-secondary); margin: 0 4px; }
.change-new { color: var(--el-color-success); font-weight: 500; }

/* 敏感信息聚合 */
.sensitive-body { display: flex; gap: 0; align-items: flex-start; }
.sens-agg-panel { width: 200px; flex-shrink: 0; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; background: var(--el-bg-color-overlay); margin-right: 14px; overflow: hidden; }
.sens-agg-item { display: flex; align-items: center; justify-content: space-between; padding: 5px 10px; cursor: pointer; border-radius: 4px; }
.sens-agg-item:hover { background: var(--el-color-primary-light-9); }
.sensitive-right { flex: 1; min-width: 0; }
</style>
