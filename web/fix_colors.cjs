const fs = require('fs')
const path = require('path')

const colorMap = {
  '#fafafa': 'var(--el-fill-color-light)',
  '#303133': 'var(--el-text-color-primary)',
  '#606266': 'var(--el-text-color-regular)',
  '#86909c': 'var(--el-text-color-secondary)',
  '#c9cdd4': 'var(--el-text-color-disabled)',
  // we will manually handle white and black because sometimes they are correct
}

function processDir(dir) {
  const files = fs.readdirSync(dir)
  for (const file of files) {
    const fullPath = path.join(dir, file)
    const stat = fs.statSync(fullPath)
    if (stat.isDirectory()) {
      processDir(fullPath)
    } else if (file.endsWith('.vue')) {
      let content = fs.readFileSync(fullPath, 'utf8')
      let changed = false
      
      for (const [hex, cssVar] of Object.entries(colorMap)) {
        const regex = new RegExp(hex + '(?![0-9a-zA-Z])', 'gi')
        if (regex.test(content)) {
          content = content.replace(regex, cssVar)
          changed = true
        }
      }
      
      // Layout specific
      if (file.endsWith('MainLayout.vue')) {
        content = content.replace(/--bg-page:\s*#f0f2f5;/g, '--bg-page: var(--el-bg-color-page);')
        content = content.replace(/--bg-sidebar:\s*#ffffff;/g, '--bg-sidebar: var(--el-bg-color-overlay);')
        content = content.replace(/--bg-header:\s*#ffffff;/g, '--bg-header: var(--el-bg-color-overlay);')
        content = content.replace(/--bg-content:\s*#f0f2f5;/g, '--bg-content: var(--el-bg-color-page);')
        content = content.replace(/--text-primary:\s*#1d2129;/g, '--text-primary: var(--el-text-color-primary);')
        content = content.replace(/--text-secondary:\s*#6b7280;/g, '--text-secondary: var(--el-text-color-regular);')
        content = content.replace(/--text-menu:\s*#4b5563;/g, '--text-menu: var(--el-text-color-primary);')
        content = content.replace(/--border-color:\s*#e5e9f0;/g, '--border-color: var(--el-border-color-light);')
        content = content.replace(/--hover-bg:\s*#f3f6ff;/g, '--hover-bg: var(--el-fill-color-light);')
        changed = true
      }
      
      if (changed) {
        fs.writeFileSync(fullPath, content)
        console.log('Fixed', fullPath)
      }
    }
  }
}

processDir('/Users/bob/nscan/web/src')
