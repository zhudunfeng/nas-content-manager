<script setup lang="ts">
/** 统一的本地线性图标参数。 */
interface Props { /** 图标名称。 */ name: string; /** 图标尺寸。 */ size?: number }
withDefaults(defineProps<Props>(), { size: 20 })
/** 与内容语义匹配的线性图标路径，始终随应用本地发布。 */
const paths: Record<string, string> = {
  home: 'M3 10 12 3l9 7v10a1 1 0 0 1-1 1h-5v-7H9v7H4a1 1 0 0 1-1-1Z',
  video: 'M4 4h16v16H4z M4 9h16 M8 4v5 M16 4v5 M10 12l5 3-5 3z',
  music: 'M9 18V5l12-2v13 M9 9l12-2 M9 18a3 3 0 1 1-3-3h3 M21 16a3 3 0 1 1-3-3h3',
  image: 'M4 4h16v16H4z M4 16l5-5 4 4 3-3 4 4 M8 8h.01',
  comic: 'M3 4h8v16H3z M14 4h7v16h-7z M6 8h2 M17 8h1 M6 12h2',
  novel: 'M3 4c3-1 6-1 9 1 3-2 6-2 9-1v15c-3-1-6-1-9 1-3-2-6-2-9-1z M12 5v15',
  heart: 'M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8Z',
  search: 'M21 21l-5-5 M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0',
  plus: 'M12 5v14 M5 12h14',
  close: 'M6 6l12 12 M6 18 18 6',
  chevron: 'm9 5 7 7-7 7',
  back: 'm15 5-7 7 7 7',
  arrow: 'M4 12h16 m-6-6 6 6-6 6',
  folder: 'M3 5h6l2 2h10v13H3z',
  settings: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8 M10 2h4l1 3 3 1 3 1v4l-2 2 1 3-2 3-3-1-3 2H9l-1-3-3-1-3-1v-4l2-2-1-3 2-3 3 1z',
  jobs: 'M9 4h6v3H9z M9 5H5v16h14V5h-4 M8 12h8 M8 16h5',
  sun: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8 M12 2v2 M12 20v2 M2 12h2 M20 12h2 M5 5l1 1 M18 18l1 1 M5 19l1-1 M18 6l1-1',
  moon: 'M20 15.5A9 9 0 0 1 8.5 4 9 9 0 1 0 20 15.5Z',
  play: 'm8 4 12 8-12 8z',
  refresh: 'M20 4v6h-6 M4 20v-6h6 M4.5 9a8 8 0 0 1 13-5L20 7 M4 17l2.5 3a8 8 0 0 0 13-5',
  logout: 'M9 4H4v16h5 M12 12h9 m-5-5 5 5-5 5',
  download: 'M12 3v12 m-5-5 5 5 5-5 M4 16v5h16v-5',
  info: 'M12 11v6 M12 7h.01 M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',
  check: 'm5 12 4 4L19 6',
  clock: 'M12 6v6l4 2 M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',
  grid: 'M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z',
  more: 'M5 12h.01 M12 12h.01 M19 12h.01',
  list: 'M8 6h13 M8 12h13 M8 18h13 M3 6h.01 M3 12h.01 M3 18h.01',
  trash: 'M3 6h18 M9 6V3h6v3 M5 6l1 15h12l1-15 M10 10v7 M14 10v7',
  warning: 'm12 3 10 18H2z M12 9v5 M12 17h.01',
  book: 'M4 3h16v18H4z M8 3v9l3-2 3 2V3',
  zoom: 'M21 21l-5-5 M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0 M6 10h8 M10 6v8',
  user: 'M16 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0 M4 21v-3a8 8 0 0 1 16 0v3',
  storage: 'M4 3h16v7H4z M4 14h16v7H4z M16 6h.01 M16 17h.01',
}
</script>

<template><svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path :d="paths[name] || paths.info" /></svg></template>
