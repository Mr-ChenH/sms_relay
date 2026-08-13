<script setup lang="ts">
import { ref } from 'vue'
import type { Device } from '../types'
import TerminalSetupGuide from '../components/TerminalSetupGuide.vue'
import { statusClass } from '../utils/ui'

const props = defineProps<{ devices: Device[]; savingId: string }>()

const emit = defineEmits<{
  openTools: [deviceId?: string]
  openEsim: [deviceId: string]
  rename: [device: Device, name: string]
}>()

const editingId = ref('')
const nameDraft = ref('')

function startRename(device: Device) {
  editingId.value = device.id
  nameDraft.value = device.name
}

function cancelRename() {
  editingId.value = ''
  nameDraft.value = ''
}

function saveRename(device: Device) {
  const name = nameDraft.value.trim()
  if (!name) return
  emit('rename', device, name)
  cancelRename()
}
</script>

<template>
  <section class="page">
    <div class="page-head"><div><h1>终端</h1><p>注册、监控并远程控制 ESP32 短信终端。</p></div><button class="btn primary" @click="emit('openTools')">终端诊断</button></div>
    <TerminalSetupGuide />
    <div class="card top-gap">
      <table>
        <thead><tr><th>终端</th><th>状态</th><th>SIM/eSIM</th><th>网络</th><th>资源</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="device in devices" :key="device.id">
            <td>
              <div v-if="editingId === device.id" class="device-name-editor"><input v-model="nameDraft" class="field" maxlength="80" autofocus @keydown.enter="saveRename(device)" @keydown.esc="cancelRename"><button class="btn small primary" :disabled="savingId === device.id || !nameDraft.trim()" @click="saveRename(device)">保存</button><button class="btn small" @click="cancelRename">取消</button></div>
              <template v-else><b>{{ device.name }}</b><small class="mono">{{ device.deviceId }}</small></template>
            </td>
            <td><span :class="['status', statusClass(device.status)]">{{ device.status === 'online' ? '在线' : '离线' }}</span></td>
            <td><span class="mono">{{ device.iccid || '-' }}</span><small>{{ device.eid || '-' }}</small><small>号码 {{ device.phoneNumber || '-' }}</small></td>
            <td>{{ device.operator || '-' }}<small>RSSI {{ device.rssi }} dBm</small></td>
            <td>Heap {{ device.freeHeapKb }} KB<small>{{ device.uptime }}</small></td>
            <td><button class="btn small" :disabled="savingId === device.id" @click="startRename(device)">改名</button><button class="btn small primary" @click="emit('openEsim', device.id)">eSIM</button><button class="btn small" @click="emit('openTools', device.id)">命令</button></td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
