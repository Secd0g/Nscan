<template>
  <div class="app-layout">
    <!-- 侧边栏 -->
    <aside class="sidebar" :class="{ collapsed }">
      <div class="logo-area">
        <div class="logo-icon">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none">
            <circle cx="12" cy="12" r="10" stroke="#4080ff" stroke-width="2"/>
            <circle cx="12" cy="12" r="4" fill="#4080ff"/>
            <line x1="12" y1="2" x2="12" y2="6.5" stroke="#4080ff" stroke-width="2" stroke-linecap="round"/>
            <line x1="12" y1="17.5" x2="12" y2="22" stroke="#4080ff" stroke-width="2" stroke-linecap="round"/>
            <line x1="2" y1="12" x2="6.5" y2="12" stroke="#4080ff" stroke-width="2" stroke-linecap="round"/>
            <line x1="17.5" y1="12" x2="22" y2="12" stroke="#4080ff" stroke-width="2" stroke-linecap="round"/>
          </svg>
        </div>
        <span v-show="!collapsed" class="logo-text">NSCAN</span>
      </div>

      <nav class="sidebar-nav">
        <div v-for="group in menuGroups" :key="group.title" class="nav-group">
          <div v-show="!collapsed" class="nav-group-title">{{ group.title }}</div>
          <router-link
            v-for="item in group.items"
            :key="item.path"
            :to="item.path"
            class="nav-item"
            :class="{ active: isActive(item) }"
          >
            <el-icon class="nav-icon"><component :is="item.icon" /></el-icon>
            <span v-show="!collapsed" class="nav-label">{{ item.label }}</span>
          </router-link>
        </div>
      </nav>

      <div class="sidebar-footer">
        <button class="collapse-btn" @click="collapsed = !collapsed">
          <el-icon><component :is="collapsed ? 'Expand' : 'Fold'" /></el-icon>
          <span v-show="!collapsed">收起菜单</span>
        </button>
      </div>
    </aside>

    <!-- 主区域 -->
    <div class="main-area">
      <!-- 顶栏 -->
      <header class="top-header">
        <div class="header-left">
          <button class="menu-toggle" @click="collapsed = !collapsed">
            <el-icon><Menu /></el-icon>
          </button>
          <el-breadcrumb separator="/">
            <el-breadcrumb-item to="/">首页</el-breadcrumb-item>
            <el-breadcrumb-item v-if="pageGroup">{{ pageGroup }}</el-breadcrumb-item>
            <el-breadcrumb-item>{{ pageTitle }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <button class="icon-btn" @click="toggleDark" :title="isDark ? '切换亮色' : '切换暗色'">
            <el-icon><component :is="isDark ? 'Sunny' : 'Moon'" /></el-icon>
          </button>
          <button class="icon-btn">
            <el-icon><Bell /></el-icon>
          </button>
          <button class="icon-btn">
            <el-icon><Setting /></el-icon>
          </button>
          <el-dropdown @command="handleAccountCommand">
            <div class="avatar" :title="isAdmin ? '管理员' : '普通用户'">{{ userInitial }}</div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item disabled>{{ username }}</el-dropdown-item>
                <el-dropdown-item command="password">修改密码</el-dropdown-item>
                <el-dropdown-item divided command="logout">退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>

      <!-- 内容 -->
      <main class="content-area">
        <router-view />
      </main>
    </div>

    <el-dialog v-model="passwordDialog" title="修改密码" width="420px" destroy-on-close>
      <el-form ref="passwordFormRef" :model="passwordForm" :rules="passwordRules" label-width="110px">
        <el-form-item label="原密码" prop="old_password">
          <el-input v-model="passwordForm.old_password" type="password" show-password autocomplete="current-password" />
        </el-form-item>
        <el-form-item label="新密码" prop="new_password">
          <el-input v-model="passwordForm.new_password" type="password" show-password autocomplete="new-password" />
        </el-form-item>
        <el-form-item label="确认新密码" prop="confirm_password">
          <el-input v-model="passwordForm.confirm_password" type="password" show-password autocomplete="new-password" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="passwordDialog = false">取消</el-button>
        <el-button type="primary" :loading="passwordLoading" @click="submitPasswordChange">确认修改</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { authApi } from '@/api'
import type { FormInstance, FormRules } from 'element-plus'

const route = useRoute()
const router = useRouter()
const collapsed = ref(false)
const isDark = ref(localStorage.getItem('nscan-theme') === 'dark')

function getUserRole(): string {
  try {
    const u = localStorage.getItem('nscan_user')
    return u ? (JSON.parse(u).role ?? '') : ''
  } catch { return '' }
}
function getUsername(): string {
  try { const u = localStorage.getItem('nscan_user'); return u ? (JSON.parse(u).username ?? '') : '' } catch { return '' }
}
const isAdmin = getUserRole() === 'admin'
const username = getUsername()
const userInitial = username ? username.slice(0, 1).toUpperCase() : (isAdmin ? '管' : '用')
const passwordDialog = ref(false)
const passwordLoading = ref(false)
const passwordFormRef = ref<FormInstance>()
const passwordForm = reactive({ old_password: '', new_password: '', confirm_password: '' })
const passwordRules: FormRules = {
  old_password: [{ required: true, message: '请输入原密码', trigger: 'blur' }],
  new_password: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 8, message: '新密码至少需要 8 位', trigger: 'blur' },
  ],
  confirm_password: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    { validator: (_rule, value, callback) => value === passwordForm.new_password ? callback() : callback(new Error('两次输入的新密码不一致')), trigger: 'blur' },
  ],
}

async function handleAccountCommand(command: string) {
  if (command === 'logout') {
    localStorage.removeItem('nscan_token'); localStorage.removeItem('nscan_user')
    await router.push('/login')
    ElMessage.success('已退出登录')
    return
  }
  if (command === 'password') {
    passwordForm.old_password = ''
    passwordForm.new_password = ''
    passwordForm.confirm_password = ''
    passwordDialog.value = true
  }
}

async function submitPasswordChange() {
  if (!passwordFormRef.value) return
  const valid = await passwordFormRef.value.validate().catch(() => false)
  if (!valid) return
  passwordLoading.value = true
  try {
    await authApi.changePassword(passwordForm)
    passwordDialog.value = false
    localStorage.removeItem('nscan_token')
    localStorage.removeItem('nscan_user')
    ElMessage.success('密码修改成功，请重新登录')
    await router.push('/login')
  } catch (err: any) {
    ElMessage.error(err.message || '修改密码失败')
  } finally {
    passwordLoading.value = false
  }
}

if (isDark.value) {
  document.documentElement.classList.add('dark')
}

function toggleDark() {
  isDark.value = !isDark.value
  if (isDark.value) {
    document.documentElement.classList.add('dark')
    localStorage.setItem('nscan-theme', 'dark')
  } else {
    document.documentElement.classList.remove('dark')
    localStorage.setItem('nscan-theme', 'light')
  }
}

const allMenuGroups = [
  { title: '概览', adminOnly: false, items: [
    { path: '/dashboard',        label: '工作台',      icon: 'Odometer' },
    { path: '/projects',         label: '项目管理',    icon: 'Folder' },
    { path: '/assets',           label: '资产管理',    icon: 'Aim' },
  ]},
  { title: '扫描', adminOnly: false, items: [
    { path: '/tasks',            label: '任务管理',    icon: 'List' },
    { path: '/scheduled',        label: '定时扫描',    icon: 'Clock' },
    { path: '/scan-templates',   label: '扫描模版',    icon: 'Grid' },
  ]},
  { title: '配置', adminOnly: true, items: [
    { path: '/nodes',            label: '扫描节点',    icon: 'Monitor' },
    { path: '/poc',              label: 'POC 管理',    icon: 'WarnTriangleFilled' },
    { path: '/fingerprint',      label: '指纹管理',    icon: 'Document' },
    { path: '/sensitive-rules',  label: '敏感规则',    icon: 'View' },
    { path: '/dicts',            label: '字典管理',    icon: 'SetUp' },
    { path: '/system',           label: '系统配置',    icon: 'Setting' },
  ]},
]
const menuGroups = computed(() => allMenuGroups.filter(g => !g.adminOnly || isAdmin))

function isActive(item: { path: string }) {
  return route.path === item.path || route.path.startsWith(item.path + '/')
}

interface PageMeta { title: string; group?: string }
const titleMap: Record<string, PageMeta> = {
  '/dashboard':        { title: '工作台',   group: '概览' },
  '/projects':         { title: '项目管理', group: '概览' },
  '/tasks':            { title: '任务管理', group: '扫描' },
  '/scan-templates':   { title: '扫描模版', group: '扫描' },
  '/scheduled':        { title: '定时扫描', group: '扫描' },
  '/nodes':            { title: '扫描节点', group: '配置' },
  '/assets':           { title: '资产管理', group: '概览' },
  '/poc':              { title: 'POC 管理', group: '配置' },
  '/fingerprint':      { title: '指纹管理', group: '配置' },
  '/dicts':             { title: '字典管理', group: '配置' },
  '/blacklist':        { title: '扫描黑名单', group: '配置' },
  '/notify':           { title: '通知设置', group: '配置' },
  '/tool-config':      { title: '工具配置', group: '配置' },
  '/sensitive-rules':  { title: '敏感规则', group: '配置' },
  '/system':           { title: '系统配置', group: '配置' },
  '/ai-config':        { title: 'AI 配置', group: '配置' },
  '/mcp-config':       { title: 'MCP 配置', group: '配置' },
}
const pageTitle = computed(() => titleMap[route.path]?.title ?? 'nscan')
const pageGroup = computed(() => titleMap[route.path]?.group ?? '')
</script>

<style scoped>
/* ── 整体布局 ──────────────────────────────────────────────── */
.app-layout {
  display: flex;
  height: 100vh;
  overflow: hidden;
  background: var(--bg-page);
  --bg-page: var(--el-bg-color-page);
  --bg-sidebar: var(--el-bg-color-overlay);
  --bg-header: var(--el-bg-color-overlay);
  --bg-content: var(--el-bg-color-page);
  --sidebar-w: 190px;
  --sidebar-w-col: 60px;
  --text-primary: var(--el-text-color-primary);
  --text-secondary: var(--el-text-color-regular);
  --text-menu: var(--el-text-color-primary);
  --border-color: var(--el-border-color-light);
  --active-bg: #4080ff;
  --active-text: #ffffff;
  --hover-bg: var(--el-fill-color-light);
  --hover-text: #4080ff;
  --shadow-sidebar: 1px 0 0 0 var(--border-color);
}

/* 暗色模式 */
:global(html.dark) .app-layout {
  --bg-page: var(--el-bg-color-page);
  --bg-sidebar: var(--el-bg-color-overlay);
  --bg-header: var(--el-bg-color-overlay);
  --bg-content: var(--el-bg-color-page);
  --text-primary: var(--el-text-color-primary);
  --text-secondary: var(--el-text-color-regular);
  --text-menu: var(--el-text-color-primary);
  --border-color: var(--el-border-color-light);
  --hover-bg: var(--el-fill-color-light);
  --hover-text: var(--el-color-primary);
  --shadow-sidebar: 1px 0 0 0 var(--border-color);
}

/* ── 侧边栏 ──────────────────────────────────────────────── */
.sidebar {
  width: var(--sidebar-w);
  min-width: var(--sidebar-w);
  background: var(--bg-sidebar);
  box-shadow: var(--shadow-sidebar);
  display: flex;
  flex-direction: column;
  transition: width 0.2s ease, min-width 0.2s ease;
  overflow: hidden;
  z-index: 100;
}
.sidebar.collapsed {
  width: var(--sidebar-w-col);
  min-width: var(--sidebar-w-col);
}

/* Logo */
.logo-area {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 14px;
  height: 56px;
  border-bottom: 1px solid var(--border-color);
  flex-shrink: 0;
  overflow: hidden;
  white-space: nowrap;
}
.sidebar.collapsed .logo-area { justify-content: center; padding: 0; }
.logo-icon {
  display: flex; align-items: center; justify-content: center;
  width: 32px; height: 32px; flex-shrink: 0;
}
.logo-text {
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 1px;
}

/* 导航菜单 */
.sidebar-nav {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 8px 8px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.sidebar-nav::-webkit-scrollbar { width: 4px; }
.sidebar-nav::-webkit-scrollbar-thumb { background: var(--border-color); border-radius: 2px; }

/* 菜单分组 */
.nav-group { display: flex; flex-direction: column; gap: 2px; }
.nav-group + .nav-group { margin-top: 6px; padding-top: 6px; border-top: 1px solid var(--border-color); }
.nav-group-title {
  padding: 6px 12px 4px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.5px;
  color: var(--text-secondary);
  opacity: 0.7;
  white-space: nowrap;
}
/* 折叠时隐藏分组标题，仅保留分隔线 */
.sidebar.collapsed .nav-group + .nav-group { margin-top: 8px; padding-top: 8px; }

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 10px;
  height: 40px;
  border-radius: 6px;
  cursor: pointer;
  text-decoration: none;
  color: var(--text-menu);
  font-size: 13.5px;
  white-space: nowrap;
  overflow: hidden;
  transition: background 0.15s, color 0.15s;
  flex-shrink: 0;
}
.nav-item:hover { background: var(--hover-bg); color: var(--hover-text); }
.nav-item.active { background: var(--active-bg); color: var(--active-text); }
.nav-item.active .nav-icon { color: var(--active-text); }

.sidebar.collapsed .nav-item { justify-content: center; padding: 0; }
.sidebar.collapsed .nav-label { display: none; }

.nav-icon { font-size: 16px; flex-shrink: 0; color: inherit; }

/* 底部折叠按钮 */
.sidebar-footer {
  border-top: 1px solid var(--border-color);
  padding: 8px;
  flex-shrink: 0;
}
.collapse-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 0 10px;
  height: 36px;
  border: none;
  background: none;
  border-radius: 6px;
  cursor: pointer;
  color: var(--text-secondary);
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  transition: background 0.15s, color 0.15s;
}
.collapse-btn:hover { background: var(--hover-bg); color: var(--hover-text); }
.sidebar.collapsed .collapse-btn { justify-content: center; padding: 0; }

/* ── 主区域 ──────────────────────────────────────────────── */
.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-width: 0;
}

/* 顶栏 */
.top-header {
  height: 56px;
  background: var(--bg-header);
  border-bottom: 1px solid var(--border-color);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  flex-shrink: 0;
  gap: 16px;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}
.header-right {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.menu-toggle {
  display: none;
  align-items: center;
  justify-content: center;
  width: 32px; height: 32px;
  border: none; background: none;
  border-radius: 6px;
  cursor: pointer;
  color: var(--text-secondary);
  font-size: 16px;
  transition: background 0.15s;
}
.menu-toggle:hover { background: var(--hover-bg); }

.icon-btn {
  display: flex; align-items: center; justify-content: center;
  width: 32px; height: 32px;
  border: none; background: none;
  border-radius: 6px;
  cursor: pointer;
  color: var(--text-secondary);
  font-size: 16px;
  transition: background 0.15s, color 0.15s;
}
.icon-btn:hover { background: var(--hover-bg); color: var(--hover-text); }

.avatar {
  width: 30px; height: 30px;
  border-radius: 50%;
  background: #4080ff;
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  display: flex; align-items: center; justify-content: center;
  cursor: pointer;
  margin-left: 4px;
}

/* 面包屑颜色适配 */
:deep(.el-breadcrumb__inner) { color: var(--text-secondary) !important; font-size: 13px; }
:deep(.el-breadcrumb__item:last-child .el-breadcrumb__inner) {
  color: var(--text-primary) !important;
  font-weight: 500;
}
:deep(.el-breadcrumb__separator) { color: var(--text-secondary) !important; }

/* 内容区 */
.content-area {
  flex: 1;
  overflow-y: auto;
  padding: 20px 24px;
  background: var(--bg-content);
}

/* ── 响应式 ──────────────────────────────────────────────── */
@media (max-width: 768px) {
  .sidebar {
    position: fixed;
    left: 0; top: 0; bottom: 0;
    z-index: 200;
    transform: translateX(0);
    transition: transform 0.2s ease, width 0.2s ease;
  }
  .sidebar.collapsed {
    transform: translateX(-100%);
    width: var(--sidebar-w);
    min-width: var(--sidebar-w);
  }
  .menu-toggle { display: flex; }
  .content-area { padding: 14px 16px; }
}
</style>
