<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import type { PublicConfig } from '../types'

const publicConfig = ref<PublicConfig | null>(null)
const configError = ref('')

const fallbackHost = computed(() => window.location.hostname)
const apiBaseUrl = computed(() => publicConfig.value?.apiBaseUrl || `http://${fallbackHost.value}:8080`)
const serverHost = computed(() => {
  const apiURL = apiBaseUrl.value
  try {
    return new URL(apiURL).hostname
  } catch {
    return fallbackHost.value
  }
})
const hotspotName = 'SMSHub-XXXXXX'
const provisioningUrl = 'http://192.168.4.1'
const hubAddress = computed(() => serverHost.value)
const usingLocalhost = computed(() => ['localhost', '127.0.0.1', '::1'].includes(serverHost.value))

onMounted(async () => {
  try {
    publicConfig.value = await api.get<PublicConfig>('/api/admin/public-config')
  } catch (err) {
    configError.value = err instanceof Error ? err.message : '无法读取服务端公开配置'
  }
})
</script>

<template>
  <div class="card setup-guide">
    <div class="card-head"><b>终端 SoftAP 接入配置</b><span class="status info">WiFi + MQTT</span></div>
    <div class="setup-body">
      <div>
        <h2>配网入口</h2>
        <dl class="setup-kv">
          <dt>终端热点</dt><dd class="mono">{{ hotspotName }}</dd>
          <dt>配网页地址</dt><dd class="mono">{{ provisioningUrl }}</dd>
          <dt>服务器地址</dt><dd class="mono">{{ hubAddress }}</dd>
          <dt>管理 API</dt><dd class="mono">{{ apiBaseUrl }}</dd>
          <dt>MQTT 端口</dt><dd class="mono">1883</dd>
        </dl>
      </div>
      <div>
        <h2>配置步骤</h2>
        <ol class="setup-steps">
          <li>打开终端，在手机或电脑的 WiFi 列表中连接 <span class="mono">{{ hotspotName }}</span> 热点。</li>
          <li>浏览器访问 <span class="mono">{{ provisioningUrl }}</span>，选择现场 WiFi 并输入密码。</li>
          <li>在“SMS Hub 服务器地址”中填写 <span class="mono">{{ hubAddress }}</span>，保存后等待终端连接。</li>
        </ol>
        <div class="setup-code mono">热点：{{ hotspotName }}</div>
        <div class="setup-code mono">SMS Hub：{{ hubAddress }}</div>
      </div>
    </div>
    <div v-if="usingLocalhost" class="setup-note danger-note">当前公开配置仍指向 localhost，请在服务端设置 SMS_HUB_PUBLIC_BASE_URL 为 ESP32 可访问的服务器 IP。</div>
    <div v-else-if="configError" class="setup-note danger-note">{{ configError }}</div>
    <div v-else class="setup-note">终端现场配置使用 SoftAP 网页。保存成功并连接 WiFi 后热点会自动关闭；连接失败时热点会保持开启，可返回配网页重新配置。</div>
  </div>
</template>
