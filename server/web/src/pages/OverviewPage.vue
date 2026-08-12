<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue'
import { ScheduleXCalendar } from '@schedule-x/vue'
import { createCalendar, createViewMonthAgenda, createViewMonthGrid } from '@schedule-x/calendar'
import { createEventsServicePlugin } from '@schedule-x/events-service'
import '@schedule-x/theme-default/dist/index.css'
import { Temporal } from 'temporal-polyfill'
import 'temporal-polyfill/global'
import type { Dashboard, EsimSubscription } from '../types'
import { formatTime, statusClass } from '../utils/ui'

const props = defineProps<{ dashboard: Dashboard }>()

const emit = defineEmits<{
  openRoutes: []
  openSms: []
}>()

const eventsService = createEventsServicePlugin()
const upcomingSubscriptions = computed(() => (props.dashboard.esimSubscriptions || [])
  .filter((sub) => sub.enabled && sub.nextRunAt)
  .sort((left, right) => new Date(left.nextRunAt).getTime() - new Date(right.nextRunAt).getTime())
  .slice(0, 6))
const monthGrid = createViewMonthGrid()
const monthAgenda = createViewMonthAgenda()
const calendarApp = createCalendar({
  locale: 'zh-CN',
  firstDayOfWeek: 1,
  views: [monthGrid, monthAgenda],
  defaultView: monthGrid.name,
  events: buildEvents(props.dashboard.esimSubscriptions || []),
  plugins: [eventsService]
})

watch(() => props.dashboard.esimSubscriptions, (subscriptions) => {
  eventsService.set(buildEvents(subscriptions || []))
}, { deep: true })

onBeforeUnmount(() => calendarApp.destroy())

function buildEvents(subscriptions: EsimSubscription[]) {
  const windowStart = new Date()
  windowStart.setFullYear(windowStart.getFullYear() - 1)
  const windowEnd = new Date()
  windowEnd.setFullYear(windowEnd.getFullYear() + 2)
  return subscriptions.flatMap((sub) => subscriptionEvents(sub, windowStart, windowEnd))
}

function subscriptionEvents(sub: EsimSubscription, windowStart: Date, windowEnd: Date) {
  if (!sub.enabled) return []
  const start = new Date(sub.startAt || sub.nextRunAt)
  if (Number.isNaN(start.getTime())) return []
  const cursor = new Date(start.getFullYear(), start.getMonth(), start.getDate())
  const interval = Math.max(1, sub.intervalDays)
  while (cursor < windowStart) cursor.setDate(cursor.getDate() + interval)

  const result = []
  while (cursor <= windowEnd) {
    const date = dateKey(cursor)
    result.push({
      id: `${sub.id}-${date}`,
      title: `${sub.type === 'recharge' ? '充值' : '短信保活'} · ${sub.profileName}`,
      start: Temporal.PlainDate.from(date),
      end: Temporal.PlainDate.from(date),
      calendarId: sub.type,
      subscription: sub
    })
    cursor.setDate(cursor.getDate() + interval)
  }
  return result
}

function dateKey(date: Date) {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}
</script>

<template>
  <section class="page">
    <div class="page-head"><div><h1>总览</h1><p>统一查看终端健康、短信流量、失败任务和 eSIM 状态。</p></div><button class="btn primary" @click="emit('openRoutes')">创建分发规则</button></div>
    <div class="grid cols-4">
      <div class="card metric"><span>在线终端</span><b>{{ dashboard.onlineDevices }} / {{ dashboard.totalDevices }}</b><small>离线终端需要处理</small></div>
      <div class="card metric"><span>今日短信</span><b>{{ dashboard.todaySms }}</b><small>验证码占主要流量</small></div>
      <div class="card metric"><span>分发失败</span><b>{{ dashboard.deliveryFailures }}</b><small>检查推送通道</small></div>
      <div class="card metric"><span>eSIM 任务</span><b>{{ dashboard.runningEsimTasks }}</b><small>运行中任务</small></div>
    </div>
    <div class="overview-calendar-layout top-gap">
      <div class="card subscription-calendar">
        <div class="card-head calendar-panel-head">
          <div><b>订阅保活日历</b><small>充值与短信保活提醒计划</small></div>
          <div class="calendar-legend"><span><i class="recharge"></i>充值提醒</span><span><i class="keepalive"></i>短信保活</span></div>
        </div>
        <div class="sx-vue-calendar-wrapper">
          <ScheduleXCalendar :calendar-app="calendarApp">
            <template #monthGridEvent="{ calendarEvent }">
              <div :class="['sx-subscription-event', calendarEvent.calendarId]">
                <i></i><span>{{ calendarEvent.title }}</span>
              </div>
            </template>
            <template #monthAgendaEvent="{ calendarEvent }">
              <div :class="['sx-agenda-event', calendarEvent.calendarId]">
                <div><b>{{ calendarEvent.title }}</b><small>{{ calendarEvent.subscription.deviceName }} · 每 {{ calendarEvent.subscription.intervalDays }} 天</small></div>
                <span>{{ calendarEvent.subscription.type === 'recharge' ? calendarEvent.subscription.rechargeAmount : calendarEvent.subscription.keepaliveNumber }}</span>
              </div>
            </template>
            <template #eventModal="{ calendarEvent }">
              <div class="sx-event-detail">
                <div class="sx-event-detail-head"><span :class="calendarEvent.calendarId"></span><div><b>{{ calendarEvent.title }}</b><small>{{ calendarEvent.subscription.deviceName }}</small></div></div>
                <dl>
                  <dt>ICCID</dt><dd class="mono">{{ calendarEvent.subscription.iccid }}</dd>
                  <dt>开始时间</dt><dd>{{ formatTime(calendarEvent.subscription.startAt) }}</dd>
                  <dt>提醒周期</dt><dd>每 {{ calendarEvent.subscription.intervalDays }} 天</dd>
                  <dt>下次提醒</dt><dd>{{ formatTime(calendarEvent.subscription.nextRunAt) }}</dd>
                  <dt>提醒内容</dt><dd>{{ calendarEvent.subscription.type === 'recharge' ? (calendarEvent.subscription.rechargeAmount || '-') : `${calendarEvent.subscription.keepaliveNumber} / ${calendarEvent.subscription.keepaliveMessage}` }}</dd>
                  <dt>备注</dt><dd>{{ calendarEvent.subscription.note || '-' }}</dd>
                </dl>
              </div>
            </template>
          </ScheduleXCalendar>
        </div>
      </div>
      <div class="card upcoming-subscriptions">
        <div class="card-head"><div><b>近期提醒</b><small>按下次执行时间排序</small></div></div>
        <div class="upcoming-list">
          <div v-for="sub in upcomingSubscriptions" :key="sub.id" class="upcoming-item">
            <span :class="['upcoming-marker', sub.type]"></span>
            <div><b>{{ sub.profileName }}</b><small>{{ sub.deviceName }} · 每 {{ sub.intervalDays }} 天</small><span>{{ sub.type === 'recharge' ? (sub.rechargeAmount || '充值提醒') : `${sub.keepaliveNumber} / ${sub.keepaliveMessage}` }}</span></div>
            <time>{{ formatTime(sub.nextRunAt) }}</time>
          </div>
          <div v-if="upcomingSubscriptions.length === 0" class="empty muted">暂无已启用的订阅提醒。</div>
        </div>
      </div>
    </div>
    <div class="grid layout-2 top-gap">
      <div class="card"><div class="card-head"><b>最近短信</b><button class="btn small" @click="emit('openSms')">查看历史</button></div><table><thead><tr><th>时间</th><th>终端</th><th>发送者</th><th>内容</th><th>分发</th></tr></thead><tbody><tr v-for="item in dashboard.recentSms" :key="item.id"><td>{{ formatTime(item.timestamp) }}</td><td>{{ item.deviceName }}</td><td class="mono">{{ item.sender }}</td><td class="truncate">{{ item.body }}</td><td><span :class="['status', statusClass(item.deliveryStatus)]">{{ item.deliverySummary }}</span></td></tr></tbody></table></div>
      <div class="card"><div class="card-head"><b>待处理告警</b></div><div class="timeline"><div v-for="alert in dashboard.alerts" :key="alert.title" class="event"><span>{{ alert.time }}</span><div><b>{{ alert.title }}</b><small>{{ alert.message }}</small></div></div><div v-if="dashboard.alerts.length === 0" class="empty muted">暂无待处理告警。</div></div></div>
    </div>
  </section>
</template>
