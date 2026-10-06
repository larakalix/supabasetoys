<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
defineProps<{ title: string; busy?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement>()
const previousFocus = document.activeElement as HTMLElement | null
onMounted(() => dialog.value?.showModal())
onUnmounted(() => previousFocus?.focus())
</script>
<template>
  <dialog ref="dialog" class="modal" aria-labelledby="modal-title" @cancel.prevent="!busy && emit('close')">
    <header><h2 id="modal-title">{{ title }}</h2><button :disabled="busy" aria-label="Close dialog" class="subtle" @click="emit('close')">×</button></header>
    <slot />
  </dialog>
</template>
