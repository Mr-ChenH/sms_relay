<script setup lang="ts">
import type { LogEntry } from '../types'
import { formatTime } from '../utils/ui'

defineProps<{ logs: LogEntry[] }>()
</script>

<template>
  <section class="page">
    <div class="page-head"><div><h1>日志</h1><p>查看终端日志和服务端事件。</p></div></div>
    <div class="card terminal"><div v-for="row in logs" :key="row.id">{{ formatTime(row.createdAt) }} [{{ row.deviceName }}] {{ row.message }}</div></div>
  </section>
</template>
