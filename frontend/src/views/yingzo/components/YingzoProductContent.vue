<template>
  <div class="yingzo-page">
    <section class="yingzo-hero" role="img" aria-label="电影片场中的完整制作团队">
      <header class="yingzo-header">
        <router-link to="/yingzo" class="yingzo-brand" aria-label="Yingzo（影作）首页">
          <img src="/yingzo/icon.png" alt="" />
          <span>Yingzo（影作）</span>
        </router-link>
        <nav aria-label="Yingzo 页面导航">
          <a href="#advantages" @click.prevent="scrollToSection('advantages')">优势</a>
          <a href="#workflow" @click.prevent="scrollToSection('workflow')">工作流</a>
          <a href="#install" @click.prevent="scrollToSection('install')">安装</a>
        </nav>
        <router-link
          :to="authStore.isAuthenticated ? '/dashboard' : '/login?redirect=/yingzo'"
          class="yingzo-header-action"
        >
          {{ authStore.isAuthenticated ? '返回控制台' : '登录' }}
        </router-link>
      </header>

      <div class="yingzo-hero-copy">
        <h1>Yingzo（影作）</h1>
        <p class="yingzo-hero-claim">让 ChatGPT Work 与 Claude Cowork<br />成为你的 AI 影视制作团队</p>
        <p class="yingzo-hero-summary">
          从故事、剧本、导演、分镜，到图片与视频生成。<br />同一个项目，同一套资产，同一条生产链。
        </p>
        <div class="yingzo-hero-actions">
          <a href="#install" class="yingzo-button yingzo-button-primary" @click.prevent="scrollToSection('install')">安装 Yingzo</a>
          <a href="#advantages" class="yingzo-button yingzo-button-ghost" @click.prevent="scrollToSection('advantages')">为什么是 Yingzo</a>
        </div>
      </div>

      <div class="yingzo-hero-rail" aria-label="Yingzo 核心价值">
        <span>完整生产链</span>
        <span>稳定资产引用</span>
        <span>多 Agent 协作</span>
        <span>创作审阅 App</span>
        <span>结果永久落地</span>
      </div>
    </section>

    <main>
      <section id="advantages" class="yingzo-section yingzo-advantages">
        <div class="yingzo-section-intro">
          <h2>Yingzo 解决的，<br />不是一次生成。</h2>
          <p>
            普通 AI 工具给你一张图、一个视频，然后一切重新开始。Yingzo 保留项目、角色、场景、镜头、版本和生成记录，让每一步都能被下一步继续使用。
          </p>
        </div>

        <ol class="yingzo-advantage-list">
          <li v-for="(item, index) in advantages" :key="item.title">
            <span>{{ String(index + 1).padStart(2, '0') }}</span>
            <h3>{{ item.title }}</h3>
            <p>{{ item.description }}</p>
          </li>
        </ol>
      </section>

      <section class="yingzo-production-band">
        <figure>
          <img src="/yingzo/storyboard.jpg" alt="故事板艺术家正在组织镜头与画面连续性" />
        </figure>
        <div class="yingzo-production-copy">
          <h2>不是拼提示词，<br />是组织生产。</h2>
          <p>
            Yingzo 先理解创作目标，再把任务拆给故事、剧本、导演、分镜、视觉、图片、视频与审查 Agent。每个 Agent 只负责自己擅长的部分，正式状态由主 Agent 统一合并。
          </p>
          <dl>
            <div><dt>同一项目</dt><dd>ChatGPT Work、Codex 与 Claude Cowork/Code 可接手同一个本地项目</dd></div>
            <div><dt>同一资产</dt><dd>参考素材按稳定 ID 与版本解析，不靠文件名猜测</dd></div>
            <div><dt>同一规则</dt><dd>依赖图、单写者与提案合并，避免互相覆盖</dd></div>
          </dl>
        </div>
      </section>

      <section id="workflow" class="yingzo-section yingzo-workflow-section">
        <div class="yingzo-section-intro yingzo-section-intro-wide">
          <h2>从一句想法，<br />到可继续迭代的成片。</h2>
          <p>每个阶段都留下结构化产物，而不是只留下聊天记录。</p>
        </div>
        <ol class="yingzo-workflow">
          <li v-for="(item, index) in workflow" :key="item.title">
            <span>{{ String(index + 1).padStart(2, '0') }}</span>
            <div>
              <h3>{{ item.title }}</h3>
              <p>{{ item.description }}</p>
            </div>
          </li>
        </ol>
      </section>

      <section class="yingzo-model-band">
        <div class="yingzo-model-heading">
          <h2>图片、视频、分析，<br />在一条生产线上。</h2>
          <p>模型只是能力，项目才是长期来源。</p>
        </div>
        <div class="yingzo-model-list">
          <article v-for="item in modelCapabilities" :key="item.title">
            <span>{{ item.model }}</span>
            <h3>{{ item.title }}</h3>
            <p>{{ item.description }}</p>
          </article>
        </div>
      </section>

      <section class="yingzo-section yingzo-assurance-section">
        <div class="yingzo-assurance-statement">
          <h2>该自动时自动，<br />该决定时由你决定。</h2>
          <p>文本和图片生成遵循宿主权限直接执行；视频可按任务类型记住审批。结果始终下载并登记为本地草稿。</p>
        </div>
        <div class="yingzo-assurance-lines">
          <div><strong>宿主权限</strong><span>完全访问自动执行，请求批准时才打断用户</span></div>
          <div><strong>本地草稿</strong><span>图片与视频都先下载到项目，再进入候选、迭代和提升</span></div>
          <div><strong>边创作边学习</strong><span>审阅结果时可查看实际提交提示词与有序参考素材</span></div>
        </div>
      </section>

      <section id="install" class="yingzo-install-section">
        <div class="yingzo-install-copy">
          <h2>把安装交给 Agent。</h2>
          <p>选择实际使用的工作模式，复制一次性安装提示词，再粘贴给对应 Agent。无需用户操作 CLI。</p>
        </div>

        <div class="yingzo-install-controls">
          <div class="yingzo-release-meta" v-if="release">
            <span>{{ release.channel === 'prerelease' ? '预发布版' : '稳定版' }} {{ release.version }}</span>
            <span>{{ compatibilityText }}</span>
          </div>
          <div class="yingzo-release-meta" v-else>
            <span>{{ releaseLoading ? '正在读取发行信息' : '管理员尚未发布安装包' }}</span>
          </div>
          <div class="yingzo-platform-controls" aria-label="安装平台">
            <label>系统
              <select v-model="selectedOS">
                <option value="macos">macOS</option>
                <option value="windows">Windows</option>
              </select>
            </label>
            <label>架构
              <select v-model="selectedArch">
                <option v-if="selectedOS === 'macos'" value="arm64">Apple Silicon</option>
                <option value="x64">{{ selectedOS === 'macos' ? 'Intel' : 'x64' }}</option>
              </select>
            </label>
            <label class="yingzo-channel-toggle">
              <input v-model="usePrerelease" type="checkbox" />
              <span>体验预发布版</span>
            </label>
          </div>

          <div v-if="authStore.isAuthenticated" class="yingzo-host-actions" aria-label="复制安装提示词">
            <button
              v-for="host in hosts"
              :key="host.id"
              type="button"
              class="yingzo-copy-button"
              :disabled="copyingHost !== null || !release"
              @click="copyInstallPrompt(host.id)"
            >
              <Icon :name="copiedHost === host.id ? 'check' : 'copy'" size="sm" />
              {{ copiedHost === host.id ? '已复制，10 分钟内粘贴使用' : `复制给 ${host.label}` }}
            </button>
          </div>
          <router-link v-else to="/login?redirect=/yingzo" class="yingzo-copy-button">
            登录后生成安装提示词
          </router-link>
          <p v-if="installError" class="yingzo-error" role="alert">{{ installError }}</p>
          <div v-if="lastInstructions?.runtime_resolution === 'probe' && lastInstructions.runtime_helper_uri" class="yingzo-runtime-fallback">
            <a :href="lastInstructions.runtime_helper_uri" class="yingzo-runtime-link">检测已安装 Runtime</a>
            <a v-if="lastInstructions.runtime_installer?.download_url" :href="lastInstructions.runtime_installer.download_url" class="yingzo-runtime-link" download>直接下载安装器</a>
          </div>
        </div>
      </section>
    </main>

    <footer class="yingzo-footer">
      <div class="yingzo-brand">
        <img src="/yingzo/icon.png" alt="" />
        <span>Yingzo（影作）</span>
      </div>
      <p>私有 Beta · 未授权再分发</p>
      <p class="yingzo-credits">
        影像：
        <a href="https://commons.wikimedia.org/wiki/File:Behind_the_Scene_shots_of_one_of_our_production_sets_by_Don_Ramey_Logan.jpg" target="_blank" rel="noreferrer">Don Ramey Logan</a>
        / <a href="https://commons.wikimedia.org/wiki/File:Carl_Urbano_working_on_a_storyboard,_1967.jpg" target="_blank" rel="noreferrer">Steve Fontanini</a>
      </p>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Icon } from '@/components/icons'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  createYingzoInstallInstructions, getLatestYingzoRelease, type YingzoArchitecture,
  type YingzoHost, type YingzoOperatingSystem, type YingzoReleaseChannel, type YingzoReleaseSummary,
} from '@/api/yingzo'

const authStore = useAuthStore()
const copiedHost = ref<YingzoHost | null>(null)
const copyingHost = ref<YingzoHost | null>(null)
const installError = ref('')
const release = ref<YingzoReleaseSummary | null>(null)
const lastInstructions = ref<Awaited<ReturnType<typeof createYingzoInstallInstructions>> | null>(null)
const releaseLoading = ref(true)
const selectedOS = ref<Exclude<YingzoOperatingSystem, 'any'>>('macos')
const selectedArch = ref<Exclude<YingzoArchitecture, 'any'>>('arm64')
const usePrerelease = ref(false)

const allHosts: Array<{ id: YingzoHost; label: string }> = [
  { id: 'chatgpt-work', label: 'ChatGPT Work' },
  { id: 'codex', label: 'Codex' },
  { id: 'claude-chat', label: 'Claude Desktop' },
  { id: 'claude-cowork', label: 'Claude Cowork' },
  { id: 'claude-code', label: 'Claude Code' },
]

const hosts = computed(() => allHosts.filter((host) => (
  host.id !== 'claude-chat' || (release.value?.distribution_schema_version || 1) >= 2
)))

const advantages = [
  { title: '完整创作链，而不是单点工具', description: '故事、剧本、导演、分镜、图片、视频与审查共享同一份项目状态。' },
  { title: '角色与场景不会每次重新开始', description: '资产使用稳定 ID、版本和生效范围，路径变化也不会破坏引用。' },
  { title: '多 Agent 协作，但不会互相覆盖', description: '依赖图决定执行顺序，子 Agent 只提交提案，主 Agent 负责正式合并。' },
  { title: '生成高效，决定仍由创作者掌握', description: '宿主权限控制自动执行边界，候选、返修与提升集中在审阅 App 完成。' },
]

const workflow = [
  { title: '故事与系列', description: '主题、角色弧、系列圣经与单集状态。' },
  { title: '剧本与导演', description: '可执行剧本、调度、视点、覆盖与声音。' },
  { title: '分镜与视觉', description: '镜头设计、角色资产、场景与统一视觉语言。' },
  { title: '图片与视频', description: '候选、编辑、参考生成、延长与草稿迭代。' },
  { title: '质量与连续性', description: '故事、导演、媒体、商品与跨集一致性审查。' },
]

const modelCapabilities = [
  { model: 'gpt-image-2', title: '图片生成与精准编辑', description: '默认 2K 中等质量，多张候选通过多个独立任务生成。' },
  { model: 'Seedance', title: '视频生成、编辑与延长', description: '参考素材按图片 1、视频 1、音频 1 的顺序进入任务。' },
  { model: 'Codex Native', title: '视频截帧与镜头分析', description: '直接读取本地视频并截帧分析，不绕行文本中转模型。' },
]

const compatibilityText = computed(() => {
  if (!release.value) return ''
  const codex = release.value.min_codex_version || '0.143.0+'
  const claude = release.value.min_claude_version || '2.1.201+'
  return `Codex ${codex} · Claude Code ${claude}`
})

function scrollToSection(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  window.history.replaceState(null, '', `#${id}`)
}

async function copyInstallPrompt(host: YingzoHost) {
  copyingHost.value = host
  installError.value = ''
  copiedHost.value = null
  lastInstructions.value = null
  try {
    const instructions = await createYingzoInstallInstructions({
      host,
      os: selectedOS.value,
      arch: selectedOS.value === 'windows' ? 'x64' : selectedArch.value,
      channel: releaseChannel(),
      runtime_capability: 'unknown',
    })
    if (instructions.host !== host) throw new Error(`Install host mismatch: expected ${host}, received ${instructions.host}`)
    await navigator.clipboard.writeText(instructions.prompt)
    lastInstructions.value = instructions
    copiedHost.value = host
    window.setTimeout(() => {
      if (copiedHost.value === host) copiedHost.value = null
    }, 5000)
  } catch (error) {
    installError.value = `生成安装提示词失败：${extractApiErrorMessage(error, '请确认当前版本已发布，并重新登录后再试。')}`
    console.error(error)
  } finally {
    copyingHost.value = null
  }
}

function releaseChannel(): YingzoReleaseChannel { return usePrerelease.value ? 'prerelease' : 'stable' }

async function loadRelease() {
  releaseLoading.value = true
  installError.value = ''
  try {
    release.value = await getLatestYingzoRelease(releaseChannel())
  } catch {
    release.value = null
    if (usePrerelease.value) installError.value = '当前没有可用的预发布版本。'
  } finally {
    releaseLoading.value = false
  }
}

async function detectPlatform() {
  const userAgent = navigator.userAgent.toLowerCase()
  selectedOS.value = userAgent.includes('windows') ? 'windows' : 'macos'
  if (selectedOS.value === 'windows') { selectedArch.value = 'x64'; return }
  const userAgentData = (navigator as Navigator & {
    userAgentData?: { getHighEntropyValues?: (hints: string[]) => Promise<{ architecture?: string }> }
  }).userAgentData
  try {
    const architecture = await userAgentData?.getHighEntropyValues?.(['architecture'])
    selectedArch.value = architecture?.architecture?.toLowerCase().includes('x86') ? 'x64' : 'arm64'
  } catch { selectedArch.value = 'arm64' }
}

watch(selectedOS, (os) => { if (os === 'windows') selectedArch.value = 'x64' })
watch(usePrerelease, () => { void loadRelease() })
onMounted(async () => { await detectPlatform(); await loadRelease() })
</script>

<style scoped>
.yingzo-page {
  --yz-red: #d64c3b;
  --yz-red-dark: #b93426;
  --yz-ink: #111111;
  --yz-muted: #666666;
  --yz-line: #d8d8d8;
  min-height: 100vh;
  overflow-x: hidden;
  background: #ffffff;
  color: var(--yz-ink);
  font-family: Inter, "PingFang SC", "Microsoft YaHei", sans-serif;
}

.yingzo-header {
  position: absolute;
  z-index: 4;
  top: 0;
  left: 50%;
  width: min(1240px, calc(100% - 48px));
  height: 72px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  transform: translateX(-50%);
  border-bottom: 1px solid rgba(255, 255, 255, .28);
}

.yingzo-brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: inherit;
  font-size: 15px;
  font-weight: 760;
  text-decoration: none;
}

.yingzo-brand img {
  width: 38px;
  height: 38px;
  object-fit: contain;
}

.yingzo-header .yingzo-brand,
.yingzo-header nav a,
.yingzo-header-action {
  color: #ffffff;
}

.yingzo-header nav {
  display: flex;
  gap: 34px;
}

.yingzo-header nav a,
.yingzo-header-action {
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;
}

.yingzo-header nav a:hover,
.yingzo-header-action:hover {
  color: #ffad9f;
}

.yingzo-header-action {
  min-height: 38px;
  display: inline-flex;
  align-items: center;
  padding: 0 14px;
  border: 1px solid rgba(255, 255, 255, .62);
  border-radius: 4px;
}

.yingzo-hero {
  position: relative;
  height: clamp(630px, calc(100svh - 140px), 820px);
  overflow: hidden;
  background-image:
    linear-gradient(90deg, rgba(0, 0, 0, .88) 0%, rgba(0, 0, 0, .68) 42%, rgba(0, 0, 0, .2) 72%, rgba(0, 0, 0, .12) 100%),
    url('/yingzo/production-set.jpg');
  background-position: center;
  background-size: cover;
  color: #ffffff;
}

.yingzo-hero-copy {
  position: relative;
  z-index: 2;
  width: min(1240px, calc(100% - 48px));
  margin: 0 auto;
  padding-top: 136px;
}

.yingzo-hero h1 {
  margin: 0;
  font-size: 76px;
  line-height: 1;
  font-weight: 820;
  letter-spacing: 0;
}

.yingzo-hero-claim {
  margin: 28px 0 0;
  font-size: 31px;
  line-height: 1.35;
  font-weight: 720;
}

.yingzo-hero-summary {
  margin: 22px 0 0;
  color: rgba(255, 255, 255, .82);
  font-size: 17px;
  line-height: 1.8;
}

.yingzo-hero-actions {
  display: flex;
  gap: 12px;
  margin-top: 34px;
}

.yingzo-button {
  min-height: 46px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0 20px;
  border-radius: 4px;
  font-size: 14px;
  font-weight: 720;
  text-decoration: none;
  transition: background-color .18s ease, border-color .18s ease, color .18s ease;
}

.yingzo-button-primary {
  background: var(--yz-red);
  color: #ffffff;
}

.yingzo-button-primary:hover {
  background: var(--yz-red-dark);
}

.yingzo-button-ghost {
  border: 1px solid rgba(255, 255, 255, .72);
  color: #ffffff;
}

.yingzo-button-ghost:hover {
  background: #ffffff;
  color: var(--yz-ink);
}

.yingzo-hero-rail {
  position: absolute;
  z-index: 2;
  right: 0;
  bottom: 0;
  left: 0;
  min-height: 66px;
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  align-items: center;
  padding: 0 max(24px, calc((100% - 1240px) / 2));
  border-top: 1px solid rgba(255, 255, 255, .3);
  background: rgba(0, 0, 0, .34);
  backdrop-filter: blur(10px);
}

.yingzo-hero-rail span {
  min-width: 0;
  padding-left: 18px;
  border-left: 1px solid rgba(255, 255, 255, .28);
  color: rgba(255, 255, 255, .84);
  font-size: 13px;
  font-weight: 650;
}

.yingzo-section {
  padding: 112px max(24px, calc((100% - 1240px) / 2));
}

.yingzo-section-intro {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(320px, .8fr);
  gap: 100px;
  align-items: end;
}

.yingzo-section h2,
.yingzo-production-copy h2,
.yingzo-model-heading h2,
.yingzo-install-copy h2 {
  margin: 0;
  font-size: 50px;
  line-height: 1.14;
  font-weight: 780;
  letter-spacing: 0;
}

.yingzo-section-intro p,
.yingzo-production-copy > p,
.yingzo-model-heading p,
.yingzo-install-copy p,
.yingzo-assurance-statement p {
  margin: 0;
  color: var(--yz-muted);
  font-size: 16px;
  line-height: 1.85;
}

.yingzo-advantage-list {
  margin: 76px 0 0;
  padding: 0;
  border-top: 1px solid var(--yz-ink);
  list-style: none;
}

.yingzo-advantage-list li {
  display: grid;
  grid-template-columns: 90px minmax(280px, .8fr) minmax(320px, 1.2fr);
  gap: 36px;
  align-items: center;
  min-height: 124px;
  border-bottom: 1px solid var(--yz-line);
}

.yingzo-advantage-list li > span {
  color: var(--yz-red);
  font-size: 13px;
  font-weight: 760;
}

.yingzo-advantage-list h3 {
  margin: 0;
  font-size: 22px;
  line-height: 1.4;
}

.yingzo-advantage-list p {
  margin: 0;
  color: var(--yz-muted);
  font-size: 15px;
  line-height: 1.7;
}

.yingzo-production-band {
  display: grid;
  grid-template-columns: minmax(0, 1.08fr) minmax(420px, .92fr);
  min-height: 760px;
  background: var(--yz-ink);
  color: #ffffff;
}

.yingzo-production-band figure {
  min-width: 0;
  margin: 0;
  overflow: hidden;
}

.yingzo-production-band figure img {
  width: 100%;
  height: 100%;
  display: block;
  object-fit: cover;
  object-position: center 42%;
  filter: contrast(1.08);
}

.yingzo-production-copy {
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 80px max(48px, calc((100vw - 1240px) / 2));
}

.yingzo-production-copy > p {
  max-width: 520px;
  margin-top: 30px;
  color: #bcbcbc;
}

.yingzo-production-copy dl {
  margin: 54px 0 0;
  border-top: 1px solid #4a4a4a;
}

.yingzo-production-copy dl > div {
  display: grid;
  grid-template-columns: 110px 1fr;
  gap: 24px;
  padding: 20px 0;
  border-bottom: 1px solid #3f3f3f;
}

.yingzo-production-copy dt {
  color: #ff8f7d;
  font-size: 13px;
  font-weight: 760;
}

.yingzo-production-copy dd {
  margin: 0;
  color: #dddddd;
  font-size: 14px;
  line-height: 1.6;
}

.yingzo-section-intro-wide {
  grid-template-columns: 1fr auto;
}

.yingzo-workflow {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  margin: 76px 0 0;
  padding: 0;
  border-top: 1px solid var(--yz-ink);
  list-style: none;
}

.yingzo-workflow li {
  position: relative;
  min-width: 0;
  min-height: 250px;
  padding: 26px 22px 30px 0;
  border-right: 1px solid var(--yz-line);
}

.yingzo-workflow li:not(:first-child) {
  padding-left: 22px;
}

.yingzo-workflow li:last-child {
  border-right: 0;
}

.yingzo-workflow li > span {
  color: var(--yz-red);
  font-size: 12px;
  font-weight: 760;
}

.yingzo-workflow li > div {
  position: absolute;
  right: 22px;
  bottom: 30px;
  left: 0;
}

.yingzo-workflow li:not(:first-child) > div {
  left: 22px;
}

.yingzo-workflow h3 {
  margin: 0 0 12px;
  font-size: 19px;
}

.yingzo-workflow p {
  margin: 0;
  color: var(--yz-muted);
  font-size: 13px;
  line-height: 1.7;
}

.yingzo-model-band {
  padding: 108px max(24px, calc((100% - 1240px) / 2));
  background: var(--yz-red);
  color: #ffffff;
}

.yingzo-model-heading {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 70px;
  align-items: end;
}

.yingzo-model-heading p {
  color: rgba(255, 255, 255, .82);
}

.yingzo-model-list {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-top: 66px;
  border-top: 1px solid rgba(255, 255, 255, .72);
}

.yingzo-model-list article {
  min-width: 0;
  min-height: 250px;
  padding: 26px 26px 32px 0;
  border-right: 1px solid rgba(255, 255, 255, .32);
}

.yingzo-model-list article:not(:first-child) {
  padding-left: 26px;
}

.yingzo-model-list article:last-child {
  border-right: 0;
}

.yingzo-model-list span {
  font-size: 13px;
  font-weight: 760;
}

.yingzo-model-list h3 {
  margin: 88px 0 12px;
  font-size: 22px;
}

.yingzo-model-list p {
  margin: 0;
  color: rgba(255, 255, 255, .82);
  font-size: 14px;
  line-height: 1.7;
}

.yingzo-assurance-section {
  display: grid;
  grid-template-columns: minmax(0, .9fr) minmax(460px, 1.1fr);
  gap: 110px;
  align-items: start;
}

.yingzo-assurance-statement p {
  max-width: 500px;
  margin-top: 26px;
}

.yingzo-assurance-lines {
  border-top: 1px solid var(--yz-ink);
}

.yingzo-assurance-lines > div {
  display: grid;
  grid-template-columns: 110px 1fr;
  gap: 26px;
  padding: 24px 0;
  border-bottom: 1px solid var(--yz-line);
}

.yingzo-assurance-lines strong {
  color: var(--yz-red);
  font-size: 14px;
}

.yingzo-assurance-lines span {
  color: var(--yz-muted);
  font-size: 14px;
  line-height: 1.7;
}

.yingzo-install-section {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(430px, .8fr);
  gap: 100px;
  padding: 106px max(24px, calc((100% - 1240px) / 2));
  background: var(--yz-ink);
  color: #ffffff;
}

.yingzo-install-copy p {
  max-width: 620px;
  margin-top: 24px;
  color: #bcbcbc;
}

.yingzo-install-controls {
  align-self: center;
}

.yingzo-release-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 18px;
  color: #bdbdbd;
  font-size: 12px;
}

.yingzo-platform-controls {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 22px;
}

.yingzo-platform-controls label {
  display: grid;
  gap: 7px;
  color: #bdbdbd;
  font-size: 13px;
  font-weight: 650;
}

.yingzo-platform-controls select {
  width: 100%;
  min-height: 42px;
  padding: 0 12px;
  border: 1px solid #565656;
  border-radius: 4px;
  background: #1c1c1c;
  color: #ffffff;
  font: inherit;
}

.yingzo-platform-controls .yingzo-channel-toggle {
  grid-column: 1 / -1;
  display: flex;
  grid-template-columns: none;
  align-items: center;
  gap: 9px;
  min-height: 40px;
  cursor: pointer;
}

.yingzo-channel-toggle input {
  width: 17px;
  height: 17px;
  accent-color: var(--yz-red);
}

.yingzo-host-actions {
  width: 100%;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 28px;
}

.yingzo-copy-button {
  width: 100%;
  min-height: 48px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-top: 28px;
  padding: 0 18px;
  border: 0;
  border-radius: 4px;
  background: var(--yz-red);
  color: #ffffff;
  font-family: inherit;
  font-size: 14px;
  font-weight: 740;
  text-decoration: none;
  cursor: pointer;
}

.yingzo-host-actions .yingzo-copy-button {
  min-width: 0;
  margin-top: 0;
}

.yingzo-copy-button:hover {
  background: var(--yz-red-dark);
}

.yingzo-copy-button:disabled {
  cursor: not-allowed;
  opacity: .5;
}

.yingzo-error {
  margin: 12px 0 0;
  color: #ff9688;
  font-size: 13px;
}

.yingzo-runtime-fallback {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 16px;
  margin-top: 14px;
  font-size: 13px;
}

.yingzo-runtime-link {
  color: #ffb1a8;
  font-weight: 650;
  text-decoration: underline;
  text-underline-offset: 3px;
}

.yingzo-footer {
  width: min(1240px, calc(100% - 48px));
  min-height: 116px;
  display: grid;
  grid-template-columns: 1fr auto auto;
  gap: 30px;
  align-items: center;
  margin: 0 auto;
}

.yingzo-footer p {
  margin: 0;
  color: #777777;
  font-size: 12px;
}

.yingzo-credits a {
  color: #666666;
}

@media (max-width: 980px) {
  .yingzo-header nav { display: none; }
  .yingzo-hero h1 { font-size: 62px; }
  .yingzo-hero-claim { font-size: 27px; }
  .yingzo-section-intro,
  .yingzo-model-heading,
  .yingzo-assurance-section,
  .yingzo-install-section { grid-template-columns: 1fr; gap: 44px; }
  .yingzo-advantage-list li { grid-template-columns: 70px minmax(240px, .8fr) minmax(260px, 1.2fr); gap: 20px; }
  .yingzo-production-band { grid-template-columns: 1fr; }
  .yingzo-production-band figure { height: 620px; }
  .yingzo-production-copy { padding: 80px 48px; }
  .yingzo-workflow { grid-template-columns: repeat(3, 1fr); }
  .yingzo-workflow li { border-bottom: 1px solid var(--yz-line); }
  .yingzo-workflow li:nth-child(3) { border-right: 0; }
  .yingzo-model-list { grid-template-columns: 1fr; }
  .yingzo-model-list article,
  .yingzo-model-list article:not(:first-child) { min-height: 0; padding: 28px 0; border-right: 0; border-bottom: 1px solid rgba(255, 255, 255, .35); }
  .yingzo-model-list article:last-child { border-bottom: 0; }
  .yingzo-model-list h3 { margin-top: 40px; }
}

@media (max-width: 640px) {
  .yingzo-header,
  .yingzo-footer { width: calc(100% - 32px); }
  .yingzo-header { height: 62px; }
  .yingzo-brand span { font-size: 13px; }
  .yingzo-header-action { min-height: 34px; padding: 0 10px; font-size: 12px; }
  .yingzo-hero { height: clamp(560px, calc(100svh - 130px), 720px); background-position: 56% center; }
  .yingzo-hero { background-image: linear-gradient(90deg, rgba(0, 0, 0, .88), rgba(0, 0, 0, .52)), url('/yingzo/production-set.jpg'); }
  .yingzo-hero-copy { width: calc(100% - 32px); padding-top: 106px; }
  .yingzo-hero h1 { font-size: 46px; overflow-wrap: anywhere; }
  .yingzo-hero-claim { margin-top: 24px; font-size: 24px; }
  .yingzo-hero-summary { font-size: 15px; }
  .yingzo-hero-actions { max-width: 220px; flex-direction: column; }
  .yingzo-hero-rail { min-height: 54px; grid-template-columns: repeat(2, 1fr); padding: 8px 16px; }
  .yingzo-hero-rail span { display: none; }
  .yingzo-hero-rail span:nth-child(1),
  .yingzo-hero-rail span:nth-child(5) { display: block; }
  .yingzo-section { padding: 78px 16px; }
  .yingzo-section h2,
  .yingzo-production-copy h2,
  .yingzo-model-heading h2,
  .yingzo-install-copy h2 { font-size: 36px; }
  .yingzo-section-intro { display: block; }
  .yingzo-section-intro p { margin-top: 26px; }
  .yingzo-advantage-list { margin-top: 52px; }
  .yingzo-advantage-list li { grid-template-columns: 42px 1fr; gap: 10px 16px; min-height: 0; padding: 24px 0; }
  .yingzo-advantage-list li p { grid-column: 2; }
  .yingzo-advantage-list h3 { font-size: 18px; }
  .yingzo-production-band figure { height: 500px; }
  .yingzo-production-copy { padding: 68px 16px 74px; }
  .yingzo-production-copy dl > div { grid-template-columns: 88px 1fr; gap: 14px; }
  .yingzo-workflow { grid-template-columns: 1fr; margin-top: 52px; }
  .yingzo-workflow li,
  .yingzo-workflow li:not(:first-child) { min-height: 150px; padding: 20px 0; border-right: 0; }
  .yingzo-workflow li > div,
  .yingzo-workflow li:not(:first-child) > div { right: 0; bottom: 20px; left: 0; }
  .yingzo-model-band { padding: 78px 16px; }
  .yingzo-model-list { margin-top: 48px; }
  .yingzo-assurance-lines > div { grid-template-columns: 88px 1fr; gap: 16px; }
  .yingzo-install-section { padding: 78px 16px; }
  .yingzo-footer { min-height: 150px; grid-template-columns: 1fr; gap: 10px; padding: 30px 0; }
}

@media (prefers-reduced-motion: reduce) {
  .yingzo-button { transition: none; }
}
</style>
