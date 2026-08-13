<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import type { Device, EsimProfile, EsimSubscription } from '../types'
import { formatTime, statusClass } from '../utils/ui'

const props = defineProps<{ profiles: EsimProfile[]; devices: Device[]; subscriptions: EsimSubscription[]; savingId: string }>()
const emit = defineEmits<{ refresh: []; save: [profile: EsimProfile, country: string, phoneNumber: string] }>()

const query = ref('')
const availability = ref('all')
const deviceId = ref('all')
const country = ref('all')
const subscription = ref('all')
const sort = ref('attention')
const editingId = ref('')
const drafts = reactive<Record<string, { country: string; phoneNumber: string }>>({})

watch(() => props.profiles, (profiles) => {
  for (const profile of profiles) {
    if (!drafts[profile.id]) drafts[profile.id] = { country: profile.country || '', phoneNumber: profile.phoneNumber || '' }
  }
}, { immediate: true })

const countryOptions = computed(() => [...new Set(props.profiles.map((profile) => profile.country).filter(Boolean))].sort())
const activeFilterCount = computed(() => [availability.value, deviceId.value, country.value, subscription.value].filter((value) => value !== 'all').length + (query.value.trim() ? 1 : 0))
const missingCount = computed(() => props.profiles.filter((profile) => !profile.available).length)
const incompleteCount = computed(() => props.profiles.filter((profile) => !profile.country || !profile.phoneNumber).length)

const rows = computed(() => props.profiles.filter((profile) => {
  const profileSubscription = subscriptionFor(profile)
  if (availability.value === 'available' && !profile.available) return false
  if (availability.value === 'missing' && profile.available) return false
  if (deviceId.value !== 'all' && profile.deviceId !== deviceId.value) return false
  if (country.value !== 'all' && profile.country !== country.value) return false
  if (subscription.value === 'configured' && !profileSubscription) return false
  if (subscription.value === 'unconfigured' && profileSubscription) return false
  if (subscription.value === 'enabled' && !profileSubscription?.enabled) return false
  if (subscription.value === 'disabled' && (!profileSubscription || profileSubscription.enabled)) return false
  const device = props.devices.find((item) => item.id === profile.deviceId)
  const text = [profile.iccid, profile.aid, profile.nickname, profile.profileName, profile.provider, profile.country, profile.phoneNumber, device?.name, device?.deviceId].join(' ').toLowerCase()
  return text.includes(query.value.trim().toLowerCase())
}).sort((a, b) => {
  if (sort.value === 'profile') return profileLabel(a).localeCompare(profileLabel(b), 'zh-CN')
  if (sort.value === 'device') return (deviceFor(a)?.name || '').localeCompare(deviceFor(b)?.name || '', 'zh-CN')
  if (sort.value === 'recent') return new Date(b.lastSeenAt || 0).getTime() - new Date(a.lastSeenAt || 0).getTime()
  const priority = (profile: EsimProfile) => (!profile.available ? 0 : (!profile.country || !profile.phoneNumber ? 1 : 2))
  return priority(a) - priority(b) || profileLabel(a).localeCompare(profileLabel(b), 'zh-CN')
}))

function clearFilters() {
  query.value = ''
  availability.value = 'all'
  deviceId.value = 'all'
  country.value = 'all'
  subscription.value = 'all'
  sort.value = 'attention'
  editingId.value = ''
}

function showAttention() {
  clearFilters()
  availability.value = 'missing'
}

function startEdit(profile: EsimProfile) {
  editingId.value = profile.id
  drafts[profile.id] = { country: profile.country || '', phoneNumber: profile.phoneNumber || '' }
}

function cancelEdit(profile: EsimProfile) {
  drafts[profile.id] = { country: profile.country || '', phoneNumber: profile.phoneNumber || '' }
  editingId.value = ''
}

function saveEdit(profile: EsimProfile) {
  const draft = drafts[profile.id]
  if (!draft || !hasChanges(profile)) return
  emit('save', profile, draft.country.trim(), draft.phoneNumber.trim())
  editingId.value = ''
}

function hasChanges(profile: EsimProfile) {
  const draft = drafts[profile.id]
  return !!draft && (draft.country.trim() !== (profile.country || '') || draft.phoneNumber.trim() !== (profile.phoneNumber || ''))
}

function deviceFor(profile: EsimProfile) {
  return props.devices.find((device) => device.id === profile.deviceId)
}

function subscriptionFor(profile: EsimProfile) {
  return props.subscriptions.find((sub) => sub.profileId === profile.id)
}

function profileLabel(profile: EsimProfile) {
  return profile.nickname || profile.profileName || profile.provider || '未命名 Profile'
}
</script>

<template>
  <section class="page profile-management-page">
    <div class="page-head">
      <div><h1>eSIM Profile 管理</h1><p>维护 Profile 归属、国家地区、号码和订阅保活状态。</p></div>
      <button class="btn" @click="emit('refresh')">刷新</button>
    </div>
    <div class="profile-summary-strip">
      <button :class="['profile-summary-item', { active: availability === 'all' }]" @click="clearFilters"><span>全部 Profile</span><b>{{ profiles.length }}</b></button>
      <button :class="['profile-summary-item danger', { active: availability === 'missing' }]" @click="showAttention"><span>终端中不存在</span><b>{{ missingCount }}</b></button>
      <button class="profile-summary-item warn" @click="clearFilters(); sort = 'attention'"><span>信息待完善</span><b>{{ incompleteCount }}</b></button>
    </div>
    <div class="card profile-management-panel">
      <div class="card-head profile-management-toolbar">
        <div class="profile-panel-heading"><div><b>Profile 列表</b><small>显示 {{ rows.length }} / {{ profiles.length }} 个</small></div><span v-if="activeFilterCount" class="status info">{{ activeFilterCount }} 个过滤条件</span></div>
        <div class="profile-filter-bar">
          <input v-model="query" class="field profile-search" placeholder="搜索终端、号码、ICCID 或运营商">
          <select v-model="deviceId" class="select"><option value="all">全部终端</option><option v-for="device in devices" :key="device.id" :value="device.id">{{ device.name }}</option></select>
          <select v-model="availability" class="select"><option value="all">全部存在状态</option><option value="available">终端中存在</option><option value="missing">已不存在</option></select>
          <select v-model="country" class="select"><option value="all">全部国家/地区</option><option v-for="item in countryOptions" :key="item" :value="item">{{ item }}</option></select>
          <select v-model="subscription" class="select"><option value="all">全部订阅状态</option><option value="configured">已配置订阅</option><option value="unconfigured">未配置订阅</option><option value="enabled">订阅已启用</option><option value="disabled">订阅已停用</option></select>
          <select v-model="sort" class="select"><option value="attention">待处理优先</option><option value="recent">最近确认</option><option value="profile">按 Profile</option><option value="device">按终端</option></select>
          <button v-if="activeFilterCount" class="btn" @click="clearFilters">清除过滤</button>
        </div>
      </div>
      <div class="profile-management-table">
        <table>
          <thead><tr><th>Profile</th><th>所属终端</th><th>状态</th><th>国家/地区</th><th>号码</th><th>订阅保活</th><th>最后确认</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="profile in rows" :key="profile.id" :class="{ 'profile-missing-row': !profile.available }">
              <td><b>{{ profileLabel(profile) }}</b><small class="mono">ICCID {{ profile.iccid || '-' }}</small><small class="mono">AID {{ profile.aid || '-' }}</small></td>
              <td><span class="profile-device-line"><b>{{ deviceFor(profile)?.name || '未知终端' }}</b><span class="mono">{{ deviceFor(profile)?.deviceId || profile.deviceId }}</span></span></td>
              <td><span :class="['status', profile.available ? statusClass(profile.state) : 'gray']">{{ profile.available ? (profile.state === 'enabled' ? '已启用' : '存在') : '终端中不存在' }}</span><small v-if="!profile.available">可能已删除或尚未同步</small></td>
              <td><template v-if="editingId === profile.id"><input v-model="drafts[profile.id].country" class="field compact-field" placeholder="例如：中国" @keydown.esc="cancelEdit(profile)"></template><span v-else>{{ profile.country || '未设置' }}</span></td>
              <td><template v-if="editingId === profile.id"><input v-model="drafts[profile.id].phoneNumber" class="field compact-field mono" placeholder="例如：+8613800138000" @keydown.enter="saveEdit(profile)" @keydown.esc="cancelEdit(profile)"></template><span v-else :class="['mono', { muted: !profile.phoneNumber }]">{{ profile.phoneNumber || '未设置' }}</span></td>
              <td><template v-if="subscriptionFor(profile)"><b>{{ subscriptionFor(profile)?.type === 'recharge' ? '充值提醒' : '短信保活' }}</b><small>每 {{ subscriptionFor(profile)?.intervalDays }} 天 · {{ subscriptionFor(profile)?.enabled ? '已启用' : '已停用' }}</small><span :class="['status', statusClass(subscriptionFor(profile)?.status || '')]">{{ subscriptionFor(profile)?.status }}</span></template><span v-else class="muted">未配置</span></td>
              <td><span v-if="profile.lastSeenAt">{{ formatTime(profile.lastSeenAt) }}</span><span v-else>-</span><small v-if="profile.missingSince">自 {{ formatTime(profile.missingSince) }} 起缺失</small></td>
              <td><div v-if="editingId === profile.id" class="profile-row-actions"><button class="btn small primary" :disabled="savingId === profile.id || !hasChanges(profile)" @click="saveEdit(profile)">{{ savingId === profile.id ? '保存中' : '保存' }}</button><button class="btn small" @click="cancelEdit(profile)">取消</button></div><button v-else class="btn small" :disabled="savingId === profile.id" @click="startEdit(profile)">编辑信息</button></td>
            </tr>
            <tr v-if="rows.length === 0"><td colspan="8" class="empty muted">没有符合条件的 eSIM Profile。</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
