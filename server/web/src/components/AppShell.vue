<script setup lang="ts">
import type { Device, Page } from '../types'
import { navItems } from '../utils/ui'

const page = defineModel<Page>('page', { required: true })
const globalSearch = defineModel<string>('globalSearch', { required: true })
const toolDeviceId = defineModel<string>('toolDeviceId', { required: true })

defineProps<{ devices: Device[] }>()

const emit = defineEmits<{
  refresh: []
  globalSearch: []
}>()
</script>

<template>
  <div class="app">
    <aside class="sidebar">
      <div class="brand"><span class="brand-mark">S</span><div><b>SMS Hub</b><small>多终端短信与 eSIM 管理</small></div></div>
      <nav class="nav">
        <template v-for="item in navItems" :key="item.page">
          <div v-if="item.section" class="nav-label">{{ item.section }}</div>
          <button :class="['nav-item', { active: page === item.page }]" @click="page = item.page"><span>{{ item.icon }}</span><span>{{ item.label }}</span></button>
        </template>
      </nav>
    </aside>

    <main class="main">
      <header class="topbar">
        <input v-model="globalSearch" class="search" placeholder="搜索终端、手机号、ICCID、短信内容" @keydown.enter="emit('globalSearch')" />
        <select v-model="toolDeviceId" class="select"><option value="">全部终端</option><option v-for="device in devices" :key="device.id" :value="device.id">{{ device.name }} / {{ device.iccid }}</option></select>
        <button class="btn" @click="emit('refresh')">刷新</button>
        <button class="btn primary" @click="page = 'devices'">终端列表</button>
      </header>

      <section class="content">
        <slot />
      </section>
    </main>
  </div>
</template>
