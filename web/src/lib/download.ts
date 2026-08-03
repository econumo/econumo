import { isNativeApp, nativePlugin } from './platform'

interface FilesystemPlugin {
  writeFile(o: { path: string; data: string; directory: string }): Promise<{ uri: string }>
}
interface SharePlugin {
  share(o: { url: string }): Promise<unknown>
}

export function localDateStamp(d: Date = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function downloadBlob(blob: Blob, filename: string): void {
  if (isNativeApp()) {
    // Cancelling the share sheet rejects; a lost export is retryable.
    shareNative(blob, filename).catch(() => {})
    return
  }
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

async function shareNative(blob: Blob, filename: string): Promise<void> {
  const fs = nativePlugin<FilesystemPlugin>('Filesystem')
  const share = nativePlugin<SharePlugin>('Share')
  if (!fs || !share) {
    return
  }
  const { uri } = await fs.writeFile({
    path: filename,
    data: await blobToBase64(blob),
    directory: 'CACHE',
  })
  await share.share({ url: uri })
}

function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    // result is a data: URL; Filesystem.writeFile wants the bare base64 payload
    reader.onload = () => resolve(String(reader.result).split(',')[1] ?? '')
    reader.onerror = () => reject(reader.error ?? new Error('blob read failed'))
    reader.readAsDataURL(blob)
  })
}
