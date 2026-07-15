<template>
  <AppLayout>
    <div class="mx-auto flex max-w-7xl flex-col gap-6">
      <section class="card overflow-hidden">
        <div class="border-b border-gray-100 p-6 dark:border-dark-700 md:p-8">
          <div class="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
            <div class="max-w-3xl">
              <div class="mb-3 inline-flex items-center gap-2 rounded-full bg-primary-50 px-3 py-1 text-xs font-semibold text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">
                <Icon name="book" size="sm" />
                {{ content.badge }}
              </div>
              <h1 class="text-3xl font-bold tracking-normal text-gray-950 dark:text-white md:text-4xl">
                {{ content.title }}
              </h1>
              <p class="mt-3 text-base leading-7 text-gray-600 dark:text-dark-300">
                {{ content.subtitle }}
              </p>
            </div>
            <div class="flex flex-wrap gap-3">
              <button type="button" class="btn btn-primary" @click="copyMarkdown">
                <Icon name="copy" size="sm" :stroke-width="2" />
                {{ content.copyMarkdown }}
              </button>
              <button type="button" class="btn btn-secondary" @click="scrollToSection('quick-start')">
                <Icon name="terminal" size="sm" />
                {{ content.quickStartButton }}
              </button>
            </div>
          </div>
        </div>

        <div class="grid gap-0 border-b border-gray-100 dark:border-dark-700 md:grid-cols-4">
          <div
            v-for="item in content.highlights"
            :key="item.label"
            class="border-b border-gray-100 p-5 last:border-b-0 dark:border-dark-700 md:border-b-0 md:border-r md:last:border-r-0"
          >
            <div class="text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ item.label }}</div>
            <div class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">{{ item.value }}</div>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ item.description }}</p>
          </div>
        </div>
      </section>

      <div class="grid gap-6 lg:grid-cols-[240px_minmax(0,1fr)]">
        <aside class="hidden lg:block">
          <div class="sticky top-24 rounded-2xl border border-gray-100 bg-white/80 p-3 shadow-card backdrop-blur dark:border-dark-700/50 dark:bg-dark-800/70">
            <div class="px-3 pb-2 text-xs font-semibold uppercase text-gray-500 dark:text-dark-400">
              {{ content.tocTitle }}
            </div>
            <button
              v-for="section in content.sections"
              :key="section.id"
              type="button"
              class="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-950 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white"
              @click="scrollToSection(section.id)"
            >
              <span class="h-1.5 w-1.5 flex-shrink-0 rounded-full bg-primary-500"></span>
              <span class="truncate">{{ section.title }}</span>
            </button>
          </div>
        </aside>

        <main class="space-y-6">
          <section
            v-for="section in content.sections"
            :id="section.id"
            :key="section.id"
            class="scroll-mt-24 rounded-2xl border border-gray-100 bg-white shadow-card dark:border-dark-700/50 dark:bg-dark-800/50"
          >
            <div class="border-b border-gray-100 p-6 dark:border-dark-700">
              <div class="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                <div>
                  <p class="text-xs font-semibold uppercase text-primary-600 dark:text-primary-400">{{ section.kicker }}</p>
                  <h2 class="mt-1 text-2xl font-bold text-gray-950 dark:text-white">{{ section.title }}</h2>
                  <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-600 dark:text-dark-300">{{ section.description }}</p>
                </div>
                <span
                  v-if="section.protocol"
                  class="inline-flex flex-shrink-0 items-center rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700 dark:bg-dark-700 dark:text-dark-200"
                >
                  {{ section.protocol }}
                </span>
              </div>
            </div>

            <div class="space-y-6 p-6">
              <div v-if="section.notice" class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm leading-6 text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
                {{ section.notice }}
              </div>

              <div v-if="section.bullets?.length" class="grid gap-3 md:grid-cols-2">
                <div
                  v-for="bullet in section.bullets"
                  :key="bullet.title"
                  class="rounded-xl border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/40"
                >
                  <div class="text-sm font-semibold text-gray-900 dark:text-white">{{ bullet.title }}</div>
                  <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ bullet.text }}</p>
                </div>
              </div>

              <div v-if="section.table" class="overflow-hidden rounded-xl border border-gray-100 dark:border-dark-700">
                <div class="overflow-x-auto">
                  <table class="w-full min-w-[680px] text-left text-sm">
                    <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-900/60 dark:text-dark-400">
                      <tr>
                        <th v-for="head in section.table.headers" :key="head" class="px-4 py-3 font-semibold">
                          {{ head }}
                        </th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                      <tr v-for="(row, rowIndex) in section.table.rows" :key="rowIndex" class="bg-white dark:bg-dark-800/40">
                        <td
                          v-for="(cell, cellIndex) in row"
                          :key="`${rowIndex}-${cellIndex}`"
                          class="px-4 py-3 align-top text-gray-700 dark:text-dark-200"
                        >
                          <code v-if="cellIndex === 0" class="code text-xs">{{ cell }}</code>
                          <span v-else>{{ cell }}</span>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>

              <div v-if="section.tables?.length" class="space-y-4">
                <div
                  v-for="table in section.tables"
                  :key="table.title"
                  class="overflow-hidden rounded-xl border border-gray-100 dark:border-dark-700"
                >
                  <div class="border-b border-gray-100 bg-gray-50 px-4 py-3 text-sm font-semibold text-gray-900 dark:border-dark-700 dark:bg-dark-900/60 dark:text-white">
                    {{ table.title }}
                  </div>
                  <div class="overflow-x-auto">
                    <table class="w-full min-w-[760px] text-left text-sm">
                      <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-900/60 dark:text-dark-400">
                        <tr>
                          <th v-for="head in table.headers" :key="head" class="px-4 py-3 font-semibold">
                            {{ head }}
                          </th>
                        </tr>
                      </thead>
                      <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                        <tr v-for="(row, rowIndex) in table.rows" :key="rowIndex" class="bg-white dark:bg-dark-800/40">
                          <td
                            v-for="(cell, cellIndex) in row"
                            :key="`${table.title}-${rowIndex}-${cellIndex}`"
                            class="px-4 py-3 align-top text-gray-700 dark:text-dark-200"
                          >
                            <code v-if="cellIndex === 0" class="code text-xs">{{ cell }}</code>
                            <span v-else>{{ cell }}</span>
                          </td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>

              <div v-if="section.items?.length" class="space-y-3">
                <div
                  v-for="item in section.items"
                  :key="item.title"
                  class="flex gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-900/40"
                >
                  <Icon name="checkCircle" size="sm" class="mt-0.5 flex-shrink-0 text-primary-600 dark:text-primary-400" :stroke-width="2" />
                  <div>
                    <div class="text-sm font-semibold text-gray-900 dark:text-white">{{ item.title }}</div>
                    <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ item.text }}</p>
                  </div>
                </div>
              </div>

              <div v-if="section.codeBlocks?.length" class="space-y-4">
                <div
                  v-for="(block, blockIndex) in section.codeBlocks"
                  :key="`${section.id}-${blockIndex}`"
                  class="overflow-hidden rounded-xl border border-gray-100 bg-dark-950 shadow-card dark:border-dark-700"
                >
                  <div class="flex items-center justify-between border-b border-white/10 bg-dark-900 px-4 py-3">
                    <div class="min-w-0">
                      <div class="truncate text-sm font-semibold text-white">{{ block.title }}</div>
                      <div class="text-xs uppercase text-dark-300">{{ block.language }}</div>
                    </div>
                    <button
                      type="button"
                      class="inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium text-dark-200 transition-colors hover:bg-white/10 hover:text-white"
                      @click="copyCode(block.code)"
                    >
                      <Icon name="copy" size="xs" />
                      {{ content.copyCode }}
                    </button>
                  </div>
                  <pre class="max-h-[560px] overflow-auto p-4 text-sm leading-6 text-dark-100"><code>{{ block.code }}</code></pre>
                </div>
              </div>
            </div>
          </section>
        </main>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'

type CodeBlock = {
  title: string
  language: string
  code: string
}

type InfoItem = {
  title: string
  text: string
}

type DocsTable = {
  headers: string[]
  rows: string[][]
}

type DocsSection = {
  id: string
  kicker: string
  title: string
  description: string
  protocol?: string
  notice?: string
  bullets?: InfoItem[]
  items?: InfoItem[]
  table?: DocsTable
  tables?: Array<DocsTable & { title: string }>
  codeBlocks?: CodeBlock[]
}

type DocsContent = {
  badge: string
  title: string
  subtitle: string
  copyMarkdown: string
  copyCode: string
  quickStartButton: string
  copySuccess: string
  tocTitle: string
  highlights: Array<{ label: string; value: string; description: string }>
  sections: DocsSection[]
}

type DocsRuntimeValues = {
  siteName: string
  baseUrl: string
  apiKey: string
}

const { locale } = useI18n()
const { copyToClipboard } = useClipboard()
const appStore = useAppStore()

const baseUrl = '{{BASE_URL}}'
const apiKey = '{{API_KEY}}'
const exampleApiKey = 'sk-your-api-key'

const zhContent: DocsContent = {
  badge: '面向下游开发者',
  title: '{{SITE_NAME}} 接口文档',
  subtitle:
    '本文档覆盖当前项目已接入的 OpenAI、Claude、Gemini 与 Seedance 视频接口。示例均使用本站 API Key，请先在控制台创建密钥并绑定对应平台分组。',
  copyMarkdown: '复制 Markdown',
  copyCode: '复制',
  quickStartButton: '快速开始',
  copySuccess: 'Markdown 文档已复制',
  tocTitle: '文档目录',
  highlights: [
    { label: '基础地址', value: baseUrl, description: '请替换为你自己的部署域名或反向代理地址。' },
    { label: '认证方式', value: 'Bearer / x-api-key / x-goog-api-key', description: '所有接口都使用本站 API Key 鉴权。' },
    { label: '计费归属', value: '按密钥绑定分组计费', description: '不同平台接口必须使用对应平台分组。' },
    { label: '复制格式', value: 'Markdown + 标准示例代码', description: '可直接提交给 AI 进行二次开发。' },
  ],
  sections: [
    {
      id: 'quick-start',
      kicker: 'Quick Start',
      title: '快速开始与通用规则',
      protocol: 'All APIs',
      description:
        '所有请求都经过本站用户、API Key、分组、额度、计费和上游账号调度。请求体尽量保持对应生态的标准格式，方便 SDK 或 AI 编程工具直接接入。',
      bullets: [
        {
          title: '不要把 API Key 放在 URL 查询参数',
          text: '本站会拒绝 key 或 api_key 查询参数。请使用 Authorization: Bearer、x-api-key 或 Gemini 生态常用的 x-goog-api-key。',
        },
        {
          title: '平台分组必须匹配',
          text: 'OpenAI 接口使用 openai 分组，Claude 标准接口使用 anthropic 分组，Gemini 标准接口使用 gemini 分组，Seedance 使用 video 分组。',
        },
        {
          title: '错误格式跟随接口生态',
          text: 'OpenAI 和视频接口返回 OpenAI 风格 error；Claude 返回 Anthropic 风格 error；Gemini 返回 Google 风格 error。',
        },
        {
          title: '用量与计费记录',
          text: '文本、图片和视频请求都会进入本站使用记录。异步视频任务创建、完成、失败与退费记录由后台任务处理。',
        },
      ],
      table: {
        headers: ['接口', '平台分组', '说明'],
        rows: [
          ['/v1/responses', 'openai', 'OpenAI Responses API，推荐用于 gpt-5.5 文本与多模态输入。'],
          ['/v1/images/generations', 'openai', 'OpenAI 标准图片生成接口，模型 gpt-image-2。'],
          ['/v1/images/edits', 'openai', 'OpenAI 标准图片编辑接口，multipart/form-data，不包含 mask。'],
          ['/v1/messages', 'anthropic', 'Claude 标准 Messages API。'],
          ['/v1beta/models/{model}:generateContent', 'gemini', 'Gemini 标准文本、文生图、图生图接口。'],
          ['/v1/videos', 'video', 'Seedance 2.0 异步视频任务创建与查询。'],
        ],
      },
      codeBlocks: [
        {
          title: '环境变量',
          language: 'bash',
          code: String.raw`export SUB2API_BASE_URL="${baseUrl}"
export SUB2API_API_KEY="${apiKey}"`,
        },
      ],
    },
    {
      id: 'openai-responses',
      kicker: 'OpenAI',
      title: 'gpt-5.5 Responses 接口',
      protocol: 'POST /v1/responses',
      description:
        'Responses API 使用 OpenAI 标准请求结构，适合文本、工具调用、多轮上下文与多模态输入。当前文档示例使用模型 gpt-5.5。',
      items: [
        { title: '模型', text: 'model 固定填写 gpt-5.5，具体可用性取决于管理员配置的 openai 分组和上游账号。' },
        { title: '输入', text: 'input 可以是字符串，也可以是 OpenAI Responses 标准消息数组。' },
        { title: '流式', text: '设置 stream: true 可使用 SSE 流式响应；非流式返回完整 JSON。' },
      ],
      codeBlocks: [
        {
          title: 'cURL 非流式请求',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.5",
    "input": "用三句话解释 {{SITE_NAME}} 的用途。",
    "stream": false
  }'`,
        },
        {
          title: 'JavaScript fetch',
          language: 'javascript',
          code: String.raw`const response = await fetch("{{BASE_URL}}/v1/responses", {
  method: "POST",
  headers: {
    "Authorization": "Bearer {{API_KEY}}",
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    model: "gpt-5.5",
    input: [
      {
        role: "user",
        content: [
          { type: "input_text", text: "写一个 TypeScript 接口设计建议。" }
        ]
      }
    ],
    stream: false
  })
});

const data = await response.json();
console.log(data.output_text ?? data);`,
        },
      ],
    },
    {
      id: 'openai-images',
      kicker: 'OpenAI Images',
      title: 'gpt-image-2 图片生成与编辑',
      protocol: '/v1/images/*',
      description:
        '图片接口对齐 OpenAI 标准图片生成与图片编辑。图片编辑使用 multipart/form-data，当前项目文档只包含无 mask 的编辑方式。',
      notice: '图片编辑接口不要上传 mask 字段；如果业务需要局部蒙版，请后续单独扩展。当前文档示例仅描述无 mask 的标准编辑。',
      bullets: [
        { title: '文生图', text: 'POST /v1/images/generations，JSON 请求体，model 使用 gpt-image-2。' },
        { title: '图生图 / 编辑', text: 'POST /v1/images/edits，multipart/form-data，至少包含 image、prompt、model。' },
      ],
      codeBlocks: [
        {
          title: '图片生成 cURL',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "A clean product hero image of a transparent smart water bottle on a light gray studio background",
    "size": "1024x1024",
    "n": 1
  }'`,
        },
        {
          title: '图片编辑 cURL（无 mask）',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -F "model=gpt-image-2" \
  -F "image=@./input.png" \
  -F "prompt=保持主体构图不变，把背景替换为高级摄影棚环境。" \
  -F "size=1024x1024"`,
        },
        {
          title: 'JavaScript 图片编辑（无 mask）',
          language: 'javascript',
          code: String.raw`const form = new FormData();
form.append("model", "gpt-image-2");
form.append("prompt", "Keep the product unchanged and replace the background with a premium studio set.");
form.append("size", "1024x1024");
form.append("image", fileInput.files[0]);

const response = await fetch("{{BASE_URL}}/v1/images/edits", {
  method: "POST",
  headers: {
    "Authorization": "Bearer {{API_KEY}}"
  },
  body: form
});

const data = await response.json();
console.log(data.data?.[0]?.url ?? data);`,
        },
      ],
    },
    {
      id: 'claude',
      kicker: 'Claude',
      title: 'Claude 标准 Messages 接口',
      protocol: 'POST /v1/messages',
      description:
        'Claude 接口保持 Anthropic Messages API 结构。请使用绑定 anthropic 分组的本站 API Key。模型名按管理员开放的 Claude 模型填写。',
      bullets: [
        { title: '请求结构', text: '使用 model、max_tokens、system、messages 等 Claude 标准字段。' },
        { title: '流式输出', text: '设置 stream: true 时返回 Anthropic 风格事件流。' },
        { title: '模型示例', text: '示例使用 claude-sonnet-4-6；也可根据分组可用模型改为 claude-opus-4-8 等。' },
      ],
      codeBlocks: [
        {
          title: 'cURL',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/messages" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 1024,
    "system": "你是一个严谨的软件架构师。",
    "messages": [
      {
        "role": "user",
        "content": "请给一个视频生成 API 的任务表设计建议。"
      }
    ]
  }'`,
        },
        {
          title: 'JavaScript fetch',
          language: 'javascript',
          code: String.raw`const response = await fetch("{{BASE_URL}}/v1/messages", {
  method: "POST",
  headers: {
    "Authorization": "Bearer {{API_KEY}}",
    "Content-Type": "application/json",
    "anthropic-version": "2023-06-01"
  },
  body: JSON.stringify({
    model: "claude-sonnet-4-6",
    max_tokens: 1024,
    messages: [
      {
        role: "user",
        content: "把这段接口需求拆成开发任务。"
      }
    ]
  })
});

const data = await response.json();
console.log(data.content?.[0]?.text ?? data);`,
        },
      ],
    },
    {
      id: 'gemini',
      kicker: 'Gemini',
      title: 'Gemini 标准文本与图片接口',
      protocol: 'POST /v1beta/models/{model}:generateContent',
      description:
        'Gemini 接口保持 Google Generative Language API v1beta 路径和请求结构。推荐使用 x-goog-api-key 传入本站 API Key。',
      bullets: [
        { title: '文本模型', text: 'gemini-3.1-pro-preview，适合文本、多模态理解和复杂推理。' },
        { title: '图片模型', text: 'gemini-3.1-flash-image-preview 与 gemini-3-pro-image-preview，支持文生图和图生图。' },
        { title: '认证', text: 'Gemini 生态推荐 x-goog-api-key；本站也兼容 Authorization: Bearer 和 x-api-key。' },
        { title: '图片输出', text: '返回内容中通常读取 candidates[].content.parts[].inlineData / inline_data。' },
      ],
      codeBlocks: [
        {
          title: '文本生成 cURL',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1beta/models/gemini-3.1-pro-preview:generateContent" \
  -H "x-goog-api-key: $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [
          { "text": "请用表格比较 Responses、Messages 和 Gemini generateContent 的差异。" }
        ]
      }
    ]
  }'`,
        },
        {
          title: '文生图 cURL',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1beta/models/gemini-3.1-flash-image-preview:generateContent" \
  -H "x-goog-api-key: $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [
          { "text": "Create a refined product photo of a matte black wireless speaker on a white acrylic table." }
        ]
      }
    ],
    "generationConfig": {
      "responseModalities": ["TEXT", "IMAGE"]
    }
  }'`,
        },
        {
          title: '图生图 JavaScript',
          language: 'javascript',
          code: String.raw`const imageBase64 = await fileToBase64(inputFile);

const response = await fetch(
  "{{BASE_URL}}/v1beta/models/gemini-3-pro-image-preview:generateContent",
  {
    method: "POST",
    headers: {
      "x-goog-api-key": "{{API_KEY}}",
      "Content-Type": "application/json"
    },
    body: JSON.stringify({
      contents: [
        {
          role: "user",
          parts: [
            { text: "Use the reference image as the product identity. Create a new premium studio scene." },
            {
              inline_data: {
                mime_type: "image/png",
                data: imageBase64
              }
            }
          ]
        }
      ],
      generationConfig: {
        responseModalities: ["TEXT", "IMAGE"]
      }
    })
  }
);

const data = await response.json();
const imagePart = data.candidates?.[0]?.content?.parts?.find(
  (part) => part.inlineData || part.inline_data
);
console.log(imagePart?.inlineData?.data ?? imagePart?.inline_data?.data);

async function fileToBase64(file) {
  const buffer = await file.arrayBuffer();
  let binary = "";
  for (const byte of new Uint8Array(buffer)) binary += String.fromCharCode(byte);
  return btoa(binary);
}`,
        },
      ],
    },
    {
      id: 'seedance',
      kicker: 'Seedance',
      title: 'Seedance 2.0 异步视频接口',
      protocol: 'POST /v1/videos · GET /v1/videos/{id}',
      description:
        'Seedance 视频接口是 OpenAI 风格异步任务协议。POST 创建本地任务并返回任务 ID；GET 查询任务状态。下游只会看到本站任务 ID、状态、视频地址和本站加工后的错误。',
      notice:
        'Seedance 参考图片和参考视频必须传 subject_type: "person"，否则无法通过真人认证。参考视频还必须提供 duration_seconds 用于本地计费。',
      bullets: [
        { title: '模型', text: 'seedance-2.0 支持 480p、720p、1080p、4K；seedance-2.0-fast 仅支持 480p、720p。' },
        { title: '生成时长', text: 'duration 必须是整数秒，Seedance 2.0 系列支持 4-15 秒；当前接口不支持 -1 智能时长。' },
        { title: '计费秒数', text: 'billableSeconds = ceil(duration) + sum(ceil(reference video duration_seconds))。失败任务会退费。' },
        { title: '状态', text: 'queued、processing、completed、failed、cancelled。completed 时返回 video_url。' },
        { title: '参考模式限制', text: '最多 9 张参考图、3 个参考视频、3 个参考音频；参考音频不能单独构成参考，至少需要 1 个图片或视频参考。' },
      ],
      table: {
        headers: ['ability_code', '输入要求', 'content role'],
        rows: [
          ['video_text_to_video', '必须有 prompt，不允许图片/视频/音频参考。', 'text'],
          ['video_image_to_video', '必须正好 1 张首帧图，不允许视频/音频参考。', 'first_frame'],
          ['video_start_end_to_video', '必须正好 2 张图片，分别是首帧和尾帧。', 'first_frame / last_frame'],
          ['video_reference_to_video', '至少 1 个参考图片或参考视频，可混合图片、视频、音频。', 'reference_image / reference_video / reference_audio'],
        ],
      },
      tables: [
        {
          title: 'Seedance 2.0 官方素材与生成限制',
          headers: ['项目', '限制', '说明'],
          rows: [
            ['生成视频时长', '4-15 秒，必须为整数秒', '当前接口要求显式传 duration，不支持 -1 智能时长。'],
            ['输出分辨率', 'seedance-2.0: 480p / 720p / 1080p / 4K；seedance-2.0-fast: 480p / 720p', '1080p 和 4K 只适用于 seedance-2.0，fast 不支持。'],
            ['输出比例', '16:9 / 4:3 / 1:1 / 3:4 / 9:16 / 21:9 / adaptive', '当前接口兼容 ratio、aspect_ratio、aspectRatio。'],
            ['参考图片数量', '首帧模式 1 张；首尾帧模式 2 张；参考模式 1-9 张', '参考模式图片 role 必须为 reference_image；首尾帧模式与参考模式不能混用。'],
            ['参考图片格式', 'jpeg / png / webp / bmp / tiff / gif / heic / heif', 'URL 或 data:image/<format>;base64,<base64>；大文件建议使用可公开访问 URL。'],
            ['参考图片尺寸', '宽高比 0.4-2.5；宽和高均为 300-6000 px；单图小于 30 MB', '请求体总大小不超过 64 MB。'],
            ['参考视频数量与时长', '最多 3 个；单个 2-15 秒；所有参考视频总时长不超过 15 秒', 'reference_video 必须传 duration_seconds，用于本站预校验和计费。'],
            ['参考视频格式', 'mp4 / mov；视频编码 H.264/AVC 或 H.265/HEVC；音频编码 AAC 或 MP3', '仅支持视频 URL；单个视频不超过 200 MB。'],
            ['参考视频尺寸', '480p / 720p / 1080p / 4k；宽高比 0.4-2.5；宽和高均为 300-6000 px', '总像素需在 409600-8295044 之间，帧率 24-60 FPS。'],
            ['参考音频数量与时长', '最多 3 段；单段 2-15 秒；所有参考音频总时长不超过 15 秒', '参考音频不能单独提交，至少还需要 1 个参考图片或参考视频。'],
            ['参考音频格式', 'wav / mp3；单文件不超过 15 MB', '支持 URL 或 data:audio/<format>;base64,<base64>；请求体总大小不超过 64 MB。'],
            ['真人认证字段', '参考图片和参考视频必须传 subject_type: "person"', '缺少该字段时真人认证可能无法通过。'],
          ],
        },
      ],
      codeBlocks: [
        {
          title: '文生视频：创建任务',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/videos" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0",
    "prompt": "A cinematic shot of a glass greenhouse at sunrise, soft camera push-in, gentle wind.",
    "content": [
      {
        "type": "text",
        "text": "A cinematic shot of a glass greenhouse at sunrise, soft camera push-in, gentle wind."
      }
    ],
    "ratio": "16:9",
    "duration": 8,
    "resolution": "720p",
    "generate_audio": true,
    "ability_code": "video_text_to_video",
    "safety_identifier": "project-or-user-id"
  }'`,
        },
        {
          title: '首帧图生视频：创建任务',
          language: 'json',
          code: String.raw`{
  "model": "seedance-2.0",
  "prompt": "Use the first frame as the opening shot, then animate the character walking through a neon street.",
  "content": [
    {
      "type": "text",
      "text": "Use the first frame as the opening shot, then animate the character walking through a neon street."
    },
    {
      "type": "image_url",
      "image_url": { "url": "https://cdn.example.com/first-frame.png" },
      "role": "first_frame",
      "subject_type": "person"
    }
  ],
  "ratio": "16:9",
  "duration": 8,
  "resolution": "720p",
  "generate_audio": true,
  "ability_code": "video_image_to_video"
}`,
        },
        {
          title: '首尾帧视频：创建任务',
          language: 'json',
          code: String.raw`{
  "model": "seedance-2.0-fast",
  "prompt": "Transform the first frame into the last frame with a smooth cinematic camera move.",
  "content": [
    {
      "type": "text",
      "text": "Transform the first frame into the last frame with a smooth cinematic camera move."
    },
    {
      "type": "image_url",
      "image_url": { "url": "https://cdn.example.com/first-frame.png" },
      "role": "first_frame",
      "subject_type": "person"
    },
    {
      "type": "image_url",
      "image_url": { "url": "https://cdn.example.com/last-frame.png" },
      "role": "last_frame",
      "subject_type": "person"
    }
  ],
  "ratio": "16:9",
  "duration": 8,
  "resolution": "720p",
  "generate_audio": true,
  "ability_code": "video_start_end_to_video"
}`,
        },
        {
          title: '参考生视频：图片 + 视频参考',
          language: 'json',
          code: String.raw`{
  "model": "seedance-2.0",
  "prompt": "Keep the character identity from the references. Create a luxury perfume commercial with slow motion fabric movement.",
  "content": [
    {
      "type": "text",
      "text": "Keep the character identity from the references. Create a luxury perfume commercial with slow motion fabric movement."
    },
    {
      "type": "image_url",
      "image_url": { "url": "https://cdn.example.com/person-reference.png" },
      "role": "reference_image",
      "subject_type": "person"
    },
    {
      "type": "video_url",
      "video_url": { "url": "https://cdn.example.com/motion-reference.mp4" },
      "role": "reference_video",
      "subject_type": "person",
      "duration_seconds": 6
    },
    {
      "type": "audio_url",
      "audio_url": { "url": "https://cdn.example.com/music-reference.mp3" },
      "role": "reference_audio"
    }
  ],
  "ratio": "9:16",
  "duration": 8,
  "resolution": "720p",
  "generate_audio": true,
  "ability_code": "video_reference_to_video",
  "safety_identifier": "project-or-user-id"
}`,
        },
        {
          title: '查询视频任务',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/videos/video_xxx" \
  -H "Authorization: Bearer $SUB2API_API_KEY"`,
        },
        {
          title: '创建响应示例',
          language: 'json',
          code: String.raw`{
  "id": "video_xxx",
  "object": "video",
  "model": "seedance-2.0",
  "status": "queued",
  "created_at": 1782700000
}`,
        },
        {
          title: '完成响应示例',
          language: 'json',
          code: String.raw`{
  "id": "video_xxx",
  "object": "video",
  "model": "seedance-2.0",
  "status": "completed",
  "video_url": "https://cdn.example.com/output.mp4",
  "created_at": 1782700000,
  "completed_at": 1782700030
}`,
        },
      ],
    },
    {
      id: 'implementation-notes',
      kicker: 'For AI Coding',
      title: '提交给 AI 编程时的约束说明',
      protocol: 'Implementation Contract',
      description:
        '把本节连同上面的示例一起复制给 AI，可以减少它误用平台、字段或鉴权方式的概率。',
      items: [
        { title: 'OpenAI 图片编辑不带 mask', text: '只实现 image + prompt 的 multipart/form-data；不要生成 mask 字段。' },
        { title: 'Gemini 使用 v1beta 标准路径', text: '文本、文生图和图生图都使用 /v1beta/models/{model}:generateContent。' },
        { title: 'Seedance 是异步任务', text: '不要把 POST /v1/videos 当同步接口；必须保存返回 id，然后轮询 GET /v1/videos/{id}。' },
        { title: 'Seedance 参考视频必须带时长', text: 'reference_video 的 duration_seconds 是必填字段；缺失会 400，且不会提交任务。' },
        { title: 'Persona 类别字段', text: 'Seedance 参考图片和参考视频传 subject_type: "person"，否则无法通过真人认证。' },
      ],
    },
  ],
}

const enContent: DocsContent = {
  badge: 'Downstream developer guide',
  title: '{{SITE_NAME}} API Reference',
  subtitle:
    'This guide covers the OpenAI, Claude, Gemini, and Seedance video APIs exposed by this project. All examples use a {{SITE_NAME}} API key created in the console and assigned to the correct platform group.',
  copyMarkdown: 'Copy Markdown',
  copyCode: 'Copy',
  quickStartButton: 'Quick Start',
  copySuccess: 'Markdown copied',
  tocTitle: 'Contents',
  highlights: [
    { label: 'Base URL', value: baseUrl, description: 'Replace this with your deployed domain or reverse proxy URL.' },
    { label: 'Auth', value: 'Bearer / x-api-key / x-goog-api-key', description: 'Every endpoint authenticates with your {{SITE_NAME}} API key.' },
    { label: 'Billing', value: 'By API key group', description: 'Each endpoint must be called with a key assigned to the matching platform group.' },
    { label: 'Copy Format', value: 'Markdown + code samples', description: 'Ready to paste into an AI coding assistant.' },
  ],
  sections: [
    {
      id: 'quick-start',
      kicker: 'Quick Start',
      title: 'Quick Start And Common Rules',
      protocol: 'All APIs',
      description:
        'Requests are processed through {{SITE_NAME}} user, API key, group, quota, billing, and account scheduling layers. Request bodies follow the native protocol shape as closely as possible.',
      bullets: [
        {
          title: 'Do not put API keys in query parameters',
          text: 'The key and api_key query parameters are rejected. Use Authorization: Bearer, x-api-key, or the Gemini-style x-goog-api-key header.',
        },
        {
          title: 'Use the matching platform group',
          text: 'OpenAI endpoints require an openai group, Claude requires anthropic, Gemini requires gemini, and Seedance requires video.',
        },
        {
          title: 'Error format follows each ecosystem',
          text: 'OpenAI and video endpoints return OpenAI-style errors, Claude returns Anthropic-style errors, and Gemini returns Google-style errors.',
        },
        {
          title: 'Usage and billing records',
          text: 'Text, image, and video requests are recorded by {{SITE_NAME}}. Async video create, completion, failure, and refund records are handled by background workers.',
        },
      ],
      table: {
        headers: ['Endpoint', 'Platform group', 'Description'],
        rows: [
          ['/v1/responses', 'openai', 'OpenAI Responses API, recommended for gpt-5.5 text and multimodal input.'],
          ['/v1/images/generations', 'openai', 'OpenAI-compatible image generation with gpt-image-2.'],
          ['/v1/images/edits', 'openai', 'OpenAI-compatible image editing with multipart/form-data, without mask.'],
          ['/v1/messages', 'anthropic', 'Claude-compatible Messages API.'],
          ['/v1beta/models/{model}:generateContent', 'gemini', 'Gemini-compatible text, text-to-image, and image-to-image API.'],
          ['/v1/videos', 'video', 'Seedance 2.0 async video create and query API.'],
        ],
      },
      codeBlocks: [
        {
          title: 'Environment',
          language: 'bash',
          code: String.raw`export SUB2API_BASE_URL="${baseUrl}"
export SUB2API_API_KEY="${apiKey}"`,
        },
      ],
    },
    {
      id: 'openai-responses',
      kicker: 'OpenAI',
      title: 'gpt-5.5 Responses API',
      protocol: 'POST /v1/responses',
      description:
        'The Responses API uses the standard OpenAI request shape and is suitable for text, tools, continuation, and multimodal input. These examples use gpt-5.5.',
      items: [
        { title: 'Model', text: 'Set model to gpt-5.5. Availability depends on the openai group and upstream accounts configured by the administrator.' },
        { title: 'Input', text: 'input may be a string or a standard OpenAI Responses input array.' },
        { title: 'Streaming', text: 'Set stream: true for SSE streaming; otherwise the endpoint returns one complete JSON object.' },
      ],
      codeBlocks: [
        {
          title: 'cURL non-streaming request',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.5",
    "input": "Explain what {{SITE_NAME}} does in three sentences.",
    "stream": false
  }'`,
        },
        {
          title: 'JavaScript fetch',
          language: 'javascript',
          code: String.raw`const response = await fetch("{{BASE_URL}}/v1/responses", {
  method: "POST",
  headers: {
    "Authorization": "Bearer {{API_KEY}}",
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    model: "gpt-5.5",
    input: [
      {
        role: "user",
        content: [
          { type: "input_text", text: "Write a TypeScript API interface design recommendation." }
        ]
      }
    ],
    stream: false
  })
});

const data = await response.json();
console.log(data.output_text ?? data);`,
        },
      ],
    },
    {
      id: 'openai-images',
      kicker: 'OpenAI Images',
      title: 'gpt-image-2 Image Generation And Editing',
      protocol: '/v1/images/*',
      description:
        'Image endpoints follow the OpenAI-compatible image generation and edit protocols. Image edit uses multipart/form-data and this guide only documents the no-mask mode.',
      notice: 'Do not upload a mask field to the image edit endpoint. If your product needs mask-based local editing, add that as a separate extension later.',
      bullets: [
        { title: 'Text-to-image', text: 'POST /v1/images/generations with a JSON body and model gpt-image-2.' },
        { title: 'Image-to-image / edit', text: 'POST /v1/images/edits with multipart/form-data containing at least image, prompt, and model.' },
      ],
      codeBlocks: [
        {
          title: 'Image generation cURL',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "A clean product hero image of a transparent smart water bottle on a light gray studio background",
    "size": "1024x1024",
    "n": 1
  }'`,
        },
        {
          title: 'Image edit cURL, no mask',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -F "model=gpt-image-2" \
  -F "image=@./input.png" \
  -F "prompt=Keep the main subject and composition unchanged, then replace the background with a premium studio environment." \
  -F "size=1024x1024"`,
        },
        {
          title: 'JavaScript image edit, no mask',
          language: 'javascript',
          code: String.raw`const form = new FormData();
form.append("model", "gpt-image-2");
form.append("prompt", "Keep the product unchanged and replace the background with a premium studio set.");
form.append("size", "1024x1024");
form.append("image", fileInput.files[0]);

const response = await fetch("{{BASE_URL}}/v1/images/edits", {
  method: "POST",
  headers: {
    "Authorization": "Bearer {{API_KEY}}"
  },
  body: form
});

const data = await response.json();
console.log(data.data?.[0]?.url ?? data);`,
        },
      ],
    },
    {
      id: 'claude',
      kicker: 'Claude',
      title: 'Claude-Compatible Messages API',
      protocol: 'POST /v1/messages',
      description:
        'The Claude endpoint keeps the Anthropic Messages API shape. Use a {{SITE_NAME}} key assigned to an anthropic group. Model availability is controlled by administrator configuration.',
      bullets: [
        { title: 'Request shape', text: 'Use standard Claude fields such as model, max_tokens, system, and messages.' },
        { title: 'Streaming', text: 'Set stream: true to receive Anthropic-style event streams.' },
        { title: 'Example model', text: 'The sample uses claude-sonnet-4-6. You can switch to other enabled models such as claude-opus-4-8.' },
      ],
      codeBlocks: [
        {
          title: 'cURL',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/messages" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 1024,
    "system": "You are a careful software architect.",
    "messages": [
      {
        "role": "user",
        "content": "Suggest a task-table design for an async video generation API."
      }
    ]
  }'`,
        },
        {
          title: 'JavaScript fetch',
          language: 'javascript',
          code: String.raw`const response = await fetch("{{BASE_URL}}/v1/messages", {
  method: "POST",
  headers: {
    "Authorization": "Bearer {{API_KEY}}",
    "Content-Type": "application/json",
    "anthropic-version": "2023-06-01"
  },
  body: JSON.stringify({
    model: "claude-sonnet-4-6",
    max_tokens: 1024,
    messages: [
      {
        role: "user",
        content: "Break this API requirement into implementation tasks."
      }
    ]
  })
});

const data = await response.json();
console.log(data.content?.[0]?.text ?? data);`,
        },
      ],
    },
    {
      id: 'gemini',
      kicker: 'Gemini',
      title: 'Gemini-Compatible Text And Image API',
      protocol: 'POST /v1beta/models/{model}:generateContent',
      description:
        'Gemini endpoints keep the Google Generative Language API v1beta path and request shape. Use x-goog-api-key for the {{SITE_NAME}} key.',
      bullets: [
        { title: 'Text model', text: 'gemini-3.1-pro-preview for text, multimodal understanding, and complex reasoning.' },
        { title: 'Image models', text: 'gemini-3.1-flash-image-preview and gemini-3-pro-image-preview for text-to-image and image-to-image.' },
        { title: 'Authentication', text: 'x-goog-api-key is recommended for Gemini clients; Authorization: Bearer and x-api-key also work.' },
        { title: 'Image output', text: 'Read candidates[].content.parts[].inlineData / inline_data for generated images.' },
      ],
      codeBlocks: [
        {
          title: 'Text generation cURL',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1beta/models/gemini-3.1-pro-preview:generateContent" \
  -H "x-goog-api-key: $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [
          { "text": "Compare Responses, Messages, and Gemini generateContent in a concise table." }
        ]
      }
    ]
  }'`,
        },
        {
          title: 'Text-to-image cURL',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1beta/models/gemini-3.1-flash-image-preview:generateContent" \
  -H "x-goog-api-key: $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [
          { "text": "Create a refined product photo of a matte black wireless speaker on a white acrylic table." }
        ]
      }
    ],
    "generationConfig": {
      "responseModalities": ["TEXT", "IMAGE"]
    }
  }'`,
        },
        {
          title: 'Image-to-image JavaScript',
          language: 'javascript',
          code: String.raw`const imageBase64 = await fileToBase64(inputFile);

const response = await fetch(
  "{{BASE_URL}}/v1beta/models/gemini-3-pro-image-preview:generateContent",
  {
    method: "POST",
    headers: {
      "x-goog-api-key": "{{API_KEY}}",
      "Content-Type": "application/json"
    },
    body: JSON.stringify({
      contents: [
        {
          role: "user",
          parts: [
            { text: "Use the reference image as the product identity. Create a new premium studio scene." },
            {
              inline_data: {
                mime_type: "image/png",
                data: imageBase64
              }
            }
          ]
        }
      ],
      generationConfig: {
        responseModalities: ["TEXT", "IMAGE"]
      }
    })
  }
);

const data = await response.json();
const imagePart = data.candidates?.[0]?.content?.parts?.find(
  (part) => part.inlineData || part.inline_data
);
console.log(imagePart?.inlineData?.data ?? imagePart?.inline_data?.data);

async function fileToBase64(file) {
  const buffer = await file.arrayBuffer();
  let binary = "";
  for (const byte of new Uint8Array(buffer)) binary += String.fromCharCode(byte);
  return btoa(binary);
}`,
        },
      ],
    },
    {
      id: 'seedance',
      kicker: 'Seedance',
      title: 'Seedance 2.0 Async Video API',
      protocol: 'POST /v1/videos · GET /v1/videos/{id}',
      description:
        'The Seedance video API is an OpenAI-style async task protocol. POST creates a local task and returns an id; GET polls the task status. Downstream clients only receive {{SITE_NAME}} task ids, status, video URLs, and normalized errors.',
      notice:
        'Seedance reference images and reference videos must send subject_type: "person"; otherwise real-person verification cannot pass. Reference videos must also include duration_seconds for local billing.',
      bullets: [
        { title: 'Models', text: 'seedance-2.0 supports 480p, 720p, 1080p, and 4K; seedance-2.0-fast only supports 480p and 720p.' },
        { title: 'Generated duration', text: 'duration must be an integer number of seconds. Seedance 2.0 series supports 4-15 seconds; this endpoint does not support -1 smart duration.' },
        { title: 'Billable seconds', text: 'billableSeconds = ceil(duration) + sum(ceil(reference video duration_seconds)). Failed tasks are refunded.' },
        { title: 'Statuses', text: 'queued, processing, completed, failed, cancelled. completed responses include video_url.' },
        { title: 'Reference mode limits', text: 'Up to 9 reference images, 3 reference videos, and 3 reference audio files. Reference audio cannot be used alone; at least one image or video reference is required.' },
      ],
      table: {
        headers: ['ability_code', 'Input requirement', 'content role'],
        rows: [
          ['video_text_to_video', 'Requires prompt and no image/video/audio references.', 'text'],
          ['video_image_to_video', 'Requires exactly one first-frame image and no video/audio references.', 'first_frame'],
          ['video_start_end_to_video', 'Requires exactly two images: first frame and last frame.', 'first_frame / last_frame'],
          ['video_reference_to_video', 'Requires at least one reference image or reference video. Image, video, and audio references may be combined.', 'reference_image / reference_video / reference_audio'],
        ],
      },
      tables: [
        {
          title: 'Official Seedance 2.0 Asset And Generation Limits',
          headers: ['Item', 'Limit', 'Notes'],
          rows: [
            ['Generated duration', '4-15 seconds, integer seconds only', 'This endpoint requires explicit duration and does not support -1 smart duration.'],
            ['Output resolution', 'seedance-2.0: 480p / 720p / 1080p / 4K; seedance-2.0-fast: 480p / 720p', '1080p and 4K are only available on seedance-2.0. Fast supports neither.'],
            ['Output ratio', '16:9 / 4:3 / 1:1 / 3:4 / 9:16 / 21:9 / adaptive', 'This endpoint accepts ratio, aspect_ratio, and aspectRatio.'],
            ['Reference image count', 'First-frame mode: 1 image; start-end mode: 2 images; reference mode: 1-9 images', 'Reference-mode image role must be reference_image. Start/end-frame mode and reference mode cannot be mixed.'],
            ['Reference image formats', 'jpeg / png / webp / bmp / tiff / gif / heic / heif', 'Use a URL or data:image/<format>;base64,<base64>. Public URLs are recommended for large files.'],
            ['Reference image dimensions', 'Aspect ratio 0.4-2.5; width and height each 300-6000 px; each image under 30 MB', 'Total request body size must not exceed 64 MB.'],
            ['Reference video count and duration', 'Up to 3 videos; each video 2-15 seconds; total reference video duration up to 15 seconds', 'reference_video must include duration_seconds for local validation and billing.'],
            ['Reference video formats', 'mp4 / mov; video codec H.264/AVC or H.265/HEVC; audio codec AAC or MP3', 'Only video URLs are supported. Each video must not exceed 200 MB.'],
            ['Reference video dimensions', '480p / 720p / 1080p / 4k; aspect ratio 0.4-2.5; width and height each 300-6000 px', 'Total pixels must be 409600-8295044. Frame rate must be 24-60 FPS.'],
            ['Reference audio count and duration', 'Up to 3 audio files; each file 2-15 seconds; total reference audio duration up to 15 seconds', 'Reference audio cannot be submitted alone; at least one reference image or video is required.'],
            ['Reference audio formats', 'wav / mp3; each file under 15 MB', 'Use a URL or data:audio/<format>;base64,<base64>. Total request body size must not exceed 64 MB.'],
            ['Real-person verification field', 'Reference images and reference videos must send subject_type: "person"', 'Missing this field may prevent real-person verification from passing.'],
          ],
        },
      ],
      codeBlocks: [
        {
          title: 'Text-to-video: create task',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/videos" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0",
    "prompt": "A cinematic shot of a glass greenhouse at sunrise, soft camera push-in, gentle wind.",
    "content": [
      {
        "type": "text",
        "text": "A cinematic shot of a glass greenhouse at sunrise, soft camera push-in, gentle wind."
      }
    ],
    "ratio": "16:9",
    "duration": 8,
    "resolution": "720p",
    "generate_audio": true,
    "ability_code": "video_text_to_video",
    "safety_identifier": "project-or-user-id"
  }'`,
        },
        {
          title: 'First-frame image-to-video: create task',
          language: 'json',
          code: String.raw`{
  "model": "seedance-2.0",
  "prompt": "Use the first frame as the opening shot, then animate the character walking through a neon street.",
  "content": [
    {
      "type": "text",
      "text": "Use the first frame as the opening shot, then animate the character walking through a neon street."
    },
    {
      "type": "image_url",
      "image_url": { "url": "https://cdn.example.com/first-frame.png" },
      "role": "first_frame",
      "subject_type": "person"
    }
  ],
  "ratio": "16:9",
  "duration": 8,
  "resolution": "720p",
  "generate_audio": true,
  "ability_code": "video_image_to_video"
}`,
        },
        {
          title: 'Start-end-frame video: create task',
          language: 'json',
          code: String.raw`{
  "model": "seedance-2.0-fast",
  "prompt": "Transform the first frame into the last frame with a smooth cinematic camera move.",
  "content": [
    {
      "type": "text",
      "text": "Transform the first frame into the last frame with a smooth cinematic camera move."
    },
    {
      "type": "image_url",
      "image_url": { "url": "https://cdn.example.com/first-frame.png" },
      "role": "first_frame",
      "subject_type": "person"
    },
    {
      "type": "image_url",
      "image_url": { "url": "https://cdn.example.com/last-frame.png" },
      "role": "last_frame",
      "subject_type": "person"
    }
  ],
  "ratio": "16:9",
  "duration": 8,
  "resolution": "720p",
  "generate_audio": true,
  "ability_code": "video_start_end_to_video"
}`,
        },
        {
          title: 'Reference-to-video: image + video references',
          language: 'json',
          code: String.raw`{
  "model": "seedance-2.0",
  "prompt": "Keep the character identity from the references. Create a luxury perfume commercial with slow motion fabric movement.",
  "content": [
    {
      "type": "text",
      "text": "Keep the character identity from the references. Create a luxury perfume commercial with slow motion fabric movement."
    },
    {
      "type": "image_url",
      "image_url": { "url": "https://cdn.example.com/person-reference.png" },
      "role": "reference_image",
      "subject_type": "person"
    },
    {
      "type": "video_url",
      "video_url": { "url": "https://cdn.example.com/motion-reference.mp4" },
      "role": "reference_video",
      "subject_type": "person",
      "duration_seconds": 6
    },
    {
      "type": "audio_url",
      "audio_url": { "url": "https://cdn.example.com/music-reference.mp3" },
      "role": "reference_audio"
    }
  ],
  "ratio": "9:16",
  "duration": 8,
  "resolution": "720p",
  "generate_audio": true,
  "ability_code": "video_reference_to_video",
  "safety_identifier": "project-or-user-id"
}`,
        },
        {
          title: 'Query video task',
          language: 'bash',
          code: String.raw`curl "$SUB2API_BASE_URL/v1/videos/video_xxx" \
  -H "Authorization: Bearer $SUB2API_API_KEY"`,
        },
        {
          title: 'Create response example',
          language: 'json',
          code: String.raw`{
  "id": "video_xxx",
  "object": "video",
  "model": "seedance-2.0",
  "status": "queued",
  "created_at": 1782700000
}`,
        },
        {
          title: 'Completed response example',
          language: 'json',
          code: String.raw`{
  "id": "video_xxx",
  "object": "video",
  "model": "seedance-2.0",
  "status": "completed",
  "video_url": "https://cdn.example.com/output.mp4",
  "created_at": 1782700000,
  "completed_at": 1782700030
}`,
        },
      ],
    },
    {
      id: 'implementation-notes',
      kicker: 'For AI Coding',
      title: 'Implementation Contract For AI Coding',
      protocol: 'Implementation Contract',
      description:
        'Paste this section together with the examples above into an AI coding assistant to reduce endpoint, field, auth, and async-flow mistakes.',
      items: [
        { title: 'OpenAI image edit has no mask', text: 'Implement multipart/form-data with image + prompt only; do not generate a mask field.' },
        { title: 'Gemini uses v1beta standard paths', text: 'Text, text-to-image, and image-to-image all use /v1beta/models/{model}:generateContent.' },
        { title: 'Seedance is async', text: 'Do not treat POST /v1/videos as synchronous. Store the returned id and poll GET /v1/videos/{id}.' },
        { title: 'Seedance reference videos require duration', text: 'duration_seconds is required on reference_video items. Missing duration returns 400 and no task is submitted.' },
        { title: 'Persona category field', text: 'Seedance reference images and reference videos must send subject_type: "person"; otherwise real-person verification cannot pass.' },
      ],
    },
  ],
}

const docsSiteName = computed(() => {
  const configured = appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API'
  return configured.trim() || 'Sub2API'
})

const docsBaseUrl = computed(() => {
  const configured = normalizeBaseUrl(appStore.cachedPublicSettings?.api_base_url || appStore.apiBaseUrl || '')
  return configured || getCurrentOrigin() || baseUrl
})

const content = computed(() =>
  renderDocsContent(locale.value === 'zh' ? zhContent : enContent, {
    siteName: docsSiteName.value,
    baseUrl: docsBaseUrl.value,
    apiKey: exampleApiKey,
  })
)

const fullMarkdown = computed(() => buildMarkdown(content.value))

function renderDocsContent(doc: DocsContent, values: DocsRuntimeValues): DocsContent {
  return {
    ...doc,
    title: renderTemplate(doc.title, values),
    subtitle: renderTemplate(doc.subtitle, values),
    highlights: doc.highlights.map((item) => ({
      label: renderTemplate(item.label, values),
      value: renderTemplate(item.value, values),
      description: renderTemplate(item.description, values),
    })),
    sections: doc.sections.map((section) => ({
      ...section,
      kicker: renderTemplate(section.kicker, values),
      title: renderTemplate(section.title, values),
      description: renderTemplate(section.description, values),
      protocol: section.protocol ? renderTemplate(section.protocol, values) : undefined,
      notice: section.notice ? renderTemplate(section.notice, values) : undefined,
      bullets: section.bullets?.map((item) => renderInfoItem(item, values)),
      items: section.items?.map((item) => renderInfoItem(item, values)),
      table: section.table
        ? {
            headers: section.table.headers.map((head) => renderTemplate(head, values)),
            rows: section.table.rows.map((row) => row.map((cell) => renderTemplate(cell, values))),
          }
        : undefined,
      tables: section.tables?.map((table) => ({
        title: renderTemplate(table.title, values),
        headers: table.headers.map((head) => renderTemplate(head, values)),
        rows: table.rows.map((row) => row.map((cell) => renderTemplate(cell, values))),
      })),
      codeBlocks: section.codeBlocks?.map((block) => ({
        ...block,
        title: renderTemplate(block.title, values),
        code: renderTemplate(block.code, values),
      })),
    })),
  }
}

function renderInfoItem(item: InfoItem, values: DocsRuntimeValues): InfoItem {
  return {
    title: renderTemplate(item.title, values),
    text: renderTemplate(item.text, values),
  }
}

function renderTemplate(value: string, values: DocsRuntimeValues): string {
  return value
    .split('{{SITE_NAME}}').join(values.siteName)
    .split('{{BASE_URL}}').join(values.baseUrl)
    .split('{{API_KEY}}').join(values.apiKey)
}

function normalizeBaseUrl(value: string): string {
  return value.trim().replace(/\/+$/, '')
}

function getCurrentOrigin(): string {
  if (typeof window === 'undefined') return ''
  return window.location.origin
}

function buildMarkdown(doc: DocsContent): string {
  const lines: string[] = [
    `# ${doc.title}`,
    '',
    doc.subtitle,
    '',
    '## Summary',
    '',
    ...doc.highlights.map((item) => `- **${item.label}**: ${item.value} - ${item.description}`),
    '',
  ]

  for (const section of doc.sections) {
    lines.push(`## ${section.title}`, '', section.description, '')
    if (section.notice) {
      lines.push(`> ${section.notice}`, '')
    }
    if (section.bullets?.length) {
      for (const bullet of section.bullets) {
        lines.push(`- **${bullet.title}**: ${bullet.text}`)
      }
      lines.push('')
    }
    if (section.items?.length) {
      for (const item of section.items) {
        lines.push(`- **${item.title}**: ${item.text}`)
      }
      lines.push('')
    }
    if (section.table) {
      lines.push(`| ${section.table.headers.join(' | ')} |`)
      lines.push(`| ${section.table.headers.map(() => '---').join(' | ')} |`)
      for (const row of section.table.rows) {
        lines.push(`| ${row.join(' | ')} |`)
      }
      lines.push('')
    }
    if (section.tables?.length) {
      for (const table of section.tables) {
        lines.push(`### ${table.title}`, '')
        lines.push(`| ${table.headers.join(' | ')} |`)
        lines.push(`| ${table.headers.map(() => '---').join(' | ')} |`)
        for (const row of table.rows) {
          lines.push(`| ${row.join(' | ')} |`)
        }
        lines.push('')
      }
    }
    if (section.codeBlocks?.length) {
      for (const block of section.codeBlocks) {
        lines.push(`### ${block.title}`, '', `\`\`\`${block.language}`, block.code, '```', '')
      }
    }
  }

  return lines.join('\n').trim() + '\n'
}

function scrollToSection(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

async function copyMarkdown() {
  await copyToClipboard(fullMarkdown.value, content.value.copySuccess)
}

async function copyCode(code: string) {
  await copyToClipboard(code)
}
</script>
