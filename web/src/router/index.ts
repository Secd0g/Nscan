import { createRouter, createWebHistory } from 'vue-router'

const adminRoutes = new Set([
  'POC', 'Fingerprint', 'Dicts', 'Nodes', 'ToolConfig',
  'Blacklist', 'Notify', 'SensitiveRules', 'System', 'AIConfig', 'MCPConfig',
])

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/Login.vue'),
    },
    {
      path: '/',
      component: () => import('@/layout/MainLayout.vue'),
      children: [
        { path: '', redirect: '/dashboard' },
        { path: 'dashboard',      name: 'Dashboard',      component: () => import('@/views/Dashboard.vue') },
        { path: 'projects',       name: 'Projects',       component: () => import('@/views/Projects.vue') },
        { path: 'tasks',          name: 'Tasks',          component: () => import('@/views/Tasks.vue') },
        { path: 'assets',         name: 'Assets',         component: () => import('@/views/Assets.vue') },
{ path: 'scheduled',      name: 'Scheduled',      component: () => import('@/views/Scheduled.vue') },
        { path: 'scan-templates', name: 'ScanTemplates',  component: () => import('@/views/ScanTemplates.vue') },
        // admin-only routes
        { path: 'tool-config',    name: 'ToolConfig',     component: () => import('@/views/ToolConfig.vue'),     meta: { adminOnly: true } },
        { path: 'poc',            name: 'POC',            component: () => import('@/views/POC.vue'),            meta: { adminOnly: true } },
        { path: 'fingerprint',    name: 'Fingerprint',    component: () => import('@/views/Fingerprint.vue'),    meta: { adminOnly: true } },
        { path: 'dicts',          name: 'Dicts',          component: () => import('@/views/Dicts.vue'),          meta: { adminOnly: true } },
        { path: 'nodes',          name: 'Nodes',          component: () => import('@/views/Nodes.vue'),          meta: { adminOnly: true } },
        { path: 'blacklist',      name: 'Blacklist',      component: () => import('@/views/Blacklist.vue'),      meta: { adminOnly: true } },
        { path: 'notify',         name: 'Notify',         component: () => import('@/views/Notify.vue'),         meta: { adminOnly: true } },
        { path: 'sensitive-rules',name: 'SensitiveRules', component: () => import('@/views/SensitiveRules.vue'), meta: { adminOnly: true } },
        { path: 'system',         name: 'System',         component: () => import('@/views/System.vue'),         meta: { adminOnly: true } },
        { path: 'ai-config',      name: 'AIConfig',      component: () => import('@/views/AIConfig.vue'),      meta: { adminOnly: true } },
        { path: 'mcp-config',     name: 'MCPConfig',     component: () => import('@/views/MCPConfig.vue'),     meta: { adminOnly: true } },
      ],
    },
  ],
})

function getUserRole(): string {
  try {
    const u = localStorage.getItem('nscan_user')
    return u ? (JSON.parse(u).role ?? '') : ''
  } catch {
    return ''
  }
}

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('nscan_token')
  if (to.name !== 'Login' && !token) {
    next({ name: 'Login' })
    return
  }
  if (to.meta?.adminOnly && getUserRole() !== 'admin') {
    next({ name: 'Dashboard' })
    return
  }
  next()
})

export { adminRoutes }
export default router
