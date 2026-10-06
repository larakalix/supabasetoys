<script setup lang="ts">
defineProps<{ name: 'box' | 'plus' | 'play' | 'stop' | 'refresh' | 'terminal' | 'check' | 'arrow' | 'settings' | 'activity' }>()
const paths = {
  box: 'm12 3 9 5v8l-9 5-9-5V8l9-5Zm0 0v18M3 8l9 5 9-5',
  plus: 'M12 5v14M5 12h14', play: 'm8 5 11 7-11 7V5Z', stop: 'M6 6h12v12H6z',
  refresh: 'M20 7v5h-5M4 17v-5h5M6.3 7a7 7 0 0 1 11.6-1L20 9M4 15l2.1 3A7 7 0 0 0 17.7 17',
  terminal: 'm5 7 5 5-5 5M13 17h6', check: 'm5 12 4 4L19 6', arrow: 'M5 12h14m-6-6 6 6-6 6',
  settings: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8M12 2v3m0 14v3M2 12h3m14 0h3M5 5l2 2m10 10 2 2M5 19l2-2M17 7l2-2',
  activity: 'M2 12h5l3-8 4 16 3-8h5',
}
</script>
<template><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path :d="paths[name]" /></svg></template>
