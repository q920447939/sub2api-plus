<!-- [plus] Markdown 渲染：原始 HTML 一律转义，再过一遍 DOMPurify；链接新窗口打开 -->
<template>
  <div class="markdown-body fb-md" v-html="html" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Marked, type Token } from 'marked'
import DOMPurify from 'dompurify'
import '@/styles/announcement-markdown.css'
import { parseImageSize } from '../format'

const props = defineProps<{ source: string }>()

const escape = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

type Inline = Token & { raw: string }
const isGap = (t: Inline) => t.type === 'br' || (t.type === 'text' && !t.raw.trim())

/**
 * 把段落切成「文字段」和「图片段」：连续的图片（同一行或相邻行，中间只有空白/换行）归为一组横向排列，
 * 其余文字照常成段。这样「标题一行 + 下面几张图」也能并排。
 */
function splitParagraph(tokens: Inline[]): { images: boolean; tokens: Inline[] }[] {
  const out: { images: boolean; tokens: Inline[] }[] = []
  let i = 0
  while (i < tokens.length) {
    if (tokens[i].type === 'image') {
      const run: Inline[] = []
      while (i < tokens.length && (tokens[i].type === 'image' || isGap(tokens[i]))) {
        if (tokens[i].type === 'image') run.push(tokens[i])
        i++
      }
      out.push({ images: true, tokens: run })
    } else {
      const run: Inline[] = []
      while (i < tokens.length && tokens[i].type !== 'image') run.push(tokens[i++])
      // 去掉紧挨图片的首尾换行，避免多出空行
      while (run.length && isGap(run[0])) run.shift()
      while (run.length && isGap(run[run.length - 1])) run.pop()
      if (run.length) out.push({ images: false, tokens: run })
    }
  }
  return out
}

const md = new Marked({
  gfm: true,
  breaks: true,
  renderer: {
    html: ({ text }) => escape(text),
    link(token) {
      const safe = /^(https?:|mailto:|\/)/i.test(token.href) ? token.href : '#'
      return `<a href="${escape(safe)}" target="_blank" rel="noopener noreferrer">${this.parser.parseInline(token.tokens)}</a>`
    },
    image({ href, title, text }) {
      if (!/^(https?:|\/)/i.test(href)) return escape(text)
      const { alt, style } = parseImageSize(text)
      const attrs = [`src="${escape(href)}"`, `alt="${escape(alt)}"`, 'loading="lazy"']
      if (title) attrs.push(`title="${escape(title)}"`)
      if (style) attrs.push(`style="${style}"`, 'class="fb-img-sized"')
      return `<img ${attrs.join(' ')}>`
    },
    paragraph({ tokens }) {
      return splitParagraph(tokens as Inline[])
        .map((seg) => (seg.images ? `<div class="fb-img-row">${this.parser.parseInline(seg.tokens)}</div>\n` : `<p>${this.parser.parseInline(seg.tokens)}</p>\n`))
        .join('')
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
/* 图片：默认最多 160px 高；写了尺寸的按尺寸（仍不超过卡片宽度） */
.fb-md :deep(img) {
  display: block;
  max-width: 100%;
  max-height: 160px;
  width: auto;
  object-fit: contain;
  margin: 0.25rem 0;
  border-radius: 0.5rem;
  box-shadow: none;
}
.fb-md :deep(img.fb-img-sized) {
  max-height: none;
}
/* 并排：自动换行，窄屏时一行放不下就折到下一行 */
.fb-md :deep(.fb-img-row) {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: 0.5rem;
  margin: 0.25rem 0 0.5rem;
}
.fb-md :deep(.fb-img-row img) {
  margin: 0;
  min-width: 0;
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
