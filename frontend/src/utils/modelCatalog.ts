export interface ModelIdentity {
  vendor: string
  descriptionZh: string
  descriptionEn: string
}

interface ModelRule extends ModelIdentity {
  matches: RegExp
}

const MODEL_RULES: ModelRule[] = [
  { matches: /claude.*opus/i, vendor: 'Anthropic', descriptionZh: 'Anthropic 的旗舰 Claude 模型，面向复杂推理、代理任务与高难度编码。', descriptionEn: 'Anthropic flagship Claude model for complex reasoning, agents, and demanding coding work.' },
  { matches: /claude.*sonnet/i, vendor: 'Anthropic', descriptionZh: 'Anthropic 的均衡型 Claude 模型，兼顾推理能力、响应速度与使用成本。', descriptionEn: 'Anthropic balanced Claude model combining strong reasoning, speed, and cost efficiency.' },
  { matches: /claude.*haiku/i, vendor: 'Anthropic', descriptionZh: 'Anthropic 的轻量 Claude 模型，适合低延迟、高吞吐的日常任务。', descriptionEn: 'Anthropic lightweight Claude model for low-latency, high-throughput everyday tasks.' },
  { matches: /claude/i, vendor: 'Anthropic', descriptionZh: 'Anthropic 的通用大语言模型，擅长文本理解、推理、写作与代码任务。', descriptionEn: 'Anthropic general-purpose language model for understanding, reasoning, writing, and code.' },
  { matches: /(^|[-_/])o[1345](?:[-_/]|$)|reasoning/i, vendor: 'OpenAI', descriptionZh: 'OpenAI 的推理模型，适合数学、科学、规划和复杂问题求解。', descriptionEn: 'OpenAI reasoning model for math, science, planning, and complex problem solving.' },
  { matches: /gpt-image|dall[ -]?e/i, vendor: 'OpenAI', descriptionZh: 'OpenAI 的图像生成模型，可根据文字或参考图创建与编辑视觉内容。', descriptionEn: 'OpenAI image model for generating and editing visuals from text or image references.' },
  { matches: /sora/i, vendor: 'OpenAI', descriptionZh: 'OpenAI 的视频生成模型，用于从文本或图像创建动态视频内容。', descriptionEn: 'OpenAI video generation model for creating motion content from text or images.' },
  { matches: /gpt|chatgpt/i, vendor: 'OpenAI', descriptionZh: 'OpenAI 的通用 GPT 模型，适合对话、内容生成、工具调用与代码任务。', descriptionEn: 'OpenAI general-purpose GPT model for chat, content, tool use, and coding.' },
  { matches: /gemini.*flash/i, vendor: 'Google', descriptionZh: 'Google 的高速度 Gemini 模型，适合低延迟、多模态和大规模调用。', descriptionEn: 'Google fast Gemini model for low-latency, multimodal, and high-volume workloads.' },
  { matches: /gemini.*pro/i, vendor: 'Google', descriptionZh: 'Google 的高能力 Gemini 模型，面向复杂推理、长上下文与多模态任务。', descriptionEn: 'Google advanced Gemini model for complex reasoning, long context, and multimodal tasks.' },
  { matches: /imagen/i, vendor: 'Google', descriptionZh: 'Google 的图像生成模型，强调高质量画面、文字理解与可控编辑。', descriptionEn: 'Google image generation model focused on quality, prompt fidelity, and controlled editing.' },
  { matches: /veo/i, vendor: 'Google', descriptionZh: 'Google 的视频生成模型，可从文字或图像生成高质量动态画面。', descriptionEn: 'Google video generation model for high-quality motion from text or image prompts.' },
  { matches: /gemini/i, vendor: 'Google', descriptionZh: 'Google 的原生多模态模型，支持文本、图像、音频和代码等任务。', descriptionEn: 'Google natively multimodal model for text, image, audio, code, and more.' },
  { matches: /grok.*imagine/i, vendor: 'xAI', descriptionZh: 'xAI 的视觉生成模型，用于创建图像或视频内容。', descriptionEn: 'xAI visual generation model for creating image or video content.' },
  { matches: /grok/i, vendor: 'xAI', descriptionZh: 'xAI 的通用模型，面向推理、实时信息检索、对话与工具调用。', descriptionEn: 'xAI general model for reasoning, live information retrieval, chat, and tool use.' },
  { matches: /deepseek.*(reasoner|r1|v4)/i, vendor: 'DeepSeek', descriptionZh: 'DeepSeek 的推理模型，侧重复杂思考、数学、代码和长链路问题。', descriptionEn: 'DeepSeek reasoning model focused on complex thought, math, code, and long-form problems.' },
  { matches: /deepseek/i, vendor: 'DeepSeek', descriptionZh: 'DeepSeek 的通用语言模型，适合对话、代码生成和知识任务。', descriptionEn: 'DeepSeek general language model for chat, coding, and knowledge tasks.' },
  { matches: /qwen|qwq/i, vendor: 'Alibaba Cloud', descriptionZh: '阿里云通义千问模型，覆盖通用对话、推理、代码与多模态场景。', descriptionEn: 'Alibaba Cloud Qwen model for general chat, reasoning, coding, and multimodal tasks.' },
  { matches: /^seedance[-_/]2[._-]?0[-_/]fast$/i, vendor: 'ByteDance', descriptionZh: 'Seedance 2.0 Fast 极速视频模型，官方直连，支持文生视频、图生视频、首尾帧和多模态参考，最多支持 9 张参考图、3 个参考视频、3 个参考音频，可输出 480p、720p 视频。', descriptionEn: 'Seedance 2.0 Fast video model with direct official access, supporting text-to-video, image-to-video, start/end frames, and multimodal references with up to 9 images, 3 videos, and 3 audio files at 480p or 720p.' },
  { matches: /^seedance[-_/]2[._-]?5$/i, vendor: 'ByteDance', descriptionZh: 'Seedance 2.5 新一代视频模型，官方直连，支持文生视频、图生视频、首尾帧和多模态参考，最多支持 30 张参考图、10 个参考视频、10 个参考音频及总计 50 个参考素材，并支持纯音频参考和 4–30 秒生成。', descriptionEn: 'Seedance 2.5 video model with direct official access, supporting text-to-video, image-to-video, start/end frames, and multimodal references with up to 30 images, 10 videos, 10 audio files, 50 total reference assets, audio-only references, and 4–30 second generation.' },
  { matches: /^seedance[-_/]2[._-]?0$/i, vendor: 'ByteDance', descriptionZh: 'Seedance 2.0 满血视频模型，官方直连，支持文生视频、图生视频、首尾帧和多模态参考，最多支持 9 张参考图、3 个参考视频、3 个参考音频，可输出 480p、720p、1080p、4K 视频。', descriptionEn: 'Full Seedance 2.0 video model with direct official access, supporting text-to-video, image-to-video, start/end frames, and multimodal references with up to 9 images, 3 videos, and 3 audio files at 480p, 720p, 1080p, or 4K.' },
  { matches: /doubao|seedance|seedream|(^|[-_/])seed[-_/]/i, vendor: 'ByteDance', descriptionZh: '字节跳动豆包/Seed 系列模型，覆盖语言、图像与视频生成场景。', descriptionEn: 'ByteDance Doubao and Seed family covering language, image, and video generation.' },
  { matches: /^(minimax[-_/])?(h3|hailuo[-_/]3)[-_/]max$/i, vendor: 'MiniMax', descriptionZh: 'MiniMax H3 Max 视频模型，支持文生视频、图生视频和多模态参考，最多支持 12 张参考图、12 个参考视频、12 个参考音频，可输出 480p、768p 视频，生成 5–15 秒。', descriptionEn: 'MiniMax H3 Max video model supporting text-to-video, image-to-video, and multimodal references with up to 12 images, 12 videos, and 12 audio files at 480p or 768p, generating 5-15 seconds.' },
  { matches: /^(minimax[-_/])?(h3|hailuo[-_/]3)$/i, vendor: 'MiniMax', descriptionZh: 'MiniMax H3 视频模型，支持文生视频、图生视频和多模态参考，最多支持 9 张参考图、3 个参考视频、3 个参考音频，可输出 768p、2K 视频，生成 5–15 秒。', descriptionEn: 'MiniMax H3 video model supporting text-to-video, image-to-video, and multimodal references with up to 9 images, 3 videos, and 3 audio files at 768p or 2K, generating 5-15 seconds.' },
  { matches: /minimax|hailuo|abab/i, vendor: 'MiniMax', descriptionZh: 'MiniMax 模型，覆盖语言、语音与视频生成场景。', descriptionEn: 'MiniMax model family covering language, speech, and video generation.' },
  { matches: /^wan[-_/.]?3(\.0)?$/i, vendor: 'Alibaba Cloud', descriptionZh: '阿里云通义万相 Wan 3 视频模型，支持文生视频、图生视频和多模态参考，最多支持 10 张参考图、5 个参考视频、5 个参考音频，可输出 480p、720p、1080p 视频，生成 2–30 秒。', descriptionEn: 'Alibaba Cloud Wan 3 video model supporting text-to-video, image-to-video, and multimodal references with up to 10 images, 5 videos, and 5 audio files at 480p, 720p, or 1080p, generating 2-30 seconds.' },
  { matches: /(^|[-_/])wan([-_/.]|$)|wanx/i, vendor: 'Alibaba Cloud', descriptionZh: '阿里云通义万相系列模型，面向图像与视频生成。', descriptionEn: 'Alibaba Cloud Wan family for image and video generation.' },
  { matches: /kimi|moonshot/i, vendor: 'Moonshot AI', descriptionZh: '月之暗面的 Kimi 模型，擅长长上下文理解、推理和工具使用。', descriptionEn: 'Moonshot AI Kimi model for long-context understanding, reasoning, and tool use.' },
  { matches: /glm|cogview|cogvideo/i, vendor: 'Zhipu AI', descriptionZh: '智谱 AI 的 GLM/Cog 系列模型，覆盖语言理解、图像与视频生成。', descriptionEn: 'Zhipu AI GLM and Cog family for language, image, and video generation.' },
  { matches: /mistral|mixtral|codestral|pixtral/i, vendor: 'Mistral AI', descriptionZh: 'Mistral AI 模型，面向高效文本生成、代码和多模态任务。', descriptionEn: 'Mistral AI model for efficient text generation, coding, and multimodal tasks.' },
  { matches: /llama/i, vendor: 'Meta', descriptionZh: 'Meta 的 Llama 开放权重模型，适合通用对话、推理与定制化部署。', descriptionEn: 'Meta open-weight Llama model for general chat, reasoning, and custom deployment.' },
]

const PLATFORM_VENDOR: Record<string, string> = {
  anthropic: 'Anthropic',
  openai: 'OpenAI',
  gemini: 'Google',
  grok: 'xAI',
  seedance: 'ByteDance'
}

export function getModelIdentity(model: string, platform: string): ModelIdentity {
  const rule = MODEL_RULES.find((candidate) => candidate.matches.test(model))
  if (rule) return rule

  const vendor = PLATFORM_VENDOR[platform.toLowerCase()] || platform || 'AI'
  return {
    vendor,
    descriptionZh: `${vendor} 提供的 AI 模型，具体能力以模型接口和上游说明为准。`,
    descriptionEn: `AI model provided by ${vendor}; capabilities depend on its API and upstream documentation.`
  }
}
