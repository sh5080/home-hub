/**
 * 올리기 전에 폰에서 줄인다(원본 12MP 를 Pi 가 처리하면 몇 분). 긴 변 1600, JPEG.
 * EXIF 회전은 직접 읽어 캔버스에 굽는다(브라우저마다 자동 회전 여부가 다르다).
 */
const MAX_EDGE = 1600

export async function shrinkImage(file: File): Promise<Blob> {
  if (!file.type.startsWith('image/')) return file
  let orientation = 1
  try {
    orientation = exifOrientation(await file.slice(0, 256 * 1024).arrayBuffer())
  } catch { /* 못 읽으면 1 */ }

  const url = URL.createObjectURL(file)
  try {
    const img = await loadImage(url)
    const w = img.naturalWidth, h = img.naturalHeight
    if (!w || !h) return file
    // 이미 작고 돌릴 것도 없으면 원본 그대로.
    if (Math.max(w, h) <= MAX_EDGE && orientation === 1 && file.type === 'image/jpeg') return file

    const scale = Math.min(1, MAX_EDGE / Math.max(w, h))
    const sw = Math.round(w * scale), sh = Math.round(h * scale)
    const swap = orientation >= 5
    const canvas = document.createElement('canvas')
    canvas.width = swap ? sh : sw
    canvas.height = swap ? sw : sh
    const ctx = canvas.getContext('2d')
    if (!ctx) return file
    switch (orientation) {
      case 2: ctx.transform(-1, 0, 0, 1, sw, 0); break
      case 3: ctx.transform(-1, 0, 0, -1, sw, sh); break
      case 4: ctx.transform(1, 0, 0, -1, 0, sh); break
      case 5: ctx.transform(0, 1, 1, 0, 0, 0); break
      case 6: ctx.transform(0, 1, -1, 0, sh, 0); break
      case 7: ctx.transform(0, -1, -1, 0, sh, sw); break
      case 8: ctx.transform(0, -1, 1, 0, 0, sw); break
    }
    ctx.drawImage(img, 0, 0, sw, sh)
    const blob = await new Promise<Blob | null>((res) => canvas.toBlob(res, 'image/jpeg', 0.85))
    return blob ?? file
  } finally {
    URL.revokeObjectURL(url)
  }
}

function loadImage(url: string) {
  return new Promise<HTMLImageElement>((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('이미지를 열지 못했어요'))
    img.src = url
  })
}

/** JPEG 의 APP1(Exif) 에서 Orientation(0x0112) 만 읽는다. 없으면 1. */
function exifOrientation(buf: ArrayBuffer): number {
  const v = new DataView(buf)
  if (v.byteLength < 4 || v.getUint16(0) !== 0xffd8) return 1
  let i = 2
  while (i + 4 <= v.byteLength) {
    if (v.getUint8(i) !== 0xff) return 1
    const marker = v.getUint8(i + 1)
    if (marker === 0xda || marker === 0xd9) return 1
    const size = v.getUint16(i + 2)
    if (size < 2 || i + 2 + size > v.byteLength) return 1
    if (marker === 0xe1) {
      const p = i + 4
      if (v.getUint32(p) !== 0x45786966) return 1 // "Exif"
      const t = p + 6
      const little = v.getUint16(t) === 0x4949
      const ifd = t + v.getUint32(t + 4, little)
      if (ifd + 2 > v.byteLength) return 1
      const n = v.getUint16(ifd, little)
      for (let k = 0; k < n; k++) {
        const e = ifd + 2 + k * 12
        if (e + 12 > v.byteLength) return 1
        if (v.getUint16(e, little) === 0x0112) {
          const o = v.getUint16(e + 8, little)
          return o >= 1 && o <= 8 ? o : 1
        }
      }
      return 1
    }
    i += 2 + size
  }
  return 1
}
