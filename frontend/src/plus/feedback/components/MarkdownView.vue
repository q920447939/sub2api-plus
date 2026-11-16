<!-- [plus] Markdown 渲染：原始 HTML 一律转义，再过一遍 DOMPurify；链接新窗口打开 -->
<template>
  <div class="markdown-body fb-md" v-html="html" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Marked } from 'marked'
import DOMPurify from 'dompurify'
import '@/styles/announcement-markdown.css'

const props = defineProps<{ source: string }>()

const escape = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

const md = new Marked({
  gfm: true,
  breaks: true,
  renderer: {
    html: ({ text }) => escape(text),
    link(token) {
      const safe = /^(https?:|mailto:|\/)/i.test(token.href) ? token.href : '#'
      return `<a href="${escape(safe)}" target="_blank" rel="noopener noreferrer">${this.parser.parseInline(token.tokens)}</a>`
    }
  }
})

const html = computed(() => DOMPurify.sanitize(md.parse(props.source || '', { async: false }) as string, { ADD_ATTR: ['target'] }))
</script>

<style scoped>
/* 沿用项目公告的 markdown-body 样式，只把字号和段距收紧到卡片里合适的大小 */
.fb-md {
  font-size: 0.875rem;
  line-height: 1.7;
}
.fb-md > :deep(:first-child) {
  margin-top: 0;
}
.fb-md > :deep(:last-child) {
  margin-bottom: 0;
}
.fb-md :deep(strong) {
  font-weight: 600;
  color: rgb(17 24 39);
}
:global(.dark) .fb-md :deep(strong) {
  color: #fff;
}
.fb-md :deep(p) {
  margin-bottom: 0.5rem;
}
.fb-md :deep(h1) {
  margin: 0.75rem 0 0.5rem;
  font-size: 1.25rem;
  padding-bottom: 0.375rem;
}
.fb-md :deep(h2) {
  margin: 0.75rem 0 0.5rem;
  font-size: 1.125rem;
  padding-bottom: 0.25rem;
}
.fb-md :deep(h3),
.fb-md :deep(h4) {
  margin: 0.5rem 0 0.375rem;
  font-size: 1rem;
}
.fb-md :deep(ul),
.fb-md :deep(ol) {
  margin-bottom: 0.5rem;
  margin-left: 1.25rem;
}
</style>
