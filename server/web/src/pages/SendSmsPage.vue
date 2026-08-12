<script setup lang="ts">
import { computed } from 'vue'
import type { CommandResult, Device } from '../types'
import { statusClass } from '../utils/ui'

const props = defineProps<{
  devices: Device[]
  sendResult: CommandResult | null
}>()

const sendForm = defineModel<{ deviceId: string; phone: string; body: string }>('sendForm', { required: true })

const selectedDevice = computed(() => props.devices.find((device) => device.id === sendForm.value.deviceId))
const canSubmit = computed(() => Boolean(
  selectedDevice.value && sendForm.value.phone.trim() && sendForm.value.body.trim()
))

function deviceOptionLabel(device: Device) {
  const status = device.status === 'online' ? '在线' : '离线'
  return `${device.name} / ${device.phoneNumber || '号码未知'} / ${device.operator || '运营商未知'} / ${status}`
}

function formatLastSeen(value: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}

const emit = defineEmits<{
  createSendTask: []
}>()
</script>

<template>
  <section class="page">
    <div class="page-head"><div><h1>发送短信</h1><p>通过指定终端创建发送任务，结果由终端领取并回传。</p></div></div>
    <div class="grid layout-tools">
      <form class="card form" @submit.prevent="emit('createSendTask')">
        <h2>新建短信任务</h2>
        <label>发送终端</label>
        <select v-model="sendForm.deviceId" class="field" required>
          <option value="" disabled>请选择发送终端</option>
          <option v-for="device in devices" :key="device.id" :value="device.id">{{ deviceOptionLabel(device) }}</option>
        </select>
        <div v-if="selectedDevice" class="send-device-summary">
          <div class="send-device-heading">
            <div><b>{{ selectedDevice.name }}</b><small class="mono">{{ selectedDevice.deviceId }}</small></div>
            <span :class="['status', statusClass(selectedDevice.status)]">{{ selectedDevice.status === 'online' ? '在线' : '离线' }}</span>
          </div>
          <dl>
            <div><dt>当前号码</dt><dd class="mono">{{ selectedDevice.phoneNumber || '未上报' }}</dd></div>
            <div><dt>运营商</dt><dd>{{ selectedDevice.operator || '未上报' }}</dd></div>
            <div><dt>ICCID</dt><dd class="mono">{{ selectedDevice.iccid || '未上报' }}</dd></div>
            <div><dt>信号</dt><dd>{{ selectedDevice.rssi && selectedDevice.rssi < 0 ? `${selectedDevice.rssi} dBm` : '未知' }}</dd></div>
            <div class="send-device-last-seen"><dt>最后在线</dt><dd>{{ formatLastSeen(selectedDevice.lastSeenAt) }}</dd></div>
          </dl>
          <p v-if="selectedDevice.status !== 'online'" class="send-device-warning">终端当前离线，任务创建后将等待终端上线领取。</p>
          <p v-else-if="!selectedDevice.phoneNumber" class="send-device-warning">终端尚未上报当前号码，请通过 ICCID 和运营商确认 SIM 卡。</p>
        </div>
        <p v-else-if="devices.length === 0" class="send-device-warning">暂无可用终端，暂时无法创建短信任务。</p>
        <label>目标号码</label>
        <input v-model="sendForm.phone" class="field" type="tel" autocomplete="off" placeholder="+8613800138000" required>
        <label>短信内容</label>
        <textarea v-model="sendForm.body" placeholder="输入短信内容" required></textarea>
        <button class="btn primary" :disabled="!canSubmit">创建发送任务</button>
      </form>
      <div class="card detail">
        <h2>任务结果</h2>
        <p v-if="!sendResult" class="muted">提交后显示命令任务状态。</p>
        <p v-else><b>{{ sendResult.status }}</b><br>{{ sendResult.message }}<br><span class="mono">{{ sendResult.commandId }}</span></p>
      </div>
    </div>
  </section>
</template>
