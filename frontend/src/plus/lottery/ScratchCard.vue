<template>
  <div
    ref="wrapRef"
    class="relative select-none overflow-hidden rounded-2xl shadow-lg"
    :style="{ width: width + 'px', height: height + 'px' }"
  >
    <!-- 底层：中奖结果（结果在刮开前已由后端确定，动画只是展示） -->
    <div
      class="absolute inset-0 flex flex-col items-center justify-center"
      :class="
        big
          ? 'bg-gradient-to-br from-rose-100 via-amber-50 to-orange-100 dark:from-rose-900/50 dark:via-amber-900/30 dark:to-orange-900/40'
          : 'bg-gradient-to-br from-amber-50 to-orange-100 dark:from-amber-900/40 dark:to-orange-900/30'
      "
    >
      <p
        class="text-sm"
        :class="big ? 'font-semibold text-rose-600 dark:text-rose-300' : 'text-amber-700 dark:text-amber-300'"
      >
        {{ big ? '手气爆棚 · 大奖' : '恭喜获得' }}
      </p>
      <p
        class="mt-1 font-extrabold tabular-nums text-orange-600 dark:text-orange-400"
        :class="[big ? 'text-5xl' : 'text-4xl', { 'amount-pop': revealed && !reduceMotion }]"
      >
        ${{ shown.toFixed(2) }}
      </p>
      <p class="mt-1 text-xs text-amber-600/80 dark:text-amber-400/80">已自动存入余额</p>
    </div>
    <!-- 揭晓彩带 -->
    <div v-if="revealed && !reduceMotion" class="pointer-events-none absolute inset-0" aria-hidden="true">
      <span
        v-for="(c, i) in confetti"
        :key="i"
        class="confetti"
        :style="{
          left: c.x + '%',
          background: c.color,
          animationDelay: c.delay + 's',
          '--dx': c.dx + 'px',
          '--rot': c.rot + 'deg'
        }"
      ></span>
    </div>
    <!-- 覆盖层：可刮涂层，揭晓后淡出 -->
    <canvas
      ref="canvasRef"
      class="absolute inset-0 cursor-pointer touch-none transition-opacity duration-500"
      :class="revealed ? 'pointer-events-none opacity-0' : ''"
      role="button"
      tabindex="0"
      aria-label="刮开涂层查看奖金，按回车键直接揭晓"
      @pointerdown="onDown"
      @pointermove="onMove"
      @pointerup="onUp"
      @pointerleave="onUp"
      @keydown.enter.prevent="reveal"
      @keydown.space.prevent="reveal"
    ></canvas>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    amount: number
    width?: number
    height?: number
    /** 刮开面积比例达到该值时自动揭晓 */
    threshold?: number
    /** 大奖样式 */
    big?: boolean
  }>(),
  { width: 300, height: 160, threshold: 0.45, big: false }
)

const emit = defineEmits<{ (e: 'revealed'): void }>()

const wrapRef = ref<HTMLDivElement | null>(null)
const canvasRef = ref<HTMLCanvasElement | null>(null)
const revealed = ref(false)
let drawing = false
let moves = 0

const reduceMotion = typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

// 揭晓前显示真实金额（被涂层盖住）；揭晓时从 0 滚动到目标值
const shown = ref(props.amount)
function countUp() {
  if (reduceMotion) return
  const start = performance.now()
  const dur = 700
  const step = (t: number) => {
    const k = Math.min(1, (t - start) / dur)
    shown.value = props.amount * (1 - Math.pow(1 - k, 3))
    if (k < 1) requestAnimationFrame(step)
    else shown.value = props.amount
  }
  requestAnimationFrame(step)
}

const COLORS = ['#f97316', '#f43f5e', '#f59e0b', '#22c55e', '#3b82f6', '#a855f7']
const confetti = Array.from({ length: 28 }, (_, i) => ({
  x: Math.random() * 100,
  dx: (Math.random() - 0.5) * 80,
  rot: Math.random() * 720 - 360,
  delay: Math.random() * 0.25,
  color: COLORS[i % COLORS.length]
}))

function paintCover() {
  const canvas = canvasRef.value
  if (!canvas) return
  const dpr = window.devicePixelRatio || 1
  canvas.width = props.width * dpr
  canvas.height = props.height * dpr
  canvas.style.width = props.width + 'px'
  canvas.style.height = props.height + 'px'
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  ctx.scale(dpr, dpr)
  const grad = ctx.createLinearGradient(0, 0, props.width, props.height)
  grad.addColorStop(0, '#b8b8b8')
  grad.addColorStop(0.5, '#d9d9d9')
  grad.addColorStop(1, '#a8a8a8')
  ctx.fillStyle = grad
  ctx.fillRect(0, 0, props.width, props.height)
  ctx.fillStyle = '#6b6b6b'
  ctx.font = 'bold 18px system-ui, sans-serif'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText('刮开此处', props.width / 2, props.height / 2 - 10)
  ctx.font = '12px system-ui, sans-serif'
  ctx.fillText('按住拖动刮开涂层', props.width / 2, props.height / 2 + 16)
  ctx.globalCompositeOperation = 'destination-out'
}

function scratchAt(e: PointerEvent) {
  const canvas = canvasRef.value
  const ctx = canvas?.getContext('2d')
  if (!canvas || !ctx) return
  const rect = canvas.getBoundingClientRect()
  ctx.beginPath()
  ctx.arc(e.clientX - rect.left, e.clientY - rect.top, 18, 0, Math.PI * 2)
  ctx.fill()
  // 采样检查进度，避免每次 move 都读全量像素
  if (++moves % 8 === 0 && clearedRatio() >= props.threshold) reveal()
}

function clearedRatio(): number {
  const canvas = canvasRef.value
  const ctx = canvas?.getContext('2d')
  if (!canvas || !ctx) return 0
  const { data } = ctx.getImageData(0, 0, canvas.width, canvas.height)
  let cleared = 0
  let total = 0
  for (let i = 3; i < data.length; i += 4 * 16) {
    total++
    if (data[i] === 0) cleared++
  }
  return total ? cleared / total : 0
}

function onDown(e: PointerEvent) {
  drawing = true
  ;(e.target as HTMLElement).setPointerCapture?.(e.pointerId)
  scratchAt(e)
}
function onMove(e: PointerEvent) {
  if (drawing) scratchAt(e)
}
function onUp() {
  drawing = false
}

function reveal() {
  if (revealed.value) return
  revealed.value = true
  countUp()
  emit('revealed')
}

defineExpose({ reveal })

// 父组件以 :key=draw_id 挂载，每张新卡都是新实例
onMounted(paintCover)
</script>

<style scoped>
.confetti {
  position: absolute;
  top: -8px;
  width: 6px;
  height: 10px;
  border-radius: 1px;
  opacity: 0;
  animation: confetti-fall 1.4s ease-out forwards;
}
@keyframes confetti-fall {
  0% {
    opacity: 1;
    transform: translate(0, 0) rotate(0);
  }
  100% {
    opacity: 0;
    transform: translate(var(--dx), 180px) rotate(var(--rot));
  }
}
.amount-pop {
  animation: amount-pop 0.6s cubic-bezier(0.34, 1.56, 0.64, 1);
}
@keyframes amount-pop {
  0% {
    transform: scale(0.6);
  }
  100% {
    transform: scale(1);
  }
}
</style>
