<script setup lang="ts">
import type { AuditLog } from '../types'
import { formatTime, statusClass } from '../utils/ui'

defineProps<{ audit: AuditLog[] }>()
</script>

<template>
  <section class="page">
    <div class="page-head"><div><h1>审计</h1><p>记录短信发送、AT、模组控制、eSIM 删除/下载等敏感操作。</p></div></div>
    <div class="card">
      <table>
        <thead><tr><th>时间</th><th>操作者</th><th>终端</th><th>动作</th><th>参数摘要</th><th>结果</th></tr></thead>
        <tbody>
          <tr v-for="row in audit" :key="row.id"><td>{{ formatTime(row.createdAt) }}</td><td>{{ row.actor }}</td><td>{{ row.deviceName }}</td><td>{{ row.action }}</td><td>{{ row.parameterSummary }}</td><td><span :class="['status', statusClass(row.result)]">{{ row.result }}</span></td></tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
