<template>
  <div class="login-container">
    <div class="login-box">
      <div class="login-header">
        <h2 class="title">Nscan</h2>
        <p class="subtitle">安全扫描平台</p>
      </div>

      <el-form
        ref="loginFormRef"
        :model="form"
        :rules="rules"
        class="login-form"
        @keyup.enter="handleLogin"
      >
        <el-form-item prop="username">
          <el-input
            v-model="form.username"
            placeholder="用户名"
            :prefix-icon="User"
            size="large"
          />
        </el-form-item>

        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="密码"
            :prefix-icon="Lock"
            size="large"
            show-password
          />
        </el-form-item>

        <el-form-item prop="captcha">
          <div class="captcha-row">
            <el-input v-model="form.captcha" placeholder="图形验证码" :prefix-icon="Key" size="large" maxlength="4" />
            <img v-if="captchaImage" :src="captchaImage" class="captcha-image" alt="图形验证码" title="点击刷新" @click="loadCaptcha" />
          </div>
        </el-form-item>

        <el-form-item>
          <el-button
            type="primary"
            class="login-btn"
            :loading="loading"
            size="large"
            @click="handleLogin"
          >
            登录
          </el-button>
        </el-form-item>
      </el-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { User, Lock, Key } from '@element-plus/icons-vue'
import type { FormInstance, FormRules } from 'element-plus'
import { authApi } from '@/api'

const router = useRouter()
const loginFormRef = ref<FormInstance>()
const loading = ref(false)
const captchaImage = ref('')
const captchaId = ref('')

const form = reactive({
  username: '',
  password: '',
  captcha: ''
})

const rules = reactive<FormRules>({
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
  captcha: [{ required: true, message: '请输入图形验证码', trigger: 'blur' }]
})

const loadCaptcha = async () => {
  try {
    const data = await authApi.captcha()
    captchaId.value = data.captcha_id
    captchaImage.value = data.image
    form.captcha = ''
  } catch (err: any) { ElMessage.error(err.message || '验证码加载失败') }
}

const handleLogin = async () => {
  if (!loginFormRef.value) return
  await loginFormRef.value.validate(async (valid) => {
    if (valid) {
      loading.value = true
      try {
        const res = await authApi.login({
          username: form.username,
          password: form.password,
          captcha_id: captchaId.value,
          captcha: form.captcha
        })
        const token = res.data.token
        const user = res.data.user
        localStorage.setItem('nscan_token', token)
        localStorage.setItem('nscan_user', JSON.stringify(user))
        ElMessage.success('登录成功')
        router.push('/')
      } catch (err: any) {
        ElMessage.error(err.message || '登录失败')
        await loadCaptcha()
      } finally {
        loading.value = false
      }
    }
  })
}

onMounted(loadCaptcha)
</script>

<style scoped>
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background: var(--el-bg-color-page);
  background-image: radial-gradient(var(--el-border-color-lighter) 1px, transparent 1px);
  background-size: 20px 20px;
}

.login-box {
  width: 100%;
  max-width: 400px;
  padding: 40px;
  background: var(--el-bg-color);
  border-radius: 12px;
  box-shadow: var(--el-box-shadow-light);
  border: 1px solid var(--el-border-color-light);
}

.login-header {
  text-align: center;
  margin-bottom: 40px;
}

.title {
  margin: 0;
  font-size: 32px;
  font-weight: 700;
  color: var(--el-text-color-primary);
  letter-spacing: 1px;
}

.subtitle {
  margin: 8px 0 0;
  font-size: 14px;
  color: var(--el-text-color-secondary);
}

.login-form {
  margin-top: 20px;
}

.login-btn {
  width: 100%;
  margin-top: 10px;
}
.captcha-row { display: flex; gap: 10px; width: 100%; }
.captcha-row .el-input { flex: 1; min-width: 0; }
.captcha-image { width: 120px; height: 40px; align-self: center; border: 1px solid var(--el-border-color); border-radius: 4px; cursor: pointer; }
</style>
