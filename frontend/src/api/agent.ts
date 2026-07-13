import { apiClient } from './client'

export interface AgentDeviceAuthorization {
  installation_id: string
  installation_name: string
  system_code: 'yingzo'
  status: 'pending'
  expires_at: string
}

export async function getAgentDeviceAuthorization(userCode: string): Promise<AgentDeviceAuthorization> {
  const { data } = await apiClient.get<AgentDeviceAuthorization>(
    `/agent/device/authorizations/${encodeURIComponent(userCode)}`,
  )
  return data
}

export async function approveAgentDeviceAuthorization(userCode: string): Promise<void> {
  await apiClient.post('/agent/device/approve', { user_code: userCode })
}

export const agentAPI = {
  getDeviceAuthorization: getAgentDeviceAuthorization,
  approveDeviceAuthorization: approveAgentDeviceAuthorization,
}
