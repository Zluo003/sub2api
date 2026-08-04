type Dict = Record<string, unknown>

/**
 * 递归合并语言包。用于把本仓库（二开）新增的键并入上游拆分后的模块，
 * 不改动上游模块文件本身，避免每次同步上游都在同一批文件上产生冲突。
 * 同名叶子键以 override 为准。
 */
export function deepMergeLocale<T extends Dict>(base: T, override: Dict): T {
  const out: Dict = { ...base }
  for (const [key, value] of Object.entries(override)) {
    const current = out[key]
    if (
      current &&
      typeof current === 'object' &&
      !Array.isArray(current) &&
      value &&
      typeof value === 'object' &&
      !Array.isArray(value)
    ) {
      out[key] = deepMergeLocale(current as Dict, value as Dict)
    } else {
      out[key] = value
    }
  }
  return out as T
}
