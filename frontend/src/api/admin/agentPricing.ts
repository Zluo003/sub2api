import { apiClient } from '../client'

export type AgentPricingMediaType = 'text' | 'image' | 'video'

export interface AgentModelPricingRule {
  id?: number
  model: string
  platform: string
  media_type: AgentPricingMediaType
  resolution: string
  unit_price: number
  input_price_per_million: number
  output_price_per_million: number
  cache_write_price_per_million: number
  cache_read_price_per_million: number
  rate_multiplier: number
  reference_multiplier: number
  enabled: boolean
}

export interface AgentModelPricingResponse {
  group_id: number
  items: AgentModelPricingRule[]
}

export async function list(groupId: number): Promise<AgentModelPricingResponse> {
  const { data } = await apiClient.get<AgentModelPricingResponse>(
    `/admin/yingzo/agent-groups/${groupId}/pricing`
  )
  return data
}

export async function update(
  groupId: number,
  items: AgentModelPricingRule[]
): Promise<AgentModelPricingResponse> {
  const { data } = await apiClient.put<AgentModelPricingResponse>(
    `/admin/yingzo/agent-groups/${groupId}/pricing`,
    { items }
  )
  return data
}

export default { list, update }
