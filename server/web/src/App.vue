<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from './api'
import AppShell from './components/AppShell.vue'
import PaginationBar from './components/PaginationBar.vue'
import AuditPage from './pages/AuditPage.vue'
import DevicesPage from './pages/DevicesPage.vue'
import LogsPage from './pages/LogsPage.vue'
import OverviewPage from './pages/OverviewPage.vue'
import SendSmsPage from './pages/SendSmsPage.vue'
import ToolsPage from './pages/ToolsPage.vue'
import type { AppriseService, AppriseTarget, AuditLog, CommandResult, CreateAppriseServiceRequest, CreateAppriseTargetRequest, CreateDeviceCommandRequest, CreateEsimSubscriptionRequest, CreateEsimTaskRequest, CreateRoutingRuleRequest, Dashboard, Device, DeviceCommand, EsimProfile, EsimSubscription, EsimTask, LogEntry, RoutingRule, SMSList, SMSMessage, Page } from './types'
import { formatTime, statusClass } from './utils/ui'

type BarcodeDetectorConstructor = new (options?: { formats?: string[] }) => {
  detect: (source: CanvasImageSource) => Promise<Array<{ rawValue: string }>>
}

declare global {
  interface Window {
    BarcodeDetector?: BarcodeDetectorConstructor
  }
}

const page = ref<Page>('overview')
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const dashboard = ref<Dashboard | null>(null)
const devices = ref<Device[]>([])
const sms = ref<SMSList | null>(null)
const channels = ref<AppriseTarget[]>([])
const appriseServices = ref<AppriseService[]>([])
const rules = ref<RoutingRule[]>([])
const profiles = ref<EsimProfile[]>([])
const esimTasks = ref<EsimTask[]>([])
const esimSubscriptions = ref<EsimSubscription[]>([])
const logs = ref<LogEntry[]>([])
const audit = ref<AuditLog[]>([])
const commands = ref<DeviceCommand[]>([])
const globalSearch = ref('')
const smsQuery = ref('')
const smsPage = ref(1)
const smsPageSize = ref(10)
const selectedSmsId = ref('')
const selectedSms = computed<SMSMessage | null>(() => sms.value?.items.find((item) => item.id === selectedSmsId.value) ?? sms.value?.items[0] ?? null)
const smsActionResult = ref('')
const selectedEsimDeviceId = ref('')
const selectedSubscriptionDeviceId = ref('')
const selectedSubscriptionProfileId = ref('')
const subscriptionDialogDeviceId = ref('')
const selectedEsimDevice = computed(() => devices.value.find((device) => device.id === selectedEsimDeviceId.value) ?? null)
const selectedSubscriptionDevice = computed(() => devices.value.find((device) => device.id === selectedSubscriptionDeviceId.value) ?? null)
const selectedDeviceProfiles = computed(() => profiles.value.filter((profile) => profile.deviceId === selectedEsimDeviceId.value))
const selectedDeviceEsimSyncLog = computed(() => logs.value.find((row) => row.deviceId === selectedEsimDeviceId.value && (row.message.includes('esim profile sync failed') || row.message.includes('eSIM profiles uploaded'))))
const selectedDeviceTasks = computed(() => esimTasks.value.filter((task) => task.deviceId === selectedEsimDeviceId.value))
const esimCommandPage = ref(1)
const esimCommandPageSize = ref(10)
const selectedEsimCommands = computed(() => commands.value
  .filter((item) => item.deviceId === selectedEsimDeviceId.value && item.type.startsWith('esim_'))
  .sort((a, b) => {
    const timeDiff = new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
    if (timeDiff !== 0) return timeDiff
    return Number(b.id.split('-').pop() || 0) - Number(a.id.split('-').pop() || 0)
  }))
const esimCommandTotalPages = computed(() => Math.max(1, Math.ceil(selectedEsimCommands.value.length / esimCommandPageSize.value)))
const pagedEsimCommands = computed(() => {
  const pageNumber = Math.min(esimCommandPage.value, esimCommandTotalPages.value)
  const start = (pageNumber - 1) * esimCommandPageSize.value
  return selectedEsimCommands.value.slice(start, start + esimCommandPageSize.value)
})
const selectedSubscriptionProfiles = computed(() => profiles.value.filter((profile) => profile.deviceId === selectedSubscriptionDeviceId.value))
const selectedSubscriptionRows = computed(() => esimSubscriptions.value.filter((sub) => sub.deviceId === selectedSubscriptionDeviceId.value))
const subscriptionDialogProfiles = computed(() => profiles.value.filter((profile) => profile.deviceId === subscriptionDialogDeviceId.value))
const availableSubscriptionDialogProfiles = computed(() => subscriptionDialogProfiles.value.filter((profile) => !esimSubscriptions.value.some((sub) => sub.profileId === profile.id)))
const subscriptionDialogSelectedProfile = computed(() => subscriptionDialogProfiles.value.find((profile) => profile.id === esimSubscriptionForm.value.profileId) ?? null)
const subscriptionDialogDevice = computed(() => devices.value.find((device) => device.id === subscriptionDialogDeviceId.value) ?? null)
const selectedSubscriptionProfile = computed(() => selectedSubscriptionProfiles.value.find((profile) => profile.id === selectedSubscriptionProfileId.value) ?? null)
const selectedSubscriptionConfig = computed(() => esimSubscriptions.value.find((sub) => sub.profileId === selectedSubscriptionProfileId.value) ?? null)
const availableSubscriptionProfiles = computed(() => selectedSubscriptionProfiles.value.filter((profile) => !esimSubscriptions.value.some((sub) => sub.profileId === profile.id)))
const sendForm = ref({ deviceId: '', phone: '', body: '' })
const esimTaskForm = ref({ activationCode: '', smdpAddress: '', confirmationCode: '' })
const esimTaskResult = ref('')
const esimCommandResult = ref('')
const profileCommandBusy = ref<Record<string, boolean>>({})
const toolDeviceId = ref('')
const toolATCommand = ref('AT+CSQ')
const toolResult = ref('')
const activeToolCommandId = ref('')
const esimQrResult = ref('')
const esimQrInput = ref<HTMLInputElement | null>(null)
const sendResult = ref<CommandResult | null>(null)
const showEsimSubscriptionDialog = ref(false)
const editingEsimSubscriptionId = ref('')
const showAppriseForm = ref(false)
const showAppriseServiceForm = ref(false)
const showRoutingRuleForm = ref(false)
const editingAppriseTargetId = ref('')
const appriseForm = ref({
  serviceId: '',
  name: '',
  configKey: 'default',
  tagsText: 'all',
  enabled: true,
  titleTemplate: '短信来自 {{sender}}',
  bodyTemplate: '{{body}}\n\n终端: {{device}}\n时间: {{timestamp}}'
})
const appriseSaveResult = ref('')
const appriseServiceForm = ref({ name: '备用 Apprise API', baseUrl: 'http://localhost:8000', enabled: true })
const editingAppriseServiceId = ref('')
const appriseServiceResult = ref('')
const routingRuleForm = ref({ name: '', senderContains: '', bodyKeywordsText: '', deviceIds: [] as string[], tagsText: '', targetIds: [] as string[], enabled: true })
const editingRoutingRuleId = ref('')
const routingRuleResult = ref('')
const esimSubscriptionForm = ref({
  profileId: '',
  enabled: true,
  type: 'recharge' as 'recharge' | 'sms_keepalive',
  intervalDays: 30,
  startAt: new Date().toISOString().slice(0, 16),
  rechargeAmount: '20 CNY',
  keepaliveNumber: '10086',
  keepaliveMessage: 'CXLL',
  targetIds: [] as string[],
  note: ''
})
const esimSubscriptionResult = ref('')

function applyDeviceDefaults(devs = devices.value) {
  if (!sendForm.value.deviceId && devs.length > 0) sendForm.value.deviceId = devs[0].id
  if (!toolDeviceId.value && devs.length > 0) toolDeviceId.value = devs[0].id
  if (!selectedEsimDeviceId.value && devs.length > 0) selectedEsimDeviceId.value = devs[0].id
  if (!selectedSubscriptionDeviceId.value && devs.length > 0) selectedSubscriptionDeviceId.value = devs[0].id
  if (!subscriptionDialogDeviceId.value && devs.length > 0) subscriptionDialogDeviceId.value = devs[0].id
}

async function loadDevices() {
  devices.value = await api.get<Device[]>('/api/admin/devices')
  applyDeviceDefaults()
}

async function loadOverview() {
  const [dash, devs] = await Promise.all([
    api.get<Dashboard>('/api/admin/dashboard'),
    api.get<Device[]>('/api/admin/devices')
  ])
  dashboard.value = dash
  devices.value = devs
  applyDeviceDefaults(devs)
}

async function loadSmsPage() {
  await searchSms(smsPage.value)
}

async function loadRoutesPage() {
  const [services, pushChannels, routeRules, auditRows, devs] = await Promise.all([
    api.get<AppriseService[]>('/api/admin/apprise-services'),
    api.get<AppriseTarget[]>('/api/admin/apprise-targets'),
    api.get<RoutingRule[]>('/api/admin/routing-rules'),
    api.get<AuditLog[]>('/api/admin/audit'),
    api.get<Device[]>('/api/admin/devices')
  ])
  devices.value = devs
  applyDeviceDefaults(devs)
  appriseServices.value = services
  if (!appriseForm.value.serviceId && services.length > 0) appriseForm.value.serviceId = services[0].id
  channels.value = pushChannels
  rules.value = routeRules
  audit.value = auditRows
}

async function loadEsimPage() {
  const [devs, esimProfiles, tasks, logRows, commandRows] = await Promise.all([
    api.get<Device[]>('/api/admin/devices'),
    api.get<EsimProfile[]>('/api/admin/esim/profiles'),
    api.get<EsimTask[]>('/api/admin/esim/tasks'),
    api.get<LogEntry[]>('/api/admin/logs'),
    api.get<DeviceCommand[]>('/api/admin/commands')
  ])
  devices.value = devs
  applyDeviceDefaults(devs)
  profiles.value = esimProfiles
  esimTasks.value = tasks
  logs.value = logRows
  commands.value = commandRows
}

async function loadSubscriptionsPage() {
  const [devs, esimProfiles, subscriptions, targets] = await Promise.all([
    api.get<Device[]>('/api/admin/devices'),
    api.get<EsimProfile[]>('/api/admin/esim/profiles'),
    api.get<EsimSubscription[]>('/api/admin/esim/subscriptions'),
    api.get<AppriseTarget[]>('/api/admin/apprise-targets')
  ])
  devices.value = devs
  channels.value = targets
  applyDeviceDefaults(devs)
  profiles.value = esimProfiles
  esimSubscriptions.value = subscriptions
  const firstSubscriptionProfile = esimProfiles.find((profile) => profile.deviceId === selectedSubscriptionDeviceId.value)
  if (!selectedSubscriptionProfileId.value && firstSubscriptionProfile) selectedSubscriptionProfileId.value = firstSubscriptionProfile.id
  const firstAvailableProfile = esimProfiles.find((profile) => profile.deviceId === subscriptionDialogDeviceId.value && !subscriptions.some((sub) => sub.profileId === profile.id))
  if (!esimSubscriptionForm.value.profileId && firstAvailableProfile) esimSubscriptionForm.value.profileId = firstAvailableProfile.id
}

async function loadCommandsPage() {
  const [devs, commandRows] = await Promise.all([
    api.get<Device[]>('/api/admin/devices'),
    api.get<DeviceCommand[]>('/api/admin/commands')
  ])
  devices.value = devs
  applyDeviceDefaults(devs)
  commands.value = commandRows
}

async function loadPageData(target: Page) {
  switch (target) {
    case 'overview':
      await loadOverview()
      break
    case 'devices':
      await loadDevices()
      break
    case 'send':
      await loadCommandsPage()
      break
    case 'sms':
      await loadSmsPage()
      break
    case 'routes':
      await loadRoutesPage()
      break
    case 'esim':
      await loadEsimPage()
      break
    case 'esim-subscriptions':
      await loadSubscriptionsPage()
      break
    case 'tools':
      await loadCommandsPage()
      break
    case 'logs':
      logs.value = await api.get<LogEntry[]>('/api/admin/logs')
      break
    case 'audit':
      audit.value = await api.get<AuditLog[]>('/api/admin/audit')
      break
  }
}

watch(selectedEsimDeviceId, () => {
  esimCommandPage.value = 1
})

watch(esimCommandPageSize, () => {
  esimCommandPage.value = 1
})

watch(esimCommandTotalPages, (totalPages) => {
  if (esimCommandPage.value > totalPages) esimCommandPage.value = totalPages
})

async function loadAll(showLoading = true) {
  if (showLoading) loading.value = true
  refreshing.value = true
  error.value = ''
  try {
    await loadPageData(page.value)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    if (showLoading) loading.value = false
    refreshing.value = false
  }
}

async function searchSms(pageNumber = 1) {
  smsPage.value = pageNumber
  sms.value = await api.get<SMSList>(`/api/admin/sms?page=${smsPage.value}&pageSize=${smsPageSize.value}&q=${encodeURIComponent(smsQuery.value)}`)
  selectedSmsId.value = sms.value.items[0]?.id || ''
}

async function changeSmsPageSize(pageSize: number) {
  smsPageSize.value = pageSize
  await searchSms(1)
}

async function runGlobalSearch() {
  smsQuery.value = globalSearch.value.trim()
  if (page.value !== 'sms') {
    page.value = 'sms'
    return
  }
  await searchSms(1)
}

function exportCurrentSms() {
  if (!sms.value) return
  const headers = ['id', 'timestamp', 'deviceName', 'sender', 'tag', 'deliveryStatus', 'body']
  const rows = sms.value.items.map((item) => [item.id, item.timestamp, item.deviceName, item.sender, item.tag, item.deliveryStatus, item.body])
  const csv = [headers, ...rows].map((row) => row.map((cell) => `"${String(cell).replaceAll('"', '""')}"`).join(',')).join('\n')
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `sms-export-page-${smsPage.value}.csv`
  link.click()
  URL.revokeObjectURL(url)
}

async function copySelectedSms() {
  smsActionResult.value = ''
  if (!selectedSms.value) return
  await navigator.clipboard.writeText(selectedSms.value.body)
  smsActionResult.value = '短信内容已复制'
}

function openSelectedSmsDevice() {
  if (!selectedSms.value) return
  selectedEsimDeviceId.value = selectedSms.value.deviceId
  toolDeviceId.value = selectedSms.value.deviceId
  page.value = 'tools'
}

async function createSendTask() {
  sendResult.value = await api.post<CommandResult>('/api/admin/outbound-sms', sendForm.value)
  commands.value = await api.get<DeviceCommand[]>('/api/admin/commands')
}

async function createEsimTask() {
  esimTaskResult.value = ''
  const payload: CreateEsimTaskRequest = {
    deviceId: selectedEsimDeviceId.value,
    activationCode: esimTaskForm.value.activationCode.trim(),
    smdpAddress: esimTaskForm.value.smdpAddress.trim(),
    confirmationCode: esimTaskForm.value.confirmationCode.trim()
  }
  const task = await api.post<EsimTask>('/api/admin/esim/tasks', payload)
  esimTasks.value = await api.get<EsimTask[]>('/api/admin/esim/tasks')
  commands.value = await api.get<DeviceCommand[]>('/api/admin/commands')
  audit.value = await api.get<AuditLog[]>('/api/admin/audit')
  esimTaskResult.value = `已创建 eSIM 下载任务 ${task.id}`
  esimTaskForm.value = { activationCode: '', smdpAddress: '', confirmationCode: '' }
}

async function createDeviceCommand(payload: CreateDeviceCommandRequest, successMessage: string) {
  const command = await api.post<DeviceCommand>('/api/admin/commands', payload)
  commands.value = await api.get<DeviceCommand[]>('/api/admin/commands')
  audit.value = await api.get<AuditLog[]>('/api/admin/audit')
  toolResult.value = `${successMessage}：${command.id}`
  return command
}

async function runDiagnosticCommand(type: string, payload: Record<string, unknown> = {}) {
  toolResult.value = ''
  const command = await createDeviceCommand({ deviceId: toolDeviceId.value, type, payload }, '已创建诊断命令')
  activeToolCommandId.value = command.id
}

function profileOptionLabel(profile: EsimProfile) {
  const name = profile.nickname || profile.profileName || profile.provider || '未命名 Profile'
  const provider = profile.provider && profile.provider !== name ? ` · ${profile.provider}` : ''
  const phone = profile.state === 'enabled' && subscriptionDialogDevice.value?.phoneNumber ? ` · ${subscriptionDialogDevice.value.phoneNumber}` : ''
  const state = profile.state === 'enabled' ? '[当前启用] ' : ''
  return `${state}${name}${provider}${phone} · ICCID ${profile.iccid}`
}

function signalLabel(rssi: number) {
  if (!rssi || rssi > 0) return '未知'
  if (rssi >= -60) return '优秀'
  if (rssi >= -75) return '良好'
  if (rssi >= -90) return '一般'
  return '较弱'
}

function signalStatusClass(rssi: number) {
  if (!rssi || rssi > 0) return 'gray'
  if (rssi >= -75) return 'ok'
  if (rssi >= -90) return 'warn'
  return 'danger'
}

function profileHasActiveCommand(profile: EsimProfile) {
  return commands.value.some((command) =>
    command.deviceId === profile.deviceId &&
    ['pending', 'claimed'].includes(command.status) &&
    command.payload?.iccid === profile.iccid
  )
}

function profileCommandLabel(profile: EsimProfile) {
  if (profile.state === 'enabled') return '当前启用'
  if (profileCommandBusy.value[profile.id] || profileHasActiveCommand(profile)) return '执行中'
  return '切换/启用'
}

async function createProfileCommand(profile: EsimProfile, type: string) {
  esimCommandResult.value = ''
  profileCommandBusy.value = { ...profileCommandBusy.value, [profile.id]: true }
  try {
    const command = await api.post<DeviceCommand>('/api/admin/commands', { deviceId: profile.deviceId, type, payload: { profileId: profile.id, iccid: profile.iccid } })
    commands.value = await api.get<DeviceCommand[]>('/api/admin/commands')
    audit.value = await api.get<AuditLog[]>('/api/admin/audit')
    esimCommandResult.value = `命令 ${command.id} 已创建，终端执行完成后将自动更新 Profile 状态。`
  } catch (err) {
    esimCommandResult.value = err instanceof Error ? `命令创建失败：${err.message}` : '命令创建失败'
  } finally {
    profileCommandBusy.value = { ...profileCommandBusy.value, [profile.id]: false }
  }
}

async function createAppriseTarget() {
  appriseSaveResult.value = ''
  const payload: CreateAppriseTargetRequest = {
    serviceId: appriseForm.value.serviceId,
    name: appriseForm.value.name.trim(),
    configKey: appriseForm.value.configKey.trim(),
    tags: appriseForm.value.tagsText.split(',').map((tag) => tag.trim()).filter(Boolean),
    enabled: appriseForm.value.enabled,
    titleTemplate: appriseForm.value.titleTemplate,
    bodyTemplate: appriseForm.value.bodyTemplate
  }
  if (editingAppriseTargetId.value) {
    const target = await api.put<AppriseTarget>(`/api/admin/apprise-targets/${editingAppriseTargetId.value}`, payload)
    channels.value = await api.get<AppriseTarget[]>('/api/admin/apprise-targets')
    audit.value = await api.get<AuditLog[]>('/api/admin/audit')
    appriseSaveResult.value = `已更新 ${target.name}`
    resetAppriseTargetForm()
    return
  }
  const target = await api.post<AppriseTarget>('/api/admin/apprise-targets', payload)
  channels.value = await api.get<AppriseTarget[]>('/api/admin/apprise-targets')
  audit.value = await api.get<AuditLog[]>('/api/admin/audit')
  appriseSaveResult.value = `已添加 ${target.name}`
  resetAppriseTargetForm()
}

function editAppriseTarget(target: AppriseTarget) {
  editingAppriseTargetId.value = target.id
  appriseForm.value = {
    serviceId: target.serviceId,
    name: target.name,
    configKey: target.configKey,
    tagsText: target.tags.join(','),
    enabled: target.enabled,
    titleTemplate: target.titleTemplate,
    bodyTemplate: target.bodyTemplate
  }
  showAppriseForm.value = true
}

function resetAppriseTargetForm() {
  editingAppriseTargetId.value = ''
  appriseForm.value.name = ''
  appriseForm.value.configKey = 'default'
  appriseForm.value.tagsText = 'all'
  appriseForm.value.enabled = true
  appriseForm.value.titleTemplate = '短信来自 {{sender}}'
  appriseForm.value.bodyTemplate = '{{body}}\n\n终端: {{device}}\n时间: {{timestamp}}'
  if (appriseServices.value.length > 0) appriseForm.value.serviceId = appriseServices.value[0].id
  showAppriseForm.value = false
}

async function deleteAppriseTarget(target: AppriseTarget) {
  await api.delete<{ status: string }>(`/api/admin/apprise-targets/${target.id}`)
  channels.value = await api.get<AppriseTarget[]>('/api/admin/apprise-targets')
  audit.value = await api.get<AuditLog[]>('/api/admin/audit')
  appriseSaveResult.value = `已删除 ${target.name}`
}

async function testAppriseTarget(target: AppriseTarget) {
  appriseSaveResult.value = ''
  await api.post('/api/admin/notify-test', { targetId: target.id, title: 'SMS Hub test', body: 'Apprise target test' })
  channels.value = await api.get<AppriseTarget[]>('/api/admin/apprise-targets')
  appriseSaveResult.value = `已发送测试通知到 ${target.name}`
}

async function saveAppriseService() {
  appriseServiceResult.value = ''
  const payload: CreateAppriseServiceRequest = {
    name: appriseServiceForm.value.name.trim(),
    baseUrl: appriseServiceForm.value.baseUrl.trim(),
    enabled: appriseServiceForm.value.enabled
  }
  if (editingAppriseServiceId.value) {
    const service = await api.put<AppriseService>(`/api/admin/apprise-services/${editingAppriseServiceId.value}`, payload)
    appriseServices.value = await api.get<AppriseService[]>('/api/admin/apprise-services')
    channels.value = await api.get<AppriseTarget[]>('/api/admin/apprise-targets')
    audit.value = await api.get<AuditLog[]>('/api/admin/audit')
    appriseServiceResult.value = `已更新 ${service.name}`
    resetAppriseServiceForm()
    return
  }
  const service = await api.post<AppriseService>('/api/admin/apprise-services', payload)
  appriseServices.value = await api.get<AppriseService[]>('/api/admin/apprise-services')
  audit.value = await api.get<AuditLog[]>('/api/admin/audit')
  appriseForm.value.serviceId = service.id
  appriseServiceResult.value = `已添加 ${service.name}`
  resetAppriseServiceForm()
}

function editAppriseService(service: AppriseService) {
  editingAppriseServiceId.value = service.id
  appriseServiceForm.value = { name: service.name, baseUrl: service.baseUrl, enabled: service.enabled }
  showAppriseServiceForm.value = true
}

function resetAppriseServiceForm() {
  editingAppriseServiceId.value = ''
  appriseServiceForm.value = { name: '备用 Apprise API', baseUrl: 'http://localhost:8000', enabled: true }
  showAppriseServiceForm.value = false
}

async function deleteAppriseService(service: AppriseService) {
  appriseServiceResult.value = ''
  await api.delete<{ status: string }>(`/api/admin/apprise-services/${service.id}`)
  appriseServices.value = await api.get<AppriseService[]>('/api/admin/apprise-services')
  audit.value = await api.get<AuditLog[]>('/api/admin/audit')
  appriseServiceResult.value = `已删除 ${service.name}`
}

async function testAppriseService(serviceId: string) {
  appriseServiceResult.value = ''
  const response = await api.post<{ service: AppriseService; result: { ok: boolean; message: string; statusCode: number } }>('/api/admin/apprise-services/test', { serviceId })
  appriseServices.value = appriseServices.value.map((item) => item.id === response.service.id ? response.service : item)
  appriseServiceResult.value = response.result.ok ? `${response.service.name} 连接成功` : `${response.service.name} 连接失败：${response.result.message}`
}

async function saveRoutingRule() {
  routingRuleResult.value = ''
  const payload: CreateRoutingRuleRequest = {
    name: routingRuleForm.value.name.trim(),
    senderContains: routingRuleForm.value.senderContains.trim(),
    bodyKeywords: routingRuleForm.value.bodyKeywordsText.split(',').map((value) => value.trim()).filter(Boolean),
    deviceIds: routingRuleForm.value.deviceIds,
    tags: routingRuleForm.value.tagsText.split(',').map((value) => value.trim()).filter(Boolean),
    targetIds: routingRuleForm.value.targetIds,
    enabled: routingRuleForm.value.enabled
  }
  if (editingRoutingRuleId.value) {
    const rule = await api.put<RoutingRule>(`/api/admin/routing-rules/${editingRoutingRuleId.value}`, payload)
    rules.value = await api.get<RoutingRule[]>('/api/admin/routing-rules')
    audit.value = await api.get<AuditLog[]>('/api/admin/audit')
    routingRuleResult.value = `已更新 ${rule.name}`
    resetRoutingRuleForm()
    return
  }
  const rule = await api.post<RoutingRule>('/api/admin/routing-rules', payload)
  rules.value = await api.get<RoutingRule[]>('/api/admin/routing-rules')
  audit.value = await api.get<AuditLog[]>('/api/admin/audit')
  routingRuleResult.value = `已添加 ${rule.name}`
  resetRoutingRuleForm()
}

function editRoutingRule(rule: RoutingRule) {
  editingRoutingRuleId.value = rule.id
  routingRuleForm.value = {
    name: rule.name,
    senderContains: rule.senderContains || '',
    bodyKeywordsText: (rule.bodyKeywords || []).join(','),
    deviceIds: rule.deviceIds || [],
    tagsText: (rule.tags || []).join(','),
    targetIds: rule.targetIds || [],
    enabled: rule.enabled
  }
  showRoutingRuleForm.value = true
}

function resetRoutingRuleForm() {
  editingRoutingRuleId.value = ''
  routingRuleForm.value = { name: '', senderContains: '', bodyKeywordsText: '', deviceIds: [], tagsText: '', targetIds: [], enabled: true }
  showRoutingRuleForm.value = false
}

function routingRuleConditions(rule: RoutingRule) {
  const conditions: string[] = []
  if (rule.senderContains) conditions.push(`发送者包含 ${rule.senderContains}`)
  if (rule.bodyKeywords?.length) conditions.push(`正文包含任一：${rule.bodyKeywords.join('、')}`)
  if (rule.deviceIds?.length) conditions.push(`终端：${rule.deviceIds.map((id) => devices.value.find((device) => device.id === id)?.name || id).join('、')}`)
  if (rule.tags?.length) conditions.push(`标签：${rule.tags.join('、')}`)
  return conditions.join('；') || '全部短信'
}

function routingRuleTargets(rule: RoutingRule) {
  return (rule.targetIds || []).map((id) => channels.value.find((target) => target.id === id)?.name || id).join('、') || '-'
}

async function deleteRoutingRule(rule: RoutingRule) {
  await api.delete<{ status: string }>(`/api/admin/routing-rules/${rule.id}`)
  rules.value = await api.get<RoutingRule[]>('/api/admin/routing-rules')
  audit.value = await api.get<AuditLog[]>('/api/admin/audit')
  routingRuleResult.value = `已删除 ${rule.name}`
}

async function saveEsimSubscription() {
  esimSubscriptionResult.value = ''
  const payload: CreateEsimSubscriptionRequest = {
    profileId: esimSubscriptionForm.value.profileId,
    enabled: esimSubscriptionForm.value.enabled,
    type: esimSubscriptionForm.value.type,
    intervalDays: Number(esimSubscriptionForm.value.intervalDays),
    startAt: new Date(esimSubscriptionForm.value.startAt).toISOString(),
    rechargeAmount: esimSubscriptionForm.value.type === 'recharge' ? esimSubscriptionForm.value.rechargeAmount.trim() : '',
    keepaliveNumber: esimSubscriptionForm.value.type === 'sms_keepalive' ? esimSubscriptionForm.value.keepaliveNumber.trim() : '',
    keepaliveMessage: esimSubscriptionForm.value.type === 'sms_keepalive' ? esimSubscriptionForm.value.keepaliveMessage.trim() : '',
    targetIds: esimSubscriptionForm.value.targetIds,
    note: esimSubscriptionForm.value.note.trim()
  }
  if (editingEsimSubscriptionId.value) {
    const updated = await api.put<EsimSubscription>(`/api/admin/esim/subscriptions/${editingEsimSubscriptionId.value}`, payload)
    esimSubscriptions.value = esimSubscriptions.value.map((item) => item.id === updated.id ? updated : item)
    audit.value = await api.get<AuditLog[]>('/api/admin/audit')
    showEsimSubscriptionDialog.value = false
    editingEsimSubscriptionId.value = ''
    esimSubscriptionResult.value = `已更新 ${updated.profileName} 的订阅策略`
    return
  }

  const sub = await api.post<EsimSubscription>('/api/admin/esim/subscriptions', payload)
  esimSubscriptions.value = await api.get<EsimSubscription[]>('/api/admin/esim/subscriptions')
  const nextAvailableProfile = availableSubscriptionDialogProfiles.value.find((profile) => !esimSubscriptions.value.some((item) => item.profileId === profile.id))
  esimSubscriptionForm.value.profileId = nextAvailableProfile?.id || ''
  showEsimSubscriptionDialog.value = false
  audit.value = await api.get<AuditLog[]>('/api/admin/audit')
  esimSubscriptionResult.value = `已为 ${sub.profileName} 添加订阅策略`
}

function openDeviceTools(deviceId: string) {
  toolDeviceId.value = deviceId
  page.value = 'tools'
}

function openEsimSubscriptionDialog() {
  editingEsimSubscriptionId.value = ''
  subscriptionDialogDeviceId.value = selectedSubscriptionDeviceId.value || devices.value[0]?.id || ''
  if (selectedSubscriptionProfile.value && !selectedSubscriptionConfig.value) {
    esimSubscriptionForm.value.profileId = selectedSubscriptionProfile.value.id
  } else {
    const firstAvailableProfile = availableSubscriptionDialogProfiles.value[0]
    esimSubscriptionForm.value.profileId = firstAvailableProfile?.id || ''
  }
  esimSubscriptionForm.value.enabled = true
  esimSubscriptionForm.value.type = 'recharge'
  esimSubscriptionForm.value.intervalDays = 30
  esimSubscriptionForm.value.startAt = new Date().toISOString().slice(0, 16)
  esimSubscriptionForm.value.rechargeAmount = '20 CNY'
  esimSubscriptionForm.value.keepaliveNumber = '10086'
  esimSubscriptionForm.value.keepaliveMessage = 'CXLL'
  esimSubscriptionForm.value.targetIds = channels.value.filter((target) => target.enabled).map((target) => target.id)
  esimSubscriptionForm.value.note = ''
  showEsimSubscriptionDialog.value = true
}

function editEsimSubscription(sub: EsimSubscription) {
  editingEsimSubscriptionId.value = sub.id
  subscriptionDialogDeviceId.value = sub.deviceId
  esimSubscriptionForm.value = {
    profileId: sub.profileId,
    enabled: sub.enabled,
    type: sub.type,
    intervalDays: sub.intervalDays,
    startAt: new Date(sub.startAt || sub.nextRunAt).toISOString().slice(0, 16),
    rechargeAmount: sub.rechargeAmount || '20 CNY',
    keepaliveNumber: sub.keepaliveNumber || '10086',
    keepaliveMessage: sub.keepaliveMessage || 'CXLL',
    targetIds: sub.targetIds?.length ? [...sub.targetIds] : channels.value.filter((target) => target.enabled).map((target) => target.id),
    note: sub.note || ''
  }
  showEsimSubscriptionDialog.value = true
}

function selectSubscriptionDialogDevice(deviceId: string) {
  subscriptionDialogDeviceId.value = deviceId
  const firstAvailableProfile = profiles.value.find((profile) => profile.deviceId === deviceId && !esimSubscriptions.value.some((sub) => sub.profileId === profile.id))
  esimSubscriptionForm.value.profileId = firstAvailableProfile?.id || ''
}

function selectSubscriptionDevice(deviceId: string) {
  selectedSubscriptionDeviceId.value = deviceId
  const firstProfile = profiles.value.find((profile) => profile.deviceId === deviceId)
  selectedSubscriptionProfileId.value = firstProfile?.id || ''
}

function parseActivationCode(value: string) {
  const code = value.trim()
  esimTaskForm.value.activationCode = code
  const parts = code.split('$')
  if (parts.length >= 2 && code.toUpperCase().startsWith('LPA:')) {
    esimTaskForm.value.smdpAddress = parts[1]
    esimTaskForm.value.confirmationCode = parts[3] || ''
  }
}

async function readEsimQr(event: Event) {
  esimQrResult.value = ''
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  if (!window.BarcodeDetector) {
    esimQrResult.value = '当前浏览器不支持二维码识别，请手动输入激活码。'
    input.value = ''
    return
  }
  try {
    const bitmap = await createImageBitmap(file)
    const detector = new window.BarcodeDetector({ formats: ['qr_code'] })
    const codes = await detector.detect(bitmap)
    bitmap.close()
    const value = codes.find((code) => code.rawValue)?.rawValue || ''
    if (!value) {
      esimQrResult.value = '未识别到二维码内容。'
      return
    }
    parseActivationCode(value)
    esimQrResult.value = value.toUpperCase().startsWith('LPA:') ? '已识别 eSIM 激活码' : '已识别二维码内容，请确认是否为 eSIM 激活码'
  } catch (err) {
    esimQrResult.value = err instanceof Error ? `二维码识别失败：${err.message}` : '二维码识别失败'
  } finally {
    input.value = ''
  }
}

const livePages = new Set<Page>(['overview', 'devices', 'send', 'esim', 'tools', 'logs', 'audit'])
let refreshTimer: number | undefined

watch(page, () => {
  void loadAll()
})

onMounted(() => {
  void loadAll()
  refreshTimer = window.setInterval(() => {
    if (!loading.value && !refreshing.value && livePages.has(page.value)) {
      void loadAll(false)
    }
  }, 2000)
})

onBeforeUnmount(() => {
  if (refreshTimer !== undefined) window.clearInterval(refreshTimer)
})
</script>

<template>
  <AppShell v-model:page="page" v-model:global-search="globalSearch" v-model:tool-device-id="toolDeviceId" :devices="devices" @refresh="loadAll" @global-search="runGlobalSearch">
        <div v-if="error" class="alert danger">{{ error }}</div>
        <div v-if="loading" class="card empty">加载中...</div>

        <OverviewPage v-if="!loading && page === 'overview' && dashboard" :dashboard="dashboard" @open-routes="page = 'routes'" @open-sms="page = 'sms'" />

        <DevicesPage v-if="!loading && page === 'devices'" :devices="devices" @open-tools="(deviceId?: string) => { if (deviceId) toolDeviceId = deviceId; page = 'tools' }" @open-esim="(deviceId: string) => { selectedEsimDeviceId = deviceId; page = 'esim' }" />

        <section v-if="!loading && page === 'sms' && sms" class="page">
          <div class="page-head"><div><h1>历史短信</h1><p>查看全部历史短信，支持搜索、分页、详情和分发记录。</p></div><button class="btn" @click="exportCurrentSms">导出当前页</button></div>
          <div class="grid cols-4">
            <div class="card metric"><span>历史总量</span><b>{{ sms.totalAll.toLocaleString() }}</b><small>保留策略：永久保存</small></div>
            <div class="card metric"><span>当前筛选</span><b>{{ sms.total.toLocaleString() }}</b><small>{{ smsQuery || '全部短信' }} / 每页 {{ smsPageSize }} 条</small></div>
            <div class="card metric"><span>最早短信</span><b class="small-value">{{ new Date(sms.earliestAt).toLocaleDateString('zh-CN') }}</b><small>可按日期回溯</small></div>
            <div class="card metric"><span>失败/重试</span><b>{{ sms.items.filter((item) => ['failed', 'retrying'].includes(item.deliveryStatus)).length }}</b><small>当前页分发异常</small></div>
          </div>
          <div class="grid layout-2 top-gap">
            <div class="card"><div class="card-head toolbar"><input v-model="smsQuery" class="field" placeholder="搜索号码、内容、短信 ID" @keydown.enter="searchSms(1)"><button class="btn" @click="searchSms(1)">搜索</button></div><table><thead><tr><th>接收时间</th><th>终端</th><th>发送者</th><th>内容摘要</th><th>标签</th><th>分发</th></tr></thead><tbody><tr v-for="item in sms.items" :key="item.id" :class="{ selected: selectedSmsId === item.id }" @click="selectedSmsId = item.id"><td>{{ formatTime(item.timestamp) }}</td><td>{{ item.deviceName }}</td><td class="mono">{{ item.sender }}</td><td class="truncate">{{ item.body }}</td><td><span class="status info">{{ item.tag }}</span></td><td><span :class="['status', statusClass(item.deliveryStatus)]">{{ item.deliverySummary }}</span></td></tr></tbody></table><PaginationBar :page="smsPage" :page-size="smsPageSize" :total="sms.total" @change="searchSms" @page-size-change="changeSmsPageSize" /></div>
            <div class="card detail"><div class="card-head"><b>短信详情</b><button class="btn small" @click="copySelectedSms">复制内容</button></div><template v-if="selectedSms"><dl><dt>短信 ID</dt><dd class="mono">{{ selectedSms.id }}</dd><dt>接收时间</dt><dd>{{ formatTime(selectedSms.timestamp) }}</dd><dt>终端</dt><dd>{{ selectedSms.deviceName }}</dd><dt>发送者</dt><dd class="mono">{{ selectedSms.sender }}</dd><dt>长短信</dt><dd>{{ selectedSms.concatInfo }}</dd></dl><label>完整内容</label><textarea readonly :value="selectedSms.body" /><label>分发记录</label><div class="timeline"><div class="event"><span>状态</span><div><b>{{ selectedSms.deliveryStatus }}</b><small>{{ selectedSms.deliverySummary }}</small></div></div></div><div v-if="smsActionResult" class="alert success">{{ smsActionResult }}</div><div class="toolbar"><button class="btn" @click="openSelectedSmsDevice">打开终端</button></div></template></div>
          </div>
        </section>

        <SendSmsPage v-if="!loading && page === 'send'" v-model:send-form="sendForm" :devices="devices" :send-result="sendResult" @create-send-task="createSendTask" />

        <section v-if="!loading && page === 'routes'" class="page routes-page">
          <div class="page-head">
            <div><h1>消息分发</h1><p>管理 Apprise 服务、通知 Target 和短信路由规则。</p></div>
            <div class="toolbar"><button class="btn" @click="showAppriseForm = true">新增 Target</button><button class="btn primary" @click="showRoutingRuleForm = true">新增规则</button></div>
          </div>

          <div class="grid cols-3 routes-metrics">
            <div class="card metric"><span>Apprise 服务</span><b>{{ appriseServices.filter((item) => item.enabled).length }} / {{ appriseServices.length }}</b><small>已启用 / 全部</small></div>
            <div class="card metric"><span>通知 Target</span><b>{{ channels.filter((item) => item.enabled).length }} / {{ channels.length }}</b><small>已启用 / 全部</small></div>
            <div class="card metric"><span>路由规则</span><b>{{ rules.filter((item) => item.enabled).length }} / {{ rules.length }}</b><small>已启用 / 全部</small></div>
          </div>

          <div v-if="appriseServiceResult" class="alert success top-gap">{{ appriseServiceResult }}</div>
          <div v-if="appriseSaveResult" class="alert success top-gap">{{ appriseSaveResult }}</div>
          <div v-if="routingRuleResult" class="alert success top-gap">{{ routingRuleResult }}</div>

          <div class="grid routes-config-grid top-gap">
            <section class="card routes-panel">
              <div class="card-head"><div><b>Apprise 服务</b><small>通知网关连接</small></div><button class="btn small" type="button" @click="showAppriseServiceForm = true">新增服务</button></div>
              <div v-if="appriseServices.length" class="routes-item-list">
                <div v-for="service in appriseServices" :key="service.id" class="routes-item">
                  <div class="routes-item-main"><div class="routes-item-title"><b>{{ service.name }}</b><span :class="['status', service.lastStatus === 'success' ? 'ok' : service.lastStatus === 'failed' ? 'danger' : 'gray']">{{ service.enabled ? service.lastStatus : 'disabled' }}</span></div><small class="mono">{{ service.baseUrl }}</small><small>{{ service.lastMessage || '尚未测试连接' }}</small></div>
                  <div class="routes-item-actions"><button class="btn small" type="button" @click="testAppriseService(service.id)">测试</button><button class="btn small" type="button" @click="editAppriseService(service)">编辑</button><button class="btn small danger" type="button" @click="deleteAppriseService(service)">删除</button></div>
                </div>
              </div>
              <div v-else class="empty"><b>暂无 Apprise 服务</b><small>添加服务后才能创建并测试通知 Target。</small></div>
            </section>

            <section class="card routes-panel">
              <div class="card-head"><div><b>通知 Target</b><small>具体接收渠道与模板</small></div><button class="btn small" type="button" :disabled="appriseServices.length === 0" @click="showAppriseForm = true">新增 Target</button></div>
              <div v-if="channels.length" class="routes-item-list">
                <div v-for="ch in channels" :key="ch.id" class="routes-item">
                  <div class="routes-item-main"><div class="routes-item-title"><b>{{ ch.name }}</b><span :class="['status', ch.lastStatus === 'success' ? 'ok' : ch.enabled ? 'warn' : 'gray']">{{ ch.enabled ? ch.lastStatus : 'disabled' }}</span></div><small>{{ ch.serviceName }} · key={{ ch.configKey }}</small><div class="tag-list"><span v-for="tag in ch.tags.length ? ch.tags : ['all']" :key="tag">{{ tag }}</span></div><small>{{ ch.description }}</small></div>
                  <div class="routes-item-actions"><button class="btn small" type="button" @click="testAppriseTarget(ch)">测试</button><button class="btn small" type="button" @click="editAppriseTarget(ch)">编辑</button><button class="btn small danger" type="button" @click="deleteAppriseTarget(ch)">删除</button></div>
                </div>
              </div>
              <div v-else class="empty"><b>暂无通知 Target</b><small>Target 用于关联 Apprise Config Key、标签和消息模板。</small></div>
            </section>
          </div>

          <section class="card routes-rules-panel top-gap">
            <div class="card-head"><div><b>短信路由规则</b><small>规则内条件同时满足；多条规则可分别触发</small></div><button class="btn small primary" type="button" :disabled="channels.length === 0" @click="showRoutingRuleForm = true">新增规则</button></div>
            <div class="routes-table-wrap"><table><thead><tr><th>规则</th><th>匹配条件</th><th>发送到</th><th>状态</th><th>操作</th></tr></thead><tbody><tr v-for="rule in rules" :key="rule.id"><td><b>{{ rule.name }}</b></td><td class="routes-condition">{{ routingRuleConditions(rule) }}</td><td>{{ routingRuleTargets(rule) }}</td><td><span :class="['status', rule.enabled ? 'ok' : 'gray']">{{ rule.enabled ? '启用' : '停用' }}</span></td><td><div class="toolbar"><button class="btn small" @click="editRoutingRule(rule)">编辑</button><button class="btn small danger" @click="deleteRoutingRule(rule)">删除</button></div></td></tr><tr v-if="rules.length === 0"><td colspan="5"><div class="empty"><b>暂无路由规则</b><small>没有规则时，短信会发送到全部已启用 Target。</small></div></td></tr></tbody></table></div>
          </section>

          <div v-if="showAppriseServiceForm" class="modal-backdrop" @click.self="resetAppriseServiceForm">
            <form class="card form modal" @submit.prevent="saveAppriseService"><div class="card-head"><div><b>{{ editingAppriseServiceId ? '编辑 Apprise 服务' : '新增 Apprise 服务' }}</b><small>配置自部署 Apprise API 地址</small></div><button class="btn small" type="button" @click="resetAppriseServiceForm">关闭</button></div><label>服务名称</label><input v-model="appriseServiceForm.name" class="field" placeholder="例如：主 Apprise API" required><label>Apprise API 地址</label><input v-model="appriseServiceForm.baseUrl" class="field" type="url" placeholder="http://localhost:8000" required><label class="checkbox-row"><input v-model="appriseServiceForm.enabled" type="checkbox"> 启用通知分发</label><div class="toolbar"><button class="btn primary">{{ editingAppriseServiceId ? '保存修改' : '添加服务' }}</button><button class="btn" type="button" @click="resetAppriseServiceForm">取消</button></div></form>
          </div>

          <div v-if="showAppriseForm" class="modal-backdrop" @click.self="resetAppriseTargetForm">
            <form class="card form modal routes-target-modal" @submit.prevent="createAppriseTarget"><div class="card-head"><div><b>{{ editingAppriseTargetId ? '编辑 Apprise Target' : '新增 Apprise Target' }}</b><small>配置接收渠道与短信模板</small></div><button class="btn small" type="button" @click="resetAppriseTargetForm">关闭</button></div><div class="form-grid-2"><div class="form-section"><label>Apprise 服务</label><select v-model="appriseForm.serviceId" class="field" required><option v-for="service in appriseServices" :key="service.id" :value="service.id">{{ service.name }} / {{ service.baseUrl }}</option></select></div><div class="form-section"><label>名称</label><input v-model="appriseForm.name" class="field" placeholder="例如：Telegram 运维群" required></div><div class="form-section"><label>Config Key</label><input v-model="appriseForm.configKey" class="field" placeholder="default" required></div><div class="form-section"><label>Tags（逗号分隔）</label><input v-model="appriseForm.tagsText" class="field" placeholder="all,verification"></div></div><label>标题模板</label><input v-model="appriseForm.titleTemplate" class="field" placeholder="短信来自 {{sender}}"><label>内容模板</label><textarea v-model="appriseForm.bodyTemplate" placeholder="{{body}}"></textarea><small v-pre>可用变量：{{sender}}、{{body}}、{{device}}、{{timestamp}}</small><label class="checkbox-row"><input v-model="appriseForm.enabled" type="checkbox"> 启用 Target</label><div class="toolbar"><button class="btn primary">{{ editingAppriseTargetId ? '保存修改' : '保存 Target' }}</button><button class="btn" type="button" @click="resetAppriseTargetForm">取消</button></div></form>
          </div>

          <div v-if="showRoutingRuleForm" class="modal-backdrop" @click.self="resetRoutingRuleForm">
            <form class="card form modal routes-rule-modal" @submit.prevent="saveRoutingRule"><div class="card-head"><div><b>{{ editingRoutingRuleId ? '编辑路由规则' : '新增路由规则' }}</b><small>同一规则中的非空条件必须同时满足</small></div><button class="btn small" type="button" @click="resetRoutingRuleForm">关闭</button></div><label>规则名称</label><input v-model="routingRuleForm.name" class="field" placeholder="例如：验证码优先" required><div class="form-grid-2"><div class="form-section"><label>发送者包含</label><input v-model="routingRuleForm.senderContains" class="field" placeholder="例如：95588；留空表示不限"></div><div class="form-section"><label>正文关键词（匹配任意一个）</label><input v-model="routingRuleForm.bodyKeywordsText" class="field" placeholder="验证码, code"></div><div class="form-section"><label>终端（不选择表示不限）</label><select v-model="routingRuleForm.deviceIds" class="field routes-multi-select" multiple><option v-for="device in devices" :key="device.id" :value="device.id">{{ device.name }} / {{ device.phoneNumber || '号码未知' }}</option></select></div><div class="form-section"><label>发送到 Target（可多选）</label><select v-model="routingRuleForm.targetIds" class="field routes-multi-select" multiple required><option v-for="target in channels" :key="target.id" :value="target.id" :disabled="!target.enabled">{{ target.name }}{{ target.enabled ? '' : '（已停用）' }}</option></select></div></div><label>短信标签（逗号分隔）</label><input v-model="routingRuleForm.tagsText" class="field" placeholder="verification, finance"><label class="checkbox-row"><input v-model="routingRuleForm.enabled" type="checkbox"> 启用规则</label><div class="toolbar"><button class="btn primary">{{ editingRoutingRuleId ? '保存修改' : '保存规则' }}</button><button class="btn" type="button" @click="resetRoutingRuleForm">取消</button></div></form>
          </div>
        </section>

        <section v-if="!loading && page === 'esim'" class="page esim-page">
          <div class="page-head">
            <div><h1>eSIM</h1><p>管理终端 eUICC Profile 与下载任务。</p></div>
            <button class="btn" @click="loadAll()">刷新</button>
          </div>

          <div class="card esim-device-bar">
            <div class="esim-device-select">
              <label>终端</label>
              <select v-model="selectedEsimDeviceId" class="field"><option v-for="device in devices" :key="device.id" :value="device.id">{{ device.name }} / {{ device.status }}</option></select>
            </div>
            <template v-if="selectedEsimDevice">
              <div class="esim-device-stat"><span>状态</span><b><span :class="['status', statusClass(selectedEsimDevice.status)]">{{ selectedEsimDevice.status === 'online' ? '在线' : '离线' }}</span></b></div>
              <div class="esim-device-stat"><span>当前号码</span><b class="mono">{{ selectedEsimDevice.phoneNumber || '-' }}</b></div>
              <div class="esim-device-stat"><span>信号</span><b><span :class="['status', signalStatusClass(selectedEsimDevice.rssi)]">{{ signalLabel(selectedEsimDevice.rssi) }}<template v-if="selectedEsimDevice.rssi && selectedEsimDevice.rssi < 0"> · {{ selectedEsimDevice.rssi }} dBm</template></span></b></div>
              <div class="esim-device-stat"><span>当前 ICCID</span><b class="mono">{{ selectedEsimDevice.iccid || '-' }}</b></div>
              <div class="esim-device-stat esim-device-secondary"><span>运营商</span><b>{{ selectedEsimDevice.operator || '-' }}</b></div>
              <div class="esim-device-stat esim-device-secondary"><span>EID</span><b class="mono">{{ selectedEsimDevice.eid || '-' }}</b></div>
            </template>
          </div>

          <div v-if="esimCommandResult" class="alert success top-gap">{{ esimCommandResult }}</div>

          <div class="grid esim-workspace top-gap">
            <div class="card esim-profiles-panel">
              <div class="card-head">
                <div><b>Profile</b><small>{{ selectedDeviceProfiles.length }} 个</small></div>
                <span v-if="selectedEsimDevice" :class="['status', statusClass(selectedEsimDevice.status)]">{{ selectedEsimDevice.status === 'online' ? '可操作' : '终端离线' }}</span>
              </div>
              <div v-if="selectedDeviceProfiles.length > 0" class="profile-list">
                <div v-for="profile in selectedDeviceProfiles" :key="profile.id" :class="['profile-row', { active: profile.state === 'enabled' }]">
                  <div class="profile-state-marker"></div>
                  <div class="profile-main">
                    <div class="profile-title"><b>{{ profile.nickname || profile.profileName || profile.provider || '未命名 Profile' }}</b><span :class="['status', statusClass(profile.state)]">{{ profile.state === 'enabled' ? '当前启用' : '已停用' }}</span></div>
                    <div class="profile-meta"><span>{{ profile.provider || '-' }}</span><span>{{ profile.profileName || '-' }}</span></div>
                    <div class="profile-identifiers"><span class="mono">ICCID {{ profile.iccid }}</span><span class="mono">AID {{ profile.aid }}</span></div>
                  </div>
                  <div class="profile-actions">
                    <button class="btn small primary" :disabled="profile.state === 'enabled' || profileCommandBusy[profile.id] || profileHasActiveCommand(profile) || selectedEsimDevice?.status !== 'online'" @click="createProfileCommand(profile, 'esim_enable_profile')">{{ profileCommandLabel(profile) }}</button>
                    <button class="btn small" :disabled="profile.state === 'enabled' || profileCommandBusy[profile.id] || profileHasActiveCommand(profile) || selectedEsimDevice?.status !== 'online'" @click="createProfileCommand(profile, 'esim_delete_profile')">删除</button>
                  </div>
                </div>
              </div>
              <div v-else class="empty"><template v-if="selectedDeviceEsimSyncLog"><b>未能读取 eSIM Profile</b><small>{{ selectedDeviceEsimSyncLog.message }}</small><button class="btn small" @click="openDeviceTools(selectedEsimDeviceId)">打开诊断工具</button></template><template v-else>当前终端暂无 eSIM Profile。</template></div>
            </div>

            <form class="card esim-task-form" @submit.prevent="createEsimTask">
              <div class="card-head"><div><b>添加 eSIM</b><small>{{ selectedEsimDevice?.name || '未选择终端' }}</small></div><button class="btn small" type="button" @click="esimTaskForm.activationCode = ''; esimTaskForm.smdpAddress = ''; esimTaskForm.confirmationCode = ''; esimQrResult = ''">清空</button></div>
              <div class="esim-form-body">
                <input ref="esimQrInput" class="visually-hidden" type="file" accept="image/*" @change="readEsimQr">
                <button class="qr-upload" type="button" @click="esimQrInput?.click()"><b>上传 eSIM 二维码</b><span>{{ esimQrResult || 'PNG、JPG 或相机照片' }}</span></button>
                <div class="form-section">
                  <label>激活码</label>
                  <textarea v-model="esimTaskForm.activationCode" class="activation-code" placeholder="LPA:1$smdp.example.com$MATCHING-ID" @input="parseActivationCode(esimTaskForm.activationCode)" required></textarea>
                </div>
                <div class="form-grid-2">
                  <div class="form-section"><label>SM-DP+ 地址</label><input v-model="esimTaskForm.smdpAddress" class="field" placeholder="smdp.example.com"></div>
                  <div class="form-section"><label>确认码</label><input v-model="esimTaskForm.confirmationCode" class="field" placeholder="可选"></div>
                </div>
                <button class="btn primary esim-submit" :disabled="!selectedEsimDevice || selectedEsimDevice.status !== 'online' || !esimTaskForm.activationCode">创建下载任务</button>
                <div v-if="esimTaskResult" class="alert success">{{ esimTaskResult }}</div>
              </div>
            </form>
          </div>

          <div class="card top-gap esim-command-panel">
            <div class="card-head toolbar"><b>终端命令</b><span class="status gray">{{ selectedEsimCommands.length }} 条</span><button class="btn small" @click="loadAll()">刷新</button></div>
            <table><thead><tr><th>时间</th><th>命令</th><th>状态</th><th>结果</th></tr></thead><tbody><tr v-for="command in pagedEsimCommands" :key="command.id"><td>{{ formatTime(command.createdAt) }}</td><td>{{ command.type }}<small class="mono">{{ command.id }}</small></td><td><span :class="['status', statusClass(command.status)]">{{ command.status }}</span></td><td>{{ command.result || '-' }}</td></tr><tr v-if="selectedEsimCommands.length === 0"><td colspan="4" class="muted">当前终端暂无 eSIM 命令。</td></tr></tbody></table>
            <PaginationBar v-if="selectedEsimCommands.length" :page="esimCommandPage" :page-size="esimCommandPageSize" :total="selectedEsimCommands.length" @change="esimCommandPage = $event" @page-size-change="esimCommandPageSize = $event" />
          </div>
          <div v-for="task in selectedDeviceTasks" :key="task.id" class="card top-gap esim-task-progress"><div><b>{{ task.type }}</b><p>{{ task.stage }}</p></div><span>{{ task.progress }}%</span><div class="progress"><span :style="{ width: task.progress + '%' }"></span></div></div>
        </section>

        <section v-if="!loading && page === 'esim-subscriptions'" class="page">
          <div class="page-head"><div><h1>订阅保活</h1><p>查看全部 eSIM 充值与短信保活策略。</p></div><button class="btn primary" @click="openEsimSubscriptionDialog">新增订阅策略</button></div>
          <div v-if="esimSubscriptionResult" class="alert success top-gap">{{ esimSubscriptionResult }}</div>
          <div class="card top-gap"><div class="card-head"><b>全部订阅保活</b><button class="btn small" @click="loadAll()">刷新</button></div><table><thead><tr><th>终端</th><th>号码 / Profile</th><th>国家/地区</th><th>策略类型</th><th>开始时间</th><th>执行周期</th><th>策略参数</th><th>提醒渠道</th><th>下次执行</th><th>状态</th><th>操作</th></tr></thead><tbody><tr v-for="sub in esimSubscriptions" :key="sub.id"><td><b>{{ sub.deviceName }}</b><small class="mono">{{ sub.deviceId }}</small></td><td><b>{{ sub.profileName }}</b><small class="mono">{{ sub.iccid }}</small><small>{{ sub.note || '-' }}</small></td><td>{{ sub.country || '-' }}</td><td>{{ sub.type === 'recharge' ? '充值提醒' : '短信保活' }}</td><td>{{ formatTime(sub.startAt) }}</td><td>{{ sub.intervalDays }} 天</td><td><template v-if="sub.type === 'recharge'">{{ sub.rechargeAmount || '-' }}</template><template v-else>{{ sub.keepaliveNumber || '-' }} / {{ sub.keepaliveMessage || '-' }}</template></td><td>{{ (sub.targetIds || []).map((id) => channels.find((target) => target.id === id)?.name || id).join('、') || '全部启用渠道（兼容）' }}</td><td>{{ formatTime(sub.nextRunAt) }}</td><td><span :class="['status', statusClass(sub.enabled ? sub.status : 'disabled')]">{{ sub.enabled ? sub.status : 'disabled' }}</span></td><td><button class="btn small primary" @click="editEsimSubscription(sub)">编辑</button></td></tr><tr v-if="esimSubscriptions.length === 0"><td colspan="11" class="muted">暂无订阅保活策略。</td></tr></tbody></table></div>
          <div v-if="showEsimSubscriptionDialog" class="modal-backdrop" @click.self="showEsimSubscriptionDialog = false"><form class="card form modal" @submit.prevent="saveEsimSubscription"><div class="card-head"><b>{{ editingEsimSubscriptionId ? '编辑订阅策略' : '新增订阅策略' }}</b><button class="btn small" type="button" @click="showEsimSubscriptionDialog = false">关闭</button></div><label>终端</label><select class="field" :value="subscriptionDialogDeviceId" :disabled="!!editingEsimSubscriptionId" @change="selectSubscriptionDialogDevice(($event.target as HTMLSelectElement).value)" required><option v-for="device in devices" :key="device.id" :value="device.id">{{ device.name }} / {{ device.eid }} / {{ device.status }}</option></select><label>号码 / Profile</label><select v-model="esimSubscriptionForm.profileId" class="field" :disabled="!!editingEsimSubscriptionId" required><option v-for="profile in editingEsimSubscriptionId ? subscriptionDialogProfiles : availableSubscriptionDialogProfiles" :key="profile.id" :value="profile.id">{{ profileOptionLabel(profile) }}</option></select><div v-if="subscriptionDialogSelectedProfile" class="subscription-profile-summary"><div><span>Profile</span><b>{{ subscriptionDialogSelectedProfile.nickname || subscriptionDialogSelectedProfile.profileName || subscriptionDialogSelectedProfile.provider || '-' }}</b></div><div><span>运营商</span><b>{{ subscriptionDialogSelectedProfile.provider || '-' }}</b></div><div><span>ICCID</span><b class="mono">{{ subscriptionDialogSelectedProfile.iccid }}</b></div><div><span>当前号码</span><b class="mono">{{ subscriptionDialogSelectedProfile.state === 'enabled' ? subscriptionDialogDevice?.phoneNumber || '-' : '非当前启用 Profile' }}</b></div></div><label>策略类型</label><select v-model="esimSubscriptionForm.type" class="field"><option value="recharge">充值提醒</option><option value="sms_keepalive">短信保活</option></select><label>开始时间</label><input v-model="esimSubscriptionForm.startAt" class="field" type="datetime-local" required><label>执行间隔（天）</label><input v-model.number="esimSubscriptionForm.intervalDays" class="field" type="number" min="1"><template v-if="esimSubscriptionForm.type === 'recharge'"><label>充值金额/套餐</label><input v-model="esimSubscriptionForm.rechargeAmount" class="field" placeholder="20 CNY"></template><template v-else><label>保活短信号码</label><input v-model="esimSubscriptionForm.keepaliveNumber" class="field" placeholder="10086" required><label>保活短信内容</label><input v-model="esimSubscriptionForm.keepaliveMessage" class="field" placeholder="CXLL" required></template><label>消息提醒 Target</label><select v-model="esimSubscriptionForm.targetIds" class="field" multiple required><option v-for="target in channels.filter((item) => item.enabled)" :key="target.id" :value="target.id">{{ target.name }} / {{ target.serviceName }} / {{ target.configKey }}</option></select><small v-if="channels.filter((item) => item.enabled).length === 0" class="muted">请先在消息分发中配置并启用 Apprise Target。</small><label>备注</label><textarea v-model="esimSubscriptionForm.note" placeholder="用途、套餐说明、注意事项"></textarea><label class="checkbox-row"><input v-model="esimSubscriptionForm.enabled" type="checkbox"> 启用策略</label><button class="btn primary" :disabled="!esimSubscriptionForm.profileId || esimSubscriptionForm.targetIds.length === 0">{{ editingEsimSubscriptionId ? '保存修改' : '保存订阅策略' }}</button></form></div>
        </section>

        <ToolsPage v-if="!loading && page === 'tools'" v-model:device-id="toolDeviceId" v-model:at-command="toolATCommand" v-model:active-command-id="activeToolCommandId" :devices="devices" :commands="commands" :result="toolResult" @run-command="runDiagnosticCommand" @refresh="loadAll" />

        <LogsPage v-if="!loading && page === 'logs'" :logs="logs" :devices="devices" />

        <AuditPage v-if="!loading && page === 'audit'" :audit="audit" />
  </AppShell>
</template>
