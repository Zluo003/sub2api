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
          text: 'OpenAI 接口使用 openai 分组，Claude 标准接口使用 anthropic 分组，Gemini 标准接口使用 gemini 分组，Seedance 使用 seedance 分组；Agent 聚合分组也可以按模型能力调度 Seedance。',
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
          ['/v1/videos', 'seedance / agent', 'Seedance 2.0、2.0 Fast、2.5 异步视频任务创建与查询。'],
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
        { title: '多张图片', text: '不要传 n。需要多张候选时，为每张图片创建一个独立任务，分别记录费用、状态和结果。' },
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
    "size": "1024x1024"
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
      title: 'Seedance 2.0 / 2.5 异步视频接口',
      protocol: 'POST /v1/videos · GET /v1/videos/{id}',
      description:
        'Seedance 视频接口是 OpenAI 风格异步任务协议。POST 创建本地任务并返回任务 ID；GET 查询任务状态。下游只会看到本站任务 ID、状态、视频地址和本站加工后的错误。',
      notice:
        'content 数组顺序就是参考素材顺序。提示词必须按同类素材出现顺序写“图片1、图片2、视频1、视频2、音频1、音频2”，不要引用文件名。reference_video 必须提供 duration_seconds；图片和视频的 subject_type 省略时，sub2api 会自动补为 person。',
      bullets: [
        { title: '鉴权', text: '创建和查询都使用 Authorization: Bearer <API Key>。API Key 必须绑定 seedance 分组或具备 Seedance 视频能力的 Agent 分组。' },
        { title: '分组与任务所有权', text: '使用 seedance 分组或具备视频能力的 Agent 分组。创建和查询必须使用同一个用户、同一把 API Key；其他密钥查询同一任务 ID 会得到 404。' },
        { title: '模型', text: 'seedance-2.0 支持 480p、720p、1080p、4K；seedance-2.0-fast 和 seedance-2.5 支持 480p、720p。' },
        { title: '默认值与生成时长', text: 'resolution 默认 720p。2.0/fast 的 ratio 默认 16:9、duration 必须为 4-15 的整数秒；2.5 支持 4-30 秒，duration: -1 会按 5 秒提交，图生视频和首尾帧模式未传比例时默认 auto，adaptive 会归一化为 auto。' },
        { title: '能力自动推断', text: 'ability_code 可省略，服务端会根据 content 推断；生产调用建议显式传入，避免 role 写错后落入其他模式。' },
        { title: '计费', text: '生成秒数和每个参考视频向上取整后计费。基础公式为（生成秒数 × 分辨率单价 + 参考视频秒数 × 分辨率单价 × 参考系数）× 分组倍率；普通 seedance 分组的参考系数为 1。' },
        { title: '状态与退款', text: 'queued、processing、completed、failed、cancelled。任务创建时预扣费；失败或取消后 refund_status 会从 pending 变为 refunded，completed 时返回 video_url。' },
        { title: '参考模式限制', text: '2.0/fast 最多 9 张参考图、3 个参考视频、3 个参考音频，且音频不能单独作为参考；2.5 最多 30 张图、10 个视频、10 个音频、总素材 50 个，并支持纯音频参考。' },
        { title: '视频编辑与延长', text: '当前统一入口使用 video_reference_to_video：把待编辑或待延长的源视频放为视频1，在 prompt 中明确“保留什么、改变什么、从哪里继续”。返回的是新视频任务，不会原地修改源文件。' },
        { title: '轮询与重试', text: '建议每 3-5 秒查询同一个任务 ID。GET 失败可以重试；POST 创建不是幂等重试入口，网络超时时不要盲目再次提交，否则可能创建新任务并再次预扣费。' },
      ],
      table: {
        headers: ['ability_code', '输入要求', 'content role'],
        rows: [
          ['video_text_to_video', '必须有 prompt，不允许图片/视频/音频参考。', 'text'],
          ['video_image_to_video', '必须正好 1 张首帧图，不允许视频/音频参考。', 'first_frame'],
          ['video_start_end_to_video', '必须正好 2 张图片，分别是首帧和尾帧。', 'first_frame / last_frame'],
          ['video_reference_to_video', '2.0/fast 至少需要 1 个图片或视频；2.5 至少需要 1 个图片、视频或音频，可使用纯音频参考。', 'reference_image / reference_video / reference_audio'],
        ],
      },
      tables: [
        {
          title: '创建请求字段',
          headers: ['字段', '类型与必填', '规则'],
          rows: [
            ['model', 'string，必填', 'seedance-2.0、seedance-2.0-fast 或 seedance-2.5。'],
            ['prompt', 'string，条件必填', '文生视频必填；其他模式强烈建议填写。为空时会取 content 中第一个非空 text。'],
            ['content', 'array，按能力填写', '元素顺序会原样保留到上游；文本、图片、视频、音频的字段见下一表。'],
            ['ability_code', 'string，可选', '可显式传四种能力代码；省略时按 content 的类型和 role 自动推断。'],
            ['ratio / aspect_ratio / aspectRatio', 'string，可选', '三个别名按此前后顺序取第一个非空值。2.5 支持 auto、16:9、4:3、1:1、3:4、9:16、21:9；adaptive 会转为 auto；2.5 图生视频和首尾帧模式默认 auto，其余默认 16:9。'],
            ['duration', 'number，必填', '2.0/fast 为 4-15 的整数秒且不接受 -1；2.5 为 4-30 的整数秒，-1 按智能时长兼容并转换为 5 秒。'],
            ['resolution', 'string，可选', '默认 720p；可用值取决于 model。大小写敏感，4K 必须写成 4K。'],
            ['generate_audio', 'boolean，可选', '是否请求模型生成音频；省略时由上游默认行为决定。'],
            ['safety_identifier', 'string，可选', '建议传项目或最终用户的稳定标识，不能放密钥或其他秘密。'],
          ],
        },
        {
          title: 'content 元素、role 与素材编号',
          headers: ['type', '载荷字段', 'role 与编号规则'],
          rows: [
            ['text', 'text', '作为提示词文本，不参与媒体编号。顶层 prompt 为空时取第一个非空 text。'],
            ['image_url', 'image_url.url', 'first_frame、last_frame 或 reference_image。按图片出现顺序编号为图片1、图片2；文件名不参与语义。'],
            ['video_url', 'video_url.url + duration_seconds', '参考模式使用 reference_video。按视频出现顺序编号为视频1、视频2；duration_seconds 必填。'],
            ['audio_url', 'audio_url.url', '参考模式使用 reference_audio。按音频出现顺序编号为音频1、音频2；仅 seedance-2.5 允许音频作为唯一参考。'],
            ['subject_type', 'person', 'image_url 和 video_url 可显式填写；省略时 sub2api 自动补为 person。2.0、2.0 Fast、2.5 均支持真人人脸输入。'],
          ],
        },
        {
          title: 'Seedance 2.0 / 2.5 素材与生成限制',
          headers: ['项目', '限制', '说明'],
          rows: [
            ['生成视频时长', '2.0/fast: 4-15 秒；2.5: 4-30 秒', '必须为整数秒。2.5 的 -1 会转换为 5 秒；2.0/fast 不接受 -1。'],
            ['输出分辨率', 'seedance-2.0: 480p / 720p / 1080p / 4K；seedance-2.0-fast、seedance-2.5: 480p / 720p', '1080p 和 4K 只适用于 seedance-2.0。'],
            ['输出比例', '2.5: auto / 16:9 / 4:3 / 1:1 / 3:4 / 9:16 / 21:9', '兼容 ratio、aspect_ratio、aspectRatio；adaptive 在 2.5 中会归一化为 auto。'],
            ['参考图片数量', '首帧模式 1 张；首尾帧模式 2 张；2.0/fast 参考模式最多 9 张；2.5 最多 30 张', '参考模式图片 role 必须为 reference_image；首尾帧模式与参考模式不能混用。'],
            ['参考图片格式', 'jpeg / png / webp / bmp / tiff / gif / heic / heif', 'URL 或 data:image/<format>;base64,<base64>；大文件建议使用可公开访问 URL。'],
            ['参考图片尺寸', '宽高比 0.4-2.5；宽和高均为 300-6000 px；单图小于 30 MB', '请求体总大小不超过 64 MB。'],
            ['参考视频数量与时长', '2.0/fast 最多 3 个且总时长不超过 15 秒；2.5 最多 10 个且总时长不超过 30 秒', 'reference_video 必须传 duration_seconds，用于本站预校验和计费；单个视频至少 2 秒。'],
            ['参考视频格式', 'mp4 / mov；视频编码 H.264/AVC 或 H.265/HEVC；音频编码 AAC 或 MP3', '仅支持视频 URL；单个视频不超过 200 MB。'],
            ['参考视频尺寸', '480p / 720p / 1080p / 4k；宽高比 0.4-2.5；宽和高均为 300-6000 px', '总像素需在 409600-8295044 之间，帧率 24-60 FPS。'],
            ['参考音频数量与时长', '2.0/fast 最多 3 段；2.5 最多 10 段且总时长不超过 30 秒', '2.0/fast 至少还需要 1 个图片或视频；2.5 支持纯音频参考。2.5 图片、视频、音频合计最多 50 个。'],
            ['参考音频格式', 'wav / mp3；单文件不超过 15 MB', '支持 URL 或 data:audio/<format>;base64,<base64>；请求体总大小不超过 64 MB。'],
            ['真人认证字段', '2.0、2.0 Fast、2.5 均支持真人人脸', '图片和视频可显式传 subject_type: "person"；省略时 sub2api 自动补齐 person。'],
          ],
        },
        {
          title: '任务状态与退款字段',
          headers: ['status', '含义', '响应规则'],
          rows: [
            ['queued', '本地任务已创建，等待提交或调度', '保存 id 并开始轮询；refund_status 通常为 not-applicable。'],
            ['processing', '上游已接受，正在生成', '继续轮询，不要重复 POST 创建同一业务任务。'],
            ['completed', '生成成功', '返回 video_url 和 completed_at；refund_status 为 not-applicable。'],
            ['failed', '提交、轮询或生成失败', '返回 error；已预扣费时 refund_status 为 pending 或 refunded。'],
            ['cancelled', '上游取消任务', '当前响应不返回 error；已预扣费时退款状态同 failed。'],
          ],
        },
        {
          title: '任务响应字段',
          headers: ['字段', '出现时机', '含义'],
          rows: [
            ['id', '始终返回', 'sub2api 生成的本地任务 ID；后续查询必须原样使用。'],
            ['object', '始终返回', '固定为 video。'],
            ['model', '始终返回', '创建时请求的下游模型名，不暴露具体上游账号或模型映射。'],
            ['status', '始终返回', 'queued / processing / completed / failed / cancelled。'],
            ['video_url', 'completed', '生成结果地址；其他状态不返回。客户端应及时下载并按业务需要持久化。'],
            ['error', 'failed', '只包含对下游安全的 code 和 message，不透传上游内部响应。'],
            ['refund_status', '始终返回', 'not-applicable / pending / refunded；失败后应等到 refunded 再决定是否创建新任务。'],
            ['created_at / completed_at', '创建时间始终返回；完成时间仅 completed 返回', 'Unix 秒级时间戳。'],
          ],
        },
        {
          title: '常见错误码',
          headers: ['error.code', 'HTTP', '处理方式'],
          rows: [
            ['invalid_video_model / invalid_video_resolution / invalid_video_duration / invalid_video_ratio / invalid_video_ability', '400', '修正模型、分辨率、对应模型的时长、2.5 比例或 ability_code。'],
            ['invalid_video_prompt / invalid_video_content', '400', '检查 prompt、content 数量、类型和 role 组合。'],
            ['reference_video_duration_required', '400', '为每个 reference_video 补充 duration_seconds。'],
            ['invalid_reference_video_duration', '400', '单个参考视频至少 2 秒；2.0/fast 总时长不得超过 15 秒，2.5 总时长不得超过 30 秒。'],
            ['video_pricing_rule_not_found', '400', '管理员尚未为当前分组、模型和分辨率启用单价。'],
            ['invalid_api_key', '401', '检查 Authorization Bearer API Key 是否存在、启用且未撤销。'],
            ['INSUFFICIENT_BALANCE / SUBSCRIPTION_NOT_FOUND', '403', '补充余额，或使用具有有效订阅的密钥。'],
            ['video_endpoint_not_available', '404', '当前 API Key 分组不是 seedance，也不是具备视频能力的 Agent 分组。'],
            ['VIDEO_TASK_NOT_FOUND', '404', '确认任务 ID，并使用创建任务时的同一用户和 API Key 查询。'],
            ['API_KEY_QUOTA_EXHAUSTED / API_KEY_RATE_5H_EXCEEDED / API_KEY_RATE_1D_EXCEEDED / API_KEY_RATE_7D_EXCEEDED / USAGE_LIMIT_EXCEEDED', '429', '等待额度窗口恢复或调整密钥、订阅额度。'],
            ['video_service_busy / video_generation_failed', '200（status=failed）', '读取任务内的 error 和 refund_status；退款完成后再按业务幂等键创建新任务。'],
            ['video_service_unavailable', '503 或 200（status=failed）', '没有兼容账号时创建请求直接失败；异步生命周期故障则写入失败任务。指数退避，且不要因一次查询失败重复创建。'],
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
  "prompt": "以图片1锁定人物身份和服装，以视频1参考运镜与动作节奏，以音频1参考音乐节拍。生成一支奢华香水广告，保留人物五官与产品几何。",
  "content": [
    {
      "type": "text",
      "text": "以图片1锁定人物身份和服装，以视频1参考运镜与动作节奏，以音频1参考音乐节拍。生成一支奢华香水广告，保留人物五官与产品几何。"
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
          title: '视频编辑或延长：源视频作为视频1',
          language: 'json',
          code: String.raw`{
  "model": "seedance-2.0",
  "prompt": "视频1是待编辑的源视频。保留人物身份、服装、镜头方向和前6秒动作，只把背景替换成雨夜街道，并从原动作末尾自然延长4秒；不要改变人物脸部和运动轴线。",
  "content": [
    {
      "type": "video_url",
      "video_url": { "url": "https://api-key.cc/media/<asset-id>/asset.mp4" },
      "role": "reference_video",
      "subject_type": "person",
      "duration_seconds": 8
    }
  ],
  "ratio": "16:9",
  "duration": 10,
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
  "refund_status": "not-applicable",
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
  "refund_status": "not-applicable",
  "created_at": 1782700000,
  "completed_at": 1782700030
}`,
        },
        {
          title: '失败与退款响应示例',
          language: 'json',
          code: String.raw`{
  "id": "video_xxx",
  "object": "video",
  "model": "seedance-2.0",
  "status": "failed",
  "error": {
    "code": "video_generation_failed",
    "message": "Video generation failed. Please retry with a different prompt or input."
  },
  "refund_status": "refunded",
  "created_at": 1782700000
}`,
        },
        {
          title: '同步校验失败响应示例',
          language: 'json',
          code: String.raw`{
  "error": {
    "type": "invalid_request_error",
    "code": "invalid_video_duration",
    "message": "Invalid video duration"
  }
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
        { title: '参考素材按类型编号', text: 'content 保持提交顺序；提示词引用图片1、视频1、音频1，不要引用文件名。图片和视频未传 subject_type 时会自动补为 person。' },
        { title: '视频编辑仍走 Seedance', text: '编辑、替换和延长都使用 video_reference_to_video，把源视频作为视频1并写清 preserve/change 约束；它会创建新任务，不会覆盖源视频。' },
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
          text: 'OpenAI endpoints require an openai group, Claude requires anthropic, Gemini requires gemini, and Seedance requires a seedance group. Agent aggregate groups may also route Seedance by model capability.',
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
          ['/v1/videos', 'seedance / agent', 'Seedance 2.0, 2.0 Fast, and 2.5 async video create and query API.'],
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
        { title: 'Multiple images', text: 'Do not send n. Create one independent task per candidate image so cost, status, and result remain independently traceable.' },
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
    "size": "1024x1024"
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
      title: 'Seedance 2.0 / 2.5 Async Video API',
      protocol: 'POST /v1/videos · GET /v1/videos/{id}',
      description:
        'The Seedance video API is an OpenAI-style async task protocol. POST creates a local task and returns an id; GET polls the task status. Downstream clients only receive {{SITE_NAME}} task ids, status, video URLs, and normalized errors.',
      notice:
        'The content array defines reference order. Prompts must refer to Image 1, Image 2, Video 1, Video 2, Audio 1, and Audio 2 by same-media appearance order, never by filename. Every reference_video requires duration_seconds. When omitted, subject_type defaults to person for images and videos.',
      bullets: [
        { title: 'Authentication', text: 'Creation and polling use Authorization: Bearer <API Key>. The key must belong to a seedance group or a Seedance-capable Agent group.' },
        { title: 'Group and task ownership', text: 'Use a seedance group or a video-capable Agent group. Create and query must use the same user and API key; another key receives 404 for the same task id.' },
        { title: 'Models', text: 'seedance-2.0 supports 480p, 720p, 1080p, and 4K; seedance-2.0-fast and seedance-2.5 support 480p and 720p.' },
        { title: 'Defaults and generated duration', text: 'resolution defaults to 720p. For 2.0/fast, ratio defaults to 16:9 and duration must be an integer from 4 through 15. Seedance 2.5 accepts 4-30 seconds; duration: -1 is submitted as 5 seconds, image-to-video and start-end mode default to auto when ratio is omitted, and adaptive is normalized to auto.' },
        { title: 'Ability inference', text: 'ability_code may be omitted and inferred from content. Production clients should send it explicitly so an incorrect role cannot silently select another mode.' },
        { title: 'Billing', text: 'Generated duration and each reference video duration are rounded up. Base formula: (generated seconds x resolution price + reference seconds x resolution price x reference multiplier) x group multiplier. The reference multiplier is 1 for standard seedance groups.' },
        { title: 'Statuses and refunds', text: 'queued, processing, completed, failed, cancelled. Creation is precharged; on failure or cancellation refund_status moves from pending to refunded. completed responses include video_url.' },
        { title: 'Reference mode limits', text: '2.0/fast allow up to 9 images, 3 videos, and 3 audio files, and audio cannot be the only reference. Seedance 2.5 allows up to 30 images, 10 videos, 10 audio files, 50 total media items, and audio-only reference input.' },
        { title: 'Video editing and extension', text: 'The unified route uses video_reference_to_video. Put the source to edit or extend first as Video 1, then state what to preserve, what to change, and where to continue. A new video task is created; the source is never modified in place.' },
        { title: 'Polling and retries', text: 'Poll the same task id every 3-5 seconds. A failed GET may be retried. POST creation is not an idempotent retry endpoint; do not blindly resubmit after a network timeout because that may create and precharge another task.' },
      ],
      table: {
        headers: ['ability_code', 'Input requirement', 'content role'],
        rows: [
          ['video_text_to_video', 'Requires prompt and no image/video/audio references.', 'text'],
          ['video_image_to_video', 'Requires exactly one first-frame image and no video/audio references.', 'first_frame'],
          ['video_start_end_to_video', 'Requires exactly two images: first frame and last frame.', 'first_frame / last_frame'],
          ['video_reference_to_video', '2.0/fast require at least one image or video. Seedance 2.5 requires at least one image, video, or audio item and supports audio-only reference input.', 'reference_image / reference_video / reference_audio'],
        ],
      },
      tables: [
        {
          title: 'Create Request Fields',
          headers: ['Field', 'Type / required', 'Rules'],
          rows: [
            ['model', 'string, required', 'seedance-2.0, seedance-2.0-fast, or seedance-2.5.'],
            ['prompt', 'string, conditionally required', 'Required for text-to-video and strongly recommended for other modes. If empty, the first non-empty content text is used.'],
            ['content', 'array, ability-dependent', 'Element order is preserved to the provider. See the next table for text, image, video, and audio shapes.'],
            ['ability_code', 'string, optional', 'Send one of the four ability codes, or omit it to infer the mode from content types and roles.'],
            ['ratio / aspect_ratio / aspectRatio', 'string, optional', 'The first non-empty alias in this order wins. Seedance 2.5 accepts auto, 16:9, 4:3, 1:1, 3:4, 9:16, and 21:9; adaptive becomes auto. Its image-to-video and start-end modes default to auto; other requests default to 16:9.'],
            ['duration', 'number, required', '2.0/fast require integer seconds from 4 through 15 and reject -1. Seedance 2.5 accepts integer seconds from 4 through 30 and converts -1 smart duration to 5 seconds.'],
            ['resolution', 'string, optional', 'Default: 720p. Values depend on model and are case-sensitive; 4K must use that exact casing.'],
            ['generate_audio', 'boolean, optional', 'Requests model-generated audio. If omitted, provider defaults apply.'],
            ['safety_identifier', 'string, optional', 'Use a stable project or end-user identifier. Never put API keys or other secrets here.'],
          ],
        },
        {
          title: 'content Items, Roles, And Reference Numbering',
          headers: ['type', 'Payload field', 'Role and numbering'],
          rows: [
            ['text', 'text', 'Prompt text; excluded from media numbering. The first non-empty text is used when top-level prompt is empty.'],
            ['image_url', 'image_url.url', 'first_frame, last_frame, or reference_image. Number as Image 1, Image 2 by image appearance order; filenames have no semantic role.'],
            ['video_url', 'video_url.url + duration_seconds', 'Use reference_video in reference mode. Number as Video 1, Video 2 by video appearance order. duration_seconds is required.'],
            ['audio_url', 'audio_url.url', 'Use reference_audio in reference mode. Number as Audio 1, Audio 2 by audio appearance order. Only seedance-2.5 permits audio as the sole reference type.'],
            ['subject_type', 'person', 'May be explicit on image_url and video_url. {{SITE_NAME}} defaults it to person when omitted. Seedance 2.0, 2.0 Fast, and 2.5 all support real-person face input.'],
          ],
        },
        {
          title: 'Seedance 2.0 / 2.5 Asset And Generation Limits',
          headers: ['Item', 'Limit', 'Notes'],
          rows: [
            ['Generated duration', '2.0/fast: 4-15 seconds; 2.5: 4-30 seconds', 'Integer seconds only. Seedance 2.5 converts -1 to 5 seconds; 2.0/fast reject -1.'],
            ['Output resolution', 'seedance-2.0: 480p / 720p / 1080p / 4K; seedance-2.0-fast and seedance-2.5: 480p / 720p', '1080p and 4K are only available on seedance-2.0.'],
            ['Output ratio', '2.5: auto / 16:9 / 4:3 / 1:1 / 3:4 / 9:16 / 21:9', 'The endpoint accepts ratio, aspect_ratio, and aspectRatio. adaptive is normalized to auto for Seedance 2.5.'],
            ['Reference image count', 'First-frame mode: 1 image; start-end mode: 2 images; 2.0/fast reference mode: up to 9; 2.5: up to 30', 'Reference-mode image role must be reference_image. Start/end-frame mode and reference mode cannot be mixed.'],
            ['Reference image formats', 'jpeg / png / webp / bmp / tiff / gif / heic / heif', 'Use a URL or data:image/<format>;base64,<base64>. Public URLs are recommended for large files.'],
            ['Reference image dimensions', 'Aspect ratio 0.4-2.5; width and height each 300-6000 px; each image under 30 MB', 'Total request body size must not exceed 64 MB.'],
            ['Reference video count and duration', '2.0/fast: up to 3 videos and 15 seconds total; 2.5: up to 10 videos and 30 seconds total', 'reference_video must include duration_seconds for local validation and billing. Each video must be at least 2 seconds.'],
            ['Reference video formats', 'mp4 / mov; video codec H.264/AVC or H.265/HEVC; audio codec AAC or MP3', 'Only video URLs are supported. Each video must not exceed 200 MB.'],
            ['Reference video dimensions', '480p / 720p / 1080p / 4k; aspect ratio 0.4-2.5; width and height each 300-6000 px', 'Total pixels must be 409600-8295044. Frame rate must be 24-60 FPS.'],
            ['Reference audio count and duration', '2.0/fast: up to 3 files; 2.5: up to 10 files and 30 seconds total', '2.0/fast also require an image or video. Seedance 2.5 supports audio-only reference input and limits all reference media to 50 items total.'],
            ['Reference audio formats', 'wav / mp3; each file under 15 MB', 'Use a URL or data:audio/<format>;base64,<base64>. Total request body size must not exceed 64 MB.'],
            ['Real-person verification field', 'Seedance 2.0, 2.0 Fast, and 2.5 support real-person faces', 'You may send subject_type: "person" explicitly on images and videos; {{SITE_NAME}} fills person when omitted.'],
          ],
        },
        {
          title: 'Task Status And Refund Fields',
          headers: ['status', 'Meaning', 'Response rule'],
          rows: [
            ['queued', 'Local task created and waiting for submission or scheduling', 'Store id and start polling. refund_status is normally not-applicable.'],
            ['processing', 'Provider accepted the task and generation is in progress', 'Keep polling; do not repeat POST for the same business task.'],
            ['completed', 'Generation succeeded', 'Returns video_url and completed_at. refund_status is not-applicable.'],
            ['failed', 'Submission, polling, or generation failed', 'Returns error. If precharged, refund_status is pending or refunded.'],
            ['cancelled', 'Provider cancelled the task', 'The current response has no error object. Refund state follows the failed rule when precharged.'],
          ],
        },
        {
          title: 'Task Response Fields',
          headers: ['Field', 'When present', 'Meaning'],
          rows: [
            ['id', 'Always', 'Local task id generated by {{SITE_NAME}}. Use it unchanged for every poll.'],
            ['object', 'Always', 'Always video.'],
            ['model', 'Always', 'The requested downstream model. Provider account and upstream model mapping are not exposed.'],
            ['status', 'Always', 'queued / processing / completed / failed / cancelled.'],
            ['video_url', 'completed', 'Generated result URL. Download promptly and persist it according to your application requirements.'],
            ['error', 'failed', 'A downstream-safe code and message. Raw provider responses are never exposed.'],
            ['refund_status', 'Always', 'not-applicable / pending / refunded. After failure, wait for refunded before deciding whether to create another task.'],
            ['created_at / completed_at', 'Creation time is always present; completion time only on completed', 'Unix timestamps in seconds.'],
          ],
        },
        {
          title: 'Common Error Codes',
          headers: ['error.code', 'HTTP', 'Action'],
          rows: [
            ['invalid_video_model / invalid_video_resolution / invalid_video_duration / invalid_video_ratio / invalid_video_ability', '400', 'Fix the model, resolution, model-specific duration, Seedance 2.5 ratio, or ability_code.'],
            ['invalid_video_prompt / invalid_video_content', '400', 'Check prompt, content counts, types, and role combination.'],
            ['reference_video_duration_required', '400', 'Add duration_seconds to every reference_video.'],
            ['invalid_reference_video_duration', '400', 'Each reference video must be at least 2 seconds. Total duration is capped at 15 seconds for 2.0/fast and 30 seconds for 2.5.'],
            ['video_pricing_rule_not_found', '400', 'The administrator has not enabled pricing for this group, model, and resolution.'],
            ['invalid_api_key', '401', 'Check that the Authorization Bearer API key exists, is enabled, and has not been revoked.'],
            ['INSUFFICIENT_BALANCE / SUBSCRIPTION_NOT_FOUND', '403', 'Add balance or use a key with an active subscription.'],
            ['video_endpoint_not_available', '404', 'The API key group is neither seedance nor a video-capable Agent group.'],
            ['VIDEO_TASK_NOT_FOUND', '404', 'Check the id and query with the same user and API key that created the task.'],
            ['API_KEY_QUOTA_EXHAUSTED / API_KEY_RATE_5H_EXCEEDED / API_KEY_RATE_1D_EXCEEDED / API_KEY_RATE_7D_EXCEEDED / USAGE_LIMIT_EXCEEDED', '429', 'Wait for the quota window or adjust key or subscription limits.'],
            ['video_service_busy / video_generation_failed', '200 (status=failed)', 'Read error and refund_status from the task. Create a new task only after refund completion and under your business idempotency rule.'],
            ['video_service_unavailable', '503 or 200 (status=failed)', 'Create fails directly when no compatible account exists; async lifecycle failures are stored on the task. Back off and never duplicate a task because one query failed.'],
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
  "prompt": "Use Image 1 for identity and wardrobe, Video 1 for camera movement and action rhythm, and Audio 1 for musical beats. Create a luxury perfume commercial while preserving facial identity and product geometry.",
  "content": [
    {
      "type": "text",
      "text": "Use Image 1 for identity and wardrobe, Video 1 for camera movement and action rhythm, and Audio 1 for musical beats. Create a luxury perfume commercial while preserving facial identity and product geometry."
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
          title: 'Video edit or extension: source is Video 1',
          language: 'json',
          code: String.raw`{
  "model": "seedance-2.0",
  "prompt": "Video 1 is the source to edit. Preserve identity, wardrobe, camera direction, and the first six seconds of motion. Replace only the background with a rainy night street, then continue naturally for four seconds from the final action. Do not change the face or motion axis.",
  "content": [
    {
      "type": "video_url",
      "video_url": { "url": "https://api-key.cc/media/<asset-id>/asset.mp4" },
      "role": "reference_video",
      "subject_type": "person",
      "duration_seconds": 8
    }
  ],
  "ratio": "16:9",
  "duration": 10,
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
  "refund_status": "not-applicable",
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
  "refund_status": "not-applicable",
  "created_at": 1782700000,
  "completed_at": 1782700030
}`,
        },
        {
          title: 'Failed and refunded response',
          language: 'json',
          code: String.raw`{
  "id": "video_xxx",
  "object": "video",
  "model": "seedance-2.0",
  "status": "failed",
  "error": {
    "code": "video_generation_failed",
    "message": "Video generation failed. Please retry with a different prompt or input."
  },
  "refund_status": "refunded",
  "created_at": 1782700000
}`,
        },
        {
          title: 'Synchronous validation error',
          language: 'json',
          code: String.raw`{
  "error": {
    "type": "invalid_request_error",
    "code": "invalid_video_duration",
    "message": "Invalid video duration"
  }
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
        { title: 'Number references by media type', text: 'content order is preserved. Prompts refer to Image 1, Video 1, and Audio 1, never filenames. subject_type defaults to person for images and videos.' },
        { title: 'Video editing still uses Seedance', text: 'Editing, replacement, and extension use video_reference_to_video. Put the source at Video 1 and state preserve/change constraints. A new task is created and the source is never overwritten.' },
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
