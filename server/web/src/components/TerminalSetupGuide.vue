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
const serverPayload = computed(() => `SERVER|${serverHost.value}`)
const wifiPayloadExample = computed(() => `你的WiFi名称|你的WiFi密码|${serverHost.value}`)
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
    <div class="card-head"><b>终端 BLE 接入配置</b><span class="status info">MQTT only</span></div>
    <div class="setup-body">
      <div>
        <h2>BLE GATT 写入值</h2>
        <dl class="setup-kv">
          <dt>服务名</dt><dd class="mono">SMSCFG-xxxxxx</dd>
          <dt>服务 UUID</dt><dd class="mono">7d6d0001-5f36-4f64-8f2b-ec2a7b3d0101</dd>
          <dt>写入特征 UUID</dt><dd class="mono">7d6d0002-5f36-4f64-8f2b-ec2a7b3d0101</dd>
          <dt>状态特征 UUID</dt><dd class="mono">7d6d0003-5f36-4f64-8f2b-ec2a7b3d0101</dd>
          <dt>服务器地址</dt><dd class="mono">{{ serverHost }}</dd>
          <dt>管理 API</dt><dd class="mono">{{ apiBaseUrl }}</dd>
          <dt>MQTT 端口</dt><dd class="mono">1883</dd>
        </dl>
      </div>
      <div>
        <h2>写入格式</h2>
        <ol class="setup-steps">
          <li>首次配置 WiFi 和服务器：<span class="mono">SSID|PASSWORD|SERVER_HOST</span></li>
          <li>只更新服务器：<span class="mono">SERVER|SERVER_HOST</span></li>
          <li>服务器后端内置 MQTT，终端会自动使用 <span class="mono">http://SERVER_HOST:8080</span> 和 <span class="mono">mqtt://SERVER_HOST:1883</span></li>
        </ol>
        <div class="setup-code mono">{{ wifiPayloadExample }}</div>
        <div class="setup-code mono">{{ serverPayload }}</div>
      </div>
    </div>
    <div v-if="usingLocalhost" class="setup-note danger-note">当前公开配置仍指向 localhost，请在服务端设置 SMS_HUB_PUBLIC_BASE_URL 为 ESP32 可访问的服务器 IP。</div>
    <div v-else-if="configError" class="setup-note danger-note">{{ configError }}</div>
    <div v-else class="setup-note">终端固件已移除本机 Web 管理页面，现场配置统一走 BLE。这里只需要写入服务器 IP；后端单进程同时提供管理 API 和内置 MQTT。</div>
  </div>
</template>
