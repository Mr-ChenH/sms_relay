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
const messageLength = computed(() => Array.from(sendForm.value.body).length)
const estimatedSegments = computed(() => {
  if (messageLength.value === 0) return 0
  const hasNonGsmCharacter = /[^\x00-\x7F]/.test(sendForm.value.body)
  const singleLimit = hasNonGsmCharacter ? 70 : 160
  const multipartLimit = hasNonGsmCharacter ? 67 : 153
  return messageLength.value <= singleLimit ? 1 : Math.ceil(messageLength.value / multipartLimit)
})
const normalizedPhone = computed(() => sendForm.value.phone.trim())
const resultStatusLabel = computed(() => {
  if (!props.sendResult) return ''
  const labels: Record<string, string> = {
    pending: '等待终端领取',
    claimed: '终端已领取',
    running: '正在发送',
    succeeded: '发送成功',
    failed: '发送失败',
    timed_out: '任务超时',
  }
  return labels[props.sendResult.status] || props.sendResult.status
})
const canSubmit = computed(() => Boolean(
  selectedDevice.value && normalizedPhone.value && sendForm.value.body.trim()
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
  <section class="page send-sms-page">
    <div class="page-head">
      <div><h1>发送短信</h1><p>选择发送终端并创建任务，终端领取后执行并回传结果。</p></div>
    </div>

    <div class="send-workspace">
      <form class="card send-compose" @submit.prevent="emit('createSendTask')">
        <div class="card-head send-panel-head">
          <div><b>短信任务</b><small>确认终端当前 SIM 信息后填写接收号码与内容</small></div>
          <span class="send-step">新建</span>
        </div>

        <div class="send-form-body">
          <div class="send-field-group">
            <label for="send-device">发送终端</label>
            <select id="send-device" v-model="sendForm.deviceId" class="field" required>
              <option value="" disabled>请选择发送终端</option>
              <option v-for="device in devices" :key="device.id" :value="device.id">{{ deviceOptionLabel(device) }}</option>
            </select>
          </div>

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

          <div class="send-field-group">
            <label for="send-phone">目标号码</label>
            <input id="send-phone" v-model="sendForm.phone" class="field mono" type="tel" inputmode="tel" autocomplete="off" placeholder="+8613800138000" required>
            <small>国际号码建议包含国家或地区代码。</small>
          </div>

          <div class="send-field-group">
            <div class="send-field-label"><label for="send-body">短信内容</label><span>{{ messageLength }} 字符 / 预计 {{ estimatedSegments }} 条</span></div>
            <textarea id="send-body" v-model="sendForm.body" placeholder="输入短信内容" required></textarea>
          </div>
        </div>

        <div class="send-submit-bar">
          <div class="send-submit-summary">
            <span>接收方</span>
            <b class="mono">{{ normalizedPhone || '尚未填写' }}</b>
          </div>
          <button class="btn primary" type="submit" :disabled="!canSubmit">创建发送任务</button>
        </div>
      </form>

      <aside class="card send-result-panel">
        <div class="card-head send-panel-head">
          <div><b>创建结果</b><small>最近一次提交</small></div>
        </div>

        <div v-if="!sendResult" class="send-result-empty">
          <b>尚未创建任务</b>
          <p>填写左侧表单并提交后，这里会显示入队结果和命令编号。</p>
        </div>

        <template v-else>
          <div :class="['send-result-summary', `is-${statusClass(sendResult.status)}`]">
            <span class="send-result-indicator" aria-hidden="true"></span>
            <div><small>任务已提交</small><b>{{ resultStatusLabel }}</b><p>{{ sendResult.message || '任务已创建，等待终端处理。' }}</p></div>
          </div>
          <dl class="send-result-details">
            <div><dt>发送终端</dt><dd>{{ selectedDevice?.name || sendForm.deviceId }}</dd></div>
            <div><dt>接收号码</dt><dd class="mono">{{ normalizedPhone }}</dd></div>
            <div><dt>短信长度</dt><dd>{{ messageLength }} 字符，预计 {{ estimatedSegments }} 条</dd></div>
            <div class="send-result-command"><dt>命令 ID</dt><dd><code>{{ sendResult.commandId }}</code></dd></div>
          </dl>
          <p class="send-result-followup">终端执行结果会记录到审计页面，可通过命令 ID 查询。</p>
        </template>

        <div class="send-notes">
          <b>发送前检查</b>
          <ul>
            <li>确认终端当前号码、ICCID 和运营商。</li>
            <li>离线终端将在重新上线后领取任务。</li>
            <li>长短信可能拆分计费，实际规则由运营商决定。</li>
          </ul>
        </div>
      </aside>
    </div>
  </section>
</template>
