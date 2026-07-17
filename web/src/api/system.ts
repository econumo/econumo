import { api, apiUrl } from './client'
import type { UpdateInfoDto } from './dto/system'

interface Envelope<T> {
  data: T
}

export async function getUpdateInfo(): Promise<UpdateInfoDto> {
  const response = await api.get<Envelope<UpdateInfoDto>>(apiUrl('/api/v1/system/get-update-info'))
  return response.data.data
}
