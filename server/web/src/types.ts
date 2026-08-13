export type Page = 'overview' | 'devices' | 'sms' | 'send' | 'routes' | 'esim' | 'esim-profiles' | 'esim-subscriptions' | 'tools' | 'logs' | 'audit'

export interface PublicConfig {
  apiBaseUrl: string
  mqttBroker: string
}

export interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: string
}

export interface Device {
  id: string
  deviceId: string
  name: string
  status: string
  firmwareVersion: string
  hardwareModel: string
  iccid: string
  eid: string
  operator: string
  phoneNumber: string
  rssi: number
  freeHeapKb: number
  uptime: string
  lastSeenAt: string
}

export interface SMSMessage {
  id: string
  deviceId: string
  deviceName: string
  sender: string
  recipient: string
  body: string
  timestamp: string
  tag: string
  deliveryStatus: string
  deliverySummary: string
  concatInfo: string
}

export interface SMSList {
  items: SMSMessage[]
  total: number
  page: number
  pageSize: number
  totalAll: number
  earliestAt: string
}

export interface Dashboard {
  onlineDevices: number
  totalDevices: number
  todaySms: number
  deliveryFailures: number
  runningEsimTasks: number
  recentSms: SMSMessage[]
  alerts: Array<{ time: string; title: string; message: string; level: string }>
  esimSubscriptions: EsimSubscription[]
}

export interface AppriseService {
  id: string
  name: string
  baseUrl: string
  enabled: boolean
  lastStatus: string
  lastMessage: string
  updatedAt: string
}

export interface CreateAppriseServiceRequest {
  name: string
  baseUrl: string
  enabled: boolean
}

export type UpdateAppriseServiceRequest = CreateAppriseServiceRequest

export interface AppriseTarget {
  id: string
  serviceId: string
  serviceName: string
  name: string
  configKey: string
  tags: string[]
  enabled: boolean
  titleTemplate: string
  bodyTemplate: string
  lastStatus: string
  description: string
}

export interface CreateAppriseTargetRequest {
  serviceId: string
  name: string
  configKey: string
  tags: string[]
  enabled: boolean
  titleTemplate: string
  bodyTemplate: string
}

export interface RoutingRule {
  id: string
  name: string
  senderContains: string
  bodyKeywords: string[]
  deviceIds: string[]
  tags: string[]
  targetIds: string[]
  enabled: boolean
}

export interface CreateRoutingRuleRequest {
  name: string
  senderContains: string
  bodyKeywords: string[]
  deviceIds: string[]
  tags: string[]
  targetIds: string[]
  enabled: boolean
}

export type UpdateRoutingRuleRequest = CreateRoutingRuleRequest

export interface EsimCapabilities {
  profileDownload: boolean
  platform: string
  reason: string
}

export interface EsimProfile {
  id: string
  deviceId: string
  iccid: string
  aid: string
  nickname: string
  provider: string
  country: string
  phoneNumber: string
  profileName: string
  state: string
  available: boolean
  missingSince?: string
  lastSeenAt: string
}

export interface EsimTaskEvent {
  status: string
  stage: string
  progress: number
  createdAt: string
}

export interface EsimOperationTask {
  id: string
  deviceId: string
  type: string
  status: string
  stage: string
  progress: number
  history?: EsimTaskEvent[]
  createdAt?: string
  updatedAt?: string
}

export interface EsimTask {
  id: string
  deviceId: string
  type: string
  status: string
  stage: string
  progress: number
  history: EsimTaskEvent[]
  createdAt: string
  updatedAt: string
}

export interface CreateEsimTaskRequest {
  deviceId: string
  activationCode: string
  smdpAddress: string
  confirmationCode: string
}

export interface EsimSubscription {
  id: string
  profileId: string
  profileName: string
  iccid: string
  deviceId: string
  deviceName: string
  country: string
  enabled: boolean
  type: 'recharge' | 'sms_keepalive'
  intervalDays: number
  startAt: string
  rechargeAmount: string
  keepaliveNumber: string
  keepaliveMessage: string
  targetIds: string[]
  nextRunAt: string
  lastRunAt: string
  status: string
  note: string
  updatedAt: string
}

export interface CreateEsimSubscriptionRequest {
  profileId: string
  enabled: boolean
  type: 'recharge' | 'sms_keepalive'
  intervalDays: number
  startAt: string
  rechargeAmount: string
  keepaliveNumber: string
  keepaliveMessage: string
  targetIds: string[]
  note: string
}

export type UpdateEsimSubscriptionRequest = Omit<CreateEsimSubscriptionRequest, 'profileId'>

export interface LogEntry {
  id: string
  deviceId: string
  deviceName: string
  level: string
  message: string
  createdAt: string
}

export interface AuditLog {
  id: string
  actor: string
  deviceName: string
  action: string
  parameterSummary: string
  result: string
  createdAt: string
}

export interface CommandResult {
  commandId: string
  status: string
  message: string
}

export interface DeviceCommand {
  id: string
  deviceId: string
  type: string
  payload: Record<string, unknown>
  status: string
  result?: string
  createdAt: string
  claimedAt?: string
  completedAt?: string
}

export interface CreateDeviceCommandRequest {
  deviceId: string
  type: string
  payload: Record<string, unknown>
}
